package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type ImageOptService struct {
	db *gorm.DB
}

func NewImageOptService(db *gorm.DB) *ImageOptService {
	return &ImageOptService{db: db}
}

// GetConfig returns the image optimization config for a site, creating default if not exists
func (s *ImageOptService) GetConfig(siteID uint64) (*models.ImageOptConfig, error) {
	var config models.ImageOptConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create default config
			config = models.ImageOptConfig{
				SiteID:      siteID,
				Enabled:     false,
				Quality:     80,
				WebPEnabled: true,
				MaxWidth:    2560,
				MaxHeight:   2560,
			}
			if err := s.db.Create(&config).Error; err != nil {
				return nil, err
			}
			return &config, nil
		}
		return nil, err
	}
	return &config, nil
}

// UpdateConfig updates image optimization settings
func (s *ImageOptService) UpdateConfig(siteID uint64, req models.UpdateImageOptConfigRequest) (*models.ImageOptConfig, error) {
	config, err := s.GetConfig(siteID)
	if err != nil {
		return nil, err
	}

	enabledChanged := req.Enabled != nil && *req.Enabled != config.Enabled
	qualityChanged := req.Quality != nil && *req.Quality != config.Quality
	webpChanged := req.WebPEnabled != nil && *req.WebPEnabled != config.WebPEnabled

	updates := map[string]interface{}{}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Quality != nil {
		updates["quality"] = *req.Quality
	}
	if req.WebPEnabled != nil {
		updates["webp_enabled"] = *req.WebPEnabled
	}
	if req.MaxWidth != nil {
		updates["max_width"] = *req.MaxWidth
	}
	if req.MaxHeight != nil {
		updates["max_height"] = *req.MaxHeight
	}

	if len(updates) > 0 {
		if err := s.db.Model(config).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	// Re-fetch updated config
	config, _ = s.GetConfig(siteID)

	// Queue toggle_image_opt task when enabled/quality/webp changed (mu-plugin needs update)
	if enabledChanged || (config.Enabled && (qualityChanged || webpChanged)) {
		s.queueToggleTask(siteID, config)
	}

	return config, nil
}

// Toggle enables/disables image auto-compression (synchronous)
func (s *ImageOptService) Toggle(siteID uint64, enabled bool) (*models.ImageOptConfig, error) {
	config, err := s.GetConfig(siteID)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(config).Update("enabled", enabled).Error; err != nil {
		return nil, err
	}
	config.Enabled = enabled

	// Queue toggle task and wait
	taskID, err := s.queueToggleTask(siteID, config)
	if err != nil {
		return config, err
	}
	result, err := WaitForTaskComplete(s.db, taskID)
	if err != nil {
		return config, nil // timeout — return config anyway
	}
	if result.Status == "failed" {
		return config, fmt.Errorf("%s", result.Output)
	}
	return config, nil
}

func (s *ImageOptService) queueToggleTask(siteID uint64, config *models.ImageOptConfig) (uint64, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return 0, fmt.Errorf("site not found")
	}
	taskData := models.AgentToggleImageOptTask{
		Domain:      site.Domain,
		Enabled:     config.Enabled,
		Quality:     config.Quality,
		WebPEnabled: config.WebPEnabled,
	}
	data, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "toggle_image_opt",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

// RunOptimization creates an optimization task for the agent
func (s *ImageOptService) RunOptimization(siteID uint64) (*models.ImageOptResult, error) {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site is not active")
	}

	// Get config
	config, err := s.GetConfig(siteID)
	if err != nil {
		return nil, err
	}

	// Create result record
	result := models.ImageOptResult{
		SiteID:    siteID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := s.db.Create(&result).Error; err != nil {
		return nil, err
	}

	// Queue agent task
	taskData := models.AgentImageOptTask{
		SiteID:      siteID,
		ResultID:    result.ID,
		Domain:      site.Domain,
		Quality:     config.Quality,
		WebPEnabled: config.WebPEnabled,
		MaxWidth:    config.MaxWidth,
		MaxHeight:   config.MaxHeight,
	}
	data, _ := json.Marshal(taskData)

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "optimize_images",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}

	return &result, nil
}

// RunOptimizationSync creates optimization task and waits for result
func (s *ImageOptService) RunOptimizationSync(siteID uint64) (*models.ImageOptResult, error) {
	result, err := s.RunOptimization(siteID)
	if err != nil {
		return nil, err
	}

	// Wait for agent to complete
	completed, err := WaitForImageOptResult(s.db, result.ID)
	if err != nil {
		return result, nil // Return partial result on timeout
	}
	return completed, nil
}

// OptimizeComplete handles agent callback with optimization results
func (s *ImageOptService) OptimizeComplete(result models.AgentImageOptResult) error {
	now := time.Now()
	updates := map[string]interface{}{
		"total_images":    result.TotalImages,
		"optimized_count": result.OptimizedCount,
		"skipped_count":   result.SkippedCount,
		"error_count":     result.ErrorCount,
		"bytes_saved":     result.BytesSaved,
		"status":          result.Status,
		"completed_at":    &now,
	}
	if result.Error != "" {
		updates["error"] = result.Error
	}

	return s.db.Model(&models.ImageOptResult{}).Where("id = ?", result.ResultID).Updates(updates).Error
}

// GetResults returns optimization history for a site
func (s *ImageOptService) GetResults(siteID uint64, page, perPage int) ([]models.ImageOptResult, int64, error) {
	var results []models.ImageOptResult
	var total int64

	query := s.db.Where("site_id = ?", siteID)
	query.Model(&models.ImageOptResult{}).Count(&total)

	offset := (page - 1) * perPage
	if err := query.Order("started_at DESC").Offset(offset).Limit(perPage).Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}
