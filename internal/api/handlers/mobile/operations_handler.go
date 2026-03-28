package mobile

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// OperationsHandler handles backup, scan, migration, WP-CLI and file manager endpoints.
type OperationsHandler struct {
	db        *gorm.DB
	backupSvc *services.BackupService
	scanSvc   *services.SecurityService
	migSvc    *services.MigrationService
	siteSvc   *services.SiteService
	fileSvc   *services.FileService
}

func NewOperationsHandler(
	db *gorm.DB,
	backupSvc *services.BackupService,
	scanSvc *services.SecurityService,
	migSvc *services.MigrationService,
	siteSvc *services.SiteService,
	fileSvc *services.FileService,
) *OperationsHandler {
	return &OperationsHandler{
		db: db, backupSvc: backupSvc, scanSvc: scanSvc,
		migSvc: migSvc, siteSvc: siteSvc, fileSvc: fileSvc,
	}
}

// ─── Backups ──────────────────────────────────────────────────────────────────

// ListBackupsAll godoc
// GET /api/mobile/v1/backups?page=1&per_page=20&type=full
func (h *OperationsHandler) ListBackupsAll(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if perPage > 100 {
		perPage = 100
	}

	backups, total, err := h.backupSvc.ListAll(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	items := toBackupItems(backups)
	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// ListSiteBackups godoc
// GET /api/mobile/v1/sites/:siteId/backups?page=1&per_page=20
func (h *OperationsHandler) ListSiteBackups(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)

	backups, total, err := h.backupSvc.List(uint64(siteID), page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOKPage(toBackupItems(backups), page, perPage, total))
}

// GetBackup godoc
// GET /api/mobile/v1/backups/:id
func (h *OperationsHandler) GetBackup(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid backup id"))
	}
	b, err := h.backupSvc.GetByID(uint64(id))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Backup not found"))
	}
	return c.JSON(models.MobileOK(toBackupItem(*b)))
}

// CreateBackup godoc
// POST /api/mobile/v1/sites/:siteId/backups
// Body: { type: "full"|"database_only", storage: "local" }
func (h *OperationsHandler) CreateBackup(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}

	var req models.CreateBackupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.Type == "" {
		req.Type = "full"
	}

	backup, err := h.backupSvc.CreateBackup(uint64(siteID), req)
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", err.Error()))
	}
	return c.Status(202).JSON(models.MobileOK(toBackupItem(*backup)))
}

// ─── Malware Scans ────────────────────────────────────────────────────────────

// ListScansAll godoc
// GET /api/mobile/v1/scans?page=1&per_page=20
func (h *OperationsHandler) ListScansAll(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)

	scans, total, err := h.scanSvc.ListAllScans(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOKPage(toScanItems(scans), page, perPage, total))
}

// ListSiteScans godoc
// GET /api/mobile/v1/sites/:siteId/scans?page=1&per_page=20
func (h *OperationsHandler) ListSiteScans(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)

	scans, total, err := h.scanSvc.ListScans(uint64(siteID), page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOKPage(toScanItems(scans), page, perPage, total))
}

// GetScan godoc
// GET /api/mobile/v1/scans/:id
// Returns scan detail with parsed threats array (not raw JSON string).
func (h *OperationsHandler) GetScan(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid scan id"))
	}
	scan, err := h.scanSvc.GetScan(uint64(id))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Scan not found"))
	}

	detail := models.MobileScanDetail{
		MobileScanItem: toScanItem(*scan),
	}
	// Parse threats JSON string into typed array
	if scan.ThreatsDetail != "" {
		json.Unmarshal([]byte(scan.ThreatsDetail), &detail.Threats)
	}
	if detail.Threats == nil {
		detail.Threats = []models.MobileThreat{}
	}

	return c.JSON(models.MobileOK(detail))
}

// StartScan godoc
// POST /api/mobile/v1/sites/:siteId/scans
// Body: { scan_type: "full"|"quick", engine: "wp_signature"|"clamav"|"both" }
func (h *OperationsHandler) StartScan(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}

	var req models.StartMalwareScanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.ScanType == "" {
		req.ScanType = "full"
	}
	if req.Engine == "" {
		req.Engine = "wp_signature"
	}

	scan, err := h.scanSvc.StartMalwareScan(uint64(siteID), req)
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", err.Error()))
	}
	return c.Status(202).JSON(models.MobileOK(toScanItem(*scan)))
}

// ─── Migrations ───────────────────────────────────────────────────────────────

// ListMigrations godoc
// GET /api/mobile/v1/migrations?page=1&per_page=20
func (h *OperationsHandler) ListMigrations(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)

	migrations, total, err := h.migSvc.List(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	items := make([]models.MobileMigrationItem, 0, len(migrations))
	for _, m := range migrations {
		items = append(items, toMigrationItem(m))
	}
	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// GetMigration godoc
// GET /api/mobile/v1/migrations/:id
func (h *OperationsHandler) GetMigration(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid migration id"))
	}
	m, err := h.migSvc.GetByID(uint64(id))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Migration not found"))
	}
	return c.JSON(models.MobileOK(toMigrationItem(*m)))
}

// GetMigrationLogs godoc
// GET /api/mobile/v1/migrations/:id/logs
func (h *OperationsHandler) GetMigrationLogs(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid migration id"))
	}
	logs, err := h.migSvc.GetLogs(uint64(id))
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(logs))
}

// CancelMigration godoc
// POST /api/mobile/v1/migrations/:id/cancel
func (h *OperationsHandler) CancelMigration(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid migration id"))
	}
	if err := h.migSvc.Cancel(uint64(id)); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", err.Error()))
	}
	return c.JSON(models.MobileOK(fiber.Map{"cancelled": true}))
}

// ─── WP-CLI ───────────────────────────────────────────────────────────────────

// ExecuteWPCLI godoc
// POST /api/mobile/v1/sites/:siteId/wpcli
// Body: { command: "plugin list" }
// Returns immediately with history_id; client polls GET /:historyId for result.
func (h *OperationsHandler) ExecuteWPCLI(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}

	var req models.MobileWPCLIRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.Command == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "command is required"))
	}

	// Security guard: reject dangerous patterns
	forbidden := []string{"shell_exec", "eval(", "system(", "passthru(", "--url=http"}
	lower := strings.ToLower(req.Command)
	for _, f := range forbidden {
		if strings.Contains(lower, f) {
			return c.Status(400).JSON(models.MobileErr("FORBIDDEN_COMMAND", "Command contains forbidden patterns"))
		}
	}

	site, err := h.siteSvc.GetByID(uint64(siteID))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Site not found"))
	}

	task, err := h.siteSvc.CreateJSONTaskReturn(site.ServerID, site.ID, "wpcli", map[string]interface{}{
		"command": "wp " + req.Command,
		"domain":  site.Domain,
	})
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	// Create history entry (running)
	history := models.WPCLIHistory{
		SiteID:  site.ID,
		Command: req.Command,
		Status:  "running",
	}
	h.db.Create(&history)

	return c.Status(202).JSON(models.MobileOK(fiber.Map{
		"history_id": history.ID,
		"task_id":    task.ID,
		"status":     "running",
	}))
}

// GetWPCLIResult godoc
// GET /api/mobile/v1/sites/:siteId/wpcli/:historyId
func (h *OperationsHandler) GetWPCLIResult(c *fiber.Ctx) error {
	historyID, err := c.ParamsInt("historyId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid history id"))
	}

	var history models.WPCLIHistory
	if err := h.db.First(&history, historyID).Error; err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "History entry not found"))
	}
	return c.JSON(models.MobileOK(history))
}

// WPCLIHistory godoc
// GET /api/mobile/v1/sites/:siteId/wpcli/history?page=1&per_page=20
func (h *OperationsHandler) WPCLIHistory(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)

	var items []models.WPCLIHistory
	var total int64
	h.db.Model(&models.WPCLIHistory{}).Where("site_id = ?", siteID).Count(&total)
	h.db.Where("site_id = ?", siteID).Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).Find(&items)

	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// ─── File Manager (read-only) ─────────────────────────────────────────────────

// FileList godoc
// GET /api/mobile/v1/sites/:siteId/files?path=/
func (h *OperationsHandler) FileList(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}
	path := c.Query("path", "/")

	site, err := h.fileSvc.GetSiteWithServer(uint64(siteID))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Site not found"))
	}

	params := services.BuildFileQueryParams(site.Domain, path)
	resp, status, err := h.fileSvc.ProxyToAgent(site, "GET", "/files/list", params, nil)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	if status >= 400 {
		return c.Status(status).JSON(models.MobileErr("AGENT_ERROR", string(resp)))
	}

	var data interface{}
	json.Unmarshal(resp, &data)
	return c.JSON(models.MobileOK(data))
}

// FileContent godoc
// GET /api/mobile/v1/sites/:siteId/files/content?path=/wp-config.php
// Only serves text files up to 512KB.
func (h *OperationsHandler) FileContent(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}
	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "path is required"))
	}

	site, err := h.fileSvc.GetSiteWithServer(uint64(siteID))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Site not found"))
	}

	params := services.BuildFileQueryParams(site.Domain, path)
	resp, status, err := h.fileSvc.ProxyToAgent(site, "GET", "/files/read", params, nil)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	if status >= 400 {
		return c.Status(status).JSON(models.MobileErr("AGENT_ERROR", string(resp)))
	}

	// Size guard: 512KB max for mobile
	if len(resp) > 512*1024 {
		return c.Status(413).JSON(models.MobileErr("FILE_TOO_LARGE", "File exceeds 512KB mobile limit"))
	}

	var data interface{}
	json.Unmarshal(resp, &data)
	return c.JSON(models.MobileOK(data))
}

// FileSearch godoc
// POST /api/mobile/v1/sites/:siteId/files/search
// Body: { pattern: "*.php", query: "eval(" }
func (h *OperationsHandler) FileSearch(c *fiber.Ctx) error {
	siteID, err := c.ParamsInt("siteId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid site id"))
	}

	var body map[string]string
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}

	site, err := h.fileSvc.GetSiteWithServer(uint64(siteID))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Site not found"))
	}

	params := services.BuildFileQueryParams(site.Domain, body["path"])
	params["pattern"] = body["pattern"]
	params["query"] = body["query"]

	resp, status, err := h.fileSvc.ProxyToAgent(site, "GET", "/files/search", params, nil)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	if status >= 400 {
		return c.Status(status).JSON(models.MobileErr("AGENT_ERROR", string(resp)))
	}

	var data interface{}
	json.Unmarshal(resp, &data)
	return c.JSON(models.MobileOK(data))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func toBackupItem(b models.Backup) models.MobileBackupItem {
	return models.MobileBackupItem{
		ID: b.ID, SiteID: b.SiteID, Domain: b.Domain,
		Type: b.Type, Storage: b.Storage, Status: b.Status,
		SizeBytes: b.SizeBytes, Error: b.Error,
		CreatedAt: b.CreatedAt, CompletedAt: b.CompletedAt,
	}
}

func toBackupItems(backups []models.Backup) []models.MobileBackupItem {
	items := make([]models.MobileBackupItem, 0, len(backups))
	for _, b := range backups {
		items = append(items, toBackupItem(b))
	}
	return items
}

func toScanItem(s models.MalwareScan) models.MobileScanItem {
	domain := ""
	if s.Site != nil {
		domain = s.Site.Domain
	}
	return models.MobileScanItem{
		ID: s.ID, SiteID: s.SiteID, Domain: domain,
		ScanType: s.ScanType, Engine: s.Engine, Status: s.Status,
		FilesScanned: s.FilesScanned, ThreatsFound: s.ThreatsFound,
		StartedAt: s.StartedAt, CompletedAt: s.CompletedAt,
	}
}

func toScanItems(scans []models.MalwareScan) []models.MobileScanItem {
	items := make([]models.MobileScanItem, 0, len(scans))
	for _, s := range scans {
		items = append(items, toScanItem(s))
	}
	return items
}

func toMigrationItem(m models.Migration) models.MobileMigrationItem {
	domain := ""
	source := ""
	target := ""
	if m.Site != nil {
		domain = m.Site.Domain
	}
	// SourceServer and TargetServer are not preloaded by default; use IDs
	item := models.MobileMigrationItem{
		ID: m.ID, SiteID: m.SiteID, Domain: domain,
		Mode: m.Mode, Phase: m.Phase, Status: m.Status, Progress: m.Progress,
		SourceServer: source, TargetServer: target,
		StartedAt: m.StartedAt, CompletedAt: m.CompletedAt,
	}
	_ = source
	_ = target
	return item
}
