package wordpress

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// siteUserForDomain derives the Linux username for a WordPress site (matches site/manager.go).
func siteUserForDomain(domain string) string {
	return "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
}

type WPCLIResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// ExecuteWPCLI runs a wp-cli command for a given domain
func ExecuteWPCLI(domain, command string) (*WPCLIResult, error) {
	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	// Security: block dangerous commands
	blocked := []string{"eval", "shell", "db export", "db import"}
	cmdLower := strings.ToLower(command)
	for _, b := range blocked {
		if strings.Contains(cmdLower, b) {
			return &WPCLIResult{
				Output:   fmt.Sprintf("Command '%s' is blocked for security reasons", b),
				ExitCode: 1,
			}, nil
		}
	}

	// Build wp command — run as the site's Linux user (not root)
	siteUser := siteUserForDomain(domain)
	wpArgs := append([]string{"--path=" + wpPath}, strings.Fields(command)...)
	allArgs := append([]string{"-u", siteUser, "wp"}, wpArgs...)

	slog.Info("Executing WP-CLI", "domain", domain, "command", command, "user", siteUser)
	cmd := exec.Command("sudo", allArgs...)
	output, err := cmd.CombinedOutput()

	// Fallback: if sudo fails, try as root with --allow-root
	if err != nil {
		rootArgs := append([]string{"--path=" + wpPath, "--allow-root"}, strings.Fields(command)...)
		cmd2 := exec.Command("wp", rootArgs...)
		if out2, err2 := cmd2.CombinedOutput(); err2 == nil {
			output = out2
			err = nil
			slog.Info("WP-CLI fell back to root", "domain", domain, "command", command)
		}
	}

	result := &WPCLIResult{
		Output:   string(output),
		ExitCode: cmd.ProcessState.ExitCode(),
	}

	if err != nil {
		result.Error = err.Error()
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
	}

	return result, nil
}
