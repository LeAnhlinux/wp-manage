package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type AlertHandler struct {
	svc     *services.AlertService
	siteSvc *services.SiteService
}

func NewAlertHandler(svc *services.AlertService, siteSvc *services.SiteService) *AlertHandler {
	return &AlertHandler{svc: svc, siteSvc: siteSvc}
}

func (h *AlertHandler) ListRules(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	rules, err := h.svc.ListRules(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: rules})
}

func (h *AlertHandler) CreateRule(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateAlertRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid request"})
	}
	if req.Metric == "" || req.Channel == "" {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "metric and channel required"})
	}

	rule, err := h.svc.CreateRule(serverID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: rule})
}

func (h *AlertHandler) UpdateRule(c *fiber.Ctx) error {
	ruleID, _ := strconv.ParseUint(c.Params("ruleId"), 10, 64)
	var req models.UpdateAlertRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid request"})
	}

	rule, err := h.svc.UpdateRule(ruleID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: rule})
}

func (h *AlertHandler) DeleteRule(c *fiber.Ctx) error {
	ruleID, _ := strconv.ParseUint(c.Params("ruleId"), 10, 64)
	if err := h.svc.DeleteRule(ruleID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "alert rule deleted"})
}

func (h *AlertHandler) GetHistory(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	history, err := h.svc.GetHistory(serverID, limit)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: history})
}

func (h *AlertHandler) GetNotificationConfig(c *fiber.Ctx) error {
	cfg, err := h.svc.GetNotificationConfig()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: cfg})
}

func (h *AlertHandler) UpdateNotificationConfig(c *fiber.Ctx) error {
	var req models.UpdateNotificationConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid request"})
	}

	cfg, err := h.svc.UpdateNotificationConfig(req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: cfg})
}

func (h *AlertHandler) PHPFPMAutotune(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	// Get a site on this server to use for PHP version
	sites, err := h.siteSvc.GetSitesByServer(serverID)
	if err != nil || len(sites) == 0 {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "no sites found on this server"})
	}

	site := sites[0]
	if err := h.svc.PHPFPMAutotune(serverID, uint64(site.ID), site.Domain, site.PHPVersion); err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "PHP-FPM autotune task queued"})
}
