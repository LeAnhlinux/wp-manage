package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type Fail2BanHandler struct {
	service *services.Fail2BanService
}

func NewFail2BanHandler(service *services.Fail2BanService) *Fail2BanHandler {
	return &Fail2BanHandler{service: service}
}

func (h *Fail2BanHandler) GetStatus(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	status, err := h.service.GetStatus(serverID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": status})
}

func (h *Fail2BanHandler) UnbanIP(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.Fail2BanUnbanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	if err := h.service.UnbanIP(serverID, req.Jail, req.IP); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "IP unbanned successfully"})
}

func (h *Fail2BanHandler) SetupDefaultJails(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.service.SetupDefaultJails(serverID); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Default jails configured (nginx + wordpress). Fail2Ban reloaded."})
}

func (h *Fail2BanHandler) ToggleService(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	if err := h.service.ToggleService(serverID, body.Enable); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	action := "stopped"
	if body.Enable {
		action = "started"
	}
	return c.JSON(fiber.Map{"success": true, "message": "Fail2Ban service " + action})
}

func (h *Fail2BanHandler) UpdateJail(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.Fail2BanUpdateJailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	if err := h.service.UpdateJail(serverID, req); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Jail updated successfully"})
}
