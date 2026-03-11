package wordpress

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// CheckUpdates uses WP-CLI to check for available updates
func CheckUpdates(task models.AgentCheckUpdatesTask) models.AgentCheckUpdatesResult {
	result := models.AgentCheckUpdatesResult{
		SiteID: task.SiteID,
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	// Check core updates
	coreResult := checkCoreUpdates(wpPath, wpUser)
	if coreResult != nil {
		result.Core = coreResult
	}

	// Check plugin updates
	result.Plugins = checkPluginUpdates(wpPath, wpUser)

	// Check theme updates
	result.Themes = checkThemeUpdates(wpPath, wpUser)

	return result
}

func checkCoreUpdates(wpPath, wpUser string) *models.WPUpdateInfo {
	// Get current version
	currentCmd := exec.Command("sudo", "-u", wpUser, "wp", "core", "version", "--path="+wpPath)
	currentOut, err := currentCmd.Output()
	if err != nil {
		slog.Error("Failed to get WP core version", "error", err)
		return nil
	}
	currentVersion := strings.TrimSpace(string(currentOut))

	// Check for update
	checkCmd := exec.Command("sudo", "-u", wpUser, "wp", "core", "check-update", "--format=json", "--path="+wpPath)
	checkOut, err := checkCmd.Output()
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

func checkPluginUpdates(wpPath, wpUser string) []models.WPUpdateInfo {
	cmd := exec.Command("sudo", "-u", wpUser, "wp", "plugin", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version")
	out, err := cmd.Output()
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

func checkThemeUpdates(wpPath, wpUser string) []models.WPUpdateInfo {
	cmd := exec.Command("sudo", "-u", wpUser, "wp", "theme", "list",
		"--update=available", "--format=json", "--path="+wpPath,
		"--fields=name,version,update_version")
	out, err := cmd.Output()
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
