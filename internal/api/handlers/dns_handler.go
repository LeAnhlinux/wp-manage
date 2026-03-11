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

func (h *DNSHandler) List(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	providers, err := h.svc.List(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: providers})
}

func (h *DNSHandler) Create(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateDNSProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	provider, err := h.svc.Create(serverID, req)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: provider})
}

func (h *DNSHandler) Delete(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("providerId"), 10, 64)
	if err := h.svc.Delete(providerID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "DNS provider deleted"})
}

// UpdateRecord is a quick upsert of an A record by domain+IP.
func (h *DNSHandler) UpdateRecord(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("providerId"), 10, 64)
	var req models.UpdateDNSRecordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	if err := h.svc.UpdateDNSRecord(providerID, req.Domain, req.IP); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "DNS record updated"})
}

// ListRecords returns all DNS records for a provider's zone from Cloudflare.
// GET /servers/:id/dns-providers/:providerId/records
func (h *DNSHandler) ListRecords(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("providerId"), 10, 64)
	records, err := h.svc.ListDNSRecords(providerID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: records})
}

// CreateRecord creates a new DNS record.
// POST /servers/:id/dns-providers/:providerId/records
func (h *DNSHandler) CreateRecord(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("providerId"), 10, 64)
	var rec services.CFDNSRecord
	if err := c.BodyParser(&rec); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	created, err := h.svc.CreateDNSRecord(providerID, rec)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Data: created})
}

// DeleteRecord deletes a DNS record by its CF record ID.
// DELETE /servers/:id/dns-providers/:providerId/records/:recordId
func (h *DNSHandler) DeleteRecord(c *fiber.Ctx) error {
	providerID, _ := strconv.ParseUint(c.Params("providerId"), 10, 64)
	recordID := c.Params("recordId")
	if err := h.svc.DeleteDNSRecord(providerID, recordID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "DNS record deleted"})
}
