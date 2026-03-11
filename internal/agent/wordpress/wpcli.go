package wordpress

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

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

	// Build wp command
	args := []string{"--path=" + wpPath, "--allow-root"}
	args = append(args, strings.Fields(command)...)

	slog.Info("Executing WP-CLI", "domain", domain, "command", command)
	cmd := exec.Command("wp", args...)
	output, err := cmd.CombinedOutput()

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
