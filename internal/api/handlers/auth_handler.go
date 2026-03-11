package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type AuthHandler struct {
	svc *services.AuthService
}

func NewAuthHandler(svc *services.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	user, err := h.svc.Register(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Registration successful",
		Data:    user,
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	token, user, requires2FA, err := h.svc.Login(req)
	if err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: err.Error()})
	}

	if requires2FA {
		return c.JSON(models.APIResponse{
			Success: true,
			Data: fiber.Map{
				"requires_2fa": true,
				"email":        user.Email,
			},
		})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data: fiber.Map{
			"token": token,
			"user":  user,
		},
	})
}

// Validate2FA validates a 2FA code and issues a JWT token
func (h *AuthHandler) Validate2FA(c *fiber.Ctx) error {
	var req models.Validate2FARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	token, user, err := h.svc.Validate2FA(req)
	if err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data: fiber.Map{
			"token": token,
			"user":  user,
		},
	})
}

// Enable2FA starts the 2FA setup process
func (h *AuthHandler) Enable2FA(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)

	setup, err := h.svc.Enable2FA(userID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data:    setup,
	})
}

// Verify2FA verifies the 2FA code and activates 2FA
func (h *AuthHandler) Verify2FA(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	var req models.Verify2FARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.Verify2FA(userID, req.Code); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "2FA enabled successfully",
	})
}

// Disable2FA turns off 2FA
func (h *AuthHandler) Disable2FA(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)

	if err := h.svc.Disable2FA(userID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "2FA disabled successfully",
	})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	user, err := h.svc.GetUserByID(userID)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "User not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: user})
}
