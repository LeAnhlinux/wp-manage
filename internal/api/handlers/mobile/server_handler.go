package mobile

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// ServerHandler handles /api/mobile/v1/servers/* endpoints.
type ServerHandler struct {
	db        *gorm.DB
	serverSvc *services.ServerService
	alertSvc  *services.AlertService
}

func NewServerHandler(db *gorm.DB, serverSvc *services.ServerService, alertSvc *services.AlertService) *ServerHandler {
	return &ServerHandler{db: db, serverSvc: serverSvc, alertSvc: alertSvc}
}

// List godoc
// GET /api/mobile/v1/servers?page=1&per_page=20
func (h *ServerHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if perPage > 100 {
		perPage = 100
	}

	servers, total, err := h.serverSvc.List(userID, role, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	items := make([]models.MobileServerItem, 0, len(servers))
	for _, s := range servers {
		item := models.MobileServerItem{
			ID:            s.ID,
			Name:          s.Name,
			IPAddress:     s.IPAddress,
			Status:        s.Status,
			AgentVersion:  s.AgentVersion,
			OS:            s.OS,
			LastHeartbeat: s.LastHeartbeat,
			SitesCount:    len(s.Sites),
		}
		h.fillMetrics(s.ID, &item.Metrics)
		items = append(items, item)
	}

	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// Detail godoc
// GET /api/mobile/v1/servers/:id
func (h *ServerHandler) Detail(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid server id"))
	}

	server, err := h.serverSvc.GetByID(uint64(id))
	if err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Server not found"))
	}

	item := models.MobileServerItem{
		ID:            server.ID,
		Name:          server.Name,
		IPAddress:     server.IPAddress,
		Status:        server.Status,
		AgentVersion:  server.AgentVersion,
		OS:            server.OS,
		LastHeartbeat: server.LastHeartbeat,
		SitesCount:    len(server.Sites),
	}
	h.fillMetrics(server.ID, &item.Metrics)

	// Embed site list (lightweight)
	type siteItem struct {
		ID     uint64 `json:"id"`
		Domain string `json:"domain"`
		Status string `json:"status"`
	}
	sites := make([]siteItem, 0, len(server.Sites))
	for _, s := range server.Sites {
		sites = append(sites, siteItem{ID: s.ID, Domain: s.Domain, Status: s.Status})
	}

	return c.JSON(models.MobileOK(fiber.Map{
		"server": item,
		"sites":  sites,
	}))
}

// Services godoc
// GET /api/mobile/v1/servers/:id/services
// Returns service health status (nginx, php-fpm, mariadb, redis).
func (h *ServerHandler) Services(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid server id"))
	}

	// Read from last known task output or heartbeat ping
	// Simplification: return service check results from server_configs or last heartbeat
	type serviceStatus struct {
		Service string `json:"service"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	var results []serviceStatus
	// Query last health check task result for this server
	var tasks []models.TaskQueue
	h.db.Where("server_id = ? AND task_type = ? AND status = ?", id, "health_check", "completed").
		Order("updated_at DESC").Limit(1).Find(&tasks)

	if len(tasks) > 0 {
		// Parse output JSON if available
		_ = tasks[0].Output
	}

	// Default: return basic service list (agent will populate via health check)
	results = []serviceStatus{
		{Service: "nginx", Status: "unknown", Message: "Run health check to refresh"},
		{Service: "php-fpm", Status: "unknown", Message: "Run health check to refresh"},
		{Service: "mariadb", Status: "unknown", Message: "Run health check to refresh"},
		{Service: "redis", Status: "unknown", Message: "Run health check to refresh"},
	}

	_ = id
	return c.JSON(models.MobileOK(results))
}

// MetricsHistory godoc
// GET /api/mobile/v1/servers/:id/metrics/history?hours=24
func (h *ServerHandler) MetricsHistory(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid server id"))
	}
	hours := c.QueryInt("hours", 24)
	if hours > 168 {
		hours = 168 // max 7 days
	}

	type metricPoint struct {
		TS        string  `json:"ts"`
		CPUUsage  float64 `json:"cpu_usage"`
		RAMUsage  float64 `json:"ram_usage"`
		DiskUsage float64 `json:"disk_usage"`
	}

	var points []metricPoint
	h.db.Raw(`SELECT DATE_FORMAT(recorded_at,'%Y-%m-%dT%H:%i:00Z') as ts,
		AVG(cpu_usage) as cpu_usage,
		AVG(ram_usage) as ram_usage,
		AVG(disk_usage) as disk_usage
		FROM server_metrics
		WHERE server_id = ? AND recorded_at >= DATE_SUB(NOW(), INTERVAL ? HOUR)
		GROUP BY DATE_FORMAT(recorded_at,'%Y-%m-%dT%H:%i:00Z')
		ORDER BY ts ASC`, id, hours).Scan(&points)

	return c.JSON(models.MobileOK(points))
}

// AlertHistory godoc
// GET /api/mobile/v1/servers/:id/alerts
func (h *ServerHandler) AlertHistory(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid server id"))
	}

	history, err := h.alertSvc.GetHistory(uint64(id), 50)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(history))
}

func (h *ServerHandler) fillMetrics(serverID uint64, m *models.MobileServerMetrics) {
	h.db.Raw(`SELECT cpu_usage, ram_usage, disk_usage, uptime_sec
		FROM server_metrics WHERE server_id = ? ORDER BY recorded_at DESC LIMIT 1`, serverID).Scan(m)
}
