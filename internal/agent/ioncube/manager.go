package ioncube

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	cacheDir    = "/opt/wp-manage/ioncube"
	downloadURL = "https://downloads.ioncube.com/loader_downloads/ioncube_loaders_lin_x86-64.tar.gz"
	tarballName = "ioncube_loaders_lin_x86-64.tar.gz"
)

// SupportedVersions lists PHP versions ionCube supports (8.4+ not yet supported).
var SupportedVersions = []string{"7.1", "7.2", "7.3", "7.4", "8.0", "8.1", "8.2", "8.3"}

// VersionStatus describes ionCube state for one PHP version.
type VersionStatus struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"` // .so file present in extension_dir
	Enabled   bool   `json:"enabled"`   // 00-ioncube.ini exists in fpm conf.d
	Supported bool   `json:"supported"` // ionCube supports this PHP version
}

// GetStatus auto-detects installed PHP versions and returns ionCube status for each.
func GetStatus() ([]VersionStatus, error) {
	var statuses []VersionStatus

	for _, ver := range allKnownVersions() {
		status := VersionStatus{
			Version:   ver,
			Supported: isSupportedVersion(ver),
		}

		// Check if PHP is actually installed
		if !phpBinaryExists(ver) {
			continue
		}

		// Check enabled: fpm conf.d 00-ioncube.ini exists
		fpmIni := iniPath(ver, "fpm")
		if _, err := os.Stat(fpmIni); err == nil {
			status.Enabled = true
		}

		// Check installed: .so file present in extension_dir
		if extDir, err := getExtensionDir(ver); err == nil {
			soPath := filepath.Join(extDir, loaderFilename(ver))
			if _, err := os.Stat(soPath); err == nil {
				status.Installed = true
			}
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Enable downloads (if needed), installs, and enables ionCube for phpVersion.
func Enable(phpVersion string) error {
	if !isSupportedVersion(phpVersion) {
		return fmt.Errorf("ionCube does not support PHP %s (supported: 7.1–8.3)", phpVersion)
	}
	if !phpBinaryExists(phpVersion) {
		return fmt.Errorf("PHP %s is not installed on this server", phpVersion)
	}

	// Step 1: Download + extract if not cached
	if err := ensureDownloaded(phpVersion); err != nil {
		return fmt.Errorf("failed to download ionCube: %w", err)
	}

	// Step 2: Find extension_dir
	extDir, err := getExtensionDir(phpVersion)
	if err != nil {
		return fmt.Errorf("could not determine PHP %s extension_dir: %w", phpVersion, err)
	}

	// Step 3: Copy .so to extension_dir
	soName := loaderFilename(phpVersion)
	srcSO := filepath.Join(cacheDir, "ioncube", soName)
	dstSO := filepath.Join(extDir, soName)

	if _, err := os.Stat(srcSO); os.IsNotExist(err) {
		return fmt.Errorf("ionCube loader for PHP %s not found in cache (%s) — may be unsupported", phpVersion, srcSO)
	}

	if err := copyFile(srcSO, dstSO); err != nil {
		return fmt.Errorf("failed to install ionCube loader: %w", err)
	}
	slog.Info("ionCube .so installed", "php", phpVersion, "path", dstSO)

	// Step 4: Write zend_extension ini for both fpm and cli
	iniContent := fmt.Sprintf("; Managed by wp-manage\nzend_extension=%s\n", soName)
	for _, sapi := range []string{"fpm", "cli"} {
		p := iniPath(phpVersion, sapi)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			slog.Warn("Could not create conf.d dir", "sapi", sapi, "err", err)
			continue
		}
		if err := os.WriteFile(p, []byte(iniContent), 0644); err != nil {
			return fmt.Errorf("failed to write %s ini: %w", sapi, err)
		}
		slog.Info("ionCube ini written", "php", phpVersion, "sapi", sapi, "path", p)
	}

	// Step 5: Reload PHP-FPM
	if err := reloadFPM(phpVersion); err != nil {
		slog.Warn("PHP-FPM reload failed (non-fatal)", "php", phpVersion, "err", err)
	}

	return nil
}

// Disable removes ionCube ini files and reloads PHP-FPM for phpVersion.
func Disable(phpVersion string) error {
	for _, sapi := range []string{"fpm", "cli"} {
		p := iniPath(phpVersion, sapi)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to remove ionCube ini", "sapi", sapi, "path", p, "err", err)
		} else {
			slog.Info("ionCube ini removed", "php", phpVersion, "sapi", sapi)
		}
	}

	if err := reloadFPM(phpVersion); err != nil {
		slog.Warn("PHP-FPM reload failed (non-fatal)", "php", phpVersion, "err", err)
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

// allKnownVersions returns all PHP versions we ever check, in order.
func allKnownVersions() []string {
	return []string{"7.1", "7.2", "7.3", "7.4", "8.0", "8.1", "8.2", "8.3", "8.4"}
}

func isSupportedVersion(ver string) bool {
	for _, v := range SupportedVersions {
		if v == ver {
			return true
		}
	}
	return false
}

func phpBinaryExists(ver string) bool {
	bin := fmt.Sprintf("/usr/bin/php%s", ver)
	_, err := os.Stat(bin)
	return err == nil
}

func iniPath(phpVersion, sapi string) string {
	return fmt.Sprintf("/etc/php/%s/%s/conf.d/00-ioncube.ini", phpVersion, sapi)
}

func loaderFilename(phpVersion string) string {
	return fmt.Sprintf("ioncube_loader_lin_%s.so", phpVersion)
}

// getExtensionDir returns the PHP extension directory for the given version.
func getExtensionDir(phpVersion string) (string, error) {
	out, err := exec.Command(fmt.Sprintf("php%s", phpVersion), "-i").Output()
	if err != nil {
		return "", fmt.Errorf("php%s -i failed: %w", phpVersion, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "extension_dir") && strings.Contains(line, "=>") {
			// Format: extension_dir => /path => /path
			parts := strings.Split(line, "=>")
			if len(parts) >= 2 {
				dir := strings.TrimSpace(parts[1])
				if dir != "" && dir != "(null)" {
					return dir, nil
				}
			}
		}
	}
	return "", fmt.Errorf("extension_dir not found in php%s -i output", phpVersion)
}

func reloadFPM(phpVersion string) error {
	svc := fmt.Sprintf("php%s-fpm", phpVersion)
	out, err := exec.Command("systemctl", "reload", svc).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl reload %s: %s", svc, string(out))
	}
	return nil
}

// ensureDownloaded downloads the ionCube tarball if not already cached
// and verifies the specific .so for phpVersion is present.
func ensureDownloaded(phpVersion string) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	soName := loaderFilename(phpVersion)
	cachedSO := filepath.Join(cacheDir, "ioncube", soName)

	// Already extracted for this version — skip download
	if _, err := os.Stat(cachedSO); err == nil {
		slog.Info("ionCube loader already cached", "php", phpVersion, "path", cachedSO)
		return nil
	}

	tarball := filepath.Join(cacheDir, tarballName)

	// Download if tarball missing
	if _, err := os.Stat(tarball); os.IsNotExist(err) {
		slog.Info("Downloading ionCube loaders", "url", downloadURL)
		if err := downloadFile(downloadURL, tarball); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		slog.Info("ionCube tarball downloaded", "path", tarball)
	}

	// Extract tarball
	slog.Info("Extracting ionCube tarball", "tarball", tarball, "dest", cacheDir)
	out, err := exec.Command("tar", "-xzf", tarball, "-C", cacheDir).CombinedOutput()
	if err != nil {
		// Remove bad tarball so next attempt re-downloads
		os.Remove(tarball)
		return fmt.Errorf("extraction failed: %s: %w", string(out), err)
	}

	// Verify the .so for the requested version is present
	if _, err := os.Stat(cachedSO); err != nil {
		return fmt.Errorf("ionCube loader for PHP %s not found after extraction — version may not be supported by ionCube", phpVersion)
	}

	return nil
}

func downloadFile(url, dst string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
