package mobile

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// AuthHandler handles all /api/mobile/v1/auth/* endpoints.
type AuthHandler struct {
	svc *services.MobileAuthService
	db  *gorm.DB
}

func NewAuthHandler(svc *services.MobileAuthService, db *gorm.DB) *AuthHandler {
	return &AuthHandler{svc: svc, db: db}
}

// Login godoc
// POST /api/mobile/v1/auth/login
// Returns full token pair (no 2FA) or requires_2fa=true + temp_token.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.MobileLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "email and password are required"))
	}

	result, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(401).JSON(models.MobileErr("AUTH_FAILED", err.Error()))
	}

	if result.Requires2FA {
		return c.JSON(models.MobileOK(fiber.Map{
			"requires_2fa": true,
			"temp_token":   result.TempToken,
		}))
	}

	return c.JSON(models.MobileOK(result.TokenResp))
}

// Complete2FA godoc
// POST /api/mobile/v1/auth/2fa/complete
// Body: { temp_token, code }
func (h *AuthHandler) Complete2FA(c *fiber.Ctx) error {
	var req models.Mobile2FACompleteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.TempToken == "" || req.Code == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "temp_token and code are required"))
	}

	tokenResp, err := h.svc.Complete2FA(req.TempToken, req.Code)
	if err != nil {
		return c.Status(401).JSON(models.MobileErr("AUTH_FAILED", err.Error()))
	}

	return c.JSON(models.MobileOK(tokenResp))
}

// Refresh godoc
// POST /api/mobile/v1/auth/refresh
// Body: { refresh_token }  — no Authorization header required.
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req models.MobileRefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.RefreshToken == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "refresh_token is required"))
	}

	tokenResp, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		return c.Status(401).JSON(models.MobileErr("TOKEN_INVALID", err.Error()))
	}

	return c.JSON(models.MobileOK(tokenResp))
}

// RegisterDevice godoc
// POST /api/mobile/v1/auth/device
// Stores FCM/APNS token for push notifications.
func (h *AuthHandler) RegisterDevice(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	var req models.MobileDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.DeviceID == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "device_id is required"))
	}

	if err := h.svc.RegisterDevice(userID, req); err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	return c.JSON(models.MobileOK(fiber.Map{"device_id": req.DeviceID}))
}

// Logout godoc
// DELETE /api/mobile/v1/auth/session
// Body: { refresh_token }
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req models.MobileLogoutRequest
	_ = c.BodyParser(&req)
	if req.RefreshToken != "" {
		h.svc.Logout(req.RefreshToken)
	}
	return c.JSON(models.MobileOK(fiber.Map{"logged_out": true}))
}

// Me godoc
// GET /api/mobile/v1/auth/me
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	email, _ := c.Locals("email").(string)
	role, _ := c.Locals("role").(string)

	// Quick DB lookup for name + 2FA status (not in JWT claims)
	var user models.User
	h.db.Select("name, two_factor_enabled").First(&user, userID)

	return c.JSON(models.MobileOK(models.MobileMe{
		ID:               userID,
		Name:             user.Name,
		Email:            email,
		Role:             role,
		TwoFactorEnabled: user.TwoFactorEnabled,
	}))
}

// ListNotifications godoc
// GET /api/mobile/v1/notifications
func (h *AuthHandler) ListNotifications(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if perPage > 100 {
		perPage = 100
	}

	items, total, err := h.svc.ListNotifications(userID, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// MarkNotificationRead godoc
// POST /api/mobile/v1/notifications/:id/read
func (h *AuthHandler) MarkNotificationRead(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	notifID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid notification id"))
	}
	if err := h.svc.MarkRead(userID, uint64(notifID)); err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(fiber.Map{"read": true}))
}

// MarkAllNotificationsRead godoc
// POST /api/mobile/v1/notifications/read-all
func (h *AuthHandler) MarkAllNotificationsRead(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	if err := h.svc.MarkAllRead(userID); err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(fiber.Map{"read": true}))
}

// UnreadCount godoc
// GET /api/mobile/v1/notifications/unread-count
func (h *AuthHandler) UnreadCount(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	count := h.svc.UnreadCount(userID)
	return c.JSON(models.MobileOK(fiber.Map{"unread": count}))
}
