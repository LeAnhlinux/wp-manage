package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type MigrationHandler struct {
	svc *services.MigrationService
}

func NewMigrationHandler(svc *services.MigrationService) *MigrationHandler {
	return &MigrationHandler{svc: svc}
}

func (h *MigrationHandler) Start(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.MigrateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.TargetServerID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "target_server_id is required"})
	}

	migration, err := h.svc.StartMigration(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Migration started",
		Data:    migration,
	})
}

func (h *MigrationHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	pg := parsePage(c)

	migrations, total, err := h.svc.List(userID, role, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch migrations"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    migrations,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *MigrationHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	migration, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Migration not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: migration})
}

func (h *MigrationHandler) Rollback(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.Rollback(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Rollback initiated"})
}

func (h *MigrationHandler) Cancel(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.Cancel(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Migration cancelled"})
}

func (h *MigrationHandler) Logs(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	logs, err := h.svc.GetLogs(id)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch logs"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: logs})
}

// MigrationUpdate is called by the agent to report migration progress
func (h *MigrationHandler) Update(c *fiber.Ctx) error {
	var update models.AgentMigrationUpdate
	if err := c.BodyParser(&update); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	if err := h.svc.UpdateMigration(update); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true})
}

// SetupSSH is called by the source agent to request temporary SSH access to the target server.
// It queues an add_authorized_key task on the target agent and waits for completion (synchronous).
func (h *MigrationHandler) SetupSSH(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if err := c.BodyParser(&req); err != nil || req.PublicKey == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "public_key is required"})
	}
	if err := h.svc.SetupMigrationSSH(id, req.PublicKey); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "SSH access granted on target server"})
}

// CleanupSSH is called by the source agent after migration to remove the temp SSH key from target.
func (h *MigrationHandler) CleanupSSH(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	h.svc.CleanupMigrationSSH(id) // best-effort, ignore errors
	return c.JSON(models.APIResponse{Success: true})
}

func (h *MigrationHandler) CleanupSource(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.CleanupSource(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Đã gửi lệnh dọn dẹp server nguồn"})
}
