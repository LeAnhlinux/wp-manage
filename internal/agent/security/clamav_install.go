package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// InstallClamAV installs ClamAV and updates virus definitions
func InstallClamAV() error {
	slog.Info("Installing ClamAV")

	// Install clamav + clamav-freshclam
	cmd := exec.Command("apt-get", "install", "-y", "clamav", "clamav-freshclam")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apt-get install failed: %w — %s", err, strings.TrimSpace(string(out)))
	}
	slog.Info("ClamAV packages installed")

	// Stop freshclam daemon if running (it locks the DB during update)
	exec.Command("systemctl", "stop", "clamav-freshclam").Run()

	// Update virus definitions
	slog.Info("Updating ClamAV virus definitions (this may take a few minutes)")
	freshclamCmd := exec.Command("freshclam")
	freshclamOut, err := freshclamCmd.CombinedOutput()
	if err != nil {
		// freshclam exits non-zero if DB is already up to date — not a real error
		output := string(freshclamOut)
		if !strings.Contains(output, "up to date") && !strings.Contains(output, "up-to-date") {
			slog.Warn("freshclam update warning", "output", output)
		}
	}
	slog.Info("ClamAV definitions updated")

	// Enable and start freshclam daemon for automatic updates
	exec.Command("systemctl", "enable", "clamav-freshclam").Run()
	exec.Command("systemctl", "start", "clamav-freshclam").Run()

	slog.Info("ClamAV installation complete")
	return nil
}

// GetClamAVStatus returns installation status and version info
func GetClamAVStatus() models.AgentClamAVStatusResult {
	// Check if clamscan binary exists
	clamscanPath, err := exec.LookPath("clamscan")
	if err != nil {
		return models.AgentClamAVStatusResult{Installed: false}
	}
	_ = clamscanPath

	// Get ClamAV version
	versionOut, err := exec.Command("clamscan", "--version").Output()
	if err != nil {
		return models.AgentClamAVStatusResult{
			Installed: false,
			Error:     fmt.Sprintf("clamscan --version failed: %s", err),
		}
	}
	version := strings.TrimSpace(strings.Split(string(versionOut), "\n")[0])

	// Get database info from freshclam --version output
	dbVersion := ""
	lastUpdated := ""

	// Try to read daily.cld or daily.cvd modification time
	for _, dbPath := range []string{
		"/var/lib/clamav/daily.cld",
		"/var/lib/clamav/daily.cvd",
	} {
		if info, err := os.Stat(dbPath); err == nil {
			lastUpdated = info.ModTime().Format("2006-01-02 15:04:05")
			// Get DB version from sigtool if available
			if sigtoolOut, err := exec.Command("sigtool", "--info", dbPath).Output(); err == nil {
				for _, line := range strings.Split(string(sigtoolOut), "\n") {
					if strings.HasPrefix(line, "Version:") {
						dbVersion = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
						dbVersion = "daily.cld v" + dbVersion
						break
					}
				}
			} else {
				dbVersion = "daily.cld (updated " + lastUpdated + ")"
			}
			break
		}
	}

	if dbVersion == "" {
		dbVersion = "unknown"
	}

	return models.AgentClamAVStatusResult{
		Installed:   true,
		Version:     version,
		DBVersion:   dbVersion,
		LastUpdated: lastUpdated,
	}
}
