package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type SecurityService struct {
	db *gorm.DB
}

func NewSecurityService(db *gorm.DB) *SecurityService {
	return &SecurityService{db: db}
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

	scan := models.MalwareScan{
		SiteID:    siteID,
		ScanType:  scanType,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := s.db.Create(&scan).Error; err != nil {
		return nil, err
	}

	// Queue scan task for agent
	taskData := models.AgentMalwareScanTask{
		ScanID:   scan.ID,
		SiteID:   siteID,
		Domain:   site.Domain,
		ScanType: scanType,
		Path:     req.Path,
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
