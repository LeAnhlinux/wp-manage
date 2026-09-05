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
	userSvc   *services.UserService
}

func NewServerHandler(db *gorm.DB, serverSvc *services.ServerService, alertSvc *services.AlertService, userSvc *services.UserService) *ServerHandler {
	return &ServerHandler{db: db, serverSvc: serverSvc, alertSvc: alertSvc, userSvc: userSvc}
}

// serverIDWithAccess parses :id and enforces the same server-scoping that
// ServerService.List applies to the collection endpoint. Without it, any
// authenticated user — including a viewer with no servers assigned — could read
// another tenant's server details, service status, metrics history and alerts
// just by changing the id in the URL.
func (h *ServerHandler) serverIDWithAccess(c *fiber.Ctx) (uint64, error) {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return 0, c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid server id"))
	}
	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)
	if !h.userSvc.HasServerAccess(userID, role, uint64(id)) {
		// 404 rather than 403: do not confirm that a server id exists to someone
		// who cannot see it.
		return 0, c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Server not found"))
	}
	return uint64(id), nil
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

	// One batched metrics lookup for the whole page instead of one query per server.
	serverIDs := make([]uint64, 0, len(servers))
	for _, s := range servers {
		serverIDs = append(serverIDs, s.ID)
	}
	metrics := h.latestMetricsFor(serverIDs)

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
			Metrics:       metrics[s.ID], // zero value when the server has no metrics yet
		}
		items = append(items, item)
	}

	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}

// Detail godoc
// GET /api/mobile/v1/servers/:id
func (h *ServerHandler) Detail(c *fiber.Ctx) error {
	id, accessErr := h.serverIDWithAccess(c)
	if accessErr != nil {
		return accessErr
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
	id, accessErr := h.serverIDWithAccess(c)
	if accessErr != nil {
		return accessErr
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
	id, accessErr := h.serverIDWithAccess(c)
	if accessErr != nil {
		return accessErr
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
	id, accessErr := h.serverIDWithAccess(c)
	if accessErr != nil {
		return accessErr
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

// latestMetricsFor returns the most recent metric row for each of serverIDs in a
// single query. List() used to call fillMetrics per server — an N+1 against
// server_metrics, which grows by ~1M rows per server per year. The cached
// cpu/ram/disk on the servers row cannot replace this outright because uptime_sec
// is not cached there, so batch the lookup instead of dropping the field.
func (h *ServerHandler) latestMetricsFor(serverIDs []uint64) map[uint64]models.MobileServerMetrics {
	out := make(map[uint64]models.MobileServerMetrics, len(serverIDs))
	if len(serverIDs) == 0 {
		return out
	}

	var rows []struct {
		ServerID  uint64
		CPUUsage  float64
		RAMUsage  float64
		DiskUsage float64
		UptimeSec int64
	}
	// ROW_NUMBER() picks the newest row per server in one pass; the
	// idx_sm_server_time (server_id, recorded_at) index feeds the partition
	// ordering directly.
	h.db.Raw(`
		SELECT server_id, cpu_usage, ram_usage, disk_usage, uptime_sec FROM (
			SELECT server_id, cpu_usage, ram_usage, disk_usage, uptime_sec,
			       ROW_NUMBER() OVER (PARTITION BY server_id ORDER BY recorded_at DESC) AS rn
			FROM server_metrics WHERE server_id IN ?
		) t WHERE rn = 1`, serverIDs).Scan(&rows)

	for _, r := range rows {
		out[r.ServerID] = models.MobileServerMetrics{
			CPUUsage:  r.CPUUsage,
			RAMUsage:  r.RAMUsage,
			DiskUsage: r.DiskUsage,
			UptimeSec: r.UptimeSec,
		}
	}
	return out
}
