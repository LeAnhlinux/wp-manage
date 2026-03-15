package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type HardeningHandler struct {
	svc *services.HardeningService
}

func NewHardeningHandler(svc *services.HardeningService) *HardeningHandler {
	return &HardeningHandler{svc: svc}
}

func (h *HardeningHandler) Harden(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.HardenWordPressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.Harden(siteID, req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "WordPress hardening applied successfully",
	})
}

func (h *HardeningHandler) RevertHarden(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.HardenWordPressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.RevertHarden(siteID, req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "WordPress hardening reverted successfully",
	})
}
