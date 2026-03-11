package opcache

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// Enable enables OPcache in php.ini
func Enable(phpVersion string) error {
	return toggleOPcache(phpVersion, true)
}

// Disable disables OPcache in php.ini
func Disable(phpVersion string) error {
	return toggleOPcache(phpVersion, false)
}

// Flush flushes OPcache by creating a temp PHP script and executing it
func Flush(phpVersion string) error {
	// Create a temp PHP script to flush OPcache
	flushScript := "/tmp/opcache_flush.php"
	content := `<?php
if (function_exists('opcache_reset')) {
    opcache_reset();
    echo json_encode(["status" => "flushed", "success" => true]);
} else {
    echo json_encode(["status" => "opcache not available", "success" => false]);
}
`
	if err := os.WriteFile(flushScript, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create flush script: %w", err)
	}
	defer os.Remove(flushScript)

	cmd := exec.Command(fmt.Sprintf("php%s", phpVersion), flushScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opcache flush failed: %s (%w)", string(out), err)
	}

	slog.Info("OPcache flushed", "output", strings.TrimSpace(string(out)))
	return nil
}

// GetStats retrieves OPcache statistics
func GetStats(phpVersion string) (string, error) {
	statsScript := "/tmp/opcache_stats.php"
	content := `<?php
if (function_exists('opcache_get_status')) {
    $status = opcache_get_status(false);
    if ($status === false) {
        echo json_encode(["enabled" => false, "error" => "OPcache not enabled"]);
    } else {
        echo json_encode([
            "enabled" => true,
            "memory_usage" => $status["memory_usage"] ?? null,
            "statistics" => $status["opcache_statistics"] ?? null,
            "configuration" => opcache_get_configuration() ?? null,
        ]);
    }
} else {
    echo json_encode(["enabled" => false, "error" => "OPcache extension not loaded"]);
}
`
	if err := os.WriteFile(statsScript, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to create stats script: %w", err)
	}
	defer os.Remove(statsScript)

	cmd := exec.Command(fmt.Sprintf("php%s", phpVersion), statsScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("opcache stats failed: %s (%w)", string(out), err)
	}

	return strings.TrimSpace(string(out)), nil
}

// toggleOPcache enables/disables OPcache for a PHP version.
//
// Ubuntu/Debian PHP loads ini files in this order:
//   1. /etc/php/{ver}/{sapi}/php.ini
//   2. /etc/php/{ver}/{sapi}/conf.d/*.ini  (alphabetical, so 10-opcache.ini loads early)
//
// Writing to php.ini has NO effect because conf.d/10-opcache.ini is loaded AFTER it
// and overrides any setting there. The correct fix is to create a priority-99 override
// file (conf.d/99-opcache-wp-manage.ini) which loads LAST and wins.
func toggleOPcache(phpVersion string, enable bool) error {
	val := "0"
	if enable {
		val = "1"
	}
	content := fmt.Sprintf("; Managed by WP Manage — do not edit manually\nopcache.enable=%s\n", val)

	// Write override to both fpm and cli conf.d (cli needed for ini_get status check)
	sapis := []string{"fpm", "cli"}
	for _, sapi := range sapis {
		confDir := fmt.Sprintf("/etc/php/%s/%s/conf.d", phpVersion, sapi)
		if _, err := os.Stat(confDir); os.IsNotExist(err) {
			continue
		}
		overridePath := fmt.Sprintf("%s/99-opcache-wp-manage.ini", confDir)
		if err := os.WriteFile(overridePath, []byte(content), 0644); err != nil {
			slog.Error("Failed to write OPcache override", "path", overridePath, "error", err)
		}
	}

	// Restart PHP-FPM to apply
	cmd := exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", phpVersion))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart php-fpm: %s (%w)", string(out), err)
	}

	action := "disabled"
	if enable {
		action = "enabled"
	}
	slog.Info("OPcache toggled", "version", phpVersion, "action", action)
	return nil
}
