package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type WordPressHandler struct {
	svc *services.WordPressService
}

func NewWordPressHandler(svc *services.WordPressService) *WordPressHandler {
	return &WordPressHandler{svc: svc}
}

// --- Staging ---

// CreateStaging creates a staging environment for a site
func (h *WordPressHandler) CreateStaging(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	staging, err := h.svc.CreateStaging(siteID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{Success: true, Data: staging})
}

// GetStaging returns staging environments for a site
func (h *WordPressHandler) GetStaging(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	stagings, err := h.svc.GetStaging(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: stagings})
}

// PushStaging pushes staging to production
func (h *WordPressHandler) PushStaging(c *fiber.Ctx) error {
	stagingID, err := strconv.ParseUint(c.Params("stagingId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid staging ID"})
	}

	if err := h.svc.PushStaging(stagingID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "pushing staging to production"})
}

// DeleteStaging removes a staging environment
func (h *WordPressHandler) DeleteStaging(c *fiber.Ctx) error {
	stagingID, err := strconv.ParseUint(c.Params("stagingId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid staging ID"})
	}

	if err := h.svc.DeleteStaging(stagingID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "staging environment deleted"})
}

// StagingComplete handles agent callback for staging tasks
func (h *WordPressHandler) StagingComplete(c *fiber.Ctx) error {
	var req struct {
		StagingID  uint64 `json:"staging_id"`
		Status     string `json:"status"`
		SiteStatus string `json:"site_status"`
		Error      string `json:"error,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid body"})
	}

	if err := h.svc.UpdateStagingStatus(req.StagingID, req.Status, req.SiteStatus); err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true})
}

// --- Auto-Update Config ---

// GetAutoUpdateConfig returns auto-update config for a site
func (h *WordPressHandler) GetAutoUpdateConfig(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	config, err := h.svc.GetAutoUpdateConfig(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// UpdateAutoUpdateConfig updates auto-update settings
func (h *WordPressHandler) UpdateAutoUpdateConfig(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	var req models.UpdateAutoUpdateConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid request body"})
	}

	config, err := h.svc.UpdateAutoUpdateConfig(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// --- Check Updates ---

// CheckUpdates triggers update check for a site and waits for result
func (h *WordPressHandler) CheckUpdates(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	result, err := h.svc.CheckUpdatesSync(siteID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: result})
}

// UpdateCheckComplete handles agent callback for update check results
func (h *WordPressHandler) UpdateCheckComplete(c *fiber.Ctx) error {
	var result models.AgentCheckUpdatesResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid body"})
	}

	if err := h.svc.SaveUpdateResult(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true})
}

// GetUpdateLogs returns update logs for a site
func (h *WordPressHandler) GetUpdateLogs(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	logs, total, err := h.svc.GetUpdateLogs(siteID, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    logs,
		Meta: models.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// --- DB Optimize ---

// OptimizeDatabase triggers DB optimization for a site and waits for result
func (h *WordPressHandler) OptimizeDatabase(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid site ID"})
	}

	result, err := h.svc.OptimizeDatabaseSync(siteID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: result})
}

// DBOptimizeComplete handles agent callback for DB optimization results
func (h *WordPressHandler) DBOptimizeComplete(c *fiber.Ctx) error {
	var result models.AgentDBOptimizeResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Success: false, Error: "invalid body"})
	}

	if err := h.svc.SaveDBOptimizeResult(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true})
}
