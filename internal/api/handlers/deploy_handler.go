package handlers

import (
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	ws "github.com/gofiber-team/wp-manage/internal/api/websocket"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type DeployHandler struct {
	svc *services.DeployService
	hub *ws.Hub
}

func NewDeployHandler(svc *services.DeployService, hub *ws.Hub) *DeployHandler {
	return &DeployHandler{svc: svc, hub: hub}
}

// AutoDeploy handles POST /api/v1/servers/deploy
// Deploys agent to VPS via SSH. Credentials are used once and never stored.
func (h *DeployHandler) AutoDeploy(c *fiber.Ctx) error {
	var req models.AutoDeployRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if req.SSHUser == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "SSH user must not be empty"})
	}
	if req.AuthMethod == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Auth method must be 'password' or 'key'"})
	}
	if req.AuthMethod != "password" && req.AuthMethod != "key" {
		return c.Status(400).JSON(models.APIResponse{Error: "Auth method must be 'password' or 'key'"})
	}
	if req.AuthMethod == "password" && req.Password == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Password is required for password auth"})
	}
	if req.AuthMethod == "key" && req.PrivateKey == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Private key is required for key auth"})
	}
	if req.ServerID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}

	// Default SSH port
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	// WebSocket channel for realtime progress
	wsChannel := fmt.Sprintf("deploy:%d", req.ServerID)

	// Run deploy in background goroutine
	go func() {
		// Clear sensitive data from request after use
		defer func() {
			req.Password = ""
			req.PrivateKey = ""
		}()

		err := h.svc.Deploy(req, func(progress models.DeployProgress) {
			h.hub.Broadcast(wsChannel, "deploy_progress", progress)
		})
		if err != nil {
			slog.Error("Auto deploy failed", "server_id", req.ServerID, "error", err)
			h.hub.Broadcast(wsChannel, "deploy_progress", models.DeployProgress{
				Step:    0,
				Total:   7,
				Message: "Deploy failed",
				Status:  "error",
				Error:   err.Error(),
			})
		}
	}()

	return c.Status(202).JSON(models.APIResponse{
		Success: true,
		Message: "Deploy started. Track progress via WebSocket.",
		Data: fiber.Map{
			"ws_channel": wsChannel,
		},
	})
}
