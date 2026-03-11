package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type MultisiteHandler struct {
	svc *services.MultisiteService
}

func NewMultisiteHandler(svc *services.MultisiteService) *MultisiteHandler {
	return &MultisiteHandler{svc: svc}
}

// Enable enables WordPress Multisite on a site
func (h *MultisiteHandler) Enable(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	var req models.EnableMultisiteRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Type == "" {
		req.Type = "subdomain"
	}
	if req.Type != "subdomain" && req.Type != "subfolder" {
		return fiber.NewError(fiber.StatusBadRequest, "Type must be 'subdomain' or 'subfolder'")
	}

	if err := h.svc.EnableMultisite(siteID, req.Type); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Multisite enable task queued",
	})
}

// EnableComplete handles agent callback after multisite is enabled
func (h *MultisiteHandler) EnableComplete(c *fiber.Ctx) error {
	var result models.AgentEnableMultisiteResult
	if err := c.BodyParser(&result); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.svc.EnableMultisiteComplete(result); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// ListSubsites returns all subsites for a multisite network
func (h *MultisiteHandler) ListSubsites(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	subsites, err := h.svc.GetSubsites(siteID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: subsites})
}

// CreateSubsite creates a new subsite
func (h *MultisiteHandler) CreateSubsite(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	var req models.CreateSubsiteRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Slug == "" || req.Title == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Slug and title are required")
	}

	if err := h.svc.CreateSubsite(siteID, req.Slug, req.Title); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Create subsite task queued",
	})
}

// CreateSubsiteComplete handles agent callback
func (h *MultisiteHandler) CreateSubsiteComplete(c *fiber.Ctx) error {
	var result models.AgentCreateSubsiteResult
	if err := c.BodyParser(&result); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.svc.CreateSubsiteComplete(result); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// DeleteSubsite deletes a subsite
func (h *MultisiteHandler) DeleteSubsite(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}
	subsiteID, err := strconv.ParseUint(c.Params("subsiteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid subsite ID")
	}

	if err := h.svc.DeleteSubsite(siteID, subsiteID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Delete subsite task queued",
	})
}

// SyncSubsites creates a task to sync subsites from WP-CLI
func (h *MultisiteHandler) SyncSubsites(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	if err := h.svc.SyncSubsites(siteID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Sync subsites task queued",
	})
}

// SyncSubsitesComplete handles agent callback with subsite list
func (h *MultisiteHandler) SyncSubsitesComplete(c *fiber.Ctx) error {
	var result models.AgentListSubsitesResult
	if err := c.BodyParser(&result); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.svc.SyncSubsitesComplete(result); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}
