package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

// stagingDBNameFromDomain computes the staging DB name from a staging domain.
// Uses the same formula as the agent so both sides are consistent.
// Format: stg_{sanitized_domain}, max 48 chars.
func stagingDBNameFromDomain(stagingDomain string) string {
	result := make([]byte, 0, len(stagingDomain))
	for _, c := range stagingDomain {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, byte(c))
		} else {
			result = append(result, '_')
		}
	}
	name := "stg_" + string(result)
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

type WordPressService struct {
	db *gorm.DB
}

func NewWordPressService(db *gorm.DB) *WordPressService {
	return &WordPressService{db: db}
}

// --- Staging ---

// CreateStaging creates a staging environment for a production site
func (s *WordPressService) CreateStaging(productionSiteID uint64) (*models.StagingSite, error) {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, productionSiteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site must be active to create staging")
	}

	// Check if staging already exists
	var count int64
	s.db.Model(&models.StagingSite{}).Where("production_id = ? AND status IN ?", productionSiteID, []string{"creating", "active"}).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("staging already exists for this site")
	}

	// Create the staging Site record (cloned from production)
	stagingDomain := fmt.Sprintf("staging-%s", site.Domain)
	stagingDB := stagingDBNameFromDomain(stagingDomain)
	stagingUser := stagingDB
	if len(stagingUser) > 32 {
		stagingUser = stagingUser[:32]
	}
	stagingPass := utils.GenerateRandomPassword(24)
	stagingSite := models.Site{
		ServerID:     site.ServerID,
		Domain:       stagingDomain,
		PHPVersion:   site.PHPVersion,
		SSLEnabled:   false,
		WPAdminUser:  site.WPAdminUser,
		WPAdminEmail: site.WPAdminEmail,
		Status:       "creating",
		DBName:       stagingDB,
		DBUser:       stagingUser,
		DBPassword:   stagingPass,
	}
	if err := s.db.Create(&stagingSite).Error; err != nil {
		return nil, fmt.Errorf("failed to create staging site record: %w", err)
	}

	// Create StagingSite junction record
	staging := models.StagingSite{
		ProductionID:  productionSiteID,
		StagingSiteID: stagingSite.ID,
		Status:        "creating",
	}
	if err := s.db.Create(&staging).Error; err != nil {
		return nil, err
	}

	// Queue staging_create task for agent
	taskData := map[string]interface{}{
		"staging_id":          staging.ID,
		"staging_site_id":     stagingSite.ID,
		"production_domain":   site.Domain,
		"staging_domain":      stagingDomain,
		"php_version":         site.PHPVersion,
		"db_name":             site.DBName,
		"db_user":             site.DBUser,
		"db_password":         site.DBPassword,
		"staging_db_name":     stagingDB,
		"staging_db_user":     stagingUser,
		"staging_db_password": stagingPass,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   productionSiteID,
		TaskType: "staging_create",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	staging.Production = &site
	staging.StagingSite2 = &stagingSite
	return &staging, nil
}

// GetStaging returns staging environments for a production site
func (s *WordPressService) GetStaging(productionSiteID uint64) ([]models.StagingSite, error) {
	var stagings []models.StagingSite
	err := s.db.Where("production_id = ?", productionSiteID).
		Preload("StagingSite2").
		Order("created_at DESC").
		Find(&stagings).Error
	return stagings, err
}

// PushStaging pushes staging to production
func (s *WordPressService) PushStaging(stagingID uint64) error {
	var staging models.StagingSite
	if err := s.db.Preload("Production").Preload("StagingSite2").First(&staging, stagingID).Error; err != nil {
		return fmt.Errorf("staging not found")
	}
	if staging.Status != "active" {
		return fmt.Errorf("staging must be active to push")
	}

	s.db.Model(&staging).Update("status", "pushing")

	taskData := map[string]interface{}{
		"staging_id":          staging.ID,
		"production_domain":   staging.Production.Domain,
		"staging_domain":      staging.StagingSite2.Domain,
		"db_name":             staging.Production.DBName,
		"db_user":             staging.Production.DBUser,
		"db_password":         staging.Production.DBPassword,
		"staging_db_name":     staging.StagingSite2.DBName,
		"staging_db_user":     staging.StagingSite2.DBUser,
		"staging_db_password": staging.StagingSite2.DBPassword,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: staging.Production.ServerID,
		SiteID:   staging.ProductionID,
		TaskType: "staging_push",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)
	return nil
}

// DeleteStaging deletes a staging environment
func (s *WordPressService) DeleteStaging(stagingID uint64) error {
	var staging models.StagingSite
	if err := s.db.Preload("Production").Preload("StagingSite2").First(&staging, stagingID).Error; err != nil {
		return fmt.Errorf("staging not found")
	}

	taskData := map[string]interface{}{
		"staging_id":          staging.ID,
		"staging_domain":      staging.StagingSite2.Domain,
		"staging_db_name":     staging.StagingSite2.DBName,
		"staging_db_user":     staging.StagingSite2.DBUser,
		"staging_db_password": staging.StagingSite2.DBPassword,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: staging.Production.ServerID,
		SiteID:   staging.ProductionID,
		TaskType: "staging_delete",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	s.db.Model(&staging).Update("status", "deleted")
	s.db.Model(&staging.StagingSite2).Update("status", "deleting")
	return nil
}

// UpdateStagingStatus updates staging status (called after agent completes task)
func (s *WordPressService) UpdateStagingStatus(stagingID uint64, status string, siteStatus string) error {
	s.db.Model(&models.StagingSite{}).Where("id = ?", stagingID).Update("status", status)
	var staging models.StagingSite
	s.db.First(&staging, stagingID)
	if siteStatus != "" {
		s.db.Model(&models.Site{}).Where("id = ?", staging.StagingSiteID).Update("status", siteStatus)
	}
	return nil
}

// --- Auto-Update Config ---

// GetAutoUpdateConfig returns auto-update config for a site (creates default if not exists)
func (s *WordPressService) GetAutoUpdateConfig(siteID uint64) (*models.AutoUpdateConfig, error) {
	var config models.AutoUpdateConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		config = models.AutoUpdateConfig{
			SiteID:        siteID,
			CoreUpdates:   false,
			PluginUpdates: false,
			ThemeUpdates:  false,
			BackupBefore:  true,
		}
		s.db.Create(&config)
		return &config, nil
	}
	return &config, err
}

// UpdateAutoUpdateConfig updates auto-update settings
func (s *WordPressService) UpdateAutoUpdateConfig(siteID uint64, req models.UpdateAutoUpdateConfigRequest) (*models.AutoUpdateConfig, error) {
	config, err := s.GetAutoUpdateConfig(siteID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.CoreUpdates != nil {
		updates["core_updates"] = *req.CoreUpdates
		config.CoreUpdates = *req.CoreUpdates
	}
	if req.PluginUpdates != nil {
		updates["plugin_updates"] = *req.PluginUpdates
		config.PluginUpdates = *req.PluginUpdates
	}
	if req.ThemeUpdates != nil {
		updates["theme_updates"] = *req.ThemeUpdates
		config.ThemeUpdates = *req.ThemeUpdates
	}
	if req.BackupBefore != nil {
		updates["backup_before"] = *req.BackupBefore
		config.BackupBefore = *req.BackupBefore
	}

	if len(updates) > 0 {
		s.db.Model(&models.AutoUpdateConfig{}).Where("id = ?", config.ID).Updates(updates)
	}
	return config, nil
}

// --- Check Updates ---

// CheckUpdates queues a task for agent to check available WP updates; returns taskID
func (s *WordPressService) CheckUpdates(siteID uint64) (uint64, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return 0, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return 0, fmt.Errorf("site must be active")
	}

	taskData := models.AgentCheckUpdatesTask{
		SiteID:     siteID,
		Domain:     site.Domain,
		PHPVersion: site.PHPVersion,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "check_updates",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

// CheckUpdatesSync queues update check and waits for result synchronously
func (s *WordPressService) CheckUpdatesSync(siteID uint64) (interface{}, error) {
	startTime := time.Now()
	taskID, err := s.CheckUpdates(siteID)
	if err != nil {
		return nil, err
	}

	// Wait for agent to mark the task as completed (works even when no updates available)
	if _, err := WaitForTaskComplete(s.db, taskID); err != nil {
		return nil, err
	}

	// Read UpdateLog records created during this check
	var logs []models.UpdateLog
	s.db.Where("site_id = ? AND created_at >= ?", siteID, startTime).
		Order("created_at DESC").Find(&logs)

	if len(logs) == 0 {
		return map[string]interface{}{
			"message": "All up to date",
			"core":    nil,
			"plugins": []interface{}{},
			"themes":  []interface{}{},
		}, nil
	}

	// Group by update_type
	var core interface{}
	var plugins []models.UpdateLog
	var themes []models.UpdateLog
	for _, l := range logs {
		switch l.UpdateType {
		case "core":
			core = l
		case "plugin":
			plugins = append(plugins, l)
		case "theme":
			themes = append(themes, l)
		}
	}
	return map[string]interface{}{
		"core":    core,
		"plugins": plugins,
		"themes":  themes,
	}, nil
}

// SaveUpdateResult saves update check result from agent
func (s *WordPressService) SaveUpdateResult(result models.AgentCheckUpdatesResult) error {
	now := time.Now()

	// Always update the site's current WP version
	if result.CurrentVersion != "" {
		s.db.Model(&models.Site{}).Where("id = ?", result.SiteID).Update("wp_version", result.CurrentVersion)
	}

	// Save core update if available
	if result.Core != nil && result.Core.NewVersion != "" {
		log := models.UpdateLog{
			SiteID:      result.SiteID,
			UpdateType:  "core",
			ItemName:    "WordPress",
			FromVersion: result.Core.OldVersion,
			ToVersion:   result.Core.NewVersion,
			Status:      result.Core.Status,
			CreatedAt:   now,
		}
		s.db.Create(&log)
	}

	// Save plugin updates
	for _, p := range result.Plugins {
		log := models.UpdateLog{
			SiteID:      result.SiteID,
			UpdateType:  "plugin",
			ItemName:    p.Name,
			FromVersion: p.OldVersion,
			ToVersion:   p.NewVersion,
			Status:      p.Status,
			CreatedAt:   now,
		}
		s.db.Create(&log)
	}

	// Save theme updates
	for _, t := range result.Themes {
		log := models.UpdateLog{
			SiteID:      result.SiteID,
			UpdateType:  "theme",
			ItemName:    t.Name,
			FromVersion: t.OldVersion,
			ToVersion:   t.NewVersion,
			Status:      t.Status,
			CreatedAt:   now,
		}
		s.db.Create(&log)
	}

	return nil
}

// SaveApplyUpdatesResult saves apply-updates result from agent and updates sites.wp_version
func (s *WordPressService) SaveApplyUpdatesResult(result models.AgentApplyUpdatesResult) error {
	now := time.Now()

	// Update site WP version if core was updated
	if result.NewWPVersion != "" {
		s.db.Model(&models.Site{}).Where("domain = ?", result.Domain).Update("wp_version", result.NewWPVersion)
	}

	// Log core update result
	if result.CoreUpdated || result.CoreError != "" {
		status := "success"
		errMsg := ""
		if result.CoreError != "" {
			status = "failed"
			errMsg = result.CoreError
		}
		s.db.Create(&models.UpdateLog{
			SiteID:    result.SiteID,
			UpdateType: "core",
			ItemName:  "WordPress",
			ToVersion: result.NewWPVersion,
			Status:    status,
			Error:     errMsg,
			CreatedAt: now,
		})
	}

	// Log plugin update results
	for _, p := range result.PluginsResult {
		s.db.Create(&models.UpdateLog{
			SiteID:      result.SiteID,
			UpdateType:  "plugin",
			ItemName:    p.Name,
			FromVersion: p.OldVersion,
			ToVersion:   p.NewVersion,
			Status:      p.Status,
			Error:       p.Error,
			CreatedAt:   now,
		})
	}

	// Log theme update results
	for _, t := range result.ThemesResult {
		s.db.Create(&models.UpdateLog{
			SiteID:      result.SiteID,
			UpdateType:  "theme",
			ItemName:    t.Name,
			FromVersion: t.OldVersion,
			ToVersion:   t.NewVersion,
			Status:      t.Status,
			Error:       t.Error,
			CreatedAt:   now,
		})
	}

	return nil
}

// GetUpdateLogs returns update logs for a site
func (s *WordPressService) GetUpdateLogs(siteID uint64, page, perPage int) ([]models.UpdateLog, int64, error) {
	var logs []models.UpdateLog
	var total int64

	query := s.db.Model(&models.UpdateLog{}).Where("site_id = ?", siteID)
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Order("created_at DESC").
		Find(&logs).Error
	return logs, total, err
}

// --- DB Optimize ---

// OptimizeDatabase queues a DB optimization task for agent; returns taskID
func (s *WordPressService) OptimizeDatabase(siteID uint64) (uint64, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return 0, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return 0, fmt.Errorf("site must be active")
	}

	taskData := models.AgentDBOptimizeTask{
		SiteID:     siteID,
		Domain:     site.Domain,
		PHPVersion: site.PHPVersion,
		DBName:     site.DBName,
		DBUser:     site.DBUser,
		DBPass:     site.DBPassword,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "db_optimize",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

// OptimizeDatabaseSync queues DB optimization and waits for result
func (s *WordPressService) OptimizeDatabaseSync(siteID uint64) (interface{}, error) {
	startTime := time.Now()
	taskID, err := s.OptimizeDatabase(siteID)
	if err != nil {
		return nil, err
	}

	// Wait for agent to mark task complete
	if _, err := WaitForTaskComplete(s.db, taskID); err != nil {
		return nil, err
	}

	// Read db_optimize result from UpdateLog
	var logs []models.UpdateLog
	s.db.Where("site_id = ? AND update_type = ? AND created_at >= ?", siteID, "db_optimize", startTime).
		Order("created_at DESC").Limit(1).Find(&logs)
	if len(logs) > 0 {
		return logs[0], nil
	}
	return map[string]string{"message": "Database optimization completed"}, nil
}

// SaveDBOptimizeResult saves DB optimization result from agent
func (s *WordPressService) SaveDBOptimizeResult(result models.AgentDBOptimizeResult) error {
	// Log optimization as an activity
	log := models.UpdateLog{
		SiteID:     result.SiteID,
		UpdateType: "db_optimize",
		ItemName:   fmt.Sprintf("tables=%d, space=%d, revisions=%d, transients=%d", result.TablesOptimized, result.SpaceSaved, result.RevisionsClean, result.TransientsClear),
		Status:     result.Status,
		Error:      result.Error,
		CreatedAt:  time.Now(),
	}
	return s.db.Create(&log).Error
}
