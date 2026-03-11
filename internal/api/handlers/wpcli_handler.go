package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type WPCLIHandler struct {
	svc     *services.WPCLIService
	siteSvc *services.SiteService
}

func NewWPCLIHandler(svc *services.WPCLIService, siteSvc *services.SiteService) *WPCLIHandler {
	return &WPCLIHandler{svc: svc, siteSvc: siteSvc}
}

func (h *WPCLIHandler) Execute(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.WPCLIRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	history, err := h.svc.Execute(siteID, req.Command)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	// Wait for agent to complete the task and return actual result
	result, err := h.svc.WaitForResult(history.ID)
	if err != nil {
		return c.Status(200).JSON(models.APIResponse{Success: true, Data: history, Message: "Task queued but result pending"})
	}

	return c.JSON(models.APIResponse{Success: true, Data: result})
}

func (h *WPCLIHandler) GetHistory(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	history, err := h.svc.GetHistory(siteID, limit)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: history})
}

func (h *WPCLIHandler) Complete(c *fiber.Ctx) error {
	var result models.AgentWPCLIResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	err := h.svc.UpdateResult(result.HistoryID, result.Output, result.ExitCode, result.Status)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "WP-CLI result recorded"})
}
