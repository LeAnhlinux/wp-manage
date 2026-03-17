package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type IonCubeHandler struct {
	svc *services.IonCubeService
}

func NewIonCubeHandler(svc *services.IonCubeService) *IonCubeHandler {
	return &IonCubeHandler{svc: svc}
}

// GetStatus godoc — GET /api/v1/servers/:id/ioncube
func (h *IonCubeHandler) GetStatus(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIResponse{Error: "Invalid server ID"})
	}

	statuses, err := h.svc.GetStatus(serverID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.APIResponse{
			Error: "Failed to get ionCube status: " + err.Error(),
		})
	}
	return c.JSON(models.APIResponse{Success: true, Data: statuses})
}

// Enable godoc — POST /api/v1/servers/:id/ioncube/:phpVersion/enable
func (h *IonCubeHandler) Enable(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	phpVersion := c.Params("phpVersion")
	if phpVersion == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIResponse{Error: "PHP version is required"})
	}

	if err := h.svc.Enable(serverID, phpVersion); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: "ionCube enabled for PHP " + phpVersion,
	})
}

// Disable godoc — POST /api/v1/servers/:id/ioncube/:phpVersion/disable
func (h *IonCubeHandler) Disable(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	phpVersion := c.Params("phpVersion")
	if phpVersion == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIResponse{Error: "PHP version is required"})
	}

	if err := h.svc.Disable(serverID, phpVersion); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: "ionCube disabled for PHP " + phpVersion,
	})
}
