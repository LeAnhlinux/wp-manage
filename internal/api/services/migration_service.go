package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type MigrationService struct {
	db *gorm.DB
}

func NewMigrationService(db *gorm.DB) *MigrationService {
	return &MigrationService{db: db}
}

// StartMigration creates a migration record and queues the task for the source server's agent
func (s *MigrationService) StartMigration(siteID uint64, req models.MigrateRequest) (*models.Migration, error) {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site must be active to migrate")
	}

	var targetServer models.Server
	if err := s.db.First(&targetServer, req.TargetServerID).Error; err != nil {
		return nil, fmt.Errorf("target server not found")
	}
	if targetServer.Status != "active" {
		return nil, fmt.Errorf("target server must be active")
	}
	if site.ServerID == req.TargetServerID {
		return nil, fmt.Errorf("cannot migrate to the same server")
	}

	// Check no existing running migration for this site
	var count int64
	s.db.Model(&models.Migration{}).Where("site_id = ? AND status = ?", siteID, "running").Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("site already has a running migration")
	}

	now := time.Now()
	migration := models.Migration{
		SiteID:         siteID,
		SourceServerID: site.ServerID,
		TargetServerID: req.TargetServerID,
		Phase:          "preparation",
		Status:         "running",
		Progress:       0,
		StartedAt:      now,
	}
	if err := s.db.Create(&migration).Error; err != nil {
		return nil, err
	}

	// Add initial log entry
	s.AddLog(migration.ID, "preparation", "info", "Migration started")

	// Queue migrate task for the source server's agent
	taskData := models.AgentMigrateTask{
		MigrationID:    migration.ID,
		SiteID:         siteID,
		Domain:         site.Domain,
		SourceServerIP: site.Server.IPAddress,
		TargetServerIP: targetServer.IPAddress,
		DBName:         site.DBName,
		DBUser:         site.DBUser,
		DBPass:         site.DBPassword,
		PHPVersion:     site.PHPVersion,
		SSLEnabled:     site.SSLEnabled,
		WPAdminUser:    site.WPAdminUser,
		WPAdminEmail:   site.WPAdminEmail,
		ChangeDomain:   req.ChangeDomain,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "migrate_site",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	// Reload with associations
	s.db.Preload("Site").Preload("SourceServer").Preload("TargetServer").First(&migration, migration.ID)

	return &migration, nil
}

func (s *MigrationService) GetByID(id uint64) (*models.Migration, error) {
	var migration models.Migration
	err := s.db.Preload("Site").Preload("SourceServer").Preload("TargetServer").First(&migration, id).Error
	return &migration, err
}

func (s *MigrationService) List(userID uint64, role string, page, perPage int) ([]models.Migration, int64, error) {
	var migrations []models.Migration
	var total int64

	query := s.db.Model(&models.Migration{})
	if role != "admin" {
		accessibleServers := s.db.Raw(`
			SELECT DISTINCT server_id FROM (
				SELECT server_id FROM team_servers WHERE team_id IN (
					SELECT team_id FROM team_members WHERE user_id = ?
				)
				UNION
				SELECT server_id FROM user_servers WHERE user_id = ?
			) AS combined
		`, userID, userID)
		query = query.Where("source_server_id IN (?) OR target_server_id IN (?)",
			accessibleServers, accessibleServers,
		)
	}
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Preload("Site").Preload("SourceServer").Preload("TargetServer").
		Order("started_at DESC").
		Find(&migrations).Error
	return migrations, total, err
}

// UpdateMigration updates migration status from agent reports
func (s *MigrationService) UpdateMigration(update models.AgentMigrationUpdate) error {
	updates := map[string]interface{}{
		"phase":    update.Phase,
		"progress": update.Progress,
		"status":   update.Status,
	}
	if update.Error != "" {
		updates["error"] = update.Error
	}
	if update.Status == "completed" || update.Status == "failed" || update.Status == "rolled_back" {
		now := time.Now()
		updates["completed_at"] = &now
	}

	if err := s.db.Model(&models.Migration{}).Where("id = ?", update.MigrationID).Updates(updates).Error; err != nil {
		return err
	}

	// Add log entry if message provided
	if update.LogMessage != "" {
		level := update.LogLevel
		if level == "" {
			level = "info"
		}
		s.AddLog(update.MigrationID, update.Phase, level, update.LogMessage)
	}

	// If migration completed, update the site's server_id to target server
	if update.Status == "completed" {
		var migration models.Migration
		if err := s.db.First(&migration, update.MigrationID).Error; err == nil {
			s.db.Model(&models.Site{}).Where("id = ?", migration.SiteID).Update("server_id", migration.TargetServerID)
		}
	}

	return nil
}

// Rollback a migration
func (s *MigrationService) Rollback(id uint64) error {
	var migration models.Migration
	if err := s.db.First(&migration, id).Error; err != nil {
		return fmt.Errorf("migration not found")
	}
	if migration.Status != "running" {
		return fmt.Errorf("can only rollback running migrations")
	}

	// Queue rollback task on source server
	taskData := map[string]interface{}{
		"migration_id": migration.ID,
		"domain":       "",
	}
	// Get site domain
	var site models.Site
	if err := s.db.First(&site, migration.SiteID).Error; err == nil {
		taskData["domain"] = site.Domain
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: migration.SourceServerID,
		SiteID:   migration.SiteID,
		TaskType: "rollback_migration",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	s.AddLog(id, migration.Phase, "warn", "Rollback requested by user")

	return nil
}

// Cancel a migration
func (s *MigrationService) Cancel(id uint64) error {
	var migration models.Migration
	if err := s.db.First(&migration, id).Error; err != nil {
		return fmt.Errorf("migration not found")
	}
	if migration.Status != "running" {
		return fmt.Errorf("can only cancel running migrations")
	}

	now := time.Now()
	s.db.Model(&migration).Updates(map[string]interface{}{
		"status":       "failed",
		"error":        "Cancelled by user",
		"completed_at": &now,
	})
	s.AddLog(id, migration.Phase, "warn", "Migration cancelled by user")

	return nil
}

// AddLog adds a log entry for a migration
func (s *MigrationService) AddLog(migrationID uint64, phase, level, message string) {
	log := models.MigrationLog{
		MigrationID: migrationID,
		Phase:       phase,
		Message:     message,
		Level:       level,
	}
	s.db.Create(&log)
}

// GetLogs returns all log entries for a migration
func (s *MigrationService) GetLogs(migrationID uint64) ([]models.MigrationLog, error) {
	var logs []models.MigrationLog
	err := s.db.Where("migration_id = ?", migrationID).Order("created_at ASC").Find(&logs).Error
	return logs, err
}
