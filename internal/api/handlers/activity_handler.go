package handlers

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type ActivityHandler struct {
	svc *services.ActivityService
}

func NewActivityHandler(svc *services.ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

func (h *ActivityHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	var userID *uint64
	if v := c.Query("user_id"); v != "" {
		id, _ := strconv.ParseUint(v, 10, 64)
		userID = &id
	}
	var serverID *uint64
	if v := c.Query("server_id"); v != "" {
		id, _ := strconv.ParseUint(v, 10, 64)
		serverID = &id
	}
	var siteID *uint64
	if v := c.Query("site_id"); v != "" {
		id, _ := strconv.ParseUint(v, 10, 64)
		siteID = &id
	}
	action := c.Query("action")

	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			from = &t
		}
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			end := t.Add(24*time.Hour - time.Second) // end of day
			to = &end
		}
	}

	logs, total, err := h.svc.List(userID, serverID, siteID, action, from, to, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch activity logs"})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    logs,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *ActivityHandler) GetActions(c *fiber.Ctx) error {
	actions, err := h.svc.GetActions()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch actions"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: actions})
}

func (h *ActivityHandler) DeleteLog(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid log ID"})
	}
	if err := h.svc.DeleteLog(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(models.APIResponse{Error: "Activity log not found"})
		}
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to delete activity log"})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Activity log deleted"})
}

func (h *ActivityHandler) Cleanup(c *fiber.Ctx) error {
	days, _ := strconv.Atoi(c.Query("days", "90"))
	if days < 1 {
		days = 90
	}
	deleted, err := h.svc.Cleanup(days)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to cleanup activity logs"})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Activity logs cleaned up",
		Data:    fiber.Map{"deleted": deleted, "retention_days": days},
	})
}
