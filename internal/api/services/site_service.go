package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

var domainRe = regexp.MustCompile(`(?i)^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)+$`)

type SiteService struct {
	db *gorm.DB
}

func NewSiteService(db *gorm.DB) *SiteService {
	return &SiteService{db: db}
}

func (s *SiteService) DB() *gorm.DB { return s.db }

func (s *SiteService) Create(req models.CreateSiteRequest) (*models.Site, string, error) {
	// Validate domain format
	if !domainRe.MatchString(req.Domain) {
		return nil, "", fmt.Errorf("invalid domain format: must be a valid FQDN like 'example.com'")
	}

	// Check domain uniqueness
	var existing models.Site
	if err := s.db.Where("domain = ?", req.Domain).First(&existing).Error; err == nil {
		return nil, "", fmt.Errorf("domain '%s' already exists", req.Domain)
	}

	var server models.Server
	if err := s.db.First(&server, req.ServerID).Error; err != nil {
		return nil, "", fmt.Errorf("server not found")
	}
	if server.Status != "active" {
		return nil, "", fmt.Errorf("server is not active")
	}

	phpVer := req.PHPVersion
	if phpVer == "" {
		phpVer = "8.2"
	}

	installType := req.InstallType
	if installType == "" {
		installType = "auto"
	}

	// For auto-install, WPAdminUser/Email are required
	if installType == "auto" && (req.WPAdminUser == "" || req.WPAdminEmail == "") {
		return nil, "", fmt.Errorf("wp_admin_user and wp_admin_email are required for auto installation")
	}

	dbName := "wp_" + sanitizeDBName(req.Domain)
	dbUser := "wp_" + sanitizeDBName(req.Domain)
	dbPass := utils.GenerateRandomPassword(24)
	wpAdminPass := utils.GenerateRandomPassword(16)

	site := models.Site{
		ServerID:     req.ServerID,
		Domain:       req.Domain,
		PHPVersion:   phpVer,
		SSLEnabled:   req.SSLEnabled,
		DBName:       dbName,
		DBUser:       dbUser,
		DBPassword:   dbPass,
		WPAdminUser:  req.WPAdminUser,
		WPAdminEmail: req.WPAdminEmail,
		WPAdminPass:  wpAdminPass,
		InstallType:  installType,
		Status:       "creating",
	}

	if err := s.db.Create(&site).Error; err != nil {
		return nil, "", err
	}

	domain := models.Domain{
		SiteID: site.ID,
		Domain: req.Domain,
		Type:   "primary",
	}
	s.db.Create(&domain)

	return &site, dbPass, nil
}

func (s *SiteService) List(userID uint64, role string, serverID uint64, page, perPage int) ([]models.Site, int64, error) {
	var sites []models.Site
	var total int64

	query := s.db.Model(&models.Site{}).Where("status != ?", "deleted")
	if serverID > 0 {
		query = query.Where("server_id = ?", serverID)
	}
	if role != "admin" {
		// Non-admin: show sites on servers from team membership + direct UserServer
		query = query.Where("server_id IN (?) OR server_id IN (?)",
			s.db.Model(&models.TeamServer{}).Select("server_id").Where("team_id IN (?)",
				s.db.Model(&models.TeamMember{}).Select("team_id").Where("user_id = ?", userID),
			),
			s.db.Model(&models.UserServer{}).Select("server_id").Where("user_id = ?", userID),
		)
	}
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Preload("Server").
		Order("created_at DESC").
		Find(&sites).Error
	return sites, total, err
}

func (s *SiteService) GetByID(id uint64) (*models.Site, error) {
	var site models.Site
	err := s.db.Preload("Server").First(&site, id).Error
	return &site, err
}

func (s *SiteService) GetByDomain(domain string) (*models.Site, error) {
	var site models.Site
	err := s.db.Where("domain = ?", domain).First(&site).Error
	return &site, err
}

func (s *SiteService) Delete(id uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Orphan backup records instead of deleting them so users don't lose restore points.
		// Set site_id = NULL so the backups survive as orphaned records browsable at server level.
		// The domain field on backups was populated at creation time, so we retain that info.
		tx.Exec("UPDATE backups SET site_id = NULL WHERE site_id = ?", id)

		// Delete transfer_site_results for this site.
		// These rows carry a unique index on (target_domain, target_server_id), so
		// keeping them (even with site_id = NULL) would block the user from
		// transferring the same domain to the same server again after deletion.
		// History at the job level (transfer_jobs) is preserved regardless.
		tx.Exec("DELETE FROM transfer_site_results WHERE site_id = ?", id)

		// Delete all other child records that FK-reference sites.id
		// NOTE: "backups" is intentionally excluded — handled above via orphan.
		tables := []string{
			"domains", "backup_schedules", "migrations", "redis_configs",
			"cache_configs", "waf_configs", "auto_update_configs", "sftp_accounts",
			"uptime_checks", "performance_metrics", "image_opt_configs",
			"image_opt_results", "malware_scans", "update_logs", "wpcli_histories",
			"external_imports", "subsites", "staging_sites",
		}
		for _, tbl := range tables {
			if tbl == "staging_sites" {
				tx.Exec("DELETE FROM staging_sites WHERE production_id = ? OR staging_site_id = ?", id, id)
				continue
			}
			tx.Exec("DELETE FROM "+tbl+" WHERE site_id = ?", id)
		}
		return tx.Delete(&models.Site{}, id).Error
	})
}

func (s *SiteService) UpdateStatus(id uint64, status string) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("status", status).Error
}

func (s *SiteService) UpdatePHP(id uint64, version string) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("php_version", version).Error
}

func (s *SiteService) GetSitesByServer(serverID uint64) ([]models.Site, error) {
	var sites []models.Site
	err := s.db.Where("server_id = ? AND status != ?", serverID, "deleted").Find(&sites).Error
	return sites, err
}

func (s *SiteService) GetPendingSites(serverID uint64) ([]models.Site, error) {
	var sites []models.Site
	err := s.db.Where("server_id = ? AND status = ?", serverID, "creating").Find(&sites).Error
	return sites, err
}

func (s *SiteService) UpdateSSL(id uint64, enabled bool) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("ssl_enabled", enabled).Error
}

func (s *SiteService) UpdateSSLExpiry(id uint64, expiresAt *time.Time) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("ssl_expires_at", expiresAt).Error
}

func (s *SiteService) UpdateSSLEnabled(id uint64, enabled bool) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("ssl_enabled", enabled).Error
}

func (s *SiteService) UpdateForceHTTPS(id uint64, enabled bool) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("force_https", enabled).Error
}

func (s *SiteService) UpdateWPVersion(id uint64, version string) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("wp_version", version).Error
}

// QueueGetWPVersionIfNeeded queues a get_wp_version task for the site if no such
// task is already pending/processing. Called on site access when wp_version is empty.
func (s *SiteService) QueueGetWPVersionIfNeeded(site *models.Site) {
	var count int64
	s.db.Model(&models.TaskQueue{}).
		Where("server_id = ? AND site_id = ? AND task_type = ? AND status IN ('pending','processing')",
			site.ServerID, site.ID, "get_wp_version").
		Count(&count)
	if count > 0 {
		return // already queued
	}
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   site.ID,
		TaskType: "get_wp_version",
		Data:     fmt.Sprintf(`{"site_id":%d,"domain":"%s","php_version":"%s"}`, site.ID, site.Domain, site.PHPVersion),
		Status:   "pending",
	}
	s.db.Create(&task)
}

func (s *SiteService) UpdateAutologinSecret(id uint64, secret string) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("autologin_secret", secret).Error
}

func (s *SiteService) UpdateLinuxUser(id uint64, user string) error {
	return s.db.Model(&models.Site{}).Where("id = ?", id).Update("linux_user", user).Error
}

// ToggleOpenBasedir flips open_basedir_enabled for a site, then synchronously
// rebuilds the PHP-FPM pool so the change takes effect immediately.
// Returns the new value of open_basedir_enabled.
func (s *SiteService) ToggleOpenBasedir(siteID uint64) (bool, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return false, fmt.Errorf("site not found")
	}

	newValue := !site.OpenBasedirEnabled

	// Persist the new value first.
	if err := s.db.Model(&models.Site{}).Where("id = ?", siteID).
		Update("open_basedir_enabled", newValue).Error; err != nil {
		return false, err
	}

	phpVersion := site.PHPVersion
	if phpVersion == "" {
		phpVersion = "8.2"
	}

	// Rebuild the PHP-FPM pool synchronously so the change takes effect now.
	taskData := models.AgentRebuildConfigTask{
		ConfigTypes:        []string{"php_fpm_pool"},
		Domain:             site.Domain,
		PHPVersion:         phpVersion,
		OpenBasedirEnabled: newValue,
	}
	task, err := s.CreateJSONTaskReturn(site.ServerID, siteID, "rebuild_config",
		map[string]interface{}{
			"config_types":         taskData.ConfigTypes,
			"domain":               taskData.Domain,
			"php_version":          taskData.PHPVersion,
			"open_basedir_enabled": taskData.OpenBasedirEnabled,
		})
	if err != nil {
		return newValue, fmt.Errorf("failed to queue rebuild task: %w", err)
	}

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return newValue, err
	}
	if result.Error != "" {
		return newValue, fmt.Errorf("%s", result.Error)
	}
	return newValue, nil
}

func (s *SiteService) GetAutologinSecret(id uint64) string {
	var site models.Site
	err := s.db.Select("autologin_secret").First(&site, id).Error
	if err != nil {
		return ""
	}
	return site.AutologinSecret
}

// GetCredentials returns site with sensitive fields (admin password)
func (s *SiteService) GetCredentials(id uint64) (map[string]string, error) {
	var site models.Site
	err := s.db.Select("wp_admin_user, wp_admin_pass, wp_admin_email, db_name, db_user, db_password, domain").
		First(&site, id).Error
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"wp_admin_user":  site.WPAdminUser,
		"wp_admin_pass":  site.WPAdminPass,
		"wp_admin_email": site.WPAdminEmail,
		"wp_admin_url":   "https://" + site.Domain + "/wp-admin",
		"db_name":        site.DBName,
		"db_user":        site.DBUser,
		"db_password":    site.DBPassword,
	}, nil
}

func (s *SiteService) CreateTask(serverID, siteID uint64, taskType string, data map[string]string) error {
	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: taskType,
		Data:     string(dataJSON),
		Status:   "pending",
	}
	return s.db.Create(&task).Error
}

// CreateJSONTask creates a task with arbitrary JSON data (map[string]interface{}).
func (s *SiteService) CreateJSONTask(serverID, siteID uint64, taskType string, data map[string]interface{}) error {
	_, err := s.CreateJSONTaskReturn(serverID, siteID, taskType, data)
	return err
}

// CreateJSONTaskReturn creates a task and returns the created record (for synchronous waiting).
func (s *SiteService) CreateJSONTaskReturn(serverID, siteID uint64, taskType string, data map[string]interface{}) (*models.TaskQueue, error) {
	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: taskType,
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *SiteService) GetPendingTasks(serverID uint64) ([]models.TaskQueue, error) {
	// Reset tasks that got stuck in 'processing' (agent crashed mid-task) after 5 minutes.
	// This is safe because any legitimately running task completes well within 5 minutes
	// for lightweight tasks; long-running tasks (transfer, backup) are handled in goroutines
	// and update their own status via callbacks.
	staleThreshold := time.Now().Add(-5 * time.Minute)
	s.db.Model(&models.TaskQueue{}).
		Where("server_id = ? AND status = ? AND updated_at < ?", serverID, "processing", staleThreshold).
		Update("status", "pending")

	var tasks []models.TaskQueue
	err := s.db.Where("server_id = ? AND status = ?", serverID, "pending").
		Order("created_at ASC").Find(&tasks).Error
	return tasks, err
}

func (s *SiteService) CompleteTask(taskID uint64, status, errMsg string) error {
	updates := map[string]interface{}{"status": status}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	return s.db.Model(&models.TaskQueue{}).Where("id = ?", taskID).Updates(updates).Error
}

func (s *SiteService) CompleteTaskWithOutput(taskID uint64, status, output, errMsg string) error {
	updates := map[string]interface{}{"status": status}
	if output != "" {
		updates["output"] = output
	}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	return s.db.Model(&models.TaskQueue{}).Where("id = ?", taskID).Updates(updates).Error
}

func sanitizeDBName(domain string) string {
	result := make([]byte, 0, len(domain))
	for _, c := range domain {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, byte(c))
		} else if c == '.' || c == '-' {
			result = append(result, '_')
		}
	}
	if len(result) > 16 {
		result = result[:16]
	}
	return string(result)
}
