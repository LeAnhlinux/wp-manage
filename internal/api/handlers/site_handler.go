package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

type SiteHandler struct {
	svc       *services.SiteService
	serverSvc *services.ServerService
}

func NewSiteHandler(svc *services.SiteService, serverSvc *services.ServerService) *SiteHandler {
	return &SiteHandler{svc: svc, serverSvc: serverSvc}
}

func (h *SiteHandler) Create(c *fiber.Ctx) error {
	var req models.CreateSiteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	site, dbPass, err := h.svc.Create(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Site creation initiated",
		Data: fiber.Map{
			"site":         site,
			"db_password":  dbPass,
			"wp_admin_url": "https://" + site.Domain + "/wp-admin",
		},
	})
}

func (h *SiteHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	serverID, _ := strconv.ParseUint(c.Query("server_id", "0"), 10, 64)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	sites, total, err := h.svc.List(userID, role, serverID, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch sites"})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    sites,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *SiteHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: site})
}

func (h *SiteHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	// Create delete task for agent
	h.svc.CreateTask(site.ServerID, id, "delete_site", map[string]string{
		"domain":  site.Domain,
		"db_name": site.DBName,
		"db_user": site.DBUser,
	})
	if err := h.svc.Delete(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Site deleted"})
}

func (h *SiteHandler) UpdatePHP(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.UpdatePHPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	// Create PHP update task for agent
	h.svc.CreateTask(site.ServerID, id, "update_php", map[string]string{
		"domain":          site.Domain,
		"old_php_version": site.PHPVersion,
		"new_php_version": req.Version,
	})
	if err := h.svc.UpdatePHP(id, req.Version); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "PHP version updated"})
}

func (h *SiteHandler) Restart(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	if err := h.svc.CreateTask(site.ServerID, id, "restart_site", map[string]string{
		"domain":      site.Domain,
		"php_version": site.PHPVersion,
	}); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to create restart task"})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Restart command sent"})
}

func (h *SiteHandler) Suspend(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	h.svc.CreateTask(site.ServerID, id, "suspend_site", map[string]string{
		"domain": site.Domain,
	})
	if err := h.svc.UpdateStatus(id, "suspended"); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Site suspended"})
}

func (h *SiteHandler) Resume(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	h.svc.CreateTask(site.ServerID, id, "resume_site", map[string]string{
		"domain": site.Domain,
	})
	if err := h.svc.UpdateStatus(id, "active"); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Site resumed"})
}

func (h *SiteHandler) IssueSSL(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	if err := h.svc.CreateTask(site.ServerID, id, "issue_ssl", map[string]string{
		"domain": site.Domain,
		"email":  site.WPAdminEmail,
	}); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to create SSL task"})
	}
	h.svc.UpdateSSL(id, true)
	return c.JSON(models.APIResponse{Success: true, Message: "SSL certificate issuance initiated"})
}

func (h *SiteHandler) Credentials(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	creds, err := h.svc.GetCredentials(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: creds})
}

// UploadSource handles zip file upload for manual site deployments.
// POST /sites/:id/upload-source  (multipart/form-data, field "file")
func (h *SiteHandler) UploadSource(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "No file provided (field: file)"})
	}

	// Validate file extension
	ext := filepath.Ext(file.Filename)
	if ext != ".zip" {
		return c.Status(400).JSON(models.APIResponse{Error: "Only .zip files are allowed"})
	}
	if file.Size > 512*1024*1024 { // 512 MB limit
		return c.Status(400).JSON(models.APIResponse{Error: "File too large (max 512 MB)"})
	}

	// Store file to temp dir — use hex token (URL-safe, alphanumeric only)
	rawToken, _ := utils.GenerateAPIKey()
	token := rawToken[:32]
	tmpDir := "/tmp/wp-manage"
	os.MkdirAll(tmpDir, 0755)
	tmpPath := fmt.Sprintf("%s/%s.zip", tmpDir, token)
	if err := c.SaveFile(file, tmpPath); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to store file: " + err.Error()})
	}

	// Queue agent task
	taskData := map[string]interface{}{
		"domain":       site.Domain,
		"download_url": fmt.Sprintf("/api/v1/agent/files/tmp/%s", token),
		"file_token":   token,
	}
	if err := h.svc.CreateJSONTask(site.ServerID, id, "upload_source_zip", taskData); err != nil {
		os.Remove(tmpPath)
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to queue upload task"})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Source file uploaded and queued for deployment",
		Data:    fiber.Map{"domain": site.Domain, "file_size": file.Size},
	})
}

// ImportDB handles SQL file upload for manual database import.
// POST /sites/:id/import-db  (multipart/form-data, field "file")
func (h *SiteHandler) ImportDB(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "No file provided (field: file)"})
	}

	// Validate file extension
	ext := filepath.Ext(file.Filename)
	if ext != ".sql" && ext != ".gz" && ext != ".zip" {
		return c.Status(400).JSON(models.APIResponse{Error: "Only .sql, .gz, or .zip files are allowed"})
	}
	if file.Size > 512*1024*1024 { // 512 MB limit
		return c.Status(400).JSON(models.APIResponse{Error: "File too large (max 512 MB)"})
	}

	// Store file to temp dir — use hex token (URL-safe, alphanumeric only)
	rawToken, _ := utils.GenerateAPIKey()
	token := rawToken[:32]
	tmpDir := "/tmp/wp-manage"
	os.MkdirAll(tmpDir, 0755)
	tmpPath := fmt.Sprintf("%s/%s%s", tmpDir, token, ext)
	if err := c.SaveFile(file, tmpPath); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to store file: " + err.Error()})
	}

	// Queue agent task and wait synchronously (up to 300s for large files)
	taskData := map[string]interface{}{
		"domain":       site.Domain,
		"db_name":      site.DBName,
		"download_url": fmt.Sprintf("/api/v1/agent/files/tmp/%s", token),
		"file_token":   token,
		"file_ext":     ext,
	}
	task, err := h.svc.CreateJSONTaskReturn(site.ServerID, id, "import_db_sql", taskData)
	if err != nil {
		os.Remove(tmpPath)
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to queue import task"})
	}
	result, err := services.WaitForTaskCompleteWithTimeout(h.svc.DB(), task.ID, 300*1000000000)
	os.Remove(tmpPath)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Import timed out or failed: " + err.Error()})
	}
	if result.Error != "" {
		return c.Status(500).JSON(models.APIResponse{Error: result.Error})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Database imported successfully",
		Data:    fiber.Map{"domain": site.Domain, "db_name": site.DBName, "file_size": file.Size},
	})
}

// GetTempFile serves a temporary file for the agent to download.
// GET /agent/files/tmp/:token  (X-Agent-Key auth)
func (h *SiteHandler) GetTempFile(c *fiber.Ctx) error {
	token := c.Params("token")
	// Validate token — alphanumeric only
	validToken := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	if !validToken.MatchString(token) {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid token"})
	}

	tmpDir := "/tmp/wp-manage"
	// Try each supported extension
	for _, ext := range []string{".zip", ".sql", ".gz"} {
		path := fmt.Sprintf("%s/%s%s", tmpDir, token, ext)
		if _, err := os.Stat(path); err == nil {
			return c.Download(path, token+ext)
		}
	}
	return c.Status(404).JSON(models.APIResponse{Error: "File not found or already processed"})
}

func (h *SiteHandler) Autologin(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	if site.Status != "active" {
		return c.Status(400).JSON(models.APIResponse{Error: "Site is not active"})
	}

	secret := h.svc.GetAutologinSecret(id)
	if secret == "" {
		// Secret not yet installed, trigger install task
		h.svc.CreateTask(site.ServerID, id, "install_autologin", map[string]string{
			"domain": site.Domain,
		})
		return c.Status(202).JSON(models.APIResponse{
			Success: false,
			Error:   "Autologin is being configured. Please try again in 30 seconds.",
		})
	}

	// Generate HMAC-signed URL
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	sig := hex.EncodeToString(mac.Sum(nil))

	scheme := "https"
	if !site.SSLEnabled {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s/?wpm_autologin=1&wpm_ts=%s&wpm_sig=%s", scheme, site.Domain, ts, sig)

	return c.JSON(models.APIResponse{
		Success: true,
		Data:    fiber.Map{"url": url},
	})
}

// ToggleOpenBasedir flips open_basedir_enabled for a site and immediately
// rebuilds the PHP-FPM pool. Returns the new boolean value.
func (h *SiteHandler) ToggleOpenBasedir(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid site ID"})
	}

	newValue, err := h.svc.ToggleOpenBasedir(id)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	msg := "open_basedir enabled"
	if !newValue {
		msg = "open_basedir disabled"
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: msg,
		Data:    fiber.Map{"open_basedir_enabled": newValue},
	})
}

