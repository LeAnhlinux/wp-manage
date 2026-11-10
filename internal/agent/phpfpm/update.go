package phpfpm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// Patch updates for installed PHP branches. The PHP packages come from a
// third-party repository that the distribution's unattended upgrades do not
// cover, so without this they stay on the version that was first installed.

var preReleaseRe = regexp.MustCompile(`(?i)(alpha|beta|rc)\d*`)

// upstreamVersion strips the epoch and the distribution revision from a
// package version: "1:7.4.33-31+ubuntu24.04.1+deb.sury.org+1" → "7.4.33".
func upstreamVersion(pkgVersion string) string {
	v := pkgVersion
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	if i := strings.LastIndex(v, "-"); i >= 0 {
		v = v[:i]
	}
	return v
}

// isPreRelease reports whether a version is an alpha, beta or release candidate.
func isPreRelease(version string) bool {
	return preReleaseRe.MatchString(version)
}

// parseAptPolicy reads the Installed / Candidate lines of `apt-cache policy`.
// "(none)" and missing lines come back as "".
func parseAptPolicy(out string) (installed, candidate string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "Installed:"); ok {
			installed = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "Candidate:"); ok {
			candidate = strings.TrimSpace(v)
		}
	}
	if installed == "(none)" {
		installed = ""
	}
	if candidate == "(none)" {
		candidate = ""
	}
	return installed, candidate
}

// buildStatus turns the package versions of php{branch}-fpm into a status row.
func buildStatus(branch, installedPkg, candidatePkg string) models.PHPUpdateStatus {
	s := models.PHPUpdateStatus{
		Version:          branch,
		Installed:        installedPkg != "",
		InstalledVersion: upstreamVersion(installedPkg),
		CandidateVersion: upstreamVersion(candidatePkg),
	}
	s.PreRelease = isPreRelease(s.CandidateVersion)
	if s.Installed && candidatePkg != "" && candidatePkg != installedPkg {
		s.UpdateAvailable = true
		s.PackageRebuild = s.CandidateVersion == s.InstalledVersion
	}
	return s
}

func aptEnv() []string {
	return append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_SUSPEND=1")
}

// aptUpdate refreshes the package lists.
func aptUpdate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", "update", "-q")
	cmd.Env = aptEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get update failed: %s", tailText(string(out), 300))
	}
	return nil
}

func tailText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

func policyFor(branch string) (installed, candidate string) {
	out, _ := exec.Command("apt-cache", "policy", "php"+branch+"-fpm").Output()
	return parseAptPolicy(string(out))
}

// UpdateStatus reports, for every managed branch, what is installed and what
// the repository offers. With refresh it runs `apt-get update` first.
func UpdateStatus(refresh bool) ([]models.PHPUpdateStatus, error) {
	if _, err := exec.LookPath("apt-cache"); err != nil {
		return nil, fmt.Errorf("PHP update checks are only supported on Debian/Ubuntu")
	}
	var refreshErr error
	if refresh {
		refreshErr = aptUpdate()
	}
	out := make([]models.PHPUpdateStatus, 0, len(models.PHPVersions))
	for _, branch := range models.PHPVersions {
		installed, candidate := policyFor(branch)
		out = append(out, buildStatus(branch, installed, candidate))
	}
	return out, refreshErr
}

// parseInstalledPackages picks the fully installed php{branch} packages out of
// `dpkg-query -W -f='${Package} ${db:Status-Abbrev}\n' 'php{branch}*'`.
func parseInstalledPackages(out, branch string) []string {
	prefix := "php" + branch
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "ii") {
			continue
		}
		name := fields[0]
		// "php8.3" itself or "php8.3-<ext>", but not "php8.30-…"
		if name == prefix || strings.HasPrefix(name, prefix+"-") {
			pkgs = append(pkgs, name)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

func installedPackages(branch string) []string {
	out, _ := exec.Command("dpkg-query", "-W", "-f=${Package} ${db:Status-Abbrev}\n", "php"+branch+"*").Output()
	return parseInstalledPackages(string(out), branch)
}

// envUsesPHP reports whether a dedicated-FPM env file runs the given branch.
func envUsesPHP(env, branch string) bool {
	for _, line := range strings.Split(env, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "FPM_BIN="); ok {
			return strings.HasSuffix(strings.TrimSpace(v), "php-fpm"+branch)
		}
	}
	return false
}

// restartDedicatedFPM restarts the per-site FPM masters (sites with resource
// limits) that run the given branch, so they pick up the new binary too.
func restartDedicatedFPM(branch string) []string {
	var restarted []string
	files, _ := filepath.Glob(wpmFPMDir + "/*.env")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || !envUsesPHP(string(data), branch) {
			continue
		}
		domain := strings.TrimSuffix(filepath.Base(f), ".env")
		unit := serviceName(domain)
		if out, _ := exec.Command("systemctl", "is-active", unit).Output(); strings.TrimSpace(string(out)) != "active" {
			continue
		}
		if err := exec.Command("systemctl", "restart", unit).Run(); err != nil {
			slog.Warn("Could not restart dedicated PHP-FPM after update", "unit", unit, "error", err)
			continue
		}
		restarted = append(restarted, domain)
	}
	return restarted
}

// UpdateBranch upgrades every installed package of one PHP branch to the
// repository's current version, keeps all existing configuration files, then
// restarts PHP-FPM for that branch and verifies it is running.
func UpdateBranch(branch string) (string, error) {
	if !models.ValidPHPVersion(branch) {
		return "", fmt.Errorf("unsupported PHP version %q", branch)
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return "", fmt.Errorf("PHP updates are only supported on Debian/Ubuntu")
	}
	if _, err := os.Stat(fmt.Sprintf("/etc/php/%s/fpm", branch)); err != nil {
		return "", fmt.Errorf("PHP %s is not installed on this server", branch)
	}
	if err := aptUpdate(); err != nil {
		slog.Warn("apt-get update failed before PHP update, using cached package lists", "error", err)
	}

	before, candidate := policyFor(branch)
	if candidate == "" || candidate == before {
		return fmt.Sprintf("PHP %s is already up to date (%s)", branch, upstreamVersion(before)), nil
	}
	pkgs := installedPackages(branch)
	if len(pkgs) == 0 {
		return "", fmt.Errorf("no installed packages found for PHP %s", branch)
	}

	// --only-upgrade: never install anything new. --force-confold: php.ini and
	// pool files the panel or the admin changed are kept, never replaced by the
	// package's defaults.
	args := []string{"install", "-y", "-q", "--only-upgrade",
		"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
	args = append(args, pkgs...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", args...)
	cmd.Env = aptEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("apt-get could not update PHP %s: %s", branch, tailText(string(out), 500))
	}

	fpmBin := "php-fpm" + branch
	if out, err := exec.Command(fpmBin, "-t").CombinedOutput(); err != nil {
		return "", fmt.Errorf("PHP %s packages were updated but the PHP-FPM config test fails — PHP-FPM was not restarted: %s", branch, tailText(string(out), 400))
	}
	service := "php" + branch + "-fpm"
	if out, err := exec.Command("systemctl", "restart", service).CombinedOutput(); err != nil {
		return "", fmt.Errorf("PHP %s packages were updated but %s failed to restart: %s", branch, service, tailText(string(out), 300))
	}
	dedicated := restartDedicatedFPM(branch)
	time.Sleep(2 * time.Second)
	if out, _ := exec.Command("systemctl", "is-active", service).Output(); strings.TrimSpace(string(out)) != "active" {
		return "", fmt.Errorf("PHP %s packages were updated but %s is not running after the restart", branch, service)
	}

	after, _ := policyFor(branch)
	msg := fmt.Sprintf("PHP %s updated: %s → %s (%d packages)", branch, upstreamVersion(before), upstreamVersion(after), len(pkgs))
	if upstreamVersion(before) == upstreamVersion(after) {
		msg = fmt.Sprintf("PHP %s packages rebuilt for %s (%d packages)", branch, upstreamVersion(after), len(pkgs))
	}
	if len(dedicated) > 0 {
		msg += fmt.Sprintf("; restarted %d dedicated PHP-FPM service(s)", len(dedicated))
	}
	slog.Info("PHP branch updated", "branch", branch, "from", before, "to", after)
	return msg, nil
}
