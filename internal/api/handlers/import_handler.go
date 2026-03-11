package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type ImportHandler struct {
	svc *services.ImportService
}

func NewImportHandler(svc *services.ImportService) *ImportHandler {
	return &ImportHandler{svc: svc}
}

func (h *ImportHandler) Start(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.ImportExternalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	imp, err := h.svc.StartImport(siteID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: imp})
}

func (h *ImportHandler) GetStatus(c *fiber.Ctx) error {
	importID, _ := strconv.ParseUint(c.Params("importId"), 10, 64)
	imp, err := h.svc.GetStatus(importID)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Import not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: imp})
}

func (h *ImportHandler) List(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	imports, err := h.svc.ListBySite(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: imports})
}

func (h *ImportHandler) Complete(c *fiber.Ctx) error {
	var result models.AgentImportResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	err := h.svc.UpdateProgress(result.ImportID, result.Progress, result.Status, result.Error)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Import status updated"})
}
