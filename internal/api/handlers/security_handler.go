package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type SecurityHandler struct {
	svc *services.SecurityService
}

func NewSecurityHandler(svc *services.SecurityService) *SecurityHandler {
	return &SecurityHandler{svc: svc}
}

// --- WAF ---

func (h *SecurityHandler) GetWAFConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	config, err := h.svc.GetWAFConfig(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get WAF config"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: config})
}

func (h *SecurityHandler) UpdateWAFConfig(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.UpdateWAFRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.UpdateWAFConfig(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "WAF config updated", Data: config})
}

// --- Malware Scan ---

func (h *SecurityHandler) StartScan(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.StartMalwareScanRequest
	if err := c.BodyParser(&req); err != nil {
		req = models.StartMalwareScanRequest{ScanType: "full"}
	}

	scan, err := h.svc.StartMalwareScan(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Malware scan started", Data: scan})
}

func (h *SecurityHandler) GetScan(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	scan, err := h.svc.GetScan(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Scan not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: scan})
}

func (h *SecurityHandler) ListSiteScans(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	pg := parsePage(c)

	scans, total, err := h.svc.ListScans(siteID, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch scans"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    scans,
		Meta:    models.PaginationMeta{Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages},
	})
}

func (h *SecurityHandler) ListAllScans(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	pg := parsePage(c)

	scans, total, err := h.svc.ListAllScans(userID, role, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch scans"})
	}

	totalPages := calcTotalPages(total, pg.PerPage)

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    scans,
		Meta:    models.PaginationMeta{Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages},
	})
}

// --- ClamAV ---

func (h *SecurityHandler) GetClamAVStatus(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	status, err := h.svc.GetClamAVStatus(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: status})
}

func (h *SecurityHandler) InstallClamAV(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.InstallClamAV(serverID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "ClamAV installed successfully"})
}

// --- Scan Schedules ---

func (h *SecurityHandler) ListSchedules(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	schedules, err := h.svc.ListSchedules(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch schedules"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: schedules})
}

func (h *SecurityHandler) CreateSchedule(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.CreateScanScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	schedule, err := h.svc.CreateSchedule(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Scan schedule created", Data: schedule})
}

func (h *SecurityHandler) UpdateSchedule(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	id, _ := strconv.ParseUint(c.Params("schedId"), 10, 64)
	var req models.UpdateScanScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	schedule, err := h.svc.UpdateSchedule(siteID, id, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Schedule updated", Data: schedule})
}

func (h *SecurityHandler) DeleteSchedule(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	id, _ := strconv.ParseUint(c.Params("schedId"), 10, 64)
	if err := h.svc.DeleteSchedule(siteID, id); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to delete schedule"})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Schedule deleted"})
}

// ScanComplete is called by the agent to report scan results
func (h *SecurityHandler) ScanComplete(c *fiber.Ctx) error {
	var result models.AgentMalwareScanResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}
	if err := h.svc.UpdateScanResult(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true})
}

// --- Threat actions: whitelist ---

func (h *SecurityHandler) AddWhitelist(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req struct {
		File       string `json:"file"`
		ThreatType string `json:"threat_type"`
		Reason     string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.File == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "file is required"})
	}
	rec, err := h.svc.AddWhitelist(siteID, req.File, req.ThreatType, req.Reason)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "File marked as safe", Data: rec})
}

func (h *SecurityHandler) ListWhitelist(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	list, err := h.svc.ListWhitelist(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch whitelist"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: list})
}

func (h *SecurityHandler) RemoveWhitelist(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.RemoveWhitelist(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Removed from whitelist"})
}

// --- Threat actions: quarantine ---

func (h *SecurityHandler) QuarantineThreat(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req struct {
		File       string `json:"file"`
		ThreatType string `json:"threat_type"`
	}
	if err := c.BodyParser(&req); err != nil || req.File == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "file is required"})
	}
	rec, err := h.svc.QuarantineThreat(siteID, req.File, req.ThreatType)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "File quarantined", Data: rec})
}

func (h *SecurityHandler) ListQuarantine(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	list, err := h.svc.ListQuarantine(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch quarantine"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: list})
}

func (h *SecurityHandler) RestoreQuarantine(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.RestoreQuarantine(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "File restored"})
}
