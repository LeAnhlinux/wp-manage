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
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	migrations, total, err := h.svc.List(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch migrations"})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    migrations,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
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
