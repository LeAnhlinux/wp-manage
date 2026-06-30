package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type SecurityService struct {
	db      *gorm.DB
	fileSvc *FileService
}

func NewSecurityService(db *gorm.DB, fileSvc *FileService) *SecurityService {
	return &SecurityService{db: db, fileSvc: fileSvc}
}

// --- WAF ---

// GetWAFConfig returns WAF config for a site (creates default if not exists)
func (s *SecurityService) GetWAFConfig(siteID uint64) (*models.WAFConfig, error) {
	var config models.WAFConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		config = models.WAFConfig{
			SiteID:        siteID,
			Enabled:       false,
			RuleSet:       "owasp-crs",
			ParanoidLevel: 1,
		}
		s.db.Create(&config)
		return &config, nil
	}
	return &config, err
}

// UpdateWAFConfig updates WAF settings and waits for agent confirmation (synchronous).
func (s *SecurityService) UpdateWAFConfig(siteID uint64, req models.UpdateWAFRequest) (*models.WAFConfig, error) {
	config, err := s.GetWAFConfig(siteID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
		config.Enabled = *req.Enabled
	}
	if req.ParanoidLevel != nil {
		updates["paranoid_level"] = *req.ParanoidLevel
		config.ParanoidLevel = *req.ParanoidLevel
	}
	if req.WhitelistedIPs != nil {
		updates["whitelisted_ips"] = *req.WhitelistedIPs
		config.WhitelistedIPs = *req.WhitelistedIPs
	}
	if req.CustomRules != nil {
		updates["custom_rules"] = *req.CustomRules
		config.CustomRules = *req.CustomRules
	}

	if len(updates) > 0 {
		s.db.Model(&models.WAFConfig{}).Where("id = ?", config.ID).Updates(updates)
	}

	// Get site for server_id + domain
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	// Queue task — synchronous: wait for agent to confirm
	taskData := models.AgentWAFTask{
		SiteID:         siteID,
		Domain:         site.Domain,
		Enabled:        config.Enabled,
		ParanoidLevel:  config.ParanoidLevel,
		WhitelistedIPs: config.WhitelistedIPs,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "configure_waf",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to queue WAF task: %w", err)
	}

	// Wait for agent to complete (max 60s)
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		// Roll back DB change so UI stays consistent with actual server state
		rollback := map[string]interface{}{}
		if req.Enabled != nil {
			old := !config.Enabled // flip back
			rollback["enabled"] = old
		}
		if len(rollback) > 0 {
			s.db.Model(&models.WAFConfig{}).Where("id = ?", config.ID).Updates(rollback)
		}
		return nil, fmt.Errorf("agent error: %s", result.Error)
	}

	return config, nil
}

// --- Malware Scan ---

// StartMalwareScan creates a scan record and queues task for agent
func (s *SecurityService) StartMalwareScan(siteID uint64, req models.StartMalwareScanRequest) (*models.MalwareScan, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site must be active to scan")
	}

	// Check no existing running scan
	var count int64
	s.db.Model(&models.MalwareScan{}).Where("site_id = ? AND status = ?", siteID, "running").Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("site already has a running scan")
	}

	scanType := req.ScanType
	if scanType == "" {
		scanType = "full"
	}
	engine := req.Engine
	if engine == "" {
		engine = "wp_signature"
	}

	scan := models.MalwareScan{
		SiteID:    siteID,
		ScanType:  scanType,
		Engine:    engine,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := s.db.Create(&scan).Error; err != nil {
		return nil, err
	}

	// Skip files the user marked as known false-positives.
	var wl []models.ThreatWhitelist
	s.db.Where("site_id = ?", siteID).Find(&wl)
	whitelist := make([]string, 0, len(wl))
	for _, w := range wl {
		whitelist = append(whitelist, w.FilePath)
	}

	// Queue scan task for agent
	taskData := models.AgentMalwareScanTask{
		ScanID:    scan.ID,
		SiteID:    siteID,
		Domain:    site.Domain,
		ScanType:  scanType,
		Engine:    engine,
		Path:      req.Path,
		Whitelist: whitelist,
	}
	dataJSON, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "malware_scan",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)

	scan.Site = &site
	return &scan, nil
}

// GetScan returns a scan by ID
func (s *SecurityService) GetScan(id uint64) (*models.MalwareScan, error) {
	var scan models.MalwareScan
	err := s.db.Preload("Site").First(&scan, id).Error
	return &scan, err
}

// ListScans returns scans for a site
func (s *SecurityService) ListScans(siteID uint64, page, perPage int) ([]models.MalwareScan, int64, error) {
	var scans []models.MalwareScan
	var total int64

	query := s.db.Model(&models.MalwareScan{}).Where("site_id = ?", siteID)
	query.Count(&total)

	err := query.Offset((page-1)*perPage).Limit(perPage).
		Order("started_at DESC").
		Find(&scans).Error
	return scans, total, err
}

// ListAllScans returns all scans with RBAC filtering
func (s *SecurityService) ListAllScans(userID uint64, role string, page, perPage int) ([]models.MalwareScan, int64, error) {
	var scans []models.MalwareScan
	var total int64

	query := s.db.Model(&models.MalwareScan{})
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

	err := query.Offset((page-1)*perPage).Limit(perPage).
		Preload("Site").
		Order("started_at DESC").
		Find(&scans).Error
	return scans, total, err
}

// UpdateScanResult updates scan from agent result
func (s *SecurityService) UpdateScanResult(result models.AgentMalwareScanResult) error {
	updates := map[string]interface{}{
		"files_scanned":  result.FilesScanned,
		"threats_found":  result.ThreatsFound,
		"threats_detail": result.ThreatsDetail,
		"status":         result.Status,
	}
	if result.Error != "" {
		updates["error"] = result.Error
	}
	if result.Status == "completed" || result.Status == "failed" {
		now := time.Now()
		updates["completed_at"] = &now
	}

	return s.db.Model(&models.MalwareScan{}).Where("id = ?", result.ScanID).Updates(updates).Error
}

// --- Scan Schedules ---

// ListSchedules returns scan schedules for a site
func (s *SecurityService) ListSchedules(siteID uint64) ([]models.MalwareScanSchedule, error) {
	var schedules []models.MalwareScanSchedule
	err := s.db.Where("site_id = ?", siteID).Preload("Site").Find(&schedules).Error
	return schedules, err
}

// GetScheduleBySite returns the schedule for a site, nil if none exists
func (s *SecurityService) GetScheduleBySite(siteID uint64) (*models.MalwareScanSchedule, error) {
	var schedule models.MalwareScanSchedule
	err := s.db.Where("site_id = ?", siteID).First(&schedule).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &schedule, err
}

// ListAllEnabledSchedules returns all enabled schedules (used by background worker)
func (s *SecurityService) ListAllEnabledSchedules() ([]models.MalwareScanSchedule, error) {
	var schedules []models.MalwareScanSchedule
	err := s.db.Where("enabled = ?", true).Find(&schedules).Error
	return schedules, err
}

// CreateSchedule creates a periodic scan schedule for a site (only 1 per site)
func (s *SecurityService) CreateSchedule(siteID uint64, req models.CreateScanScheduleRequest) (*models.MalwareScanSchedule, error) {
	var count int64
	s.db.Model(&models.MalwareScanSchedule{}).Where("site_id = ?", siteID).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("site already has a scan schedule — update or delete it first")
	}

	intervalH := req.IntervalH
	if intervalH <= 0 {
		intervalH = 24
	}
	scanType := req.ScanType
	if scanType == "" {
		scanType = "full"
	}
	engine := req.Engine
	if engine == "" {
		engine = "wp_signature"
	}

	nextRun := time.Now().Add(time.Duration(intervalH) * time.Hour)
	schedule := models.MalwareScanSchedule{
		SiteID:    siteID,
		ScanType:  scanType,
		Engine:    engine,
		IntervalH: intervalH,
		Enabled:   true,
		NextRunAt: &nextRun,
	}
	if err := s.db.Create(&schedule).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

// UpdateSchedule updates an existing scan schedule
func (s *SecurityService) UpdateSchedule(siteID, id uint64, req models.UpdateScanScheduleRequest) (*models.MalwareScanSchedule, error) {
	var schedule models.MalwareScanSchedule
	if err := s.db.Where("id = ? AND site_id = ?", id, siteID).First(&schedule).Error; err != nil {
		return nil, fmt.Errorf("schedule not found")
	}

	updates := map[string]interface{}{}
	if req.ScanType != "" {
		updates["scan_type"] = req.ScanType
	}
	if req.Engine != "" {
		updates["engine"] = req.Engine
	}
	if req.IntervalH > 0 {
		updates["interval_h"] = req.IntervalH
		nextRun := time.Now().Add(time.Duration(req.IntervalH) * time.Hour)
		updates["next_run_at"] = nextRun
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	s.db.Model(&schedule).Updates(updates)
	s.db.First(&schedule, id)
	return &schedule, nil
}

// DeleteSchedule removes a scan schedule
func (s *SecurityService) DeleteSchedule(siteID, id uint64) error {
	return s.db.Where("id = ? AND site_id = ?", id, siteID).Delete(&models.MalwareScanSchedule{}).Error
}

// MarkScheduleRan updates last_run_at and advances next_run_at
func (s *SecurityService) MarkScheduleRan(id uint64, intervalH int) error {
	now := time.Now()
	nextRun := now.Add(time.Duration(intervalH) * time.Hour)
	return s.db.Model(&models.MalwareScanSchedule{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_run_at": now,
		"next_run_at": nextRun,
	}).Error
}

// --- ClamAV Management ---

// InstallClamAV queues task for agent to install ClamAV, waits for result (synchronous, up to 10min)
func (s *SecurityService) InstallClamAV(serverID uint64) error {
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "install_clamav",
		Data:     "{}",
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue install_clamav task: %w", err)
	}
	// ClamAV install + freshclam can take several minutes
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}

// GetClamAVStatus asks agent for ClamAV installation status (synchronous)
func (s *SecurityService) GetClamAVStatus(serverID uint64) (*models.AgentClamAVStatusResult, error) {
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "clamav_status",
		Data:     "{}",
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to queue clamav_status task: %w", err)
	}
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("agent error: %s", result.Error)
	}

	var status models.AgentClamAVStatusResult
	if err := json.Unmarshal([]byte(result.Output), &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}
	return &status, nil
}

// --- Threat actions: whitelist (mark as safe) ---

// AddWhitelist marks a file as a known false-positive for a site.
// filePath is relative to /var/www/<domain>/public (the threat's file field).
func (s *SecurityService) AddWhitelist(siteID uint64, filePath, threatType, reason string) (*models.ThreatWhitelist, error) {
	filePath = strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}
	// Idempotent: if already whitelisted, return the existing row.
	var existing models.ThreatWhitelist
	if err := s.db.Where("site_id = ? AND file_path = ?", siteID, filePath).First(&existing).Error; err == nil {
		return &existing, nil
	}
	rec := models.ThreatWhitelist{
		SiteID:     siteID,
		FilePath:   filePath,
		ThreatType: threatType,
		Reason:     reason,
		CreatedAt:  time.Now(),
	}
	if err := s.db.Create(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListWhitelist returns all whitelisted files for a site.
func (s *SecurityService) ListWhitelist(siteID uint64) ([]models.ThreatWhitelist, error) {
	var list []models.ThreatWhitelist
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Find(&list).Error
	return list, err
}

// RemoveWhitelist removes a whitelist entry so the file is scanned again.
func (s *SecurityService) RemoveWhitelist(id uint64) error {
	return s.db.Delete(&models.ThreatWhitelist{}, id).Error
}

// --- Threat actions: quarantine ---

// QuarantineThreat asks the agent to move a flagged file into the site's
// .quarantine dir (disabled), then records it so it can be restored later.
func (s *SecurityService) QuarantineThreat(siteID uint64, filePath, threatType string) (*models.QuarantinedFile, error) {
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Server == nil {
		return nil, fmt.Errorf("server not found for site")
	}

	// Threat paths are relative to /public; the agent expects paths relative to /var/www/<domain>.
	agentPath := "/public/" + strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	body, _ := json.Marshal(map[string]string{"path": agentPath})

	resp, status, err := s.fileSvc.ProxyToAgent(&site, "POST", "/files/quarantine", map[string]string{"domain": site.Domain}, body)
	if err != nil {
		return nil, fmt.Errorf("failed to reach agent: %w", err)
	}
	var ar models.APIResponse
	json.Unmarshal(resp, &ar)
	if status != 200 || !ar.Success {
		if ar.Error != "" {
			return nil, fmt.Errorf("%s", ar.Error)
		}
		return nil, fmt.Errorf("agent returned status %d", status)
	}
	qPath, _ := ar.Data.(string)

	rec := models.QuarantinedFile{
		SiteID:         siteID,
		OriginalPath:   agentPath,
		QuarantinePath: qPath,
		ThreatType:     threatType,
		QuarantinedAt:  time.Now(),
	}
	if err := s.db.Create(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListQuarantine returns quarantined files for a site.
func (s *SecurityService) ListQuarantine(siteID uint64) ([]models.QuarantinedFile, error) {
	var list []models.QuarantinedFile
	err := s.db.Where("site_id = ?", siteID).Order("quarantined_at DESC").Find(&list).Error
	return list, err
}

// RestoreQuarantine moves a quarantined file back and removes the record.
func (s *SecurityService) RestoreQuarantine(quarantineID uint64) error {
	var rec models.QuarantinedFile
	if err := s.db.First(&rec, quarantineID).Error; err != nil {
		return fmt.Errorf("quarantine record not found")
	}
	var site models.Site
	if err := s.db.Preload("Server").First(&site, rec.SiteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}
	if site.Server == nil {
		return fmt.Errorf("server not found for site")
	}

	body, _ := json.Marshal(map[string]string{
		"quarantine_path": rec.QuarantinePath,
		"original_path":   rec.OriginalPath,
	})
	resp, status, err := s.fileSvc.ProxyToAgent(&site, "POST", "/files/restore-quarantine", map[string]string{"domain": site.Domain}, body)
	if err != nil {
		return fmt.Errorf("failed to reach agent: %w", err)
	}
	var ar models.APIResponse
	json.Unmarshal(resp, &ar)
	if status != 200 || !ar.Success {
		if ar.Error != "" {
			return fmt.Errorf("%s", ar.Error)
		}
		return fmt.Errorf("agent returned status %d", status)
	}
	return s.db.Delete(&rec).Error
}
