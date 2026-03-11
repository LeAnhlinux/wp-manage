package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type DomainHandler struct {
	svc *services.DomainService
}

func NewDomainHandler(svc *services.DomainService) *DomainHandler {
	return &DomainHandler{svc: svc}
}

func (h *DomainHandler) List(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	domains, err := h.svc.ListBySite(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list domains"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: domains})
}

func (h *DomainHandler) Create(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.CreateDomainRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	domain, err := h.svc.Create(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Domain added", Data: domain})
}

func (h *DomainHandler) Delete(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	domainID, _ := strconv.ParseUint(c.Params("domainId"), 10, 64)

	if err := h.svc.Delete(siteID, domainID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Domain removed"})
}
