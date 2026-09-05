package handlers

import (
	"fmt"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type BackupHandler struct {
	svc *services.BackupService
}

func NewBackupHandler(svc *services.BackupService) *BackupHandler {
	return &BackupHandler{svc: svc}
}

func (h *BackupHandler) Create(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.CreateBackupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Type == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Backup type is required (full, database_only)"})
	}

	backup, err := h.svc.CreateBackup(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Backup initiated",
		Data:    backup,
	})
}

func (h *BackupHandler) ListBySite(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	pg := parsePage(c)

	backups, total, err := h.svc.List(siteID, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch backups"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    backups,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *BackupHandler) ListAll(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	pg := parsePage(c)

	backups, total, err := h.svc.ListAll(userID, role, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch backups"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    backups,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *BackupHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	backup, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Backup not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: backup})
}

// Download streams a local backup archive to the client, proxied from the agent.
func (h *BackupHandler) Download(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	resp, filename, err := h.svc.ProxyDownload(id)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	// Forward agent-side errors (404/403/500) verbatim.
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		c.Set("Content-Type", "application/json")
		return c.Status(resp.StatusCode).Send(body)
	}

	c.Set("Content-Type", "application/gzip")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(200)

	// Stream straight through. Do NOT io.Copy into Response.BodyWriter(): that
	// writer appends to an in-memory buffer, so a multi-GB backup is fully
	// allocated in RAM (with slice growth, ~2x peak) before a single byte is sent
	// — an OOM of the whole API process. SendStream hands the reader to fasthttp,
	// which streams it out.
	//
	// resp.Body is deliberately NOT closed here: fasthttp reads the stream after
	// this handler returns and closes it itself (it is an io.Closer).
	size := -1 // unknown length -> chunked
	if n, convErr := strconv.Atoi(resp.Header.Get("Content-Length")); convErr == nil && n >= 0 {
		size = n
	}
	return c.SendStream(resp.Body, size)
}

func (h *BackupHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.Delete(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Backup deleted"})
}

func (h *BackupHandler) Restore(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.RestoreRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.BackupID == 0 || req.ConfirmDomain == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "backup_id and confirm_domain are required"})
	}

	if err := h.svc.Restore(siteID, req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Restore initiated"})
}

// RestoreByID restores from a backup using its ID directly.
// Works for both normal and orphaned (site_id=NULL) backups.
// Optional server_id in body: used as fallback for old backups that pre-date server_id storage.
func (h *BackupHandler) RestoreByID(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req struct {
		ConfirmDomain string `json:"confirm_domain"`
		ServerID      uint64 `json:"server_id"` // optional fallback for legacy backups
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.ConfirmDomain == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "confirm_domain is required"})
	}

	if err := h.svc.RestoreByID(id, req.ConfirmDomain, req.ServerID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Restore initiated"})
}

// ListByServer returns all backups for every site on a server, including orphaned ones.
func (h *BackupHandler) ListByServer(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	pg := parsePage(c, 30)

	backups, total, err := h.svc.ListByServer(serverID, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch server backups"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    backups,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages,
		},
	})
}

// --- Backup Schedules ---

func (h *BackupHandler) CreateSchedule(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.CreateBackupScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Type == "" || req.CronExpr == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "type and cron_expr are required"})
	}

	schedule, err := h.svc.CreateSchedule(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Backup schedule created",
		Data:    schedule,
	})
}

func (h *BackupHandler) ListSchedules(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	schedules, err := h.svc.ListSchedules(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch schedules"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: schedules})
}

func (h *BackupHandler) UpdateSchedule(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.UpdateBackupScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	schedule, err := h.svc.UpdateSchedule(id, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Schedule updated", Data: schedule})
}

func (h *BackupHandler) DeleteSchedule(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.DeleteSchedule(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Schedule deleted"})
}
