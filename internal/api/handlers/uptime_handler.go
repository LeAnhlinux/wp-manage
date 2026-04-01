package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type UptimeHandler struct {
	svc *services.UptimeService
}

func NewUptimeHandler(svc *services.UptimeService) *UptimeHandler {
	return &UptimeHandler{svc: svc}
}

func (h *UptimeHandler) List(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	checks, err := h.svc.List(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: checks})
}

func (h *UptimeHandler) Create(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.CreateUptimeCheckRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid request"})
	}
	if req.URL == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "url is required"})
	}

	check, err := h.svc.Create(siteID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: check})
}

func (h *UptimeHandler) Update(c *fiber.Ctx) error {
	checkID, _ := strconv.ParseUint(c.Params("checkId"), 10, 64)
	var req models.UpdateUptimeCheckRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid request"})
	}

	check, err := h.svc.Update(checkID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: check})
}

func (h *UptimeHandler) Delete(c *fiber.Ctx) error {
	checkID, _ := strconv.ParseUint(c.Params("checkId"), 10, 64)
	if err := h.svc.Delete(checkID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "uptime check deleted"})
}

func (h *UptimeHandler) GetHistory(c *fiber.Ctx) error {
	checkID, _ := strconv.ParseUint(c.Params("checkId"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	logs, err := h.svc.GetHistory(checkID, limit)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: logs})
}

// ListAll returns all uptime checks across all sites.
func (h *UptimeHandler) ListAll(c *fiber.Ctx) error {
	checks, err := h.svc.ListAll()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: checks})
}
