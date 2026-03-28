package wordpress

import (
	"fmt"
	"log/slog"
	"os"
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

// phpBinaryForVersion returns the PHP binary path for a given version string (e.g., "8.1" → "/usr/bin/php8.1").
// Falls back to the system default "php" if version is empty or the binary doesn't exist.
func phpBinaryForVersion(phpVersion string) string {
	if phpVersion == "" {
		return "php"
	}
	bin := fmt.Sprintf("/usr/bin/php%s", phpVersion)
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	return "php"
}

// wpcliBinary returns the path to the wp-cli phar/binary.
func wpcliBinary() string {
	for _, p := range []string{"/usr/local/bin/wp", "/usr/bin/wp"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "wp"
}

// allowedWPCLISubcommands is the whitelist of permitted WP-CLI top-level subcommands.
// Anything not in this list is rejected before reaching the shell.
// Dangerous commands (eval, shell, db export/import, scaffold, package) are intentionally excluded.
var allowedWPCLISubcommands = map[string]bool{
	"plugin":         true,
	"theme":          true,
	"core":           true,
	"option":         true,
	"user":           true,
	"post":           true,
	"cache":          true,
	"rewrite":        true,
	"cron":           true,
	"search-replace": true,
	"media":          true,
	"language":       true,
	"widget":         true,
	"menu":           true,
	"site":           true,
	"config":         true,
	"transient":      true,
	"maintenance-mode": true,
	"comment":        true,
	"term":           true,
	"taxonomy":       true,
	"role":           true,
	"cap":            true,
	"import":         true,
	"export":         true,
}

// ExecuteWPCLI runs a wp-cli command for a given domain using the site's configured PHP version.
// The WP-CLI process is invoked via "php<version> /usr/local/bin/wp" so that ionCube-encoded
// theme/plugin files are loaded in the correct PHP runtime.
func ExecuteWPCLI(domain, phpVersion, command string) (*WPCLIResult, error) {
	wpPath := fmt.Sprintf("/var/www/%s/public", domain)

	// Security: whitelist-only approach.
	// Extract the first word (subcommand) and check against the allowed list.
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return &WPCLIResult{Output: "empty command", ExitCode: 1}, nil
	}
	subCmd := strings.ToLower(fields[0])
	if !allowedWPCLISubcommands[subCmd] {
		return &WPCLIResult{
			Output:   fmt.Sprintf("WP-CLI subcommand '%s' is not permitted. Allowed: plugin, theme, core, option, user, post, cache, rewrite, cron, search-replace, media, language, widget, menu, site, config, transient, maintenance-mode, comment, term, taxonomy, role, cap, import, export", subCmd),
			ExitCode: 1,
		}, nil
	}

	// Secondary guard: reject any argument that looks like a shell escape attempt
	for _, arg := range fields[1:] {
		if strings.ContainsAny(arg, ";&|`$(){}\\") {
			return &WPCLIResult{
				Output:   "command contains disallowed shell metacharacters",
				ExitCode: 1,
			}, nil
		}
	}

	phpBin := phpBinaryForVersion(phpVersion)
	wpBin := wpcliBinary()
	siteUser := siteUserForDomain(domain)

	// Invoke WP-CLI via the site's PHP binary so ionCube-encoded files run in the correct PHP version.
	// e.g.: sudo -u wp_example_com /usr/bin/php8.1 /usr/local/bin/wp --path=... plugin list
	wpArgs := append([]string{phpBin, wpBin, "--path=" + wpPath}, strings.Fields(command)...)
	allArgs := append([]string{"-u", siteUser}, wpArgs...)

	slog.Info("Executing WP-CLI", "domain", domain, "command", command, "user", siteUser, "php", phpBin)
	cmd := exec.Command("sudo", allArgs...)
	output, err := cmd.CombinedOutput()

	// Fallback: try as root with --allow-root using the same PHP binary
	if err != nil {
		rootArgs := []string{phpBin, wpBin, "--path=" + wpPath, "--allow-root"}
		rootArgs = append(rootArgs, strings.Fields(command)...)
		cmd2 := exec.Command("sudo", rootArgs...)
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
