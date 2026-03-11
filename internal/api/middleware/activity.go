package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
)

// ActivityLogger provides helper methods to log activities from handlers
type ActivityLogger struct {
	svc *services.ActivityService
}

func NewActivityLogger(svc *services.ActivityService) *ActivityLogger {
	return &ActivityLogger{svc: svc}
}

// LogFromRequest extracts user info from fiber context and logs the activity
func (a *ActivityLogger) LogFromRequest(c *fiber.Ctx, action, details string, serverID *uint64, siteID *uint64) {
	var userID *uint64
	if uid, ok := c.Locals("user_id").(uint64); ok {
		userID = &uid
	}
	ipAddress := c.IP()
	// Fire and forget — don't block the request
	go a.svc.Log(userID, serverID, siteID, action, details, ipAddress)
}

// ActivityMiddleware logs every mutating request (POST/PUT/DELETE) automatically
func ActivityMiddleware(activitySvc *services.ActivityService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only log mutating requests
		method := c.Method()
		if method != "POST" && method != "PUT" && method != "DELETE" {
			return c.Next()
		}

		// Execute the handler first
		err := c.Next()

		// Only log successful requests (2xx status)
		status := c.Response().StatusCode()
		if status >= 200 && status < 300 {
			var userID *uint64
			if uid, ok := c.Locals("user_id").(uint64); ok {
				userID = &uid
			}
			action := method + " " + c.Path()
			go activitySvc.Log(userID, nil, nil, action, "", c.IP())
		}

		return err
	}
}
