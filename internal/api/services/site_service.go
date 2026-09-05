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
			"image_opt_results", "malware_scans", "malware_scan_schedules", "update_logs", "wpcli_histories",
			"threat_whitelists", "quarantined_files",
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

// ─── Resource limits ───

// UpdateResourceLimits persists disk/cpu/mem limits. If the CPU or memory cap
// changed, it queues a set_resource_limit task (apply or remove) and waits for
// the agent to converge so the response reflects the real result.
func (s *SiteService) UpdateResourceLimits(siteID uint64, req models.UpdateResourceLimitsRequest) (*models.Site, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	updates := map[string]interface{}{}
	if req.DiskLimitMB != nil {
		if *req.DiskLimitMB == 0 {
			updates["disk_limit_mb"] = nil
		} else {
			updates["disk_limit_mb"] = *req.DiskLimitMB
		}
	}
	if req.OverLimitAction != nil {
		switch *req.OverLimitAction {
		case "none", "alert", "suspend":
			updates["over_limit_action"] = *req.OverLimitAction
		default:
			return nil, fmt.Errorf("invalid over_limit_action")
		}
	}

	// Detect CPU/mem change → decide apply vs remove.
	cpuChanged := req.CPUQuotaPercent != nil
	memChanged := req.MemoryMaxMB != nil
	newCPU := uintPtrVal(site.CPUQuotaPercent)
	newMem := u64PtrVal(site.MemoryMaxMB)
	if cpuChanged {
		newCPU = *req.CPUQuotaPercent
	}
	if memChanged {
		newMem = *req.MemoryMaxMB
	}

	if cpuChanged {
		if *req.CPUQuotaPercent == 0 {
			updates["cpu_quota_percent"] = nil
		} else {
			updates["cpu_quota_percent"] = *req.CPUQuotaPercent
		}
	}
	if memChanged {
		if *req.MemoryMaxMB == 0 {
			updates["memory_max_mb"] = nil
		} else {
			updates["memory_max_mb"] = *req.MemoryMaxMB
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(&models.Site{}).Where("id = ?", siteID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	// Apply cgroup isolation only when CPU/mem actually changed.
	if cpuChanged || memChanged {
		phpVersion := site.PHPVersion
		if phpVersion == "" {
			phpVersion = "8.2"
		}
		wantLimit := newCPU > 0 || newMem > 0

		action := "apply"
		if !wantLimit {
			action = "remove"
		}
		// Skip a no-op remove (not isolated and nothing to remove).
		if !(action == "remove" && !site.FPMIsolated) {
			task, err := s.CreateJSONTaskReturn(site.ServerID, siteID, "set_resource_limit",
				map[string]interface{}{
					"action":            action,
					"domain":            site.Domain,
					"php_version":       phpVersion,
					"cpu_quota_percent": newCPU,
					"memory_max_mb":     newMem,
				})
			if err != nil {
				return nil, fmt.Errorf("queue resource limit task: %w", err)
			}
			result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 120*time.Second)
			if err != nil {
				return nil, err
			}
			if result.Error != "" {
				return nil, fmt.Errorf("%s", result.Error)
			}
			s.db.Model(&models.Site{}).Where("id = ?", siteID).
				Update("fpm_isolated", wantLimit)
		}
	}

	return s.GetByID(siteID)
}

// ScanDiskNow queues a synchronous disk scan for this single site, updates the
// stored usage + state, and returns the refreshed site.
func (s *SiteService) ScanDiskNow(siteID uint64) (*models.Site, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	task, err := s.CreateJSONTaskReturn(site.ServerID, siteID, "scan_disk_usage",
		map[string]interface{}{
			"sites": []map[string]string{{"domain": site.Domain, "db_name": site.DBName}},
		})
	if err != nil {
		return nil, fmt.Errorf("queue scan task: %w", err)
	}
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 120*time.Second)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	var scan []models.DiskScanSiteResult
	if err := json.Unmarshal([]byte(result.Output), &scan); err != nil || len(scan) == 0 {
		return s.GetByID(siteID)
	}
	usedMB := uint64((scan[0].FilesBytes + scan[0].DBBytes) / (1024 * 1024))
	s.ApplyDiskUsage(siteID, usedMB, site.DiskLimitMB)
	return s.GetByID(siteID)
}

// ApplyDiskUsage persists used MB + recomputes disk_state. Returns the new
// state so callers (worker) can act on transitions. Shared by manual scan and
// the periodic worker.
func (s *SiteService) ApplyDiskUsage(siteID, usedMB uint64, limitMB *uint64) string {
	state := "ok"
	if limitMB != nil && *limitMB > 0 {
		if usedMB >= *limitMB {
			state = "over"
		} else if usedMB*100 >= *limitMB*80 {
			state = "warning"
		}
	}
	now := time.Now()
	s.db.Model(&models.Site{}).Where("id = ?", siteID).Updates(map[string]interface{}{
		"disk_used_mb":    usedMB,
		"disk_state":      state,
		"disk_checked_at": &now,
	})
	return state
}

func uintPtrVal(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}
func u64PtrVal(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
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

// longRunningTaskTypes are task types that legitimately run far longer than the
// default stale threshold. `updated_at` is stamped when the agent picks a task up
// and is not refreshed while it runs, so it measures dispatch age, not liveness —
// which means the reset below cannot tell "the agent crashed" from "still working".
//
// Re-dispatching these is not merely wasteful, it is destructive: a second
// create_backup runs a concurrent mysqldump/tar writing the same output path
// (corrupt archive), a second scan_disk_usage puts competing `du` sweeps on the
// same disk, and a second malware_scan stacks up behind the agent's 2-slot scan
// semaphore. Give them a long threshold so only a genuinely dead agent trips it.
var longRunningTaskTypes = []string{
	"create_backup", "restore_backup", "delete_backup_file",
	"malware_scan", "scan_disk_usage",
	"migrate_site", "rollback_migration", "cleanup_source_migration", "cleanup_target_migration",
	"transfer_discover", "transfer_import_site",
	"create_site", "import_db_sql", "import_external", "upload_source_zip",
	"apply_updates", "check_updates",
	"optimize_images", "revert_images", "delete_image_originals", "image_originals_stats",
	"staging_create", "staging_push", "staging_delete",
	"enable_multisite", "create_subsite", "delete_subsite", "sync_subsites",
	"install_service", "install_clamav", "install_php_version",
	"db_optimize", "harden_wordpress", "agent_update",
}

const (
	// staleTaskAge is how long a short task may sit in 'processing' before we
	// assume the agent died. Short tasks are agent round-trips of seconds; the
	// slowest blocking handlers wait 120s.
	staleTaskAge = 5 * time.Minute
	// staleLongTaskAge is the same for longRunningTaskTypes. A full ClamAV scan is
	// capped at 30 minutes agent-side and a multi-GB backup can take longer, so 2h
	// is comfortably past any real run while still recovering from a dead agent.
	staleLongTaskAge = 2 * time.Hour
)

func (s *SiteService) GetPendingTasks(serverID uint64) ([]models.TaskQueue, error) {
	// Requeue tasks abandoned by a crashed agent, using a threshold that depends on
	// how long the task type can legitimately run (see longRunningTaskTypes).
	now := time.Now()
	s.db.Model(&models.TaskQueue{}).
		Where("server_id = ? AND status = ? AND task_type NOT IN ? AND updated_at < ?",
			serverID, "processing", longRunningTaskTypes, now.Add(-staleTaskAge)).
		Update("status", "pending")

	s.db.Model(&models.TaskQueue{}).
		Where("server_id = ? AND status = ? AND task_type IN ? AND updated_at < ?",
			serverID, "processing", longRunningTaskTypes, now.Add(-staleLongTaskAge)).
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
