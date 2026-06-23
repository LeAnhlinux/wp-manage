package services

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/gofiber-team/wp-manage/internal/cron"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type BackupService struct {
	db *gorm.DB
}

func NewBackupService(db *gorm.DB) *BackupService {
	return &BackupService{db: db}
}

// CreateBackup creates a backup record and queues a task for the agent
func (s *BackupService) CreateBackup(siteID uint64, req models.CreateBackupRequest) (*models.Backup, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site must be active to create backup")
	}

	storage := req.Storage
	if storage == "" {
		storage = "local"
	}

	backup := models.Backup{
		SiteID:   &siteID,
		ServerID: site.ServerID, // Captured so restore works even after site deletion
		Domain:   site.Domain,   // Captured so identity survives site deletion
		Type:     req.Type,
		Storage:  storage,
		Status:   "running",
	}
	if err := s.db.Create(&backup).Error; err != nil {
		return nil, err
	}

	// Queue backup task for agent
	taskData := models.AgentBackupTask{
		BackupID:   backup.ID,
		Domain:     site.Domain,
		Type:       req.Type,
		DBName:     site.DBName,
		DBUser:     site.DBUser,
		DBPass:     site.DBPassword,
		PHPVersion: site.PHPVersion,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "create_backup",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	return &backup, nil
}

// List returns backups for a site with pagination
func (s *BackupService) List(siteID uint64, page, perPage int) ([]models.Backup, int64, error) {
	var backups []models.Backup
	var total int64

	query := s.db.Model(&models.Backup{})
	if siteID > 0 {
		query = query.Where("site_id = ?", siteID)
	}
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Preload("Site").
		Order("created_at DESC").
		Find(&backups).Error
	return backups, total, err
}

// ListAll returns all backups with RBAC filtering
func (s *BackupService) ListAll(userID uint64, role string, page, perPage int) ([]models.Backup, int64, error) {
	var backups []models.Backup
	var total int64

	query := s.db.Model(&models.Backup{})
	if role != "admin" {
		query = query.Where("site_id IN (?)",
			s.db.Model(&models.Site{}).Select("id").Where("server_id IN (?) OR server_id IN (?)",
				s.db.Model(&models.TeamServer{}).Select("server_id").Where("team_id IN (?)",
					s.db.Model(&models.TeamMember{}).Select("team_id").Where("user_id = ?", userID),
				),
				s.db.Model(&models.UserServer{}).Select("server_id").Where("user_id = ?", userID),
			),
		)
	}
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Preload("Site").
		Order("created_at DESC").
		Find(&backups).Error
	return backups, total, err
}

func (s *BackupService) GetByID(id uint64) (*models.Backup, error) {
	var backup models.Backup
	err := s.db.Preload("Site").First(&backup, id).Error
	return &backup, err
}

// ProxyDownload streams a local backup archive from the agent that holds it.
// Returns the agent's HTTP response (caller forwards the body) and the suggested filename.
// Only completed, local backups can be downloaded; remote-storage backups (s3/r2/...) are not proxied.
func (s *BackupService) ProxyDownload(id uint64) (*http.Response, string, error) {
	var backup models.Backup
	if err := s.db.First(&backup, id).Error; err != nil {
		return nil, "", fmt.Errorf("backup not found")
	}
	if backup.Status != "completed" || backup.FilePath == "" {
		return nil, "", fmt.Errorf("backup file is not available for download")
	}
	if backup.Storage != "local" {
		return nil, "", fmt.Errorf("only local backups can be downloaded from the panel")
	}
	if backup.ServerID == 0 {
		return nil, "", fmt.Errorf("cannot determine which server holds this backup")
	}

	var server models.Server
	if err := s.db.First(&server, backup.ServerID).Error; err != nil {
		return nil, "", fmt.Errorf("server not found for backup")
	}

	params := url.Values{}
	params.Set("path", backup.FilePath)
	u := fmt.Sprintf("https://%s:%d/backup/download?%s", server.IPAddress, agentPort, params.Encode())

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Agent-Key", server.AgentAPIKey)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed cert on internal network
		},
		Timeout: 30 * time.Minute, // backups can be multi-GB
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to reach agent: %w", err)
	}
	return resp, filepath.Base(backup.FilePath), nil
}

func (s *BackupService) Delete(id uint64) error {
	// Fetch the backup first to get file_path and server_id
	var backup models.Backup
	if err := s.db.First(&backup, id).Error; err != nil {
		return fmt.Errorf("backup not found")
	}

	// Queue a task to delete the actual file on the agent server (fire-and-forget)
	if backup.FilePath != "" && backup.ServerID != 0 {
		taskData, _ := json.Marshal(map[string]string{"file_path": backup.FilePath})
		task := models.TaskQueue{
			ServerID: backup.ServerID,
			TaskType: "delete_backup_file",
			Data:     string(taskData),
			Status:   "pending",
		}
		// Non-fatal: log if queue fails but still remove DB record
		if err := s.db.Create(&task).Error; err != nil {
			fmt.Printf("[BackupDelete] failed to queue delete_backup_file task: %v\n", err)
		}
	}

	// Remove DB record
	return s.db.Delete(&models.Backup{}, id).Error
}

// UpdateBackupResult updates the backup record after agent completes the backup
func (s *BackupService) UpdateBackupResult(backupID uint64, filePath string, sizeBytes int64, checksum string) error {
	now := time.Now()
	return s.db.Model(&models.Backup{}).Where("id = ?", backupID).Updates(map[string]interface{}{
		"file_path":    filePath,
		"size_bytes":   sizeBytes,
		"checksum":     checksum,
		"status":       "completed",
		"completed_at": &now,
	}).Error
}

func (s *BackupService) UpdateBackupFailed(backupID uint64, errMsg string) error {
	now := time.Now()
	return s.db.Model(&models.Backup{}).Where("id = ?", backupID).Updates(map[string]interface{}{
		"status":       "failed",
		"error":        errMsg,
		"completed_at": &now,
	}).Error
}

// Restore creates a restore task for the agent
func (s *BackupService) Restore(siteID uint64, req models.RestoreRequest) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	if site.Domain != req.ConfirmDomain {
		return fmt.Errorf("domain confirmation does not match")
	}

	var backup models.Backup
	if err := s.db.First(&backup, req.BackupID).Error; err != nil {
		return fmt.Errorf("backup not found")
	}
	if backup.Status != "completed" {
		return fmt.Errorf("backup is not completed")
	}
	// SiteID is now a pointer (nullable) — check both orphaned and site mismatch cases.
	if backup.SiteID == nil || *backup.SiteID != siteID {
		return fmt.Errorf("backup does not belong to this site")
	}

	// Queue restore task for agent
	taskData := models.AgentRestoreTask{
		BackupID:   backup.ID,
		Domain:     site.Domain,
		Type:       backup.Type,
		FilePath:   backup.FilePath,
		DBName:     site.DBName,
		DBUser:     site.DBUser,
		DBPass:     site.DBPassword,
		PHPVersion: site.PHPVersion,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "restore_backup",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// RestoreByID restores directly from a backup record by its ID.
// Works for both normal and orphaned backups (site_id = NULL).
// For orphaned backups the task is dispatched using backup.ServerID + backup.Domain;
// DB credentials are embedded in the manifest.json inside the archive.
// For non-orphaned backups, DB credentials are pulled from the live site record.
// fallbackServerID may be non-zero for old backups that were created before server_id was stored.
func (s *BackupService) RestoreByID(backupID uint64, confirmDomain string, fallbackServerID uint64) error {
	var backup models.Backup
	if err := s.db.First(&backup, backupID).Error; err != nil {
		return fmt.Errorf("backup not found")
	}
	if backup.Status != "completed" {
		return fmt.Errorf("backup is not completed")
	}
	if backup.Domain != confirmDomain {
		return fmt.Errorf("domain confirmation does not match (expected: %s)", backup.Domain)
	}

	// Resolve the server to dispatch to.
	serverID := backup.ServerID
	if serverID == 0 {
		serverID = fallbackServerID
	}
	if serverID == 0 {
		return fmt.Errorf("cannot determine which server to restore to — please select a server")
	}

	// Persist server_id so future restores don't need a fallback.
	if backup.ServerID == 0 {
		s.db.Model(&backup).Update("server_id", serverID)
	}

	// Determine DB credentials: prefer live site record, fall back to empty (manifest will provide).
	var dbName, dbUser, dbPass, phpVersion string
	if backup.SiteID != nil {
		var site models.Site
		if err := s.db.First(&site, *backup.SiteID).Error; err == nil {
			dbName = site.DBName
			dbUser = site.DBUser
			dbPass = site.DBPassword
			phpVersion = site.PHPVersion
		}
	}
	// If site is deleted (orphaned backup), leave creds empty → agent reads from manifest.

	taskData := models.AgentRestoreTask{
		BackupID:   backup.ID,
		Domain:     backup.Domain,
		Type:       backup.Type,
		FilePath:   backup.FilePath,
		DBName:     dbName,
		DBUser:     dbUser,
		DBPass:     dbPass,
		PHPVersion: phpVersion,
	}
	dataJSON, _ := json.Marshal(taskData)

	var siteID uint64
	if backup.SiteID != nil {
		siteID = *backup.SiteID
	}

	task := models.TaskQueue{
		ServerID: serverID, // use resolved serverID (may differ from backup.ServerID for old backups)
		SiteID:   siteID,
		TaskType: "restore_backup",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// RecreateOrphanedSiteRecord creates a new sites row in the panel DB after a
// successful orphaned-backup restore. This makes the restored site visible in
// the panel again.  It also re-links the backup to the newly-created site.
func (s *BackupService) RecreateOrphanedSiteRecord(backupID uint64, domain, dbName, dbUser, dbPass, phpVersion string) error {
	// Look up the backup to get server_id
	var backup models.Backup
	if err := s.db.First(&backup, backupID).Error; err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}
	if backup.ServerID == 0 {
		return fmt.Errorf("backup has no server_id — cannot determine target server")
	}

	// Avoid duplicates — if a site with this domain already exists, just re-link the backup
	var existing models.Site
	if err := s.db.Where("domain = ?", domain).First(&existing).Error; err == nil {
		siteID := existing.ID
		s.db.Model(&backup).Updates(map[string]interface{}{"site_id": siteID})
		return nil
	}

	phpVer := phpVersion
	if phpVer == "" {
		phpVer = "8.2"
	}

	site := models.Site{
		ServerID:    backup.ServerID,
		Domain:      domain,
		PHPVersion:  phpVer,
		DBName:      dbName,
		DBUser:      dbUser,
		DBPassword:  dbPass,
		Status:      "active",
		InstallType: "auto",
	}
	if err := s.db.Create(&site).Error; err != nil {
		return fmt.Errorf("failed to create site record: %w", err)
	}

	// Re-link backup to the newly created site
	siteID := site.ID
	s.db.Model(&backup).Updates(map[string]interface{}{"site_id": siteID})
	return nil
}

// ListByServer returns all backups belonging to any site on the given server,
// including orphaned backups (site_id = NULL) identified by the domain column.
// Orphaned backups are listed first so they are easy to spot.
func (s *BackupService) ListByServer(serverID uint64, page, perPage int) ([]models.Backup, int64, error) {
	var backups []models.Backup
	var total int64

	// Subquery: domain names of sites on this server
	siteDomainsSubquery := s.db.Model(&models.Site{}).Select("domain").Where("server_id = ?", serverID)

	query := s.db.Model(&models.Backup{}).
		Where("domain IN (?) OR site_id IN (?)",
			siteDomainsSubquery,
			s.db.Model(&models.Site{}).Select("id").Where("server_id = ?", serverID),
		)

	query.Count(&total)
	err := query.
		Offset((page-1)*perPage).Limit(perPage).
		Order("ISNULL(site_id) DESC, created_at DESC"). // orphaned (NULL site_id) first
		Find(&backups).Error

	return backups, total, err
}

// --- Backup Schedules ---

func (s *BackupService) CreateSchedule(siteID uint64, req models.CreateBackupScheduleRequest) (*models.BackupSchedule, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	storage := req.Storage
	if storage == "" {
		storage = "local"
	}
	retention := req.Retention
	if retention == 0 {
		retention = 7
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	schedule := models.BackupSchedule{
		SiteID:    siteID,
		Type:      req.Type,
		Storage:   storage,
		CronExpr:  req.CronExpr,
		Retention: retention,
		Enabled:   enabled,
	}
	// Pre-calculate next_run_at so the worker can trigger without parsing all exprs on every tick
	if enabled {
		if next, err := cron.Next(req.CronExpr, time.Now()); err == nil {
			schedule.NextRunAt = &next
		}
	}
	if err := s.db.Create(&schedule).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *BackupService) ListSchedules(siteID uint64) ([]models.BackupSchedule, error) {
	var schedules []models.BackupSchedule
	err := s.db.Where("site_id = ?", siteID).Preload("Site").Order("created_at DESC").Find(&schedules).Error
	return schedules, err
}

func (s *BackupService) UpdateSchedule(id uint64, req models.UpdateBackupScheduleRequest) (*models.BackupSchedule, error) {
	var schedule models.BackupSchedule
	if err := s.db.First(&schedule, id).Error; err != nil {
		return nil, fmt.Errorf("schedule not found")
	}

	updates := map[string]interface{}{}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.Storage != nil {
		updates["storage"] = *req.Storage
	}
	cronChanged := req.CronExpr != nil
	enabledChanged := req.Enabled != nil
	if cronChanged {
		updates["cron_expr"] = *req.CronExpr
	}
	if req.Retention != nil {
		updates["retention"] = *req.Retention
	}
	if enabledChanged {
		updates["enabled"] = *req.Enabled
	}

	// Recalculate next_run_at whenever cron_expr or enabled changes
	if cronChanged || enabledChanged {
		expr := schedule.CronExpr
		if cronChanged {
			expr = *req.CronExpr
		}
		enabled := schedule.Enabled
		if enabledChanged {
			enabled = *req.Enabled
		}
		if enabled {
			if next, err := cron.Next(expr, time.Now()); err == nil {
				updates["next_run_at"] = next
			}
		} else {
			updates["next_run_at"] = nil // disabled → clear next_run_at
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(&schedule).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	s.db.First(&schedule, id)
	return &schedule, nil
}

func (s *BackupService) DeleteSchedule(id uint64) error {
	return s.db.Delete(&models.BackupSchedule{}, id).Error
}
