package handlers

import (
	"strconv"

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

// GetStatus returns current panel version and whether an update is available.
// GET /settings/panel-update
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

// CreateRelease registers a new panel release (download URL + checksum + notes).
// POST /settings/panel-update/releases
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

// ListReleases returns all registered panel releases.
// GET /settings/panel-update/releases
func (h *PanelUpdateHandler) ListReleases(c *fiber.Ctx) error {
	releases, err := h.service.ListReleases()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": releases})
}

// TriggerUpdate stages the release binary and starts wp-manage-updater.
// The API will restart — client receives 202 and must poll until API is back.
// POST /settings/panel-update/releases/:id/apply
func (h *PanelUpdateHandler) TriggerUpdate(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "invalid release ID"})
	}

	// TriggerUpdate returns as soon as the updater service is started.
	// The API process will be stopped by the updater shortly after.
	if err := h.service.TriggerUpdate(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	// 202 Accepted: update is in progress, panel is restarting.
	// Frontend must poll GET /settings/panel-update or GET /health until API responds.
	return c.Status(202).JSON(fiber.Map{
		"success": true,
		"message": "Update started. Panel is restarting — please wait up to 30s.",
	})
}

// GetLastUpdateResult returns the result of the most recent panel self-update.
// Written by wp-manage-updater to ResultFilePath after applying the update.
// GET /settings/panel-update/last-result
func (h *PanelUpdateHandler) GetLastUpdateResult(c *fiber.Ctx) error {
	result, err := h.service.GetLastUpdateResult()
	if err != nil {
		// File not present = no previous update recorded
		return c.JSON(fiber.Map{"success": true, "data": nil})
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}
