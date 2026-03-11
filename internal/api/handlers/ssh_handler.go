package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type SSHHandler struct {
	svc *services.SSHService
}

func NewSSHHandler(svc *services.SSHService) *SSHHandler {
	return &SSHHandler{svc: svc}
}

func (h *SSHHandler) List(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}

	keys, err := h.svc.List(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: keys})
}

func (h *SSHHandler) Create(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}

	var req models.CreateSSHKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if req.Name == "" || req.PublicKey == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Name and public_key are required"})
	}

	key, err := h.svc.Create(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "SSH key added successfully",
		Data:    key,
	})
}

func (h *SSHHandler) Delete(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}

	keyID, err := strconv.ParseUint(c.Params("keyId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid key ID"})
	}

	if err := h.svc.Delete(serverID, keyID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "SSH key removed successfully",
	})
}
