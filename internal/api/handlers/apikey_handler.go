package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type APIKeyHandler struct {
	svc *services.APIKeyService
}

func NewAPIKeyHandler(svc *services.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

func (h *APIKeyHandler) Create(c *fiber.Ctx) error {
	var req models.CreateAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Name is required"})
	}

	userID := c.Locals("user_id").(uint64)
	apiKey, plainKey, err := h.svc.Create(userID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "API key created. Save this key — it won't be shown again.",
		Data: fiber.Map{
			"api_key": apiKey,
			"key":     plainKey,
		},
	})
}

func (h *APIKeyHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	keys, err := h.svc.List(userID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch API keys"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: keys})
}

func (h *APIKeyHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	userID := c.Locals("user_id").(uint64)

	if err := h.svc.Delete(id, userID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "API key deleted"})
}
