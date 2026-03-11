package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type StorageHandler struct {
	service *services.StorageService
}

func NewStorageHandler(service *services.StorageService) *StorageHandler {
	return &StorageHandler{service: service}
}

func (h *StorageHandler) List(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	creds, err := h.service.List(serverID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": creds})
}

func (h *StorageHandler) Create(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateStorageCredentialRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	cred, err := h.service.Create(serverID, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"success": true, "data": cred})
}

func (h *StorageHandler) Delete(c *fiber.Ctx) error {
	credID, _ := strconv.ParseUint(c.Params("credId"), 10, 64)
	if err := h.service.Delete(credID); err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Storage credential deleted"})
}
