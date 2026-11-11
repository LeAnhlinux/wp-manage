package wordpress

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"

	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// CheckUpdates uses WP-CLI to check for available updates
func CheckUpdates(task models.AgentCheckUpdatesTask) models.AgentCheckUpdatesResult {
	result := models.AgentCheckUpdatesResult{
		SiteID: task.SiteID,
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := utils.SiteLinuxUser(task.Domain)
	phpBin := phpBinaryForVersion(task.PHPVersion)

	// Always get current WP version (even if no updates available)
	result.CurrentVersion = getCurrentWPVersion(wpPath, wpUser, phpBin)

	// Check core updates
	coreResult := checkCoreUpdates(wpPath, wpUser, phpBin)
	if coreResult != nil {
		result.Core = coreResult
	}

	// Check plugin updates
	result.Plugins = checkPluginUpdates(wpPath, wpUser, phpBin)

	// Check theme updates
	result.Themes = checkThemeUpdates(wpPath, wpUser, phpBin)

	return result
}

// ApplyUpdates runs wp core/plugin/theme updates via WP-CLI
func ApplyUpdates(task models.AgentApplyUpdatesTask) models.AgentApplyUpdatesResult {
	result := models.AgentApplyUpdatesResult{
		SiteID: task.SiteID,
		Domain: task.Domain,
		Status: "success",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := utils.SiteLinuxUser(task.Domain)
	phpBin := phpBinaryForVersion(task.PHPVersion)

	// Update WordPress core
	if task.CoreUpdates {
		out, err := wpCmd(phpBin, wpUser, "core", "update", "--path="+wpPath).CombinedOutput()
		if err != nil {
			result.CoreError = strings.TrimSpace(string(out))
			result.Status = "partial"
			slog.Error("WP core update failed", "domain", task.Domain, "error", result.CoreError)
		} else {
			result.CoreUpdated = true
			slog.Info("WP core updated", "domain", task.Domain)
		}
	}

	// Update plugins
	if task.PluginUpdates {
		result.PluginsResult = applyPluginUpdates(wpPath, wpUser, phpBin)
	}

	// Update themes
	if task.ThemeUpdates {
		result.ThemesResult = applyThemeUpdates(wpPath, wpUser, phpBin)
	}

	// Get new WP version after updates
	result.NewWPVersion = getCurrentWPVersion(wpPath, wpUser, phpBin)

	return result
}

// wpCmd builds a sudo command that invokes WP-CLI via the correct PHP binary.
// This ensures ionCube-encoded theme/plugin files are loaded in the right PHP version.
// Equivalent to: sudo -u wpUser /usr/bin/php8.1 /usr/local/bin/wp <args...>
func wpCmd(phpBin, wpUser string, args ...string) *exec.Cmd {
	// args: WP-CLI sub-command args (e.g., "plugin", "list", "--path=...", ...)
	cmdArgs := append([]string{"-u", wpUser, phpBin, wpcliBinary()}, args...)
	return exec.Command("sudo", cmdArgs...)
}

// getCurrentWPVersion returns the current WP version via wp-cli
func getCurrentWPVersion(wpPath, wpUser, phpBin string) string {
	cmd := wpCmd(phpBin, wpUser, "core", "version", "--path="+wpPath)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func applyPluginUpdates(wpPath, wpUser, phpBin string) []models.WPUpdateInfo {
	// Get list of plugins with updates before applying
	listOut, _ := wpCmd(phpBin, wpUser, "plugin", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version").Output()

	var plugins []struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
	}
	json.Unmarshal(listOut, &plugins)

	// Apply all plugin updates
	var result []models.WPUpdateInfo
	for _, p := range plugins {
		out, err := wpCmd(phpBin, wpUser, "plugin", "update", p.Name, "--path="+wpPath).CombinedOutput()
		status := "success"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = strings.TrimSpace(string(out))
		}
		result = append(result, models.WPUpdateInfo{
			Name:       p.Name,
			Slug:       p.Name,
			OldVersion: p.Version,
			NewVersion: p.UpdateVersion,
			Status:     status,
			Error:      errMsg,
		})
	}
	return result
}

func applyThemeUpdates(wpPath, wpUser, phpBin string) []models.WPUpdateInfo {
	listOut, _ := wpCmd(phpBin, wpUser, "theme", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version").Output()

	var themes []struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
	}
	json.Unmarshal(listOut, &themes)

	var result []models.WPUpdateInfo
	for _, t := range themes {
		out, err := wpCmd(phpBin, wpUser, "theme", "update", t.Name, "--path="+wpPath).CombinedOutput()
		status := "success"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = strings.TrimSpace(string(out))
		}
		result = append(result, models.WPUpdateInfo{
			Name:       t.Name,
			Slug:       t.Name,
			OldVersion: t.Version,
			NewVersion: t.UpdateVersion,
			Status:     status,
			Error:      errMsg,
		})
	}
	return result
}

func checkCoreUpdates(wpPath, wpUser, phpBin string) *models.WPUpdateInfo {
	// Get current version
	currentVersion := getCurrentWPVersion(wpPath, wpUser, phpBin)
	if currentVersion == "" {
		slog.Error("Failed to get WP core version", "path", wpPath)
		return nil
	}

	// Check for update
	checkOut, err := wpCmd(phpBin, wpUser, "core", "check-update", "--format=json", "--path="+wpPath).Output()
	if err != nil {
		// Exit code 0 = no update, non-zero may mean update available or error
		return nil
	}

	var updates []struct {
		Version    string `json:"version"`
		UpdateType string `json:"update_type"`
	}
	if err := json.Unmarshal(checkOut, &updates); err != nil || len(updates) == 0 {
		return nil
	}

	return &models.WPUpdateInfo{
		Name:       "WordPress",
		Slug:       "wordpress",
		OldVersion: currentVersion,
		NewVersion: updates[0].Version,
		Status:     "available",
	}
}

func checkPluginUpdates(wpPath, wpUser, phpBin string) []models.WPUpdateInfo {
	out, err := wpCmd(phpBin, wpUser, "plugin", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version").Output()
	if err != nil {
		slog.Error("Failed to check plugin updates", "error", err)
		return nil
	}

	var plugins []struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
	}
	if err := json.Unmarshal(out, &plugins); err != nil {
		return nil
	}

	var result []models.WPUpdateInfo
	for _, p := range plugins {
		result = append(result, models.WPUpdateInfo{
			Name:       p.Name,
			Slug:       p.Name,
			OldVersion: p.Version,
			NewVersion: p.UpdateVersion,
			Status:     "available",
		})
	}
	return result
}

func checkThemeUpdates(wpPath, wpUser, phpBin string) []models.WPUpdateInfo {
	out, err := wpCmd(phpBin, wpUser, "theme", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version").Output()
	if err != nil {
		slog.Error("Failed to check theme updates", "error", err)
		return nil
	}

	var themes []struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
	}
	if err := json.Unmarshal(out, &themes); err != nil {
		return nil
	}

	var result []models.WPUpdateInfo
	for _, t := range themes {
		result = append(result, models.WPUpdateInfo{
			Name:       t.Name,
			Slug:       t.Name,
			OldVersion: t.Version,
			NewVersion: t.UpdateVersion,
			Status:     "available",
		})
	}
	return result
}
