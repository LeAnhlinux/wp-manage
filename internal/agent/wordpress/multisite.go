package wordpress

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// EnableMultisite enables WordPress Multisite on a site using WP-CLI
func EnableMultisite(task models.AgentEnableMultisiteTask) models.AgentEnableMultisiteResult {
	result := models.AgentEnableMultisiteResult{
		SiteID: task.SiteID,
		Status: "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	// Check if multisite is already installed
	checkCmd := exec.Command("sudo", "-u", wpUser, "wp", "core", "is-installed", "--network", "--path="+wpPath)
	if checkCmd.Run() == nil {
		slog.Info("Multisite already enabled", "domain", task.Domain)
		return result
	}

	// Enable multisite via WP-CLI
	args := []string{"-u", wpUser, "wp", "core", "multisite-install",
		"--path=" + wpPath,
		"--title=" + task.Domain,
		"--url=https://" + task.Domain,
	}
	if task.Type == "subdomain" {
		args = append(args, "--subdomains")
	}

	cmd := exec.Command("sudo", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("wp multisite-install failed: %s: %s", err, string(out))
		slog.Error("Failed to enable multisite", "domain", task.Domain, "error", result.Error)
		return result
	}
	slog.Info("Multisite enabled via WP-CLI", "domain", task.Domain, "output", string(out))

	// For subdomain mode, update Nginx config with wildcard server_name
	if task.Type == "subdomain" {
		updateNginxForSubdomains(task.Domain)
	}

	return result
}

// updateNginxForSubdomains updates Nginx vhost to support wildcard subdomains
func updateNginxForSubdomains(domain string) {
	vhostPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)

	content, err := os.ReadFile(vhostPath)
	if err != nil {
		slog.Error("Failed to read Nginx vhost", "path", vhostPath, "error", err)
		return
	}

	original := string(content)

	// Add wildcard to server_name if not already present
	wildcard := "*." + domain
	if !strings.Contains(original, wildcard) {
		updated := strings.Replace(original, "server_name "+domain, "server_name "+domain+" "+wildcard, 1)
		if err := os.WriteFile(vhostPath, []byte(updated), 0644); err != nil {
			slog.Error("Failed to write Nginx vhost", "error", err)
			return
		}
	}

	// Test and reload Nginx
	testCmd := exec.Command("nginx", "-t")
	if out, err := testCmd.CombinedOutput(); err != nil {
		slog.Error("Nginx config test failed", "error", err, "output", string(out))
		// Restore original
		os.WriteFile(vhostPath, []byte(original), 0644)
		return
	}

	reloadCmd := exec.Command("systemctl", "reload", "nginx")
	if err := reloadCmd.Run(); err != nil {
		slog.Error("Failed to reload Nginx", "error", err)
	}

	slog.Info("Nginx updated for subdomain multisite", "domain", domain)
}

// CreateSubsite creates a new subsite using WP-CLI
func CreateSubsite(task models.AgentCreateSubsiteTask) models.AgentCreateSubsiteResult {
	result := models.AgentCreateSubsiteResult{
		SiteID: task.SiteID,
		Status: "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	// Create subsite via WP-CLI
	cmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "create",
		"--slug="+task.Slug,
		"--title="+task.Title,
		"--path="+wpPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("wp site create failed: %s: %s", err, string(out))
		slog.Error("Failed to create subsite", "domain", task.Domain, "slug", task.Slug, "error", result.Error)
		return result
	}

	slog.Info("Subsite created", "domain", task.Domain, "slug", task.Slug, "output", string(out))

	// Get the newly created site details
	listCmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "list",
		"--path="+wpPath,
		"--format=json",
		"--fields=blog_id,domain,path",
	)
	listOut, err := listCmd.Output()
	if err == nil {
		var sites []struct {
			BlogID int    `json:"blog_id"`
			Domain string `json:"domain"`
			Path   string `json:"path"`
		}
		if json.Unmarshal(listOut, &sites) == nil {
			// Find the newly created site (last one or matching slug)
			for _, s := range sites {
				if strings.Contains(s.Path, task.Slug) || strings.Contains(s.Domain, task.Slug) {
					result.WPBlogID = s.BlogID
					result.Domain = s.Domain
					result.Path = s.Path
					break
				}
			}
			// If not found by slug, use the last entry (newest)
			if result.WPBlogID == 0 && len(sites) > 0 {
				last := sites[len(sites)-1]
				result.WPBlogID = last.BlogID
				result.Domain = last.Domain
				result.Path = last.Path
			}
		}
	}

	return result
}

// DeleteSubsite deletes a subsite using WP-CLI
func DeleteSubsite(task models.AgentDeleteSubsiteTask) error {
	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	cmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "delete",
		fmt.Sprintf("%d", task.WPBlogID),
		"--yes",
		"--path="+wpPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wp site delete failed: %s: %s", err, string(out))
	}

	slog.Info("Subsite deleted", "domain", task.Domain, "blog_id", task.WPBlogID)
	return nil
}

// ListSubsites lists all subsites using WP-CLI
func ListSubsites(task models.AgentListSubsitesTask) models.AgentListSubsitesResult {
	result := models.AgentListSubsitesResult{
		SiteID: task.SiteID,
		Status: "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	cmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "list",
		"--path="+wpPath,
		"--format=json",
		"--fields=blog_id,domain,path,blogname",
	)
	out, err := cmd.Output()
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("wp site list failed: %s", err)
		return result
	}

	var sites []struct {
		BlogID   int    `json:"blog_id"`
		Domain   string `json:"domain"`
		Path     string `json:"path"`
		BlogName string `json:"blogname"`
	}
	if err := json.Unmarshal(out, &sites); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to parse site list: %s", err)
		return result
	}

	for _, s := range sites {
		result.Subsites = append(result.Subsites, struct {
			BlogID int    `json:"blog_id"`
			Domain string `json:"domain"`
			Path   string `json:"path"`
			Title  string `json:"title"`
		}{
			BlogID: s.BlogID,
			Domain: s.Domain,
			Path:   s.Path,
			Title:  s.BlogName,
		})
	}

	return result
}
