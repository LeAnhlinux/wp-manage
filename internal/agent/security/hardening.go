package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// HardenWordPress applies security hardening measures to a WordPress site
func HardenWordPress(domain string, opts HardenOptions) error {
	slog.Info("Hardening WordPress", "domain", domain)
	var errors []string

	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	if opts.DisableXMLRPC {
		if err := disableXMLRPC(domain); err != nil {
			errors = append(errors, fmt.Sprintf("xmlrpc: %v", err))
		}
	}

	if opts.HideWPVersion {
		if err := hideWPVersion(wpPath); err != nil {
			errors = append(errors, fmt.Sprintf("wp-version: %v", err))
		}
	}

	if opts.SecureFilePerms {
		if err := secureFilePermissions(wpPath); err != nil {
			errors = append(errors, fmt.Sprintf("file-perms: %v", err))
		}
	}

	if opts.SecurityHeaders {
		if err := addSecurityHeaders(domain); err != nil {
			errors = append(errors, fmt.Sprintf("headers: %v", err))
		}
	}

	if opts.DisableFileEditor {
		if err := disableFileEditor(wpPath); err != nil {
			errors = append(errors, fmt.Sprintf("file-editor: %v", err))
		}
	}

	if opts.LimitLoginAttempts {
		if err := limitLoginAttempts(domain); err != nil {
			errors = append(errors, fmt.Sprintf("login-limit: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("hardening partial errors: %s", strings.Join(errors, "; "))
	}

	slog.Info("WordPress hardening completed", "domain", domain)
	return nil
}

type HardenOptions struct {
	DisableXMLRPC      bool
	HideWPVersion      bool
	SecureFilePerms    bool
	SecurityHeaders    bool
	DisableFileEditor  bool
	LimitLoginAttempts bool
}

// firstServerBlockEnd returns the index of the closing `}` of the first
// server { } block in an nginx config (the HTTPS block in Certbot 2-block configs).
func firstServerBlockEnd(content string) int {
	depth := 0
	inFirst := false
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
			if depth == 1 {
				inFirst = true
			}
		case '}':
			depth--
			if inFirst && depth == 0 {
				return i
			}
		}
	}
	return -1
}

// insertBeforePHPLocation returns the byte index of the line containing
// "location ~ \.php$" inside the FIRST server block, so a new location block
// can be prepended before it. This ensures nginx evaluates the more-specific
// wp-login regex before the generic \.php$ regex.
// Returns -1 if the pattern is not found within the first server block.
func insertBeforePHPLocation(content string) int {
	firstEnd := firstServerBlockEnd(content)
	if firstEnd < 0 {
		return -1
	}
	firstBlock := content[:firstEnd]
	idx := strings.Index(firstBlock, "location ~ \\.php$")
	if idx < 0 {
		// try without backslash escape (some configs may not have it)
		idx = strings.Index(firstBlock, "location ~ .php$")
	}
	if idx < 0 {
		return -1
	}
	// Walk back to the start of the line
	lineStart := strings.LastIndex(firstBlock[:idx], "\n") + 1
	return lineStart
}

// removeXMLRPCBlock strips the XML-RPC location block from content (for cleanup before re-insert).
func removeXMLRPCBlock(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inBlock := false
	depth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.Contains(trimmed, "# Block XML-RPC") {
				continue
			}
			if trimmed == "location = /xmlrpc.php {" {
				inBlock = true
				depth = 1
				continue
			}
			out = append(out, line)
		} else {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				inBlock = false
			}
		}
	}
	result := strings.Join(out, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

func disableXMLRPC(domain string) error {
	slog.Info("Disabling XML-RPC", "domain", domain)

	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}
	content := string(data)

	// Check if rule already exists inside the FIRST (HTTPS) server block
	firstEnd := firstServerBlockEnd(content)
	if firstEnd > 0 && strings.Contains(content[:firstEnd], "xmlrpc.php") {
		return nil // Already in the right block
	}

	// Remove stale xmlrpc rule if it ended up in the wrong block
	if strings.Contains(content, "xmlrpc.php") {
		content = removeXMLRPCBlock(content)
	}

	// Insert into the FIRST server block (HTTPS block with real content)
	xmlrpcBlock := `
    # Block XML-RPC
    location = /xmlrpc.php {
        deny all;
        access_log off;
        log_not_found off;
    }
`
	insertAt := firstServerBlockEnd(content)
	if insertAt < 0 {
		insertAt = strings.LastIndex(content, "}")
	}
	if insertAt < 0 {
		return fmt.Errorf("could not find server block end in nginx config")
	}
	content = content[:insertAt] + xmlrpcBlock + content[insertAt:]

	if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

func hideWPVersion(wpPath string) error {
	slog.Info("Hiding WP version", "path", wpPath)

	// Create mu-plugin to remove WP version
	muPluginDir := wpPath + "/wp-content/mu-plugins"
	os.MkdirAll(muPluginDir, 0755)

	content := `<?php
// Hide WordPress version
remove_action('wp_head', 'wp_generator');
add_filter('the_generator', '__return_empty_string');
`
	return os.WriteFile(muPluginDir+"/wp-manage-hide-version.php", []byte(content), 0644)
}

func secureFilePermissions(wpPath string) error {
	slog.Info("Securing file permissions", "path", wpPath)

	// Set directory permissions to 755 (batched — much faster than per-file)
	exec.Command("find", wpPath, "-type", "d", "-exec", "chmod", "755", "{}", "+").Run()

	// Set file permissions to 644 (batched)
	exec.Command("find", wpPath, "-type", "f", "-exec", "chmod", "644", "{}", "+").Run()

	// wp-config.php should be 600
	exec.Command("chmod", "600", wpPath+"/wp-config.php").Run()

	// .htaccess should be 644
	exec.Command("chmod", "644", wpPath+"/.htaccess").Run()

	return nil
}

func addSecurityHeaders(domain string) error {
	slog.Info("Adding security headers", "domain", domain)

	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}
	content := string(data)

	// Check if headers already exist inside the FIRST (HTTPS) server block
	firstEnd := firstServerBlockEnd(content)
	if firstEnd > 0 && strings.Contains(content[:firstEnd], "X-Content-Type-Options") {
		return nil // Already in the right block
	}

	// Remove stale headers if they ended up in the wrong block
	if strings.Contains(content, "X-Content-Type-Options") {
		lines := strings.Split(content, "\n")
		var out []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "# Security Headers") ||
				strings.HasPrefix(trimmed, "add_header X-Content-Type-Options") ||
				strings.HasPrefix(trimmed, "add_header X-Frame-Options") ||
				strings.HasPrefix(trimmed, "add_header X-XSS-Protection") ||
				strings.HasPrefix(trimmed, "add_header Referrer-Policy") ||
				strings.HasPrefix(trimmed, "add_header Permissions-Policy") {
				continue
			}
			out = append(out, line)
		}
		content = strings.Join(out, "\n")
	}

	headersBlock := `
    # Security Headers
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
`
	insertAt := firstServerBlockEnd(content)
	if insertAt < 0 {
		insertAt = strings.LastIndex(content, "}")
	}
	if insertAt < 0 {
		return fmt.Errorf("could not find server block end in nginx config")
	}
	content = content[:insertAt] + headersBlock + content[insertAt:]

	if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

// ==================== Revert Functions ====================

// RevertHardenWordPress undoes previously applied hardening measures.
func RevertHardenWordPress(domain string, opts HardenOptions) error {
	slog.Info("Reverting WordPress hardening", "domain", domain)
	wpPath := fmt.Sprintf("/var/www/%s/public", domain)
	var errors []string

	if opts.DisableXMLRPC {
		if err := revertXMLRPC(domain); err != nil {
			errors = append(errors, fmt.Sprintf("xmlrpc: %v", err))
		}
	}
	if opts.HideWPVersion {
		if err := revertHideVersion(wpPath); err != nil {
			errors = append(errors, fmt.Sprintf("wp-version: %v", err))
		}
	}
	if opts.SecurityHeaders {
		if err := revertSecurityHeaders(domain); err != nil {
			errors = append(errors, fmt.Sprintf("headers: %v", err))
		}
	}
	if opts.DisableFileEditor {
		if err := revertDisableFileEditor(wpPath); err != nil {
			errors = append(errors, fmt.Sprintf("file-editor: %v", err))
		}
	}
	if opts.LimitLoginAttempts {
		if err := revertLimitLogin(domain); err != nil {
			errors = append(errors, fmt.Sprintf("login-limit: %v", err))
		}
	}
	// SecureFilePerms intentionally not reverted (permissions 755/644 are safe to keep)

	if len(errors) > 0 {
		return fmt.Errorf("revert partial errors: %s", strings.Join(errors, "; "))
	}
	slog.Info("WordPress hardening reverted", "domain", domain)
	return nil
}

func revertXMLRPC(domain string) error {
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}

	content := string(data)
	if !strings.Contains(content, "xmlrpc.php") {
		return nil // Already removed
	}

	// Remove the xmlrpc location block line by line
	lines := strings.Split(content, "\n")
	var newLines []string
	inBlock := false
	braceDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.Contains(trimmed, "# Block XML-RPC") {
				continue // skip comment
			}
			if trimmed == "location = /xmlrpc.php {" {
				inBlock = true
				braceDepth = 1
				continue
			}
			newLines = append(newLines, line)
		} else {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if braceDepth <= 0 {
				inBlock = false
			}
			// skip lines inside block
		}
	}

	newContent := strings.Join(newLines, "\n")
	for strings.Contains(newContent, "\n\n\n") {
		newContent = strings.ReplaceAll(newContent, "\n\n\n", "\n\n")
	}
	if err := os.WriteFile(nginxConf, []byte(newContent), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

func revertHideVersion(wpPath string) error {
	muPlugin := wpPath + "/wp-content/mu-plugins/wp-manage-hide-version.php"
	if err := os.Remove(muPlugin); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func revertSecurityHeaders(domain string) error {
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}

	content := string(data)
	if !strings.Contains(content, "X-Content-Type-Options") {
		return nil // Already removed
	}

	lines := strings.Split(content, "\n")
	var newLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "# Security Headers") ||
			strings.HasPrefix(trimmed, "add_header X-Content-Type-Options") ||
			strings.HasPrefix(trimmed, "add_header X-Frame-Options") ||
			strings.HasPrefix(trimmed, "add_header X-XSS-Protection") ||
			strings.HasPrefix(trimmed, "add_header Referrer-Policy") ||
			strings.HasPrefix(trimmed, "add_header Permissions-Policy") {
			continue
		}
		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")
	for strings.Contains(newContent, "\n\n\n") {
		newContent = strings.ReplaceAll(newContent, "\n\n\n", "\n\n")
	}
	if err := os.WriteFile(nginxConf, []byte(newContent), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

func revertDisableFileEditor(wpPath string) error {
	wpConfig := wpPath + "/wp-config.php"
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		return err
	}

	content := string(data)
	if !strings.Contains(content, "DISALLOW_FILE_EDIT") {
		return nil
	}

	lines := strings.Split(content, "\n")
	var newLines []string
	for _, line := range lines {
		if strings.Contains(line, "DISALLOW_FILE_EDIT") {
			continue
		}
		newLines = append(newLines, line)
	}
	return os.WriteFile(wpConfig, []byte(strings.Join(newLines, "\n")), 0644)
}

// ==================== Login Rate Limit + Fail2ban ====================

const cloudflareRealIPConf = `/etc/nginx/conf.d/cloudflare-real-ip.conf`
const wpLoginLimitConf = `/etc/nginx/conf.d/wp-login-limit.conf`

func limitLoginAttempts(domain string) error {
	slog.Info("Applying login rate limit", "domain", domain)

	// 1. Write Cloudflare real-IP conf (idempotent)
	if _, err := os.Stat(cloudflareRealIPConf); os.IsNotExist(err) {
		cfConf := `# Cloudflare Real IP
set_real_ip_from 103.21.244.0/22;
set_real_ip_from 103.22.200.0/22;
set_real_ip_from 103.31.4.0/22;
set_real_ip_from 104.16.0.0/13;
set_real_ip_from 104.24.0.0/14;
set_real_ip_from 108.162.192.0/18;
set_real_ip_from 131.0.72.0/22;
set_real_ip_from 141.101.64.0/18;
set_real_ip_from 162.158.0.0/15;
set_real_ip_from 172.64.0.0/13;
set_real_ip_from 173.245.48.0/20;
set_real_ip_from 188.114.96.0/20;
set_real_ip_from 190.93.240.0/20;
set_real_ip_from 197.234.240.0/22;
set_real_ip_from 198.41.128.0/17;
set_real_ip_from 2400:cb00::/32;
set_real_ip_from 2606:4700::/32;
set_real_ip_from 2803:f800::/32;
set_real_ip_from 2405:b500::/32;
set_real_ip_from 2405:8100::/32;
set_real_ip_from 2a06:98c0::/29;
set_real_ip_from 2c0f:f248::/32;
real_ip_header CF-Connecting-IP;
`
		if err := os.WriteFile(cloudflareRealIPConf, []byte(cfConf), 0644); err != nil {
			return fmt.Errorf("write cloudflare-real-ip.conf: %w", err)
		}
	}

	// 2. Write global rate-limit zone conf (idempotent)
	if _, err := os.Stat(wpLoginLimitConf); os.IsNotExist(err) {
		zoneConf := "# WP Login Rate Limiting\nlimit_req_zone $binary_remote_addr zone=wplogin:10m rate=5r/m;\n"
		if err := os.WriteFile(wpLoginLimitConf, []byte(zoneConf), 0644); err != nil {
			return fmt.Errorf("write wp-login-limit.conf: %w", err)
		}
	}

	// 3. Inject location block into nginx site config
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return fmt.Errorf("read nginx config: %w", err)
	}
	content := string(data)

	// Idempotency check — look inside the first server block
	firstEnd := firstServerBlockEnd(content)
	if firstEnd > 0 && strings.Contains(content[:firstEnd], "wp-login") {
		slog.Info("Login rate limit already configured in nginx", "domain", domain)
	} else {
		// Extract PHP socket path from existing location ~ \.php$ block
		socketPath := extractPHPSocket(content, domain)
		loginBlock := fmt.Sprintf(`
    # Rate limit WP login (brute force protection)
    location ~* /wp-login\.php {
        limit_req zone=wplogin burst=3 nodelay;
        limit_req_status 429;
        fastcgi_pass %s;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_read_timeout 300;
    }
`, socketPath)

		// Insert BEFORE location ~ \.php$ so our regex location is matched first.
		// Nginx evaluates regex locations in declaration order; if \.php$ comes
		// first it would match wp-login.php before our rate-limit block fires.
		insertAt := insertBeforePHPLocation(content)
		if insertAt < 0 {
			// Fallback: insert before end of first server block
			insertAt = firstServerBlockEnd(content)
		}
		if insertAt < 0 {
			insertAt = strings.LastIndex(content, "}")
		}
		if insertAt < 0 {
			return fmt.Errorf("could not find server block end in nginx config")
		}
		content = content[:insertAt] + loginBlock + content[insertAt:]
		if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
			return fmt.Errorf("write nginx config: %w", err)
		}
	}

	// 4. Write Fail2ban filter (idempotent)
	os.MkdirAll("/etc/fail2ban/filter.d", 0755)
	filterConf := `[Definition]
failregex = ^<HOST> .* "POST .*wp-login\.php
ignoreregex =
`
	if err := os.WriteFile("/etc/fail2ban/filter.d/wordpress.conf", []byte(filterConf), 0644); err != nil {
		return fmt.Errorf("write fail2ban filter: %w", err)
	}

	// 5. Write site-specific Fail2ban jail
	domainEscaped := strings.ReplaceAll(domain, ".", "-")
	os.MkdirAll("/etc/fail2ban/jail.d", 0755)
	jailConf := fmt.Sprintf(`[wordpress-%s]
enabled = true
filter = wordpress
logpath = /var/www/%s/logs/access.log
maxretry = 5
findtime = 60
bantime = 3600
port = http,https
`, domainEscaped, domain)
	jailPath := fmt.Sprintf("/etc/fail2ban/jail.d/wordpress-%s.conf", domainEscaped)
	if err := os.WriteFile(jailPath, []byte(jailConf), 0644); err != nil {
		return fmt.Errorf("write fail2ban jail: %w", err)
	}

	// 6. Reload nginx + restart fail2ban
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	exec.Command("systemctl", "restart", "fail2ban").Run()

	slog.Info("Login rate limit applied", "domain", domain)
	return nil
}

// extractPHPSocket reads the nginx config content and returns the fastcgi_pass
// value from the existing "location ~ \.php$" block. Falls back to a
// constructed path based on the site's PHP version/domain if not found.
func extractPHPSocket(content, domain string) string {
	re := regexp.MustCompile(`(?s)location\s+~\s+\\\.php\$\s*\{[^}]*fastcgi_pass\s+(unix:[^;]+);`)
	if m := re.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	// Fallback: try to read the socket from a simpler inline pattern
	re2 := regexp.MustCompile(`fastcgi_pass\s+(unix:/var/run/php/[^;]+);`)
	if m := re2.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	// Last-resort fallback (PHP 8.1 is common default)
	return fmt.Sprintf("unix:/var/run/php/php8.1-fpm-%s.sock", domain)
}

func revertLimitLogin(domain string) error {
	slog.Info("Reverting login rate limit", "domain", domain)

	// 1. Remove location block from nginx site config
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return fmt.Errorf("read nginx config: %w", err)
	}
	content := string(data)

	if strings.Contains(content, "wp-login") {
		content = removeWPLoginBlock(content)
		if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
			return fmt.Errorf("write nginx config: %w", err)
		}
	}

	// 2. Remove site-specific Fail2ban jail
	domainEscaped := strings.ReplaceAll(domain, ".", "-")
	jailPath := fmt.Sprintf("/etc/fail2ban/jail.d/wordpress-%s.conf", domainEscaped)
	if err := os.Remove(jailPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove fail2ban jail: %w", err)
	}

	// 3. Reload nginx + restart fail2ban
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	exec.Command("systemctl", "restart", "fail2ban").Run()

	slog.Info("Login rate limit reverted", "domain", domain)
	return nil
}

// removeWPLoginBlock strips the "# Rate limit WP login" location block from content.
func removeWPLoginBlock(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inBlock := false
	depth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.Contains(trimmed, "# Rate limit WP login") {
				continue
			}
			if strings.Contains(trimmed, "location ~* /wp-login") {
				inBlock = true
				depth = 1
				continue
			}
			out = append(out, line)
		} else {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				inBlock = false
			}
		}
	}
	result := strings.Join(out, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

// ==================== Original Apply Functions ====================

func disableFileEditor(wpPath string) error {
	slog.Info("Disabling WP file editor", "path", wpPath)

	wpConfig := wpPath + "/wp-config.php"
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		return err
	}

	if strings.Contains(string(data), "DISALLOW_FILE_EDIT") {
		return nil // Already configured
	}

	// Add before "That's all, stop editing!" comment or before require_once
	content := string(data)
	marker := "/* That's all, stop editing!"
	insertStr := "define('DISALLOW_FILE_EDIT', true);\n"

	if idx := strings.Index(content, marker); idx >= 0 {
		content = content[:idx] + insertStr + content[idx:]
	} else {
		// Fallback: add after opening <?php
		content = strings.Replace(content, "<?php", "<?php\n"+insertStr, 1)
	}

	return os.WriteFile(wpConfig, []byte(content), 0644)
}
