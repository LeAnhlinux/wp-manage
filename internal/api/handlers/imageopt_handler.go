package handlers

import (
	"math"
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
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	config, err := h.svc.GetConfig(siteID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// UpdateConfig updates image optimization settings
func (h *ImageOptHandler) UpdateConfig(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	var req models.UpdateImageOptConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	config, err := h.svc.UpdateConfig(siteID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: config})
}

// Toggle enables/disables auto-compress on upload (installs/removes mu-plugin)
func (h *ImageOptHandler) Toggle(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	config, err := h.svc.Toggle(siteID, body.Enabled)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Success: false, Error: err.Error(), Data: config})
	}
	msg := "Image optimization disabled"
	if body.Enabled {
		msg = "Image optimization enabled — auto-compress on upload active"
	}
	return c.JSON(models.APIResponse{Success: true, Message: msg, Data: config})
}

// RunOptimization starts an image optimization run
func (h *ImageOptHandler) RunOptimization(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	result, err := h.svc.RunOptimizationSync(siteID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data:    result,
	})
}

// OptimizeComplete handles agent callback with results
func (h *ImageOptHandler) OptimizeComplete(c *fiber.Ctx) error {
	var result models.AgentImageOptResult
	if err := c.BodyParser(&result); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := h.svc.OptimizeComplete(result); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Updated"})
}

// GetResults returns optimization history
func (h *ImageOptHandler) GetResults(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid site ID")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	if page < 1 { page = 1 }
	if perPage < 1 { perPage = 20 }

	results, total, err := h.svc.GetResults(siteID, page, perPage)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    results,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}
