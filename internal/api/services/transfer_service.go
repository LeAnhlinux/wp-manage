package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type TransferService struct {
	db *gorm.DB
}

func NewTransferService(db *gorm.DB) *TransferService {
	return &TransferService{db: db}
}

// TestConnection dials the source server's SSH from the API server and returns latency + hostname.
// This gives the user instant feedback before creating a TransferJob (which queues an agent task).
func (s *TransferService) TestConnection(req models.TransferTestConnectionRequest) models.TransferTestConnectionResult {
	port := req.Port
	if port == 0 {
		port = 22
	}
	user := req.User
	if user == "" {
		user = "root"
	}

	// Build auth method
	var authMethod ssh.AuthMethod
	if req.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(req.SSHKey))
		if err != nil {
			return models.TransferTestConnectionResult{
				Success: false,
				Error:   "SSH key không hợp lệ: " + err.Error(),
			}
		}
		authMethod = ssh.PublicKeys(signer)
	} else if req.Password != "" {
		authMethod = ssh.Password(req.Password)
	} else {
		return models.TransferTestConnectionResult{
			Success: false,
			Error:   "Cần cung cấp SSH key hoặc password",
		}
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", req.Host, port)
	start := time.Now()
	client, err := ssh.Dial("tcp", addr, cfg)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return models.TransferTestConnectionResult{
			Success:   false,
			LatencyMs: latencyMs,
			Error:     err.Error(),
		}
	}
	defer client.Close()

	// Run `hostname` to give the user a sanity-check on which server they're connecting to
	hostname := ""
	if sess, err := client.NewSession(); err == nil {
		defer sess.Close()
		if out, err := sess.Output("hostname"); err == nil {
			hostname = strings.TrimSpace(string(out))
		}
	}

	return models.TransferTestConnectionResult{
		Success:   true,
		LatencyMs: latencyMs,
		Hostname:  hostname,
	}
}

// Connect creates a TransferJob and queues a transfer_discover task on the target server's agent.
// SSH credentials are stored ONLY in the TaskQueue.Data (pending row) — never persisted in TransferJob.
func (s *TransferService) Connect(userID uint64, req models.TransferConnectRequest) (*models.TransferJob, error) {
	// Validate target server
	var server models.Server
	if err := s.db.First(&server, req.TargetServerID).Error; err != nil {
		return nil, fmt.Errorf("target server not found")
	}
	if server.Status != "active" {
		return nil, fmt.Errorf("target server must be active")
	}

	// Validate auth method
	if req.SSHKey == "" && req.Password == "" {
		return nil, fmt.Errorf("provide either ssh_key or password for source authentication")
	}

	port := req.Port
	if port == 0 {
		port = 22
	}
	user := req.User
	if user == "" {
		user = "root"
	}

	job := models.TransferJob{
		UserID:            userID,
		TargetServerID:    req.TargetServerID,
		PanelType:         req.PanelType,
		SourceHost:        req.Host,
		SourcePort:        port,
		SourceUser:        user,
		Status:            "discovering",
		// Initialize JSON columns with valid JSON to satisfy MariaDB JSON_VALID() CHECK.
		// (MariaDB json column type rejects empty string ""; "null" is valid JSON.)
		DiscoveredSites:   "null",
		TargetPHPVersions: "null",
		SelectedSites:     "null",
	}
	if err := s.db.Create(&job).Error; err != nil {
		return nil, fmt.Errorf("failed to create transfer job: %w", err)
	}

	// Build discover task payload (SSH creds only live in TaskQueue row, scrubbed after agent fetch)
	task := models.AgentTransferDiscoverTask{
		JobID:       job.ID,
		PanelType:   req.PanelType,
		SourceHost:  req.Host,
		SourcePort:  port,
		SourceUser:  user,
		SSHKey:      req.SSHKey,
		SSHPassword: req.Password,
	}
	taskJSON, _ := json.Marshal(task)
	taskRow := models.TaskQueue{
		ServerID: req.TargetServerID,
		TaskType: "transfer_discover",
		Data:     string(taskJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&taskRow).Error; err != nil {
		// Rollback job creation
		s.db.Delete(&job)
		return nil, fmt.Errorf("failed to queue discover task: %w", err)
	}

	return &job, nil
}

// GetJob returns a TransferJob with its site results (no SSH creds exposed).
func (s *TransferService) GetJob(id, userID uint64, role string) (*models.TransferJob, error) {
	var job models.TransferJob
	q := s.db.Preload("TargetServer").Preload("Results")
	if role != "admin" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&job, id).Error; err != nil {
		return nil, fmt.Errorf("transfer job not found")
	}
	return &job, nil
}

// List returns all transfer jobs for the user (admin sees all).
func (s *TransferService) List(userID uint64, role string, page, perPage int) ([]models.TransferJob, int64, error) {
	var jobs []models.TransferJob
	var total int64
	q := s.db.Model(&models.TransferJob{}).Preload("TargetServer")
	if role != "admin" {
		q = q.Where("user_id = ?", userID)
	}
	q.Count(&total)
	err := q.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&jobs).Error
	return jobs, total, err
}

// StartTransfer validates site selections and queues per-site import tasks.
func (s *TransferService) StartTransfer(id, userID uint64, role string, req models.TransferStartRequest) (*models.TransferJob, error) {
	job, err := s.GetJob(id, userID, role)
	if err != nil {
		return nil, err
	}
	if job.Status != "awaiting_selection" {
		return nil, fmt.Errorf("job must be in 'awaiting_selection' state to start transfer (current: %s)", job.Status)
	}
	if len(req.Sites) == 0 {
		return nil, fmt.Errorf("select at least one site to transfer")
	}

	// Parse discovered sites and SSH creds from the original task row
	var discoverTask models.AgentTransferDiscoverTask
	var taskRow models.TaskQueue
	if err := s.db.Where("server_id = ? AND task_type = ? AND status IN ('done','completed')", job.TargetServerID, "transfer_discover").
		Order("id DESC").First(&taskRow).Error; err != nil {
		return nil, fmt.Errorf("discover task not found — run discovery first")
	}
	// Note: creds are scrubbed from done tasks; we look for a separate credential store or re-use job data.
	// For this flow the FE must pass no creds — they were used in discover. For import we use the same creds
	// stored in a separate "transfer_creds" column that we add inline here for safety.
	// Actually: agent needs SSH creds again for import. We stored them in the job — wait, we did NOT persist
	// them in job. The design decision is to ask the user to re-confirm or we persist an encrypted version.
	// For simplicity, we store them encrypted in the discover task output. The agent returns a session token.
	// Simpler approach: The FE passes the SSH creds on /start if needed.
	// For now we rely on the creds being cached in the browser session on FE side and re-submitted.
	// This is handled by TransferStartRequest.Sites — the actual creds come from the job's TaskQueue row.
	// Re-query discover task (the one that's done but we need to re-read its Data for creds):
	_ = discoverTask
	_ = taskRow

	// Validate domain conflicts (layers A+B+C — quick DB checks)
	for _, sel := range req.Sites {
		if conflict := s.checkDomainConflict(sel.TargetDomain, job.TargetServerID, job.ID); !conflict.Available {
			return nil, fmt.Errorf("domain %s: %s", sel.TargetDomain, conflict.ConflictDetail)
		}
		for _, alias := range sel.IncludeAliases {
			if conflict := s.checkDomainConflict(alias, job.TargetServerID, job.ID); !conflict.Available {
				return nil, fmt.Errorf("alias domain %s: %s", alias, conflict.ConflictDetail)
			}
		}
	}

	// Parse discovered sites JSON to build import tasks
	var discovered []models.DiscoveredSiteInfo
	if job.DiscoveredSites != "" {
		if err := json.Unmarshal([]byte(job.DiscoveredSites), &discovered); err != nil {
			return nil, fmt.Errorf("invalid discovery data")
		}
	}
	discoveredMap := make(map[string]models.DiscoveredSiteInfo)
	for _, d := range discovered {
		discoveredMap[d.PrimaryDomain] = d
	}

	// Parse SSH creds — we need to get them from the original pending task
	// Strategy: store a masked copy in a separate DB field or re-queue using a stored token.
	// For now, creds must be included in the start request (security: user re-confirms intent).
	// This is reflected in TransferStartRequest but we handle it in the handler.

	// Create result rows + import tasks
	now := time.Now()
	totalSites := len(req.Sites)
	for _, sel := range req.Sites {
		php := sel.PHPOverride
		if php == "" {
			if d, ok := discoveredMap[sel.SourceDomain]; ok {
				php = d.PHPVersion
			}
		}
		if php == "" {
			php = "8.2" // safe default
		}
		// Remove any finished result row from a previous transfer of the same
		// domain→server pair. The unique index (uidx_transfer_domain_server) on
		// (target_domain, target_server_id) would block a new INSERT otherwise.
		// Only remove if NOT currently running/pending — those indicate an active
		// transfer that we must not clobber.
		var existing models.TransferSiteResult
		if err := s.db.Where(
			"target_domain = ? AND target_server_id = ?",
			sel.TargetDomain, job.TargetServerID,
		).First(&existing).Error; err == nil {
			if existing.Status == "running" || existing.Status == "pending" {
				return nil, fmt.Errorf("domain %s is already being transferred to this server", sel.TargetDomain)
			}
			// Done / failed / rolled_back → safe to delete and retry
			s.db.Delete(&existing)
		}

		result := models.TransferSiteResult{
			JobID:          job.ID,
			SourceDomain:   sel.SourceDomain,
			TargetDomain:   sel.TargetDomain,
			TargetServerID: job.TargetServerID,
			Status:         "pending",
			SSLMode:        sel.SSLMode,
			PHPVersion:     php,
		}
		if err := s.db.Create(&result).Error; err != nil {
			return nil, fmt.Errorf("failed to create result row for %s: %w", sel.SourceDomain, err)
		}
	}

	// Update job
	selectedJSON, _ := json.Marshal(req.Sites)
	s.db.Model(&job).Updates(map[string]interface{}{
		"status":         "transferring",
		"selected_sites": string(selectedJSON),
		"total_sites":    totalSites,
		"done_sites":     0,
		"started_at":     &now,
	})

	// Actual import tasks will be queued by the handler (it has access to SSH creds from the request)
	job.Status = "transferring"
	return job, nil
}

// SaveDiscoveryResult is called by the agent callback after discovery completes.
func (s *TransferService) SaveDiscoveryResult(result models.AgentTransferDiscoverResult) error {
	var job models.TransferJob
	if err := s.db.First(&job, result.JobID).Error; err != nil {
		return fmt.Errorf("transfer job not found")
	}

	sitesJSON, _ := json.Marshal(result.Sites)
	phpJSON, _ := json.Marshal(result.TargetPHPVersions)

	updates := map[string]interface{}{
		"discovered_sites":    string(sitesJSON),
		"target_php_versions": string(phpJSON),
		"total_sites":         len(result.Sites),
	}

	if result.Status == "failed" {
		updates["status"] = "failed"
		updates["error"] = result.Error
	} else {
		updates["status"] = "awaiting_selection"
	}

	return s.db.Model(&job).Updates(updates).Error
}

// UpdateSiteProgress is called by agent callbacks during import.
func (s *TransferService) UpdateSiteProgress(update models.AgentTransferUpdate) error {
	updates := map[string]interface{}{
		"phase":    update.Phase,
		"progress": update.Progress,
		"status":   update.Status,
	}
	if update.Error != "" {
		updates["error"] = update.Error
	}
	if update.LogMessage != "" {
		// Append to log excerpt (keep last 2000 chars)
		var result models.TransferSiteResult
		s.db.First(&result, update.ResultID)
		log := result.LogExcerpt + "\n" + update.LogMessage
		if len(log) > 2000 {
			log = log[len(log)-2000:]
		}
		updates["log_excerpt"] = log
	}
	// If agent sent site registration info in the final "done" callback,
	// create the sites DB record now so the site appears in the panel.
	if update.RegisterSite != nil && update.SiteID == 0 {
		reg := update.RegisterSite
		siteRecord := models.Site{
			ServerID:    reg.ServerID,
			Domain:      reg.Domain,
			PHPVersion:  reg.PHPVersion,
			DBName:      reg.DBName,
			DBUser:      reg.DBUser,
			DBPassword:  reg.DBPassword,
			Status:      "active",
			InstallType: "transfer",
		}
		if err := s.db.Create(&siteRecord).Error; err == nil {
			update.SiteID = siteRecord.ID
			// Also create the primary domain record
			s.db.Create(&models.Domain{
				SiteID: siteRecord.ID,
				Domain: reg.Domain,
				Type:   "primary",
			})
		}
	}

	if update.SiteID > 0 {
		updates["site_id"] = update.SiteID
	}

	var result models.TransferSiteResult
	if err := s.db.First(&result, update.ResultID).Error; err != nil {
		return fmt.Errorf("result not found")
	}

	if update.Status == "done" || update.Status == "failed" || update.Status == "rolled_back" {
		now := time.Now()
		updates["completed_at"] = &now
	}
	if result.StartedAt == nil && update.Status == "running" {
		now := time.Now()
		updates["started_at"] = &now
	}

	if err := s.db.Model(&result).Updates(updates).Error; err != nil {
		return err
	}

	// Update job counters
	if update.Status == "done" || update.Status == "failed" || update.Status == "rolled_back" {
		s.db.Model(&models.TransferJob{}).Where("id = ?", update.JobID).
			Update("done_sites", gorm.Expr("done_sites + 1"))
		// Check if all sites finished → update job status
		var job models.TransferJob
		s.db.First(&job, update.JobID)
		if job.DoneSites >= job.TotalSites {
			// Determine final status
			var failCount int64
			s.db.Model(&models.TransferSiteResult{}).
				Where("job_id = ? AND status IN ('failed','rolled_back')", update.JobID).
				Count(&failCount)
			finalStatus := "completed"
			if failCount == int64(job.TotalSites) {
				finalStatus = "failed"
			}
			now := time.Now()
			s.db.Model(&job).Updates(map[string]interface{}{
				"status":       finalStatus,
				"completed_at": &now,
			})
		}
	}

	return nil
}

// CancelJob cancels a pending/discovering/awaiting/transferring job.
func (s *TransferService) CancelJob(id, userID uint64, role string) error {
	job, err := s.GetJob(id, userID, role)
	if err != nil {
		return err
	}
	if job.Status == "completed" || job.Status == "cancelled" || job.Status == "failed" {
		return fmt.Errorf("job already in terminal state: %s", job.Status)
	}
	return s.db.Model(job).Update("status", "cancelled").Error
}

// DeleteJob permanently removes a transfer job and all its site results.
// Only jobs that are NOT actively transferring can be deleted.
func (s *TransferService) DeleteJob(id, userID uint64, role string) error {
	job, err := s.GetJob(id, userID, role)
	if err != nil {
		return err
	}
	if job.Status == "discovering" || job.Status == "transferring" {
		return fmt.Errorf("không thể xóa job đang chạy (status: %s)", job.Status)
	}
	// Delete child results first, then the job
	if err := s.db.Where("job_id = ?", id).Delete(&models.TransferSiteResult{}).Error; err != nil {
		return fmt.Errorf("failed to delete site results: %w", err)
	}
	return s.db.Delete(&models.TransferJob{}, id).Error
}

// CheckDomainAvailability exposes domain conflict check for the handler.
func (s *TransferService) CheckDomainAvailability(domain string, targetServerID, jobID uint64) models.ConflictCheckResult {
	return s.checkDomainConflict(domain, targetServerID, jobID)
}

// checkDomainConflict runs layers A+B+C (DB-only, fast).
// Layer D (orphaned nginx vhost) requires agent involvement and is done at pre-flight.
func (s *TransferService) checkDomainConflict(domain string, targetServerID, excludeJobID uint64) models.ConflictCheckResult {
	// Layer A: primary site domain
	var site models.Site
	if err := s.db.Where("domain = ? AND server_id = ?", domain, targetServerID).First(&site).Error; err == nil {
		return models.ConflictCheckResult{
			Available:      false,
			ConflictType:   "existing_site",
			ConflictDetail: fmt.Sprintf("Domain đang được dùng bởi site #%d (%s)", site.ID, site.Domain),
			ConflictSiteID: &site.ID,
		}
	}

	// Layer B: alias domain
	var domainRow models.Domain
	if err := s.db.Joins("JOIN sites ON sites.id = domains.site_id").
		Where("domains.domain = ? AND sites.server_id = ?", domain, targetServerID).
		First(&domainRow).Error; err == nil {
		return models.ConflictCheckResult{
			Available:      false,
			ConflictType:   "existing_alias",
			ConflictDetail: fmt.Sprintf("Domain đang là alias của site #%d", domainRow.SiteID),
			ConflictSiteID: &domainRow.SiteID,
		}
	}

	// Layer C: pending/running transfer result (other jobs)
	var pending models.TransferSiteResult
	q := s.db.Where("target_domain = ? AND target_server_id = ? AND status IN ('pending','running')", domain, targetServerID)
	if excludeJobID > 0 {
		q = q.Where("job_id != ?", excludeJobID)
	}
	if err := q.First(&pending).Error; err == nil {
		return models.ConflictCheckResult{
			Available:      false,
			ConflictType:   "pending_transfer",
			ConflictDetail: fmt.Sprintf("Domain đang được transfer bởi job #%d", pending.JobID),
		}
	}

	return models.ConflictCheckResult{Available: true}
}

// QueueImportTasks queues individual transfer_import_site tasks after user starts the transfer.
// Called from handler; if sshKey/sshPassword are empty, creds are recovered from the original
// transfer_discover task row (which stores them until the import tasks are queued).
func (s *TransferService) QueueImportTasks(job *models.TransferJob, selections []models.SiteImportOption,
	discovered []models.DiscoveredSiteInfo, sshKey, sshPassword string) error {

	// If no creds supplied by caller, recover from the completed discover task
	if sshKey == "" && sshPassword == "" {
		var taskRow models.TaskQueue
		if err := s.db.Where("server_id = ? AND task_type = ? AND status IN ('completed','done')", job.TargetServerID, "transfer_discover").
			Order("id DESC").First(&taskRow).Error; err == nil {
			var discoverTask models.AgentTransferDiscoverTask
			if err := json.Unmarshal([]byte(taskRow.Data), &discoverTask); err == nil {
				sshKey = discoverTask.SSHKey
				sshPassword = discoverTask.SSHPassword
			}
		}
	}

	discoveredMap := make(map[string]models.DiscoveredSiteInfo)
	for _, d := range discovered {
		discoveredMap[d.PrimaryDomain] = d
	}

	// Load result rows (created in StartTransfer)
	var results []models.TransferSiteResult
	s.db.Where("job_id = ?", job.ID).Find(&results)
	resultMap := make(map[string]models.TransferSiteResult)
	for _, r := range results {
		resultMap[r.SourceDomain] = r
	}

	for _, sel := range selections {
		site, ok := discoveredMap[sel.SourceDomain]
		if !ok {
			continue
		}
		result, ok := resultMap[sel.SourceDomain]
		if !ok {
			continue
		}

		php := sel.PHPOverride
		if php == "" {
			php = site.PHPVersion
		}
		if php == "" {
			php = "8.2"
		}

		task := models.AgentTransferImportTask{
			JobID:          job.ID,
			ResultID:       result.ID,
			TargetServerID: job.TargetServerID,
			SourceHost:     job.SourceHost,
			SourcePort:     job.SourcePort,
			SourceUser:     job.SourceUser,
			SSHKey:         sshKey,
			SSHPassword:    sshPassword,
			Site:           site,
			TargetDomain:   sel.TargetDomain,
			PHPVersion:     php,
			SSLMode:        sel.SSLMode,
			IncludeAliases: sel.IncludeAliases,
		}
		taskJSON, _ := json.Marshal(task)
		taskRow := models.TaskQueue{
			ServerID: job.TargetServerID,
			TaskType: "transfer_import_site",
			Data:     string(taskJSON),
			Status:   "pending",
		}
		if err := s.db.Create(&taskRow).Error; err != nil {
			return fmt.Errorf("failed to queue import task for %s: %w", sel.SourceDomain, err)
		}
	}
	return nil
}
