package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type RedisHandler struct {
	svc *services.RedisService
}

func NewRedisHandler(svc *services.RedisService) *RedisHandler {
	return &RedisHandler{svc: svc}
}

func (h *RedisHandler) GetConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	config, err := h.svc.GetConfig(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get Redis config"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: config})
}

func (h *RedisHandler) UpdateConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.RedisConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.UpdateConfig(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Redis config updated", Data: config})
}

func (h *RedisHandler) Toggle(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.Toggle(siteID, body.Enabled)
	if err != nil {
		// Still return updated config with error message
		return c.Status(500).JSON(models.APIResponse{
						Error:   err.Error(),
			Data:    config,
		})
	}

	msg := "Redis disabled"
	if body.Enabled {
		msg = "Redis enabled"
	}
	return c.JSON(models.APIResponse{Success: true, Message: msg, Data: config})
}

func (h *RedisHandler) GetStats(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	stats, err := h.svc.GetStats(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get Redis stats: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: map[string]string{"raw": stats}})
}

func (h *RedisHandler) Flush(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	if err := h.svc.Flush(siteID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to flush Redis: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Redis cache flushed"})
}

// ─── Server-level Redis handlers ─────────────────────────────────────────────

func (h *RedisHandler) GetServerStats(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	stats, err := h.svc.GetServerStats(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get Redis stats: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: map[string]string{"raw": stats}})
}

func (h *RedisHandler) FlushServer(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.FlushServer(serverID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to flush Redis: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Redis cache flushed"})
}

func (h *RedisHandler) GetServerSiteStatuses(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	statuses, err := h.svc.GetServerSiteStatuses(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list Redis site statuses: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: statuses})
}
