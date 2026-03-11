package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type CacheHandler struct {
	svc *services.CacheService
}

func NewCacheHandler(svc *services.CacheService) *CacheHandler {
	return &CacheHandler{svc: svc}
}

func (h *CacheHandler) GetConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	cfg, err := h.svc.GetConfig(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: cfg})
}

func (h *CacheHandler) UpdateConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.UpdateCacheConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	cfg, err := h.svc.UpdateConfig(siteID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: cfg})
}

func (h *CacheHandler) Toggle(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	result, err := h.svc.ToggleCacheSync(siteID, req.Enabled)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	action := "disabled"
	if req.Enabled {
		action = "enabled"
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cache " + action, Data: result})
}

func (h *CacheHandler) Purge(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	result, err := h.svc.PurgeCacheSync(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cache purged", Data: result})
}
