package ssh

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const authorizedKeysPath = "/root/.ssh/authorized_keys"

// AddKey adds an SSH public key to the authorized_keys file
func AddKey(name, publicKey string) error {
	slog.Info("Adding SSH key", "name", name)

	// Ensure .ssh directory exists
	if err := os.MkdirAll("/root/.ssh", 0700); err != nil {
		return fmt.Errorf("failed to create .ssh dir: %w", err)
	}

	// Read existing keys
	existing, _ := os.ReadFile(authorizedKeysPath)

	// Check if key already exists
	if strings.Contains(string(existing), strings.TrimSpace(publicKey)) {
		slog.Info("SSH key already exists", "name", name)
		return nil
	}

	// Append key with comment
	entry := fmt.Sprintf("\n# %s\n%s\n", name, strings.TrimSpace(publicKey))
	f, err := os.OpenFile(authorizedKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open authorized_keys: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	slog.Info("SSH key added successfully", "name", name)
	return nil
}

// RemoveKey removes an SSH public key from the authorized_keys file
func RemoveKey(name, publicKey string) error {
	slog.Info("Removing SSH key", "name", name)

	data, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		return fmt.Errorf("failed to read authorized_keys: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	keyTrimmed := strings.TrimSpace(publicKey)
	skipNext := false

	for _, line := range lines {
		// Skip the comment line preceding the key
		if strings.TrimSpace(line) == fmt.Sprintf("# %s", name) {
			skipNext = true
			continue
		}
		if skipNext && strings.TrimSpace(line) == keyTrimmed {
			skipNext = false
			continue
		}
		skipNext = false

		// Also remove if the key line matches directly (without comment)
		if strings.TrimSpace(line) == keyTrimmed {
			continue
		}

		newLines = append(newLines, line)
	}

	if err := os.WriteFile(authorizedKeysPath, []byte(strings.Join(newLines, "\n")), 0600); err != nil {
		return fmt.Errorf("failed to write authorized_keys: %w", err)
	}

	slog.Info("SSH key removed successfully", "name", name)
	return nil
}

// TestConnection tests SSH connectivity
func TestConnection() error {
	cmd := exec.Command("ssh", "-V")
	return cmd.Run()
}
