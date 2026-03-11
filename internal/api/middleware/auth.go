package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

func AuthRequired(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Missing authorization header"})
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Invalid authorization format"})
		}

		claims, err := utils.ParseJWT(parts[1], cfg.JWT.Secret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Invalid or expired token"})
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		return c.Next()
	}
}

// APIKeyOrAuthRequired checks X-API-Key header first, falls back to JWT Bearer token
func APIKeyOrAuthRequired(cfg *config.Config, apiKeySvc *services.APIKeyService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1) Check X-API-Key header
		apiKey := c.Get("X-API-Key")
		if apiKey != "" && strings.HasPrefix(apiKey, "wpm_") {
			keyModel, err := apiKeySvc.ValidateKey(apiKey)
			if err != nil {
				return c.Status(401).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			c.Locals("user_id", keyModel.UserID)
			c.Locals("email", keyModel.User.Email)
			c.Locals("role", keyModel.User.Role)
			c.Locals("api_key_id", keyModel.ID)
			c.Locals("api_key_rate_limit", keyModel.RateLimit)
			return c.Next()
		}

		// 2) Fallback to JWT
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Missing authorization"})
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Invalid authorization format"})
		}

		claims, err := utils.ParseJWT(parts[1], cfg.JWT.Secret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Invalid or expired token"})
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		return c.Next()
	}
}

func RoleRequired(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole, _ := c.Locals("role").(string)
		for _, r := range roles {
			if userRole == r {
				return c.Next()
			}
		}
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "Insufficient permissions"})
	}
}

// PermissionRequired checks role-based permissions using a hardcoded matrix
func PermissionRequired(resource, action string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if !checkPermission(role, resource, action) {
			return c.Status(403).JSON(fiber.Map{"success": false, "error": "Insufficient permissions"})
		}
		return c.Next()
	}
}

// ServerAccessRequired checks that the current user has access to the server referenced in route params
func ServerAccessRequired(userSvc *services.UserService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role == "admin" {
			return c.Next()
		}

		userID, _ := c.Locals("user_id").(uint64)

		// Try :id first (server routes), then :serverId
		serverIDStr := c.Params("id")
		if serverIDStr == "" {
			serverIDStr = c.Params("serverId")
		}
		if serverIDStr == "" {
			return c.Next()
		}

		var serverID uint64
		for _, ch := range serverIDStr {
			if ch >= '0' && ch <= '9' {
				serverID = serverID*10 + uint64(ch-'0')
			}
		}
		if serverID == 0 {
			return c.Next()
		}

		if !userSvc.HasServerAccess(userID, role, serverID) {
			return c.Status(403).JSON(fiber.Map{"success": false, "error": "No access to this server"})
		}
		return c.Next()
	}
}

func AgentAuthRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-Agent-Key")
		if apiKey == "" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Missing agent API key"})
		}
		c.Locals("agent_api_key", apiKey)
		return c.Next()
	}
}

// checkPermission implements a hardcoded permission matrix for 3 roles
func checkPermission(role, resource, action string) bool {
	if role == "admin" {
		return true
	}
	switch resource {
	case "servers":
		switch action {
		case "read":
			return true // filtered by assigned servers in service layer
		case "create", "delete", "deploy":
			return false
		}
	case "sites":
		switch action {
		case "read":
			return true
		case "create", "delete", "manage":
			return role == "manager"
		}
	case "files":
		switch action {
		case "read":
			return role == "manager" || role == "viewer"
		case "write":
			return role == "manager"
		}
	case "backups":
		switch action {
		case "read":
			return role == "manager" || role == "viewer"
		case "create", "restore", "manage":
			return role == "manager"
		}
	case "teams":
		switch action {
		case "read":
			return true // filtered by membership in handler
		case "create":
			return role == "manager"
		case "manage", "delete":
			return false // admin only
		default:
			return false
		}
	case "infrastructure":
		switch action {
		case "read":
			return role == "manager" || role == "viewer"
		case "manage":
			return role == "manager"
		}
	case "users", "settings":
		return false
	}
	return false
}
