package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type PanelUpdateHandler struct {
	service *services.PanelUpdateService
}

func NewPanelUpdateHandler(service *services.PanelUpdateService) *PanelUpdateHandler {
	return &PanelUpdateHandler{service: service}
}

func (h *PanelUpdateHandler) GetStatus(c *fiber.Ctx) error {
	currentVersion := h.service.GetCurrentVersion()
	latest, err := h.service.GetLatestRelease()

	result := fiber.Map{
		"current_version":  currentVersion,
		"update_available": false,
	}

	if err == nil && latest != nil && latest.Version != currentVersion {
		result["update_available"] = true
		result["latest"] = latest
	}

	return c.JSON(fiber.Map{"success": true, "data": result})
}

func (h *PanelUpdateHandler) CreateRelease(c *fiber.Ctx) error {
	var req models.CreatePanelReleaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	release, err := h.service.CreateRelease(req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"success": true, "data": release})
}

func (h *PanelUpdateHandler) ListReleases(c *fiber.Ctx) error {
	releases, err := h.service.ListReleases()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": releases})
}
