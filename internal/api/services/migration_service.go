package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type MigrationService struct {
	db         *gorm.DB
	dnsService *DNSService // optional — used to auto-update DNS after keep_domain migration
}

func NewMigrationService(db *gorm.DB, dnsService *DNSService) *MigrationService {
	return &MigrationService{db: db, dnsService: dnsService}
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

	// Validate change_domain mode
	mode := req.Mode
	if mode == "" {
		mode = "keep_domain"
	}
	if mode != "keep_domain" && mode != "change_domain" {
		return nil, fmt.Errorf("mode phải là 'keep_domain' hoặc 'change_domain'")
	}
	if mode == "change_domain" && req.NewDomain == "" {
		return nil, fmt.Errorf("new_domain là bắt buộc khi dùng mode change_domain")
	}

	// Check no existing running migration for this site
	var count int64
	s.db.Model(&models.Migration{}).Where("site_id = ? AND status = ?", siteID, "running").Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("site đang có migration đang chạy")
	}

	now := time.Now()
	migration := models.Migration{
		SiteID:         siteID,
		SourceServerID: site.ServerID,
		TargetServerID: req.TargetServerID,
		Mode:           mode,
		NewDomain:      req.NewDomain,
		UpdateDNS:      req.UpdateDNS,
		Phase:          "preparation",
		Status:         "running",
		Progress:       0,
		StartedAt:      now,
	}
	if err := s.db.Create(&migration).Error; err != nil {
		return nil, err
	}

	modeLabel := "giữ nguyên domain"
	if mode == "change_domain" {
		modeLabel = fmt.Sprintf("đổi domain → %s", req.NewDomain)
	}
	s.AddLog(migration.ID, "preparation", "info", fmt.Sprintf("Bắt đầu migration [%s]", modeLabel))

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
		LinuxUser:      site.LinuxUser,
		Mode:           mode,
		NewDomain:      req.NewDomain,
		CleanupSource:  req.CleanupSource,
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

	// If migration completed, update site's server_id and optionally auto-update DNS
	if update.Status == "completed" {
		var migration models.Migration
		if err := s.db.Preload("Site").Preload("TargetServer").First(&migration, update.MigrationID).Error; err == nil {
			// Update site to point to target server
			newDomain := migration.Site.Domain
			if migration.Mode == "change_domain" && migration.NewDomain != "" {
				newDomain = migration.NewDomain
				// Also update domain on the site record
				s.db.Model(&models.Site{}).Where("id = ?", migration.SiteID).Updates(map[string]interface{}{
					"server_id": migration.TargetServerID,
					"domain":    newDomain,
				})
			} else {
				s.db.Model(&models.Site{}).Where("id = ?", migration.SiteID).Update("server_id", migration.TargetServerID)
			}

			// Auto-update DNS A record for keep_domain mode
			if migration.Mode == "keep_domain" && migration.UpdateDNS && s.dnsService != nil && migration.TargetServer != nil {
				targetIP := migration.TargetServer.IPAddress
				if err := s.dnsService.AutoUpdateRecord(newDomain, targetIP); err != nil {
					s.AddLog(migration.ID, "cleanup", "warn",
						fmt.Sprintf("DNS tự động cập nhật thất bại: %v — Vui lòng cập nhật DNS thủ công trỏ %s → %s", err, newDomain, targetIP))
				} else {
					s.AddLog(migration.ID, "cleanup", "info",
						fmt.Sprintf("DNS đã được cập nhật: %s → %s", newDomain, targetIP))
				}
			} else if migration.Mode == "keep_domain" && !migration.UpdateDNS {
				s.AddLog(migration.ID, "cleanup", "info",
					fmt.Sprintf("Vui lòng cập nhật DNS thủ công: trỏ %s → %s", newDomain, migration.TargetServer.IPAddress))
			}
		}
	}

	return nil
}

// Rollback a migration.
// keep_domain: re-enables source nginx + cleans up target artifacts.
// change_domain: source was never suspended; still clean up target artifacts.
func (s *MigrationService) Rollback(id uint64) error {
	var migration models.Migration
	if err := s.db.Preload("Site").First(&migration, id).Error; err != nil {
		return fmt.Errorf("migration not found")
	}
	if migration.Status == "rolled_back" {
		return fmt.Errorf("migration đã được rollback trước đó")
	}

	domain := ""
	dbName, dbUser, dbPass := "", "", ""
	if migration.Site != nil {
		domain = migration.Site.Domain
		dbName = migration.Site.DBName
		dbUser = migration.Site.DBUser
		dbPass = migration.Site.DBPassword
	}

	targetDomain := domain
	if migration.Mode == "change_domain" && migration.NewDomain != "" {
		targetDomain = migration.NewDomain
	}

	if migration.Mode == "keep_domain" {
		// Task 1 (keep_domain only): re-enable nginx on source server
		// change_domain never suspends source — no need to restore
		sourceData, _ := json.Marshal(map[string]interface{}{
			"migration_id": migration.ID,
			"domain":       domain,
			"mode":         migration.Mode,
		})
		s.db.Create(&models.TaskQueue{
			ServerID: migration.SourceServerID,
			SiteID:   migration.SiteID,
			TaskType: "rollback_migration",
			Data:     string(sourceData),
			Status:   "pending",
		})
	}

	// Task 2: clean up target artifacts (new domain for change_domain, target server for keep_domain)
	targetData, _ := json.Marshal(models.AgentMigrateTask{
		MigrationID:   migration.ID,
		SiteID:        migration.SiteID,
		Domain:        targetDomain,
		DBName:        dbName,
		DBUser:        dbUser,
		DBPass:        dbPass,
		CleanupSource: true, // reuse cleanupSourceSite logic
	})
	targetServerID := migration.TargetServerID
	if migration.Mode == "change_domain" {
		// change_domain: source and target are the same server
		targetServerID = migration.SourceServerID
	}
	s.db.Create(&models.TaskQueue{
		ServerID: targetServerID,
		SiteID:   migration.SiteID,
		TaskType: "cleanup_target_migration",
		Data:     string(targetData),
		Status:   "pending",
	})

	logMsg := fmt.Sprintf("Rollback được yêu cầu [mode: %s] — dọn dẹp domain mới (%s)", migration.Mode, targetDomain)
	if migration.Mode == "keep_domain" {
		logMsg = fmt.Sprintf("Rollback được yêu cầu [mode: %s] — đang khôi phục nguồn và dọn dẹp server đích (%s)", migration.Mode, targetDomain)
	}
	s.AddLog(id, migration.Phase, "warn", logMsg)
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

// SetupMigrationSSH queues an add_authorized_key task on the target agent and waits for it.
// This grants temporary SSH access from the source server to the target server.
func (s *MigrationService) SetupMigrationSSH(migrationID uint64, publicKey string) error {
	var migration models.Migration
	if err := s.db.First(&migration, migrationID).Error; err != nil {
		return fmt.Errorf("migration not found")
	}
	comment := fmt.Sprintf("wpm-migration-%d-temp", migrationID)
	dataJSON, _ := json.Marshal(map[string]string{
		"public_key": publicKey,
		"comment":    comment,
	})
	task := models.TaskQueue{
		ServerID: migration.TargetServerID,
		SiteID:   migration.SiteID,
		TaskType: "add_authorized_key",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("target agent không phản hồi: %v", err)
	}
	if result.Status == "failed" {
		return fmt.Errorf("target agent từ chối SSH key: %s", result.Error)
	}
	s.AddLog(migrationID, "preparation", "info", "SSH key tạm thời đã được thêm vào server đích")
	return nil
}

// CleanupMigrationSSH removes the temp SSH key from the target server (best-effort, fire-and-forget).
func (s *MigrationService) CleanupMigrationSSH(migrationID uint64) {
	var migration models.Migration
	if err := s.db.First(&migration, migrationID).Error; err != nil {
		return
	}
	comment := fmt.Sprintf("wpm-migration-%d-temp", migrationID)
	dataJSON, _ := json.Marshal(map[string]string{"comment": comment})
	task := models.TaskQueue{
		ServerID: migration.TargetServerID,
		SiteID:   migration.SiteID,
		TaskType: "remove_authorized_key",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)
	s.AddLog(migrationID, "cleanup", "info", "SSH key tạm thời đã được xóa khỏi server đích")
}

// CleanupSource queues a cleanup_source_migration task on the SOURCE agent to remove
// all site artifacts (files, DB, nginx/php-fpm configs, SSL cert) after migration.
// Only allowed on completed migrations that haven't been cleaned up yet.
func (s *MigrationService) CleanupSource(id uint64) error {
	var migration models.Migration
	if err := s.db.Preload("Site").First(&migration, id).Error; err != nil {
		return fmt.Errorf("migration not found")
	}
	if migration.Status != "completed" {
		return fmt.Errorf("chỉ có thể dọn dẹp migration đã hoàn tất")
	}
	if migration.SourceCleanedUp {
		return fmt.Errorf("server nguồn đã được dọn dẹp trước đó")
	}

	domain := ""
	if migration.Site != nil {
		domain = migration.Site.Domain
	}

	taskData, _ := json.Marshal(models.AgentMigrateTask{
		MigrationID:   migration.ID,
		SiteID:        migration.SiteID,
		Domain:        domain,
		DBName:        migration.Site.DBName,
		DBUser:        migration.Site.DBUser,
		DBPass:        migration.Site.DBPassword,
		CleanupSource: true,
	})
	task := models.TaskQueue{
		ServerID: migration.SourceServerID,
		SiteID:   migration.SiteID,
		TaskType: "cleanup_source_migration",
		Data:     string(taskData),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	// Mark as cleaned up
	s.db.Model(&migration).Update("source_cleaned_up", true)
	s.AddLog(id, "cleanup", "warn",
		fmt.Sprintf("Dọn dẹp server nguồn được yêu cầu — xóa files, DB, configs của %s", domain))
	return nil
}

// GetLogs returns all log entries for a migration
func (s *MigrationService) GetLogs(migrationID uint64) ([]models.MigrationLog, error) {
	var logs []models.MigrationLog
	err := s.db.Where("migration_id = ?", migrationID).Order("created_at ASC").Find(&logs).Error
	return logs, err
}
