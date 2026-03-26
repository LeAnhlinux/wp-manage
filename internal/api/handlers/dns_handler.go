package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type DNSHandler struct {
	svc *services.DNSService
}

func NewDNSHandler(svc *services.DNSService) *DNSHandler {
	return &DNSHandler{svc: svc}
}

// CreateGlobal creates a DNS provider not bound to any server.
func (h *DNSHandler) CreateGlobal(c *fiber.Ctx) error {
	var req models.CreateGlobalDNSProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	provider, err := h.svc.CreateGlobal(req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: provider})
}

// DeleteGlobal deletes a DNS provider by ID (no server check).
func (h *DNSHandler) DeleteGlobal(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.Delete(providerID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "DNS provider deleted"})
}

// LookupZone finds Zone ID from a domain name.
func (h *DNSHandler) LookupZone(c *fiber.Ctx) error {
	var req models.LookupZoneRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	result, err := h.svc.LookupZone(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: result})
}

// ListRecordsGlobal lists records for a provider (no server scoping).
func (h *DNSHandler) ListRecordsGlobal(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	records, err := h.svc.ListRecordsByProvider(providerID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: records})
}

// CreateRecordGlobal creates a record for a provider (no server scoping).
func (h *DNSHandler) CreateRecordGlobal(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var rec services.CFDNSRecord
	if err := c.BodyParser(&rec); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	created, err := h.svc.CreateRecordByProvider(providerID, rec)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: created})
}

// UpdateRecordGlobal updates a record for a provider (no server scoping).
func (h *DNSHandler) UpdateRecordGlobal(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	recordID := c.Params("recordId")
	var rec services.CFDNSRecord
	if err := c.BodyParser(&rec); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	updated, err := h.svc.UpdateRecordByProvider(providerID, recordID, rec)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: updated})
}

// DeleteRecordGlobal deletes a record for a provider (no server scoping).
func (h *DNSHandler) DeleteRecordGlobal(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	recordID := c.Params("recordId")
	if err := h.svc.DeleteRecordByProvider(providerID, recordID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "DNS record deleted"})
}

// AutoCreateRecord finds the matching DNS provider for a domain and creates/updates an A record pointing to server_ip.
func (h *DNSHandler) AutoCreateRecord(c *fiber.Ctx) error {
	var req models.AutoCreateDNSRecordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	result, err := h.svc.AutoCreateRecord(req.Domain, req.ServerIP)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: result})
}

func (h *DNSHandler) TestProvider(c *fiber.Ctx) error {
	var req models.TestDNSProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	result, err := h.svc.TestProvider(req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: result})
}

func (h *DNSHandler) ListAll(c *fiber.Ctx) error {
	providers, err := h.svc.ListAll()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: providers})
}

