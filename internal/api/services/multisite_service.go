package services

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type MultisiteService struct {
	db *gorm.DB
}

func NewMultisiteService(db *gorm.DB) *MultisiteService {
	return &MultisiteService{db: db}
}

// EnableMultisite creates a task to enable WordPress Multisite on a site
func (s *MultisiteService) EnableMultisite(siteID uint64, msType string) error {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return fmt.Errorf("site must be active to enable multisite")
	}
	if site.IsMultisite {
		return fmt.Errorf("multisite is already enabled")
	}

	taskData := models.AgentEnableMultisiteTask{
		SiteID:     siteID,
		Domain:     site.Domain,
		Type:       msType,
		PHPVersion: site.PHPVersion,
	}
	data, _ := json.Marshal(taskData)

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "enable_multisite",
		Data:     string(data),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// EnableMultisiteComplete handles agent callback after multisite is enabled
func (s *MultisiteService) EnableMultisiteComplete(result models.AgentEnableMultisiteResult) error {
	if result.Status == "completed" {
		updates := map[string]interface{}{
			"is_multisite": true,
		}
		// Bug 3 fix: save multisite_type from echoed result
		if result.Type != "" {
			updates["multisite_type"] = result.Type
		}
		return s.db.Model(&models.Site{}).Where("id = ?", result.SiteID).Updates(updates).Error
	}
	// Fix #9: on failure, log the error but don't change site record
	if result.Error != "" {
		slog.Error("Enable multisite failed on agent", "site_id", result.SiteID, "error", result.Error)
	}
	return nil
}

// GetSubsites returns all subsites for a multisite network
func (s *MultisiteService) GetSubsites(siteID uint64) ([]models.Subsite, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if !site.IsMultisite {
		return nil, fmt.Errorf("site is not a multisite network")
	}

	var subsites []models.Subsite
	if err := s.db.Where("site_id = ? AND status != ?", siteID, "deleted").Order("wp_blog_id ASC").Find(&subsites).Error; err != nil {
		return nil, err
	}
	return subsites, nil
}

// CreateSubsite creates a task to create a new subsite
func (s *MultisiteService) CreateSubsite(siteID uint64, slug, title string) error {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if !site.IsMultisite {
		return fmt.Errorf("site is not a multisite network")
	}

	taskData := models.AgentCreateSubsiteTask{
		SiteID:        siteID,
		Domain:        site.Domain,
		Slug:          slug,
		Title:         title,
		MultisiteType: site.MultisiteType, // Fix #10: pass multisite type for correct --url construction
	}
	data, _ := json.Marshal(taskData)

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "create_subsite",
		Data:     string(data),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// CreateSubsiteComplete handles agent callback after subsite is created
func (s *MultisiteService) CreateSubsiteComplete(result models.AgentCreateSubsiteResult) error {
	if result.Status == "completed" {
		subsite := models.Subsite{
			SiteID:   result.SiteID,
			WPBlogID: result.WPBlogID,
			Domain:   result.Domain,
			Path:     result.Path,
			Title:    result.Title, // Bug 2 fix: use title from agent
			Status:   "active",
		}
		return s.db.Create(&subsite).Error
	}
	return nil
}

// DeleteSubsite creates a task to delete a subsite
// Bug 1 fix: do NOT mark as deleted here — wait for agent callback to confirm
func (s *MultisiteService) DeleteSubsite(siteID, subsiteID uint64) error {
	var subsite models.Subsite
	if err := s.db.First(&subsite, subsiteID).Error; err != nil {
		return fmt.Errorf("subsite not found")
	}
	if subsite.SiteID != siteID {
		return fmt.Errorf("subsite does not belong to this site")
	}

	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	taskData := models.AgentDeleteSubsiteTask{
		SiteID:   siteID,
		Domain:   site.Domain,
		WPBlogID: subsite.WPBlogID,
	}
	data, _ := json.Marshal(taskData)

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "delete_subsite",
		Data:     string(data),
		Status:   "pending",
	}
	// Mark as "deleting" (pending) — actual deletion confirmed via callback
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}
	return s.db.Model(&subsite).Update("status", "deleting").Error
}

// DeleteSubsiteComplete handles agent callback after subsite is deleted
// Bug 1 fix: only mark deleted after agent confirms, rollback on failure
func (s *MultisiteService) DeleteSubsiteComplete(result models.AgentDeleteSubsiteResult) error {
	if result.Status == "completed" {
		return s.db.Model(&models.Subsite{}).
			Where("site_id = ? AND wp_blog_id = ?", result.SiteID, result.WPBlogID).
			Update("status", "deleted").Error
	}
	// Agent failed — rollback to active so user can retry
	return s.db.Model(&models.Subsite{}).
		Where("site_id = ? AND wp_blog_id = ? AND status = ?", result.SiteID, result.WPBlogID, "deleting").
		Update("status", "active").Error
}

// SyncSubsites creates a task to sync subsites from WP-CLI
func (s *MultisiteService) SyncSubsites(siteID uint64) error {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if !site.IsMultisite {
		return fmt.Errorf("site is not a multisite network")
	}

	taskData := models.AgentListSubsitesTask{
		SiteID: siteID,
		Domain: site.Domain,
	}
	data, _ := json.Marshal(taskData)

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "sync_subsites",
		Data:     string(data),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// SyncSubsitesComplete handles agent callback with subsite list
func (s *MultisiteService) SyncSubsitesComplete(result models.AgentListSubsitesResult) error {
	if result.Status != "completed" {
		return nil
	}

	// Upsert: update existing, insert new, mark missing as deleted
	// Bug (sync destructive) fix: use transaction + upsert instead of delete-all-recreate
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Build map of blog_id → subsite data from agent
		agentMap := make(map[int]struct {
			Domain string
			Path   string
			Title  string
		})
		for _, sub := range result.Subsites {
			if sub.BlogID == 1 {
				continue // skip main site
			}
			agentMap[sub.BlogID] = struct {
				Domain string
				Path   string
				Title  string
			}{sub.Domain, sub.Path, sub.Title}
		}

		// Fetch existing subsites from DB
		var existing []models.Subsite
		if err := tx.Where("site_id = ?", result.SiteID).Find(&existing).Error; err != nil {
			return err
		}
		existingMap := make(map[int]uint64) // blog_id → db id
		for _, s := range existing {
			existingMap[s.WPBlogID] = s.ID
		}

		// Update or insert from agent data
		for blogID, data := range agentMap {
			if dbID, exists := existingMap[blogID]; exists {
				// Update existing
				tx.Model(&models.Subsite{}).Where("id = ?", dbID).Updates(map[string]interface{}{
					"domain": data.Domain,
					"path":   data.Path,
					"title":  data.Title,
					"status": "active",
				})
			} else {
				// Insert new
				tx.Create(&models.Subsite{
					SiteID:   result.SiteID,
					WPBlogID: blogID,
					Domain:   data.Domain,
					Path:     data.Path,
					Title:    data.Title,
					Status:   "active",
				})
			}
		}

		// Mark as deleted if not in agent response
		for _, sub := range existing {
			if sub.WPBlogID == 1 {
				continue
			}
			if _, inAgent := agentMap[sub.WPBlogID]; !inAgent {
				tx.Model(&sub).Update("status", "deleted")
			}
		}

		return nil
	})
}
