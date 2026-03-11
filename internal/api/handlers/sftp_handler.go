package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type SFTPHandler struct {
	svc *services.SFTPService
}

func NewSFTPHandler(svc *services.SFTPService) *SFTPHandler {
	return &SFTPHandler{svc: svc}
}

func (h *SFTPHandler) List(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	accounts, err := h.svc.List(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{Success: true, Data: accounts})
}

func (h *SFTPHandler) Create(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	var req models.CreateSFTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if req.Username == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Username is required"})
	}

	result, err := h.svc.Create(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "SFTP account created successfully",
		Data:    result,
	})
}

func (h *SFTPHandler) Delete(c *fiber.Ctx) error {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}

	accountID, err := strconv.ParseUint(c.Params("accountId"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid account ID"})
	}

	if err := h.svc.Delete(siteID, accountID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "SFTP account deleted successfully",
	})
}
