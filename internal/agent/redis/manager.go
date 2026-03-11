package redis

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnableForSite enables Redis object cache for a WordPress site
func EnableForSite(domain string, maxMemoryMB int, prefix string) error {
	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	// 1. Ensure redis-server is installed and running
	exec.Command("systemctl", "start", "redis-server").Run()
	exec.Command("systemctl", "enable", "redis-server").Run()

	// 2. Add Redis config to wp-config.php first
	wpConfig := fmt.Sprintf("%s/wp-config.php", wpPath)
	addRedisConfig(wpConfig, prefix, maxMemoryMB)

	// 3. Install wp-redis plugin via WP-CLI (run as www-data or root)
	installWPCLI(wpPath, "plugin", "install", "redis-cache", "--activate")

	// 4. Enable Redis object cache
	out, err := runWPCLI(wpPath, "redis", "enable")
	if err != nil {
		// Fallback: manually copy object-cache.php
		srcFile := fmt.Sprintf("%s/wp-content/plugins/redis-cache/includes/object-cache.php", wpPath)
		dstFile := fmt.Sprintf("%s/wp-content/object-cache.php", wpPath)
		if _, statErr := os.Stat(srcFile); statErr == nil {
			if copyErr := exec.Command("cp", srcFile, dstFile).Run(); copyErr != nil {
				return fmt.Errorf("failed to enable redis: %s; fallback copy also failed: %v", out, copyErr)
			}
			// Fix ownership
			owner := getSiteOwner(wpPath)
			exec.Command("chown", fmt.Sprintf("%s:%s", owner, owner), dstFile).Run()
		} else {
			return fmt.Errorf("failed to enable redis object cache: %s", out)
		}
	}

	return nil
}

// DisableForSite disables Redis object cache for a WordPress site
func DisableForSite(domain string) error {
	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	// 1. Disable via WP-CLI
	runWPCLI(wpPath, "redis", "disable")

	// 2. Always remove object-cache.php directly to be sure
	objectCache := fmt.Sprintf("%s/wp-content/object-cache.php", wpPath)
	os.Remove(objectCache)

	// 3. Remove Redis config from wp-config.php
	wpConfig := fmt.Sprintf("%s/wp-config.php", wpPath)
	removeRedisConfig(wpConfig)

	return nil
}

// FlushCache flushes the Redis cache.
// If domain is empty → server-level flush (FLUSHALL, intentional admin operation).
// If domain is set   → site-level flush: only delete keys belonging to that site's prefix.
func FlushCache(domain string) error {
	// Server-level: flush everything (admin explicitly asked for this)
	if domain == "" {
		out, err := exec.Command("redis-cli", "FLUSHALL").Output()
		if err != nil {
			return fmt.Errorf("FLUSHALL failed: %s", string(out))
		}
		return nil
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	// Try WP-CLI first — it respects the site's key prefix automatically
	_, err := runWPCLI(wpPath, "cache", "flush")
	if err == nil {
		return nil
	}

	// Fallback: read the WP_REDIS_PREFIX from wp-config.php and delete only those keys.
	// We must NOT use FLUSHALL here — that would wipe cache for all other sites.
	prefix := getWPRedisPrefix(wpPath)
	if prefix == "" {
		return fmt.Errorf("wp cache flush failed and WP_REDIS_PREFIX not found in wp-config.php — refusing FLUSHALL to avoid affecting other sites")
	}
	return flushByPrefix(prefix)
}

// flushByPrefix deletes all Redis keys whose name starts with prefix.
func flushByPrefix(prefix string) error {
	// redis-cli --scan --pattern 'prefix*' | xargs -r redis-cli del
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("redis-cli --scan --pattern '%s*' | xargs -r redis-cli del", prefix))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prefix flush failed (prefix=%q): %s", prefix, string(out))
	}
	return nil
}

// getWPRedisPrefix reads WP_REDIS_PREFIX value from a site's wp-config.php.
func getWPRedisPrefix(wpPath string) string {
	content, err := os.ReadFile(fmt.Sprintf("%s/wp-config.php", wpPath))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, "WP_REDIS_PREFIX") {
			continue
		}
		// Handles: define('WP_REDIS_PREFIX', 'some_prefix');
		//       or: define("WP_REDIS_PREFIX", "some_prefix");
		for _, sep := range []string{"'", `"`} {
			parts := strings.Split(line, sep)
			if len(parts) >= 4 {
				return parts[3]
			}
		}
	}
	return ""
}

// GetStats retrieves Redis server statistics
func GetStats() (map[string]string, error) {
	out, err := exec.Command("redis-cli", "INFO", "all").Output()
	if err != nil {
		return nil, fmt.Errorf("redis-cli INFO failed: %w", err)
	}
	return parseRedisInfo(string(out)), nil
}

// ---- internal helpers ----

// runWPCLI runs a wp-cli command in a WordPress path, trying www-data then root
func runWPCLI(wpPath string, args ...string) (string, error) {
	allArgs := append([]string{fmt.Sprintf("--path=%s", wpPath), "--allow-root"}, args...)

	// Try as www-data user first
	cmd := exec.Command("sudo", append([]string{"-u", "www-data", "wp"}, allArgs...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}

	// Fallback: run as root with --allow-root
	cmd = exec.Command("wp", allArgs...)
	out, err = cmd.CombinedOutput()
	return string(out), err
}

// installWPCLI installs a WP-CLI plugin without failing on error
func installWPCLI(wpPath string, args ...string) {
	runWPCLI(wpPath, args...)
}

// getSiteOwner returns the owner of wp-config.php (site user)
func getSiteOwner(wpPath string) string {
	out, err := exec.Command("stat", "-c", "%U", fmt.Sprintf("%s/wp-config.php", wpPath)).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "www-data"
	}
	return strings.TrimSpace(string(out))
}

func addRedisConfig(wpConfigPath, prefix string, maxMemoryMB int) {
	content, err := os.ReadFile(wpConfigPath)
	if err != nil {
		return
	}

	configStr := string(content)

	// Check if Redis config already exists
	if strings.Contains(configStr, "WP_REDIS_HOST") {
		return
	}

	if prefix == "" {
		prefix = "wp_"
	}

	redisConfig := fmt.Sprintf(`
// Redis Object Cache Configuration — managed by WP Manage
define('WP_REDIS_HOST', '127.0.0.1');
define('WP_REDIS_PORT', 6379);
define('WP_REDIS_PREFIX', '%s');
define('WP_REDIS_MAXTTL', 86400);
`, prefix)

	// Insert before "That's all, stop editing!" or before the first require
	marker := "/* That's all, stop editing!"
	if idx := strings.Index(configStr, marker); idx > 0 {
		configStr = configStr[:idx] + redisConfig + "\n" + configStr[idx:]
	} else {
		configStr = strings.Replace(configStr, "<?php", "<?php\n"+redisConfig, 1)
	}

	os.WriteFile(wpConfigPath, []byte(configStr), 0644)
}

func removeRedisConfig(wpConfigPath string) {
	content, err := os.ReadFile(wpConfigPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inBlock := false

	for _, line := range lines {
		if strings.Contains(line, "Redis Object Cache Configuration") {
			inBlock = true
			continue
		}
		if inBlock {
			if strings.Contains(line, "WP_REDIS_") ||
				strings.Contains(line, "define('WP_REDIS") ||
				strings.TrimSpace(line) == "" {
				continue
			}
			inBlock = false
		}
		newLines = append(newLines, line)
	}

	os.WriteFile(wpConfigPath, []byte(strings.Join(newLines, "\n")), 0644)
}

// parseRedisInfo parses `redis-cli INFO` output into key-value pairs
func parseRedisInfo(info string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}
