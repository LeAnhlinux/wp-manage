package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	ws "github.com/gofiber-team/wp-manage/internal/api/websocket"
	"github.com/gofiber-team/wp-manage/internal/api/workers"
	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type ServerHandler struct {
	svc         *services.ServerService
	siteSvc     *services.SiteService
	backupSvc   *services.BackupService
	cfg         *config.Config
	hub         *ws.Hub
	alertWorker *workers.AlertWorker
}

func (h *ServerHandler) SetAlertWorker(w *workers.AlertWorker) {
	h.alertWorker = w
}

func NewServerHandler(svc *services.ServerService, siteSvc *services.SiteService, backupSvc *services.BackupService, cfg *config.Config, hub *ws.Hub) *ServerHandler {
	return &ServerHandler{svc: svc, siteSvc: siteSvc, backupSvc: backupSvc, cfg: cfg, hub: hub}
}

func (h *ServerHandler) Create(c *fiber.Ctx) error {
	var req models.CreateServerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	userID := c.Locals("user_id").(uint64)
	server, apiKey, err := h.svc.Create(userID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	installCmd := h.svc.GenerateInstallCommand(server, apiKey, h.cfg.Panel.Domain)

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "Server created. Run the install command on your VPS.",
		Data: fiber.Map{
			"server":          server,
			"install_command": installCmd,
		},
	})
}

func (h *ServerHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	servers, total, err := h.svc.List(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch servers"})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    servers,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *ServerHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	// Access check is done by ServerAccessRequired middleware
	server, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Server not found"})
	}

	return c.JSON(models.APIResponse{Success: true, Data: server})
}

func (h *ServerHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	// Permission check done by PermissionRequired middleware
	if err := h.svc.Delete(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Server deleted"})
}

func (h *ServerHandler) Heartbeat(c *fiber.Ctx) error {
	apiKey := c.Locals("agent_api_key").(string)
	server, err := h.svc.GetByAPIKey(apiKey)
	if err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: "Invalid agent API key"})
	}

	var hb models.AgentHeartbeat
	if err := c.BodyParser(&hb); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid heartbeat data"})
	}

	if err := h.svc.UpdateHeartbeat(server.ID, hb); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to update heartbeat"})
	}

	// Broadcast server metrics to WebSocket subscribers
	if h.hub != nil {
		h.hub.Broadcast(fmt.Sprintf("server:%d:metrics", server.ID), "server_metrics", map[string]interface{}{
			"server_id":  server.ID,
			"cpu_usage":  hb.CPUUsage,
			"ram_usage":  hb.RAMUsage,
			"disk_usage": hb.DiskUsage,
			"uptime":     hb.Uptime,
			"version":    hb.Version,
		})
		// Also broadcast to the global dashboard channel
		h.hub.Broadcast("dashboard:metrics", "server_metrics", map[string]interface{}{
			"server_id":   server.ID,
			"server_name": server.Name,
			"ip_address":  server.IPAddress,
			"cpu_usage":   hb.CPUUsage,
			"ram_usage":   hb.RAMUsage,
			"disk_usage":  hb.DiskUsage,
			"uptime":      hb.Uptime,
			"status":      server.Status,
		})
	}

	// Check alert rules on each heartbeat
	if h.alertWorker != nil {
		go h.alertWorker.CheckServerMetrics(server.ID, hb.CPUUsage, hb.RAMUsage, hb.DiskUsage)
	}

	return c.JSON(models.APIResponse{Success: true})
}

func (h *ServerHandler) SystemInfo(c *fiber.Ctx) error {
	apiKey := c.Locals("agent_api_key").(string)
	server, err := h.svc.GetByAPIKey(apiKey)
	if err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: "Invalid agent API key"})
	}

	var info models.AgentSystemInfo
	if err := c.BodyParser(&info); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid system info data"})
	}

	if err := h.svc.UpdateSystemInfo(server.ID, int(info.RAMMB), info.CPUCores, int(info.DiskGB), info.OS); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to update system info"})
	}

	return c.JSON(models.APIResponse{Success: true})
}

func (h *ServerHandler) Tasks(c *fiber.Ctx) error {
	apiKey := c.Locals("agent_api_key").(string)
	server, err := h.svc.GetByAPIKey(apiKey)
	if err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: "Invalid agent API key"})
	}

	var tasks []models.AgentTask

	// 1) Pending sites (create_site tasks)
	sites, err := h.siteSvc.GetPendingSites(server.ID)
	if err == nil {
		for _, s := range sites {
			installType := s.InstallType
			if installType == "" {
				installType = "auto"
			}
			tasks = append(tasks, models.AgentTask{
				TaskType: "create_site",
				SiteID:   s.ID,
				Data: models.AgentCreateSiteTask{
					Domain:       s.Domain,
					PHPVersion:   s.PHPVersion,
					SSLEnabled:   s.SSLEnabled,
					InstallType:  installType,
					DBName:       s.DBName,
					DBUser:       s.DBUser,
					DBPassword:   s.DBPassword,
					WPAdminUser:  s.WPAdminUser,
					WPAdminEmail: s.WPAdminEmail,
					WPAdminPass:  s.WPAdminPass,
				},
			})
			// Mark as provisioning to prevent re-dispatch on next agent poll
			h.siteSvc.UpdateStatus(s.ID, "provisioning")
		}
	}

	// 2) Queued tasks (restart, suspend, resume, delete, ssl, php update)
	queuedTasks, err := h.siteSvc.GetPendingTasks(server.ID)
	if err == nil {
		for _, t := range queuedTasks {
			// Parse the JSON string into a proper object so it doesn't get double-escaped
			var taskData interface{}
			if t.Data != "" {
				json.Unmarshal([]byte(t.Data), &taskData)
			}
			tasks = append(tasks, models.AgentTask{
				TaskID:   t.ID,
				TaskType: t.TaskType,
				SiteID:   t.SiteID,
				Data:     taskData,
			})
			// Mark as processing
			h.siteSvc.CompleteTask(t.ID, "processing", "")
		}
	}

	return c.JSON(models.APIResponse{Success: true, Data: tasks})
}

func (h *ServerHandler) TaskComplete(c *fiber.Ctx) error {
	apiKey := c.Locals("agent_api_key").(string)
	if _, err := h.svc.GetByAPIKey(apiKey); err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: "Invalid agent API key"})
	}

	var result models.AgentTaskResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	// Update site status if applicable
	if result.SiteID > 0 && result.Status != "" {
		h.siteSvc.UpdateStatus(result.SiteID, result.Status)
	}

	// Update WP version if provided
	if result.SiteID > 0 && result.WPVersion != "" {
		h.siteSvc.UpdateWPVersion(result.SiteID, result.WPVersion)
	}

	// Save autologin secret if provided
	if result.SiteID > 0 && result.AutologinSecret != "" {
		h.siteSvc.UpdateAutologinSecret(result.SiteID, result.AutologinSecret)
	}

	// Save the per-site Linux user if reported (set during site creation)
	if result.SiteID > 0 && result.LinuxUser != "" {
		h.siteSvc.UpdateLinuxUser(result.SiteID, result.LinuxUser)
	}

	// Update SSL expiry if reported (after issue or renew)
	if result.SiteID > 0 && result.SSLExpiresAt != nil {
		h.siteSvc.UpdateSSLExpiry(result.SiteID, result.SSLExpiresAt)
	}

	// Update ssl_enabled if reported (after force-https toggle)
	if result.SiteID > 0 && result.SSLEnabled != nil {
		h.siteSvc.UpdateSSLEnabled(result.SiteID, *result.SSLEnabled)
	}

	// Mark task as complete if task_id provided
	if result.TaskID > 0 {
		errMsg := ""
		if result.Error != "" {
			errMsg = result.Error
		}
		status := "completed"
		if result.Error != "" {
			status = "failed"
		}
		if result.Output != "" {
			h.siteSvc.CompleteTaskWithOutput(result.TaskID, status, result.Output, errMsg)
		} else {
			h.siteSvc.CompleteTask(result.TaskID, status, errMsg)
		}
	}

	// Orphaned backup restore completed — recreate the panel site record so the
	// restored site becomes visible in the panel again.
	if result.RestoredDomain != "" && result.SiteID == 0 && result.Error == "" {
		if err := h.backupSvc.RecreateOrphanedSiteRecord(
			result.RestoredBackupID,
			result.RestoredDomain,
			result.RestoredDBName,
			result.RestoredDBUser,
			result.RestoredDBPass,
			result.RestoredPHPVersion,
		); err != nil {
			// Log but don't fail — restore succeeded on the server even if panel record fails
			fmt.Printf("[TaskComplete] failed to recreate site record for %s: %v\n", result.RestoredDomain, err)
		}
	}

	// After any restore (orphaned or normal): update ssl_enabled and wp_version.
	// For orphaned restores, siteID is 0 — look up by domain after record was created above.
	if result.Error == "" && (result.RestoredHasSSL != nil || result.WPVersion != "") {
		siteIDToUpdate := result.SiteID
		if siteIDToUpdate == 0 && result.RestoredDomain != "" {
			if s, err := h.siteSvc.GetByDomain(result.RestoredDomain); err == nil {
				siteIDToUpdate = s.ID
			}
		}
		if siteIDToUpdate > 0 {
			if result.RestoredHasSSL != nil {
				h.siteSvc.UpdateSSLEnabled(siteIDToUpdate, *result.RestoredHasSSL)
			}
			if result.WPVersion != "" {
				h.siteSvc.UpdateWPVersion(siteIDToUpdate, result.WPVersion)
			}
		}
	}

	return c.JSON(models.APIResponse{Success: true})
}

// GetLogs queues a task to tail server logs (Nginx, PHP-FPM, MySQL, system)
func (h *ServerHandler) GetLogs(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	logType := c.Query("type", "nginx_access") // nginx_access, nginx_error, php_error, mysql_error, syslog
	lines, _ := strconv.Atoi(c.Query("lines", "100"))
	if lines < 1 || lines > 1000 {
		lines = 100
	}

	// Queue a log-tail task for the agent
	taskData := fmt.Sprintf(`{"log_type":"%s","lines":%d}`, logType, lines)
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "tail_logs",
		Data:     taskData,
		Status:   "pending",
	}
	if err := h.svc.CreateTask(&task); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to queue log task"})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Log tail task queued",
		Data:    fiber.Map{"task_id": task.ID, "log_type": logType, "lines": lines},
	})
}

// StreamLogs triggers WebSocket streaming of live logs for a server
func (h *ServerHandler) StreamLogs(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	logType := c.Query("type", "nginx_access")

	// Queue a stream-logs task — agent will tail -f and send via callback
	taskData := fmt.Sprintf(`{"log_type":"%s","stream":true}`, logType)
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "stream_logs",
		Data:     taskData,
		Status:   "pending",
	}
	if err := h.svc.CreateTask(&task); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to queue stream task"})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Log streaming started",
		Data:    fiber.Map{"task_id": task.ID, "channel": fmt.Sprintf("server:%d:logs", serverID)},
	})
}

func (h *ServerHandler) BackupComplete(c *fiber.Ctx) error {
	apiKey := c.Locals("agent_api_key").(string)
	if _, err := h.svc.GetByAPIKey(apiKey); err != nil {
		return c.Status(401).JSON(models.APIResponse{Error: "Invalid agent API key"})
	}

	var result models.AgentBackupResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request"})
	}

	if result.Status == "completed" {
		h.backupSvc.UpdateBackupResult(result.BackupID, result.FilePath, result.SizeBytes, result.Checksum)
	} else {
		h.backupSvc.UpdateBackupFailed(result.BackupID, result.Error)
	}

	// Also complete the task queue entry
	if result.TaskID > 0 {
		status := "completed"
		errMsg := ""
		if result.Error != "" {
			status = "failed"
			errMsg = result.Error
		}
		h.siteSvc.CompleteTask(result.TaskID, status, errMsg)
	}

	return c.JSON(models.APIResponse{Success: true})
}
