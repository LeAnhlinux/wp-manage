package migration

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// siteUser derives the Linux username for a WordPress site (matches site/manager.go).
func siteUser(domain string) string {
	return "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
}

type ImportRunner struct {
	apiURL string
	apiKey string
}

func NewImportRunner(apiURL, apiKey string) *ImportRunner {
	return &ImportRunner{apiURL: apiURL, apiKey: apiKey}
}

// RunImport imports a WordPress site from an external server
func (r *ImportRunner) RunImport(domain, sourceIP, sshUser string, sshPort int, sshKey, dbName, dbUser, dbPass, phpVersion string) error {
	wpPath := fmt.Sprintf("/var/www/%s/html", domain)
	sshTarget := fmt.Sprintf("%s@%s", sshUser, sourceIP)
	sshOpts := fmt.Sprintf("-o StrictHostKeyChecking=no -p %d", sshPort)
	if sshKey != "" {
		sshOpts += " -i /tmp/import_key"
		// Write SSH key temporarily
		if err := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' > /tmp/import_key && chmod 600 /tmp/import_key", sshKey)).Run(); err != nil {
			return fmt.Errorf("failed to write SSH key: %w", err)
		}
		defer exec.Command("rm", "-f", "/tmp/import_key").Run()
	}

	slog.Info("Starting import", "domain", domain, "source", sourceIP)

	// Step 1: Rsync WordPress files
	slog.Info("Importing files via rsync", "domain", domain)
	rsyncCmd := fmt.Sprintf("rsync -avz --delete -e 'ssh %s' %s:/var/www/%s/html/ %s/",
		sshOpts, sshTarget, domain, wpPath)
	if output, err := exec.Command("bash", "-c", rsyncCmd).CombinedOutput(); err != nil {
		slog.Warn("Rsync warning", "output", string(output))
		// Try alternative path
		rsyncCmd = fmt.Sprintf("rsync -avz --delete -e 'ssh %s' %s:/home/%s/public_html/ %s/",
			sshOpts, sshTarget, domain, wpPath)
		if output2, err2 := exec.Command("bash", "-c", rsyncCmd).CombinedOutput(); err2 != nil {
			return fmt.Errorf("rsync failed: %s / %s", string(output), string(output2))
		}
	}

	// Step 2: Import database
	if dbName != "" {
		slog.Info("Importing database", "domain", domain, "db", dbName)
		dumpCmd := fmt.Sprintf("ssh %s %s 'mysqldump --single-transaction %s' | mysql %s",
			sshOpts, sshTarget, dbName, dbName)
		if output, err := exec.Command("bash", "-c", dumpCmd).CombinedOutput(); err != nil {
			return fmt.Errorf("database import failed: %s - %w", string(output), err)
		}
	}

	// Step 3: Update wp-config.php with local DB credentials
	if dbUser != "" && dbPass != "" {
		slog.Info("Updating wp-config.php", "domain", domain)
		wpConfigFile := wpPath + "/wp-config.php"
		sedCmds := []string{
			fmt.Sprintf("sed -i \"s/define.*DB_USER.*/define('DB_USER', '%s');/\" %s", dbUser, wpConfigFile),
			fmt.Sprintf("sed -i \"s/define.*DB_PASSWORD.*/define('DB_PASSWORD', '%s');/\" %s", dbPass, wpConfigFile),
			fmt.Sprintf("sed -i \"s/define.*DB_HOST.*/define('DB_HOST', 'localhost');/\" %s", wpConfigFile),
		}
		for _, cmd := range sedCmds {
			exec.Command("bash", "-c", cmd).Run()
		}
	}

	// Step 4: Fix permissions — use the site's dedicated Linux user
	slog.Info("Fixing permissions", "domain", domain)
	su := siteUser(domain)
	exec.Command("chown", "-R", fmt.Sprintf("%s:%s", su, su), wpPath).Run()
	exec.Command("find", wpPath, "-type", "f", "-exec", "chmod", "644", "{}", ";").Run()
	exec.Command("find", wpPath, "-type", "d", "-exec", "chmod", "2755", "{}", ";").Run()

	// Step 5: Search-replace if domain changed
	slog.Info("Running search-replace", "domain", domain)
	srCmd := fmt.Sprintf("wp search-replace 'http://%s' 'https://%s' --path=%s --allow-root --all-tables 2>/dev/null || true",
		domain, domain, wpPath)
	exec.Command("bash", "-c", srCmd).Run()

	slog.Info("Import completed", "domain", domain)
	return nil
}

// QuickImport does a simple file + database import without SSH key
func QuickImport(domain, sourceIP, dbName string) error {
	runner := &ImportRunner{}
	return runner.RunImport(domain, sourceIP, "root", 22, "", dbName, "", "", "8.2")
}
