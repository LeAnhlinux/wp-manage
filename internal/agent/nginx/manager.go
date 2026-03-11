package nginx

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// ApplyCustomConfig writes a custom Nginx config snippet and reloads
func ApplyCustomConfig(domain, configType, name, content string, enabled bool) error {
	// Determine config file path
	confDir := "/etc/nginx/snippets/custom"
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("failed to create snippets dir: %w", err)
	}

	// Sanitize name for filename
	safeName := strings.ReplaceAll(strings.ReplaceAll(name, " ", "_"), "/", "_")
	confFile := fmt.Sprintf("%s/%s_%s.conf", confDir, safeName, configType)

	if !enabled {
		// Disable by removing
		os.Remove(confFile)
		return testAndReload()
	}

	// Build config content based on type
	var finalContent string
	switch configType {
	case "redirect":
		finalContent = content // User provides the full rewrite/return directive
	case "custom_location":
		finalContent = content // User provides the full location block
	case "rate_limit":
		finalContent = fmt.Sprintf("# Rate limit: %s\n%s", name, content)
	case "security_header":
		finalContent = fmt.Sprintf("# Security header: %s\n%s", name, content)
	default:
		finalContent = fmt.Sprintf("# Custom config: %s\n%s", name, content)
	}

	if err := os.WriteFile(confFile, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Include in vhost if domain specified
	if domain != "" {
		if err := ensureInclude(domain, confFile); err != nil {
			slog.Warn("Could not auto-include snippet", "error", err)
		}
	}

	return testAndReload()
}

// RemoveCustomConfig removes a custom Nginx config snippet
func RemoveCustomConfig(configType, name string) error {
	confDir := "/etc/nginx/snippets/custom"
	safeName := strings.ReplaceAll(strings.ReplaceAll(name, " ", "_"), "/", "_")
	confFile := fmt.Sprintf("%s/%s_%s.conf", confDir, safeName, configType)

	os.Remove(confFile)
	return testAndReload()
}

// TestConfig runs nginx -t
func TestConfig() error {
	cmd := exec.Command("nginx", "-t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s (%w)", string(out), err)
	}
	return nil
}

// Reload reloads Nginx
func Reload() error {
	cmd := exec.Command("systemctl", "reload", "nginx")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload failed: %s (%w)", string(out), err)
	}
	return nil
}

// testAndReload tests config then reloads
func testAndReload() error {
	if err := TestConfig(); err != nil {
		return err
	}
	return Reload()
}

// ensureInclude adds an include directive to a site's vhost if not already present
func ensureInclude(domain, confFile string) error {
	vhostPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(vhostPath)
	if err != nil {
		return err
	}

	includeLine := fmt.Sprintf("    include %s;", confFile)
	if strings.Contains(string(data), includeLine) {
		return nil // already included
	}

	// Insert include before the last closing brace of the server block
	content := string(data)
	lastBrace := strings.LastIndex(content, "}")
	if lastBrace < 0 {
		return fmt.Errorf("invalid vhost config")
	}

	newContent := content[:lastBrace] + "\n" + includeLine + "\n" + content[lastBrace:]
	return os.WriteFile(vhostPath, []byte(newContent), 0644)
}
