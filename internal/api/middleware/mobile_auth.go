package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// MobileAuthRequired validates the short-lived JWT access token issued by the
// mobile auth flow. Sets user_id, email, role into fiber.Ctx locals.
func MobileAuthRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractBearerToken(c)
		if token == "" {
			return c.Status(401).JSON(models.MobileErr("UNAUTHORIZED", "Missing or invalid Authorization header"))
		}

		claims, err := utils.ParseJWT(token, cfg.JWT.Secret)
		if err != nil {
			return c.Status(401).JSON(models.MobileErr("TOKEN_INVALID", "Invalid or expired access token"))
		}

		// Reject temp tokens (used only for 2FA completion)
		if claims.Role == "2fa_pending" {
			return c.Status(401).JSON(models.MobileErr("TOKEN_INVALID", "Temp token cannot be used for API calls"))
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		return c.Next()
	}
}

// MobileManagerRequired is a convenience middleware: auth + manager/admin role.
func MobileManagerRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractBearerToken(c)
		if token == "" {
			return c.Status(401).JSON(models.MobileErr("UNAUTHORIZED", "Missing or invalid Authorization header"))
		}

		claims, err := utils.ParseJWT(token, cfg.JWT.Secret)
		if err != nil {
			return c.Status(401).JSON(models.MobileErr("TOKEN_INVALID", "Invalid or expired access token"))
		}

		if claims.Role != "admin" && claims.Role != "manager" {
			return c.Status(403).JSON(models.MobileErr("FORBIDDEN", "Insufficient permissions"))
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		return c.Next()
	}
}

// MobileAdminRequired allows only admin role.
func MobileAdminRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractBearerToken(c)
		if token == "" {
			return c.Status(401).JSON(models.MobileErr("UNAUTHORIZED", "Missing or invalid Authorization header"))
		}

		claims, err := utils.ParseJWT(token, cfg.JWT.Secret)
		if err != nil {
			return c.Status(401).JSON(models.MobileErr("TOKEN_INVALID", "Invalid or expired access token"))
		}

		if claims.Role != "admin" {
			return c.Status(403).JSON(models.MobileErr("FORBIDDEN", "Admin access required"))
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		return c.Next()
	}
}

func extractBearerToken(c *fiber.Ctx) string {
	auth := c.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}
