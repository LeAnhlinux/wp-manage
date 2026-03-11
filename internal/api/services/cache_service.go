package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type CacheService struct {
	db *gorm.DB
}

func NewCacheService(db *gorm.DB) *CacheService {
	return &CacheService{db: db}
}

func (s *CacheService) GetConfig(siteID uint64) (*models.CacheConfig, error) {
	var cfg models.CacheConfig
	err := s.db.Where("site_id = ?", siteID).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// Create default config so toggle/update can reference it
		cfg = models.CacheConfig{SiteID: siteID, CacheTime: "60m", Enabled: false}
		if err := s.db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	return &cfg, err
}

func (s *CacheService) UpdateConfig(siteID uint64, req models.UpdateCacheConfigRequest) (*models.CacheConfig, error) {
	var cfg models.CacheConfig
	err := s.db.Where("site_id = ?", siteID).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = models.CacheConfig{SiteID: siteID, CacheTime: "60m"}
		s.db.Create(&cfg)
	}

	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.CacheTime != nil {
		cfg.CacheTime = *req.CacheTime
	}
	if req.Excludes != nil {
		cfg.Excludes = *req.Excludes
	}

	err = s.db.Save(&cfg).Error
	return &cfg, err
}

func (s *CacheService) ToggleCache(siteID uint64, enable bool) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return err
	}

	var cfg models.CacheConfig
	s.db.Where("site_id = ?", siteID).First(&cfg)

	action := "disable"
	if enable {
		action = "enable"
	}

	taskData := models.AgentCacheTask{
		Action:    action,
		Domain:    site.Domain,
		CacheTime: cfg.CacheTime,
		Excludes:  cfg.Excludes,
	}
	data, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "manage_cache",
		Data:     string(data),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// ToggleCacheSync toggles cache and waits for completion
func (s *CacheService) ToggleCacheSync(siteID uint64, enable bool) (*models.TaskQueue, error) {
	if err := s.ToggleCache(siteID, enable); err != nil {
		return nil, err
	}
	// Find the task we just created
	var task models.TaskQueue
	s.db.Where("site_id = ? AND task_type = 'manage_cache'", siteID).
		Order("created_at DESC").First(&task)

	// Upsert cache config enabled state in DB
	var cfg models.CacheConfig
	if s.db.Where("site_id = ?", siteID).First(&cfg).Error == gorm.ErrRecordNotFound {
		cfg = models.CacheConfig{SiteID: siteID, CacheTime: "60m", Enabled: enable}
		s.db.Create(&cfg)
	} else {
		s.db.Model(&cfg).Update("enabled", enable)
	}

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}

func (s *CacheService) PurgeCache(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return err
	}

	taskData := models.AgentCacheTask{
		Action: "purge",
		Domain: site.Domain,
	}
	data, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "manage_cache",
		Data:     string(data),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// PurgeCacheSync purges cache and waits for completion
func (s *CacheService) PurgeCacheSync(siteID uint64) (*models.TaskQueue, error) {
	if err := s.PurgeCache(siteID); err != nil {
		return nil, err
	}
	var task models.TaskQueue
	s.db.Where("site_id = ? AND task_type = 'manage_cache'", siteID).
		Order("created_at DESC").First(&task)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}
