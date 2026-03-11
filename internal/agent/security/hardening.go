package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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

	if len(errors) > 0 {
		return fmt.Errorf("hardening partial errors: %s", strings.Join(errors, "; "))
	}

	slog.Info("WordPress hardening completed", "domain", domain)
	return nil
}

type HardenOptions struct {
	DisableXMLRPC     bool
	HideWPVersion     bool
	SecureFilePerms   bool
	SecurityHeaders   bool
	DisableFileEditor bool
}

func disableXMLRPC(domain string) error {
	slog.Info("Disabling XML-RPC", "domain", domain)

	// Add Nginx location block to deny xmlrpc.php
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}

	if strings.Contains(string(data), "xmlrpc.php") {
		return nil // Already configured
	}

	// Add deny block before the closing brace of the server block
	xmlrpcBlock := `
    # Block XML-RPC
    location = /xmlrpc.php {
        deny all;
        access_log off;
        log_not_found off;
    }
`
	content := string(data)
	// Find last closing brace and insert before it
	lastBrace := strings.LastIndex(content, "}")
	if lastBrace > 0 {
		content = content[:lastBrace] + xmlrpcBlock + content[lastBrace:]
	}

	if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
		return err
	}

	// Test and reload Nginx
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

	// Set directory permissions to 755
	exec.Command("find", wpPath, "-type", "d", "-exec", "chmod", "755", "{}", ";").Run()

	// Set file permissions to 644
	exec.Command("find", wpPath, "-type", "f", "-exec", "chmod", "644", "{}", ";").Run()

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

	if strings.Contains(string(data), "X-Content-Type-Options") {
		return nil // Already configured
	}

	headersBlock := `
    # Security Headers
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
`
	content := string(data)
	lastBrace := strings.LastIndex(content, "}")
	if lastBrace > 0 {
		content = content[:lastBrace] + headersBlock + content[lastBrace:]
	}

	if err := os.WriteFile(nginxConf, []byte(content), 0644); err != nil {
		return err
	}

	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()

	return nil
}

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
