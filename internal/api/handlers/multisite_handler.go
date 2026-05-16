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
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.EnableMultisiteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Type == "" {
		req.Type = "subdomain"
	}
	if req.Type != "subdomain" && req.Type != "subfolder" {
		return c.Status(400).JSON(models.APIResponse{Error: "Type must be 'subdomain' or 'subfolder'"})
	}

	if err := h.svc.EnableMultisite(siteID, req.Type); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
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
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.EnableMultisiteComplete(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// ListSubsites returns all subsites for a multisite network
func (h *MultisiteHandler) ListSubsites(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	subsites, err := h.svc.GetSubsites(siteID)
	if err != nil {
		// Site exists but is not a multisite network — return empty array, not an error
		if err.Error() == "site is not a multisite network" {
			return c.JSON(models.APIResponse{Success: true, Data: []models.Subsite{}})
		}
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: subsites})
}

// CreateSubsite creates a new subsite
func (h *MultisiteHandler) CreateSubsite(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.CreateSubsiteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Slug == "" || req.Title == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Slug and title are required"})
	}

	if err := h.svc.CreateSubsite(siteID, req.Slug, req.Title); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
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
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.CreateSubsiteComplete(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// DeleteSubsite deletes a subsite
func (h *MultisiteHandler) DeleteSubsite(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	subsiteID, err := strconv.ParseUint(c.Params("subsiteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid subsite ID"})
	}

	if err := h.svc.DeleteSubsite(siteID, subsiteID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
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
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	if err := h.svc.SyncSubsites(siteID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Sync subsites task queued",
	})
}

// DeleteSubsiteComplete handles agent callback after subsite is deleted
// Bug 1 fix: confirms deletion or rolls back to "active" on failure
func (h *MultisiteHandler) DeleteSubsiteComplete(c *fiber.Ctx) error {
	var result models.AgentDeleteSubsiteResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.DeleteSubsiteComplete(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// SyncSubsitesComplete handles agent callback with subsite list
func (h *MultisiteHandler) SyncSubsitesComplete(c *fiber.Ctx) error {
	var result models.AgentListSubsitesResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.SyncSubsitesComplete(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}
