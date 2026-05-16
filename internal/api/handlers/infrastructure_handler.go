package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type InfrastructureHandler struct {
	svc *services.InfrastructureService
}

func NewInfrastructureHandler(svc *services.InfrastructureService) *InfrastructureHandler {
	return &InfrastructureHandler{svc: svc}
}

// ==================== Firewall ====================

func (h *InfrastructureHandler) ListFirewallRules(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	rules, err := h.svc.ListFirewallRules(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list firewall rules"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: rules})
}

func (h *InfrastructureHandler) CreateFirewallRule(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateFirewallRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	rule, err := h.svc.CreateFirewallRule(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Firewall rule created", Data: rule})
}

func (h *InfrastructureHandler) DeleteFirewallRule(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	ruleID, _ := strconv.ParseUint(c.Params("ruleId"), 10, 64)

	err := h.svc.DeleteFirewallRule(serverID, ruleID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Firewall rule deleted"})
}

func (h *InfrastructureHandler) GetFirewallStatus(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	status, err := h.svc.GetFirewallStatus(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get firewall status: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: map[string]string{"status": status}})
}

// ==================== Cloudflare Firewall ====================

func (h *InfrastructureHandler) GetCloudflareConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	cfg, err := h.svc.GetCloudflareConfig(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: cfg})
}

func (h *InfrastructureHandler) EnableCloudflareFirewall(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.EnableCloudflareFirewall(serverID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cloudflare-only firewall enabled for ports 80 and 443"})
}

func (h *InfrastructureHandler) DisableCloudflareFirewall(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	if err := h.svc.DisableCloudflareFirewall(serverID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Cloudflare-only firewall disabled, ports 80 and 443 restored to open access"})
}

// ==================== Nginx Configs ====================

func (h *InfrastructureHandler) ListNginxConfigs(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	configs, err := h.svc.ListNginxConfigs(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list nginx configs"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: configs})
}

func (h *InfrastructureHandler) GetNginxConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	configID, _ := strconv.ParseUint(c.Params("configId"), 10, 64)

	config, err := h.svc.GetNginxConfig(serverID, configID)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: config})
}

func (h *InfrastructureHandler) CreateNginxConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateNginxConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.CreateNginxConfig(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Nginx config created", Data: config})
}

func (h *InfrastructureHandler) UpdateNginxConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	configID, _ := strconv.ParseUint(c.Params("configId"), 10, 64)
	var req models.UpdateNginxConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	config, err := h.svc.UpdateNginxConfig(serverID, configID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Nginx config updated", Data: config})
}

func (h *InfrastructureHandler) DeleteNginxConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	configID, _ := strconv.ParseUint(c.Params("configId"), 10, 64)

	err := h.svc.DeleteNginxConfig(serverID, configID)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Nginx config deleted"})
}

// ==================== OPcache ====================

func (h *InfrastructureHandler) OPcacheAction(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.OPcacheActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	phpVersion := c.Query("php_version", "8.2")
	err := h.svc.OPcacheAction(serverID, req.Action, phpVersion)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	actionLabels := map[string]string{
		"enable":  "OPcache enabled successfully",
		"disable": "OPcache disabled successfully",
		"flush":   "OPcache cache flushed successfully",
		"stats":   "OPcache stats retrieved",
	}
	msg := actionLabels[req.Action]
	if msg == "" {
		msg = "OPcache action completed: " + req.Action
	}
	return c.JSON(models.APIResponse{Success: true, Message: msg})
}

// ==================== Rebuild Config ====================

func (h *InfrastructureHandler) RebuildConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.RebuildConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	output, err := h.svc.RebuildConfig(serverID, req)
	if err != nil {
		// Partial success: return output + error so UI can show what succeeded/failed
		return c.Status(207).JSON(models.APIResponse{
						Error:   err.Error(),
			Data:    map[string]string{"output": output},
		})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Config rebuilt successfully",
		Data:    map[string]string{"output": output},
	})
}

// ==================== Services Status ====================

func (h *InfrastructureHandler) InstallService(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	service := c.Params("service")
	if service == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "service name required"})
	}
	if err := h.svc.InstallService(serverID, service); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Install failed: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: service + " installed successfully"})
}

func (h *InfrastructureHandler) CheckServices(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	output, err := h.svc.CheckServices(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Service check failed: " + err.Error()})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Data:    map[string]string{"services": output},
	})
}

// ==================== Health Check ====================

func (h *InfrastructureHandler) RunHealthCheck(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	output, err := h.svc.RunHealthCheck(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Health check failed: " + err.Error()})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Health check completed",
		Data:    map[string]string{"results": output},
	})
}

// ==================== PHP-FPM Pool ====================

func (h *InfrastructureHandler) GetPHPFPMPool(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	pool, err := h.svc.GetPHPFPMPool(serverID, siteID)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: pool})
}

func (h *InfrastructureHandler) UpdatePHPFPMPool(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	var req models.PHPFPMPoolConfig
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.UpdatePHPFPMPool(serverID, siteID, req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "PHP-FPM pool update queued"})
}

// ==================== PHP Version Management ====================

func (h *InfrastructureHandler) InstallPHPVersion(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.InstallPHPVersionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.InstallPHPVersion(serverID, req.Version); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "PHP " + req.Version + " installed successfully"})
}

func (h *InfrastructureHandler) ChangeSitePHPVersion(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)
	var req models.ChangePHPVersionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.ChangeSitePHPVersion(serverID, siteID, req.NewVersion); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "PHP version changed to " + req.NewVersion})
}

// ==================== Server Logs ====================

func (h *InfrastructureHandler) GetServerLogs(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	logType := c.Query("type", "nginx_access")
	lines, _ := strconv.Atoi(c.Query("lines", "100"))
	if lines < 1 || lines > 1000 {
		lines = 100
	}

	content, err := h.svc.GetServerLogs(serverID, logType, lines)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to read logs: " + err.Error()})
	}
	return c.JSON(models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"content":  content,
			"log_type": logType,
			"lines":    lines,
		},
	})
}

// ==================== Nginx Vhost ====================

func (h *InfrastructureHandler) ReadVhost(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	content, err := h.svc.ReadVhost(siteID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: map[string]string{
		"content":   content,
		"file_path": "/etc/nginx/sites-available/<domain>.conf",
	}})
}

func (h *InfrastructureHandler) WriteVhost(c *fiber.Ctx) error {
	siteID, _ := strconv.ParseUint(c.Params("siteId"), 10, 64)

	var req struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.Content == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Content is required"})
	}

	if err := h.svc.WriteVhost(siteID, req.Content); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Vhost updated successfully"})
}

// ==================== MariaDB Management ====================

func (h *InfrastructureHandler) ListMariaDBUsers(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	users, err := h.svc.ListMariaDBUsers(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list MariaDB users"})
	}
	if users == nil {
		users = []models.MariaDBUser{}
	}
	return c.JSON(models.APIResponse{Success: true, Data: users})
}

func (h *InfrastructureHandler) CreateMariaDBUser(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.MariaDBUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.CreateMariaDBUser(serverID, req); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "User '" + req.Username + "' created successfully"})
}

func (h *InfrastructureHandler) DeleteMariaDBUser(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	username := c.Params("username")

	if err := h.svc.DeleteMariaDBUser(serverID, username); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "User '" + username + "' deleted successfully"})
}

func (h *InfrastructureHandler) GetMariaDBConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	config, err := h.svc.GetMariaDBConfig(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get MariaDB config"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: config})
}

func (h *InfrastructureHandler) UpdateMariaDBConfig(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.MariaDBConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	if err := h.svc.UpdateMariaDBConfig(serverID, req); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "MariaDB configuration updated and service restarted"})
}

func (h *InfrastructureHandler) ListMariaDBDatabases(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	dbs, err := h.svc.ListMariaDBDatabases(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list databases: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: dbs})
}

func (h *InfrastructureHandler) CreateMariaDBDatabase(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.CreateDatabaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if err := h.svc.CreateMariaDBDatabase(serverID, req); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.Status(201).JSON(models.APIResponse{Success: true, Message: "Database '" + req.Name + "' created successfully"})
}

func (h *InfrastructureHandler) DeleteMariaDBDatabase(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	dbName := c.Params("dbName")
	if err := h.svc.DeleteMariaDBDatabase(serverID, dbName); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Database '" + dbName + "' deleted successfully"})
}

func (h *InfrastructureHandler) ImportMariaDBDatabase(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	dbName := c.Params("dbName")
	if dbName == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Database name is required"})
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "SQL file is required (field: file)"})
	}
	// Limit: 512 MB
	if file.Size > 512*1024*1024 {
		return c.Status(400).JSON(models.APIResponse{Error: "File too large (max 512 MB)"})
	}
	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to read uploaded file"})
	}
	defer f.Close()
	sqlData := make([]byte, file.Size)
	if _, err := f.Read(sqlData); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to read file content"})
	}
	if err := h.svc.ImportMariaDBDatabase(serverID, dbName, sqlData); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Database '" + dbName + "' imported successfully"})
}

func (h *InfrastructureHandler) ListInstalledPHPVersions(c *fiber.Ctx) error {
	serverID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	versions, err := h.svc.ListInstalledPHPVersions(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to list PHP versions: " + err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Data: versions})
}
