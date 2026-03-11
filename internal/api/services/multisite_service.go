package services

import (
	"encoding/json"
	"fmt"

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
		return s.db.Model(&models.Site{}).Where("id = ?", result.SiteID).Updates(map[string]interface{}{
			"is_multisite": true,
		}).Error
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
	if err := s.db.Where("site_id = ?", siteID).Order("wp_blog_id ASC").Find(&subsites).Error; err != nil {
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
		SiteID: siteID,
		Domain: site.Domain,
		Slug:   slug,
		Title:  title,
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
			Title:    "",
			Status:   "active",
		}
		return s.db.Create(&subsite).Error
	}
	return nil
}

// DeleteSubsite creates a task to delete a subsite
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
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	// Mark as deleted
	return s.db.Model(&subsite).Update("status", "deleted").Error
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

	// Delete existing subsites for this site and recreate
	s.db.Where("site_id = ?", result.SiteID).Delete(&models.Subsite{})

	for _, sub := range result.Subsites {
		if sub.BlogID == 1 {
			continue // Skip main site (blog_id=1)
		}
		subsite := models.Subsite{
			SiteID:   result.SiteID,
			WPBlogID: sub.BlogID,
			Domain:   sub.Domain,
			Path:     sub.Path,
			Title:    sub.Title,
			Status:   "active",
		}
		s.db.Create(&subsite)
	}
	return nil
}
