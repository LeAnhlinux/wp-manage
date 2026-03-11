package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type PerformanceHandler struct {
	svc *services.PerformanceService
}

func NewPerformanceHandler(svc *services.PerformanceService) *PerformanceHandler {
	return &PerformanceHandler{svc: svc}
}

// RunAudit triggers a PageSpeed Insights audit for a site
func (h *PerformanceHandler) RunAudit(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	var req models.RunPageSpeedRequest
	if err := c.BodyParser(&req); err != nil {
		req.DeviceType = "mobile"
	}

	metric, err := h.svc.RunPageSpeed(siteID, req.DeviceType)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{Success: true, Data: metric})
}

// GetMetrics returns performance metrics history for a site
func (h *PerformanceHandler) GetMetrics(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	deviceType := c.Query("device_type")
	days, _ := strconv.Atoi(c.Query("days", "30"))
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	metrics, total, err := h.svc.GetMetrics(siteID, deviceType, days, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    metrics,
		Meta: models.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetLatest returns the latest performance metrics for both devices
func (h *PerformanceHandler) GetLatest(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	latest, err := h.svc.GetLatest(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: latest})
}
