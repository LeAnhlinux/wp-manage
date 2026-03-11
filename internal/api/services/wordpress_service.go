package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

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
	stagingSite := models.Site{
		ServerID:     site.ServerID,
		Domain:       stagingDomain,
		PHPVersion:   site.PHPVersion,
		SSLEnabled:   false,
		WPAdminUser:  site.WPAdminUser,
		WPAdminEmail: site.WPAdminEmail,
		Status:       "creating",
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
		"staging_id":     staging.ID,
		"staging_site_id": stagingSite.ID,
		"production_domain": site.Domain,
		"staging_domain":    stagingDomain,
		"php_version":       site.PHPVersion,
		"db_name":           site.DBName,
		"db_user":           site.DBUser,
		"db_password":       site.DBPassword,
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
		"staging_id":         staging.ID,
		"production_domain":  staging.Production.Domain,
		"staging_domain":     staging.StagingSite2.Domain,
		"db_name":            staging.Production.DBName,
		"db_user":            staging.Production.DBUser,
		"db_password":        staging.Production.DBPassword,
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
		"staging_id":      staging.ID,
		"staging_domain":  staging.StagingSite2.Domain,
		"staging_db_name": staging.StagingSite2.DBName,
		"staging_db_user": staging.StagingSite2.DBUser,
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

// CheckUpdates queues a task for agent to check available WP updates
func (s *WordPressService) CheckUpdates(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return fmt.Errorf("site must be active")
	}

	taskData := models.AgentCheckUpdatesTask{
		SiteID: siteID,
		Domain: site.Domain,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "check_updates",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// CheckUpdatesSync queues update check and waits for result synchronously
func (s *WordPressService) CheckUpdatesSync(siteID uint64) (interface{}, error) {
	startTime := time.Now()
	if err := s.CheckUpdates(siteID); err != nil {
		return nil, err
	}

	// Wait for agent to callback with results
	logs, err := WaitForUpdateLogs(s.db, siteID, startTime)
	if err != nil {
		return nil, err
	}

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

// OptimizeDatabase queues a DB optimization task for agent
func (s *WordPressService) OptimizeDatabase(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return fmt.Errorf("site must be active")
	}

	taskData := models.AgentDBOptimizeTask{
		SiteID: siteID,
		Domain: site.Domain,
		DBName: site.DBName,
		DBUser: site.DBUser,
		DBPass: site.DBPassword,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "db_optimize",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// OptimizeDatabaseSync queues DB optimization and waits for result
func (s *WordPressService) OptimizeDatabaseSync(siteID uint64) (interface{}, error) {
	startTime := time.Now()
	if err := s.OptimizeDatabase(siteID); err != nil {
		return nil, err
	}

	// Wait for db_optimize result in UpdateLog
	logs, _ := WaitForUpdateLogs(s.db, siteID, startTime)
	for _, l := range logs {
		if l.UpdateType == "db_optimize" {
			return l, nil
		}
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
