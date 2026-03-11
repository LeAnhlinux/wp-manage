package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type CronHandler struct {
	svc *services.CronService
}

func NewCronHandler(svc *services.CronService) *CronHandler {
	return &CronHandler{svc: svc}
}

func (h *CronHandler) ListBySite(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	jobs, err := h.svc.ListBySite(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list cron jobs"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: jobs})
}

func (h *CronHandler) ListByServer(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobs, err := h.svc.ListByServer(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list cron jobs"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: jobs})
}

func (h *CronHandler) Create(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if serverID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "Server ID required"})
	}

	var req models.CreateCronJobRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	job, err := h.svc.Create(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Cron job created", Data: job})
}

func (h *CronHandler) Update(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Params("jobId"), 10, 64)

	var req models.UpdateCronJobRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	job, err := h.svc.Update(serverID, jobID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cron job updated", Data: job})
}

func (h *CronHandler) Toggle(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Params("jobId"), 10, 64)

	job, err := h.svc.Toggle(serverID, jobID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	status := "disabled"
	if job.Enabled {
		status = "enabled"
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cron job " + status, Data: job})
}

func (h *CronHandler) Delete(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Params("jobId"), 10, 64)

	if err := h.svc.Delete(serverID, jobID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cron job deleted"})
}

func (h *CronHandler) RunNow(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Params("jobId"), 10, 64)

	out, err := h.svc.RunNow(serverID, jobID)
	if err != nil {
		return c.JSON(models.APIResponse{Success: false, Error: err.Error(), Data: map[string]string{"output": out}})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cron job executed", Data: map[string]string{"output": out}})
}

func (h *CronHandler) GetLogs(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Params("jobId"), 10, 64)
	lines, _ := strconv.Atoi(c.Query("lines", "100"))

	logs, err := h.svc.GetLogs(serverID, jobID, lines)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: map[string]string{"logs": logs}})
}
