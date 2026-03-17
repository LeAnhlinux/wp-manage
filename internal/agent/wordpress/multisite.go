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
		Type:   task.Type, // Bug 3 fix: echo back so API can save multisite_type
		Status: "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

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
		if err := updateNginxForSubdomains(task.Domain); err != nil {
			// Nginx update failed — report as warning, not fatal (site still works as subfolder)
			slog.Error("Failed to update Nginx for subdomain multisite", "domain", task.Domain, "error", err)
			result.Error = fmt.Sprintf("Multisite enabled but Nginx wildcard update failed: %s", err)
		}
	}

	return result
}

// updateNginxForSubdomains updates Nginx vhost to support wildcard subdomains
func updateNginxForSubdomains(domain string) error {
	// Check both sites-available and sites-enabled
	vhostPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	if _, err := os.Stat(vhostPath); err != nil {
		// Fall back to sites-enabled (some setups use direct symlinks)
		vhostPath = fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain)
	}

	content, err := os.ReadFile(vhostPath)
	if err != nil {
		return fmt.Errorf("failed to read Nginx vhost %s: %w", vhostPath, err)
	}

	original := string(content)

	// Add wildcard to server_name if not already present
	wildcard := "*." + domain
	if !strings.Contains(original, wildcard) {
		updated := strings.Replace(original, "server_name "+domain, "server_name "+domain+" "+wildcard, 1)
		if err := os.WriteFile(vhostPath, []byte(updated), 0644); err != nil {
			return fmt.Errorf("failed to write Nginx vhost: %w", err)
		}
	}

	// Test and reload Nginx
	testCmd := exec.Command("nginx", "-t")
	if out, err := testCmd.CombinedOutput(); err != nil {
		// Restore original on failure
		_ = os.WriteFile(vhostPath, []byte(original), 0644)
		return fmt.Errorf("nginx -t failed: %s", string(out))
	}

	if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
		return fmt.Errorf("nginx reload failed: %w", err)
	}

	slog.Info("Nginx updated for subdomain multisite", "domain", domain)
	return nil
}

// CreateSubsite creates a new subsite using WP-CLI
func CreateSubsite(task models.AgentCreateSubsiteTask) models.AgentCreateSubsiteResult {
	result := models.AgentCreateSubsiteResult{
		SiteID: task.SiteID,
		Status: "completed",
		Title:  task.Title, // pre-fill with requested title; overwrite from WP-CLI if available
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

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

	// Bug 2 & 4 fix: query with blogname + exact path/domain match
	listCmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "list",
		"--path="+wpPath,
		"--format=json",
		"--fields=blog_id,domain,path,blogname",
	)
	listOut, err := listCmd.Output()
	if err == nil {
		var sites []struct {
			BlogID   int    `json:"blog_id"`
			Domain   string `json:"domain"`
			Path     string `json:"path"`
			BlogName string `json:"blogname"`
		}
		if json.Unmarshal(listOut, &sites) == nil {
			slug := strings.Trim(task.Slug, "/")
			// Bug 4 fix: exact match — path must be "/slug/" or domain must start with "slug."
			for _, s := range sites {
				pathSlug := strings.Trim(s.Path, "/")
				domainPrefix := strings.SplitN(s.Domain, ".", 2)[0]
				if pathSlug == slug || domainPrefix == slug {
					result.WPBlogID = s.BlogID
					result.Domain = s.Domain
					result.Path = s.Path
					result.Title = s.BlogName // Bug 2 fix: capture actual blogname
					break
				}
			}
			// Fallback: use last entry if exact match failed (e.g. subdomain mode with full domain)
			if result.WPBlogID == 0 && len(sites) > 1 {
				last := sites[len(sites)-1]
				result.WPBlogID = last.BlogID
				result.Domain = last.Domain
				result.Path = last.Path
				result.Title = last.BlogName
				slog.Warn("CreateSubsite: exact slug match failed, using last entry", "slug", task.Slug, "fallback_domain", last.Domain)
			}
		}
	}

	return result
}

// DeleteSubsite deletes a subsite using WP-CLI and returns a result for callback
// Bug 1 fix: returns AgentDeleteSubsiteResult instead of bare error
func DeleteSubsite(task models.AgentDeleteSubsiteTask) models.AgentDeleteSubsiteResult {
	result := models.AgentDeleteSubsiteResult{
		SiteID:   task.SiteID,
		WPBlogID: task.WPBlogID,
		Status:   "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

	cmd := exec.Command("sudo", "-u", wpUser, "wp", "site", "delete",
		fmt.Sprintf("%d", task.WPBlogID),
		"--yes",
		"--path="+wpPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("wp site delete failed: %s: %s", err, string(out))
		slog.Error("Failed to delete subsite", "domain", task.Domain, "blog_id", task.WPBlogID, "error", result.Error)
		return result
	}

	slog.Info("Subsite deleted", "domain", task.Domain, "blog_id", task.WPBlogID)
	return result
}

// ListSubsites lists all subsites using WP-CLI
func ListSubsites(task models.AgentListSubsitesTask) models.AgentListSubsitesResult {
	result := models.AgentListSubsitesResult{
		SiteID: task.SiteID,
		Status: "completed",
	}

	wpPath := fmt.Sprintf("/var/www/%s/public", task.Domain)
	wpUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(task.Domain, ".", "_"), "-", "_")

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
