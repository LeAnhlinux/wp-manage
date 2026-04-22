package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type ImageOptHandler struct {
	svc *services.ImageOptService
}

func NewImageOptHandler(svc *services.ImageOptService) *ImageOptHandler {
	return &ImageOptHandler{svc: svc}
}

// GetConfig returns image optimization config for a site
func (h *ImageOptHandler) GetConfig(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	config, err := h.svc.GetConfig(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// UpdateConfig updates image optimization settings
func (h *ImageOptHandler) UpdateConfig(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.UpdateImageOptConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.UpdateConfig(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// Toggle enables/disables auto-compress on upload (installs/removes mu-plugin)
func (h *ImageOptHandler) Toggle(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	config, err := h.svc.Toggle(siteID, body.Enabled)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error(), Data: config})
	}
	msg := "Image optimization disabled"
	if body.Enabled {
		msg = "Image optimization enabled — auto-compress on upload active"
	}
	return c.JSON(models.APIResponse{Success: true, Message: msg, Data: config})
}

// ToggleNginxWebP enables/disables WebP auto-serving in nginx for the site
func (h *ImageOptHandler) ToggleNginxWebP(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	config, err := h.svc.ToggleNginxWebP(siteID, body.Enabled)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error(), Data: config})
	}
	msg := "WebP nginx serving disabled"
	if body.Enabled {
		msg = "WebP nginx serving enabled — browser tự nhận .webp thay vì .jpg/.png"
	}
	return c.JSON(models.APIResponse{Success: true, Message: msg, Data: config})
}

// RunOptimization starts an image optimization run
func (h *ImageOptHandler) RunOptimization(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	result, err := h.svc.RunOptimizationSync(siteID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data:    result,
	})
}

// ProgressUpdate handles intermediate progress updates from agent (every ~50 images).
// Returns {cancel: true} if user has cancelled, so agent can stop early.
func (h *ImageOptHandler) ProgressUpdate(c *fiber.Ctx) error {
	var result models.AgentImageOptResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	cancelled, err := h.svc.ProgressUpdate(result)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "cancel": cancelled})
}

// CancelOptimization cancels a running image optimization
func (h *ImageOptHandler) CancelOptimization(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	if err := h.svc.CancelOptimization(siteID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Optimization cancelled"})
}

// OptimizeComplete handles agent callback with results
func (h *ImageOptHandler) OptimizeComplete(c *fiber.Ctx) error {
	var result models.AgentImageOptResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.OptimizeComplete(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// GetOriginalsStats returns count and size of .wpmanage-orig backup files on the agent
func (h *ImageOptHandler) GetOriginalsStats(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	stats, err := h.svc.GetOriginalsStats(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: stats})
}

// RevertImages restores all backed-up originals and removes optimization markers
func (h *ImageOptHandler) RevertImages(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	res, err := h.svc.RevertImages(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: res, Message: fmt.Sprintf("Reverted %d images to originals", res.Count)})
}

// DeleteOriginals removes all .wpmanage-orig backup files to free disk space
func (h *ImageOptHandler) DeleteOriginals(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	res, err := h.svc.DeleteOriginals(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: res, Message: fmt.Sprintf("Deleted %d original files, freed %d bytes", res.Count, res.SizeBytes)})
}

// GetResults returns optimization history
func (h *ImageOptHandler) GetResults(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	pg := parsePage(c)
	if pg.Page < 1 { pg.Page = 1 }
	if pg.PerPage < 1 { pg.PerPage = 20 }

	results, total, err := h.svc.GetResults(siteID, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	totalPages := calcTotalPages(total, pg.PerPage)
	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    results,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages,
		},
	})
}
