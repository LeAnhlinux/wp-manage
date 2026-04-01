package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type RolloutHandler struct {
	svc *services.RolloutService
}

func NewRolloutHandler(svc *services.RolloutService) *RolloutHandler {
	return &RolloutHandler{svc: svc}
}

// --- Releases ---

// CreateRelease creates a new agent release
func (h *RolloutHandler) CreateRelease(c *fiber.Ctx) error {
	var req models.CreateReleaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid request body"})
	}

	release, err := h.svc.CreateRelease(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{Success: true, Data: release})
}

// ListReleases lists all agent releases
func (h *RolloutHandler) ListReleases(c *fiber.Ctx) error {
	releases, err := h.svc.ListReleases()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: releases})
}

// GetRelease returns a release by ID
func (h *RolloutHandler) GetRelease(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid release ID"})
	}

	release, err := h.svc.GetRelease(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "release not found"})
	}

	return c.JSON(models.APIResponse{Success: true, Data: release})
}

// DeleteRelease deletes a release
func (h *RolloutHandler) DeleteRelease(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid release ID"})
	}

	if err := h.svc.DeleteRelease(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "release deleted"})
}

// --- Rollouts ---

// CreateRollout creates a new rollout plan
func (h *RolloutHandler) CreateRollout(c *fiber.Ctx) error {
	var req models.CreateRolloutRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid request body"})
	}

	plan, err := h.svc.CreateRollout(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{Success: true, Data: plan})
}

// ListRollouts lists all rollouts
func (h *RolloutHandler) ListRollouts(c *fiber.Ctx) error {
	pg := parsePage(c)
	if pg.Page < 1 {
		pg.Page = 1
	}
	if pg.PerPage < 1 || pg.PerPage > 100 {
		pg.PerPage = 20
	}

	plans, total, err := h.svc.ListRollouts(pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    plans,
		Meta: models.PaginationMeta{
			Page:       pg.Page,
			PerPage:    pg.PerPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetRollout returns a rollout by ID
func (h *RolloutHandler) GetRollout(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid rollout ID"})
	}

	plan, err := h.svc.GetRollout(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "rollout not found"})
	}

	return c.JSON(models.APIResponse{Success: true, Data: plan})
}

// StartRollout starts a rollout
func (h *RolloutHandler) StartRollout(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid rollout ID"})
	}

	if err := h.svc.StartRollout(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "rollout started"})
}

// PauseRollout pauses a running rollout
func (h *RolloutHandler) PauseRollout(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid rollout ID"})
	}

	if err := h.svc.PauseRollout(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "rollout paused"})
}

// UpdateComplete handles agent callback for agent update results
func (h *RolloutHandler) UpdateComplete(c *fiber.Ctx) error {
	var result models.AgentUpdateResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "invalid body"})
	}

	if err := h.svc.HandleUpdateResult(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true})
}
