package firewall

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// EnsureUFWInstalled installs and enables UFW if not present.
// CRITICAL: pre-allows SSH (22), HTTP (80), HTTPS (443), and Agent (8443)
// before enabling to avoid being locked out of the server.
func EnsureUFWInstalled() error {
	if _, err := exec.LookPath("ufw"); err == nil {
		return nil // already installed
	}

	slog.Info("UFW not found, installing...")
	out, err := exec.Command("apt-get", "install", "-y", "ufw").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install ufw: %s: %w", string(out), err)
	}

	// Pre-allow critical ports BEFORE enabling to avoid SSH lockout
	critical := []string{"22/tcp", "80/tcp", "443/tcp", "8443/tcp"}
	for _, rule := range critical {
		if out, err := exec.Command("ufw", "allow", rule).CombinedOutput(); err != nil {
			slog.Warn("Failed to pre-allow port", "rule", rule, "error", string(out))
		}
	}

	// Enable UFW non-interactively
	out, err = exec.Command("ufw", "--force", "enable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable ufw: %s: %w", string(out), err)
	}

	slog.Info("UFW installed and enabled")
	return nil
}

// EnsureEnabled ensures UFW is active even if already installed
func EnsureEnabled() {
	out, _ := exec.Command("ufw", "status").Output()
	if strings.Contains(string(out), "inactive") {
		// Pre-allow critical ports before enabling
		for _, rule := range []string{"22/tcp", "80/tcp", "443/tcp", "8443/tcp"} {
			exec.Command("ufw", "allow", rule).Run()
		}
		exec.Command("ufw", "--force", "enable").Run()
		slog.Info("UFW enabled")
	}
}

// AddRule adds a UFW firewall rule
func AddRule(ruleType, protocol, port, source, comment string) error {
	args := buildUFWArgs(ruleType, protocol, port, source, comment)
	slog.Info("Adding firewall rule", "args", strings.Join(args, " "))

	cmd := exec.Command("ufw", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw %s failed: %s (%w)", strings.Join(args, " "), string(out), err)
	}
	slog.Info("Firewall rule added", "output", strings.TrimSpace(string(out)))
	return nil
}

// DeleteRule removes a UFW firewall rule
func DeleteRule(ruleType, protocol, port, source string) error {
	args := []string{"delete"}
	args = append(args, buildUFWArgs(ruleType, protocol, port, source, "")...)
	slog.Info("Deleting firewall rule", "args", strings.Join(args, " "))

	cmd := exec.Command("ufw", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw delete failed: %s (%w)", string(out), err)
	}
	slog.Info("Firewall rule deleted", "output", strings.TrimSpace(string(out)))
	return nil
}

// GetStatus returns current UFW status and rules
func GetStatus() (string, error) {
	cmd := exec.Command("ufw", "status", "numbered")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ufw status failed: %s (%w)", string(out), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// buildUFWArgs constructs UFW command arguments
func buildUFWArgs(ruleType, protocol, port, source, comment string) []string {
	var args []string

	args = append(args, ruleType) // allow or deny

	if source != "" {
		args = append(args, "from", source)
	}

	args = append(args, "to", "any")

	if protocol == "both" {
		args = append(args, "port", port)
	} else {
		args = append(args, "port", port, "proto", protocol)
	}

	if comment != "" {
		args = append(args, "comment", comment)
	}

	return args
}
