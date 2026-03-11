package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
)

type SSLHandler struct {
	service *services.SSLService
}

func NewSSLHandler(service *services.SSLService) *SSLHandler {
	return &SSLHandler{service: service}
}

func (h *SSLHandler) GetStatus(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	status, err := h.service.GetSSLStatus(siteID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "error": "Site not found"})
	}
	return c.JSON(fiber.Map{"success": true, "data": status})
}

func (h *SSLHandler) Renew(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err := h.service.RenewSSL(siteID); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "SSL certificate renewed successfully"})
}

func (h *SSLHandler) Enable(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err := h.service.EnableSSL(siteID); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "SSL certificate issued successfully"})
}

func (h *SSLHandler) Disable(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err := h.service.DisableSSL(siteID); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "SSL disabled. Site now serves HTTP only."})
}

func (h *SSLHandler) UploadCustom(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
		CaPEM   string `json:"ca_pem"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	if err := h.service.UploadCustomSSL(siteID, req.CertPEM, req.KeyPEM, req.CaPEM); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Custom SSL certificate installed successfully. Site now serves HTTPS."})
}

// ForceHTTPS enables/disables HTTP→HTTPS redirect in Nginx.
// POST /sites/:siteId/ssl/force-https  { "enable": true/false }
func (h *SSLHandler) ForceHTTPS(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request"})
	}
	if err := h.service.ForceHTTPS(siteID, req.Enable); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	msg := "Force HTTPS enabled"
	if !req.Enable {
		msg = "Force HTTPS disabled"
	}
	return c.JSON(fiber.Map{"success": true, "message": msg})
}
