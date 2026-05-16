package handlers

import (
	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/api/websocket"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type TransferHandler struct {
	svc *services.TransferService
	hub *websocket.Hub
}

func NewTransferHandler(svc *services.TransferService, hub *websocket.Hub) *TransferHandler {
	return &TransferHandler{svc: svc, hub: hub}
}

// TestConnection — POST /api/v1/transfer/test-connection
// Tests SSH reachability from the API server to the source server.
// Returns success + latency + hostname without creating any job or task.
// Does NOT require target_server_id — tests source creds only.
func (h *TransferHandler) TestConnection(c *fiber.Ctx) error {
	var req models.TransferTestConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Host == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "host is required"})
	}
	if req.SSHKey == "" && req.Password == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "provide ssh_key or password"})
	}

	result := h.svc.TestConnection(req)
	return c.JSON(models.APIResponse{Success: result.Success, Data: result})
}

// Connect — POST /api/v1/transfer/connect
// Creates a job + queues discovery task. Returns immediately with job_id and status "discovering".
func (h *TransferHandler) Connect(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	var req models.TransferConnectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.TargetServerID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "target_server_id is required"})
	}
	if req.Host == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "host is required"})
	}
	if req.SSHKey == "" && req.Password == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "provide ssh_key or password"})
	}

	job, err := h.svc.Connect(userID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Discovery started",
		Data:    job,
	})
}

// Get — GET /api/v1/transfer/:id
// Returns job + discovered sites + target PHP versions. Polled by Step 2 loading state.
func (h *TransferHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	job, err := h.svc.GetJob(id, userID, role)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Transfer job not found"})
	}

	// Parse JSON columns for a richer response
	type jobResponse struct {
		*models.TransferJob
		DiscoveredSitesParsed  []models.DiscoveredSiteInfo `json:"discovered_sites_parsed,omitempty"`
		TargetPHPVersionsParsed []string                   `json:"target_php_versions_parsed,omitempty"`
	}
	resp := jobResponse{TransferJob: job}
	if job.DiscoveredSites != "" {
		json.Unmarshal([]byte(job.DiscoveredSites), &resp.DiscoveredSitesParsed) //nolint:errcheck
	}
	if job.TargetPHPVersions != "" {
		json.Unmarshal([]byte(job.TargetPHPVersions), &resp.TargetPHPVersionsParsed) //nolint:errcheck
	}

	return c.JSON(models.APIResponse{Success: true, Data: resp})
}

// List — GET /api/v1/transfer
// Returns paginated transfer job history for the current user.
func (h *TransferHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	pg := parsePage(c)

	jobs, total, err := h.svc.List(userID, role, pg.Page, pg.PerPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch transfer jobs"})
	}
	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    jobs,
		Meta: models.PaginationMeta{
			Page: pg.Page, PerPage: pg.PerPage, Total: total,
			TotalPages: calcTotalPages(total, pg.PerPage),
		},
	})
}

// Start — POST /api/v1/transfer/:id/start
// User confirms site list (Step 2) → queues per-site import tasks → moves to Step 3.
func (h *TransferHandler) Start(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	var req models.TransferStartRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if len(req.Sites) == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "Select at least one site"})
	}

	// Optional re-submitted SSH creds (see service comment)
	sshKey := c.Get("X-SSH-Key")
	sshPassword := c.Get("X-SSH-Password")

	job, err := h.svc.StartTransfer(id, userID, role, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	// Parse discovered sites for task queuing
	var discovered []models.DiscoveredSiteInfo
	if job.DiscoveredSites != "" {
		json.Unmarshal([]byte(job.DiscoveredSites), &discovered) //nolint:errcheck
	}

	if err := h.svc.QueueImportTasks(job, req.Sites, discovered, sshKey, sshPassword); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to queue import tasks: " + err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Transfer started",
		Data:    job,
	})
}

// Status — GET /api/v1/transfer/:id/status
// Returns full job + per-site result list with progress. Polled by Step 3.
func (h *TransferHandler) Status(c *fiber.Ctx) error {
	return h.Get(c) // same payload, different semantic intent
}

// Cancel — POST /api/v1/transfer/:id/cancel
func (h *TransferHandler) Cancel(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if err := h.svc.CancelJob(id, userID, role); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Job cancelled"})
}

// Delete — DELETE /api/v1/transfer/:id
// Permanently removes a job and its results. Not allowed while discovering/transferring.
func (h *TransferHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if err := h.svc.DeleteJob(id, userID, role); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Job deleted"})
}

// CheckDomain — GET /api/v1/transfer/check-domain?domain=&target_server_id=&job_id=
// Real-time domain availability check (layers A+B+C, debounced from frontend).
func (h *TransferHandler) CheckDomain(c *fiber.Ctx) error {
	domain := c.Query("domain")
	serverID, _ := strconv.ParseUint(c.Query("target_server_id"), 10, 64)
	jobID, _ := strconv.ParseUint(c.Query("job_id"), 10, 64)

	if domain == "" || serverID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "domain and target_server_id are required"})
	}

	result := h.svc.CheckDomainAvailability(domain, serverID, jobID)
	return c.JSON(models.APIResponse{Success: true, Data: result})
}

// ─── Agent callbacks ─────────────────────────────────────────────────────────

// DiscoveryComplete — POST /api/v1/agent/transfer/:id/discovered
// Called by the agent on the target server when discovery is done.
func (h *TransferHandler) DiscoveryComplete(c *fiber.Ctx) error {
	var result models.AgentTransferDiscoverResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid payload"})
	}

	if err := h.svc.SaveDiscoveryResult(result); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	// Notify frontend via WebSocket
	h.hub.Broadcast(
		"transfer:"+strconv.FormatUint(result.JobID, 10),
		"transfer_update",
		map[string]interface{}{
			"event":  "discovery_complete",
			"job_id": result.JobID,
			"status": result.Status,
		},
	)

	return c.JSON(models.APIResponse{Success: true})
}

// ProgressUpdate — POST /api/v1/agent/transfer/update
// Called by the agent repeatedly during site import to push phase/progress updates.
func (h *TransferHandler) ProgressUpdate(c *fiber.Ctx) error {
	var update models.AgentTransferUpdate
	if err := c.BodyParser(&update); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid payload"})
	}

	if err := h.svc.UpdateSiteProgress(update); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}

	// Push realtime update to frontend
	h.hub.Broadcast(
		"transfer:"+strconv.FormatUint(update.JobID, 10),
		"transfer_update",
		map[string]interface{}{
			"event":     "site_progress",
			"job_id":    update.JobID,
			"result_id": update.ResultID,
			"phase":     update.Phase,
			"progress":  update.Progress,
			"status":    update.Status,
			"error":     update.Error,
		},
	)

	return c.JSON(models.APIResponse{Success: true})
}
