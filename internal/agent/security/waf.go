package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// ConfigureWAF enables/disables ModSecurity WAF for a site's nginx config
func ConfigureWAF(task models.AgentWAFTask) error {
	slog.Info("Configuring WAF", "domain", task.Domain, "enabled", task.Enabled)

	// Ensure ModSecurity is installed
	if err := ensureModSecurityInstalled(); err != nil {
		return fmt.Errorf("failed to install ModSecurity: %w", err)
	}

	// Build modsecurity config for this site
	confPath := fmt.Sprintf("/etc/nginx/modsec/%s.conf", task.Domain)

	if task.Enabled {
		// Create modsecurity config
		config := buildModSecConfig(task)
		if err := os.WriteFile(confPath, []byte(config), 0644); err != nil {
			return fmt.Errorf("failed to write modsec config: %w", err)
		}

		// Ensure modsecurity_rules_file is included in nginx vhost
		nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", task.Domain)
		if err := ensureModSecInNginx(nginxConf, confPath); err != nil {
			return fmt.Errorf("failed to update nginx config: %w", err)
		}
	} else {
		// Remove modsecurity config and nginx include
		os.Remove(confPath)
		nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", task.Domain)
		removeModSecFromNginx(nginxConf)
	}

	// Reload nginx
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("WAF configured", "domain", task.Domain, "enabled", task.Enabled)
	return nil
}

func ensureModSecurityInstalled() error {
	// Check if modsecurity module exists
	if _, err := os.Stat("/etc/nginx/modules-enabled/50-mod-http-modsecurity.conf"); err == nil {
		return nil // already installed
	}
	// Also check for libmodsecurity
	if _, err := exec.LookPath("modsecurity"); err == nil {
		return nil
	}

	slog.Info("Installing ModSecurity for Nginx")
	cmds := [][]string{
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "libnginx-mod-http-modsecurity"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("ModSecurity install command output", "cmd", args, "output", string(out))
			// Don't fail — modsecurity might work without the package on some distros
		}
	}

	// Create modsec directory
	os.MkdirAll("/etc/nginx/modsec", 0755)

	// Create main modsecurity.conf if it doesn't exist
	mainConf := "/etc/nginx/modsec/modsecurity.conf"
	if _, err := os.Stat(mainConf); os.IsNotExist(err) {
		defaultConf := `SecRuleEngine On
SecRequestBodyAccess On
SecResponseBodyAccess Off
SecRequestBodyLimit 13107200
SecRequestBodyNoFilesLimit 131072
SecAuditEngine RelevantOnly
SecAuditLogRelevantStatus "^(?:5|4(?!04))"
SecAuditLogType Serial
SecAuditLog /var/log/nginx/modsec_audit.log
`
		os.WriteFile(mainConf, []byte(defaultConf), 0644)
	}

	return nil
}

func buildModSecConfig(task models.AgentWAFTask) string {
	var sb strings.Builder

	sb.WriteString("# WAF Configuration for " + task.Domain + "\n")
	sb.WriteString("SecRuleEngine On\n\n")

	// Set paranoid level
	sb.WriteString(fmt.Sprintf("# Paranoid Level: %d\n", task.ParanoidLevel))
	sb.WriteString(fmt.Sprintf("SecAction \"id:900000,phase:1,nolog,pass,t:none,setvar:tx.paranoia_level=%d\"\n\n", task.ParanoidLevel))

	// Whitelist IPs
	if task.WhitelistedIPs != "" {
		ips := strings.Split(task.WhitelistedIPs, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				sb.WriteString(fmt.Sprintf("SecRule REMOTE_ADDR \"@ipMatch %s\" \"id:900100,phase:1,allow,nolog\"\n", ip))
			}
		}
		sb.WriteString("\n")
	}

	// Include OWASP CRS if available
	crsPath := "/usr/share/modsecurity-crs"
	if _, err := os.Stat(crsPath); err == nil {
		sb.WriteString("Include " + crsPath + "/crs-setup.conf\n")
		sb.WriteString("Include " + crsPath + "/rules/*.conf\n")
	}

	return sb.String()
}

func ensureModSecInNginx(nginxConf, modsecConf string) error {
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return err
	}

	content := string(data)
	modsecLine := fmt.Sprintf("    modsecurity on;\n    modsecurity_rules_file %s;", modsecConf)

	// Check if already included
	if strings.Contains(content, "modsecurity on") {
		return nil
	}

	// Add after first "server {" block's opening
	content = strings.Replace(content, "server {", "server {\n"+modsecLine, 1)
	return os.WriteFile(nginxConf, []byte(content), 0644)
}

func removeModSecFromNginx(nginxConf string) {
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	var newLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "modsecurity ") || strings.HasPrefix(trimmed, "modsecurity_rules_file") {
			continue
		}
		newLines = append(newLines, line)
	}
	os.WriteFile(nginxConf, []byte(strings.Join(newLines, "\n")), 0644)
}
