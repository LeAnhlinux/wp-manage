package mobile

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// MonitoringHandler handles uptime, alerts, and activity log endpoints.
type MonitoringHandler struct {
	db          *gorm.DB
	uptimeSvc   *services.UptimeService
	alertSvc    *services.AlertService
	activitySvc *services.ActivityService
	serverSvc   *services.ServerService
}

func NewMonitoringHandler(
	db *gorm.DB,
	uptimeSvc *services.UptimeService,
	alertSvc *services.AlertService,
	activitySvc *services.ActivityService,
	serverSvc *services.ServerService,
) *MonitoringHandler {
	return &MonitoringHandler{
		db: db, uptimeSvc: uptimeSvc, alertSvc: alertSvc,
		activitySvc: activitySvc, serverSvc: serverSvc,
	}
}

// ─── Uptime ───────────────────────────────────────────────────────────────────

// ListUptime godoc
// GET /api/mobile/v1/monitoring/uptime
func (h *MonitoringHandler) ListUptime(c *fiber.Ctx) error {
	checks, err := h.uptimeSvc.ListAll()
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	items := make([]models.MobileUptimeItem, 0, len(checks))
	for _, u := range checks {
		item := models.MobileUptimeItem{
			ID: u.ID, URL: u.URL, SiteID: u.SiteID,
			LastStatus: u.LastStatus, LastCheck: u.LastCheck,
		}
		if u.Site != nil {
			item.SiteDomain = u.Site.Domain
		}
		var avgMs float64
		h.db.Raw("SELECT COALESCE(AVG(response_ms),0) FROM uptime_logs WHERE check_id = ? ORDER BY created_at DESC LIMIT 10", u.ID).Scan(&avgMs)
		item.ResponseMsAvg = int(avgMs)
		items = append(items, item)
	}

	return c.JSON(models.MobileOK(items))
}

// UptimeHistory godoc
// GET /api/mobile/v1/monitoring/uptime/:checkId/history?limit=50
func (h *MonitoringHandler) UptimeHistory(c *fiber.Ctx) error {
	checkID, err := c.ParamsInt("checkId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid check id"))
	}
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}

	logs, err := h.uptimeSvc.GetHistory(uint64(checkID), limit)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(logs))
}

// ToggleUptime godoc
// POST /api/mobile/v1/monitoring/uptime/:checkId/toggle
func (h *MonitoringHandler) ToggleUptime(c *fiber.Ctx) error {
	checkID, err := c.ParamsInt("checkId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid check id"))
	}

	var check models.UptimeCheck
	if err := h.db.First(&check, checkID).Error; err != nil {
		return c.Status(404).JSON(models.MobileErr("NOT_FOUND", "Uptime check not found"))
	}

	newEnabled := !check.Enabled
	if err := h.db.Model(&check).Update("enabled", newEnabled).Error; err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	return c.JSON(models.MobileOK(fiber.Map{"enabled": newEnabled}))
}

// CreateUptimeCheck godoc
// POST /api/mobile/v1/monitoring/uptime
func (h *MonitoringHandler) CreateUptimeCheck(c *fiber.Ctx) error {
	var req models.CreateUptimeCheckRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid request body"))
	}
	if req.URL == "" {
		return c.Status(400).JSON(models.MobileErr("VALIDATION_ERROR", "url is required"))
	}

	var siteID uint64
	if req.SiteID > 0 {
		siteID = req.SiteID
	} else if sid := c.QueryInt("site_id", 0); sid > 0 {
		siteID = uint64(sid)
	}
	check, err := h.uptimeSvc.Create(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", err.Error()))
	}
	return c.Status(201).JSON(models.MobileOK(check))
}

// DeleteUptimeCheck godoc
// DELETE /api/mobile/v1/monitoring/uptime/:checkId
func (h *MonitoringHandler) DeleteUptimeCheck(c *fiber.Ctx) error {
	checkID, err := c.ParamsInt("checkId")
	if err != nil {
		return c.Status(400).JSON(models.MobileErr("BAD_REQUEST", "Invalid check id"))
	}
	if err := h.uptimeSvc.Delete(uint64(checkID)); err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}
	return c.JSON(models.MobileOK(fiber.Map{"deleted": true}))
}

// ─── Alerts ───────────────────────────────────────────────────────────────────

// AlertHistory godoc
// GET /api/mobile/v1/alerts/history?page=1&per_page=20
func (h *MonitoringHandler) AlertHistoryAll(c *fiber.Ctx) error {
	limit := c.QueryInt("per_page", 20)
	if limit > 100 {
		limit = 100
	}

	// GetHistory with serverID=0 returns all servers
	history, err := h.alertSvc.GetHistory(0, limit)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	// Enrich with server name
	type enrichedAlert struct {
		models.AlertHistory
		ServerName string `json:"server_name"`
	}
	result := make([]enrichedAlert, 0, len(history))
	// Cache server names to avoid N+1
	serverNames := map[uint64]string{}
	for _, a := range history {
		if _, ok := serverNames[a.ServerID]; !ok {
			var s models.Server
			if h.db.Select("name").First(&s, a.ServerID).Error == nil {
				serverNames[a.ServerID] = s.Name
			}
		}
		result = append(result, enrichedAlert{AlertHistory: a, ServerName: serverNames[a.ServerID]})
	}

	return c.JSON(models.MobileOK(result))
}

// AlertRulesAll godoc
// GET /api/mobile/v1/alerts/rules
func (h *MonitoringHandler) AlertRulesAll(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	// Get accessible servers first
	servers, _, _ := h.serverSvc.List(userID, role, 1, 200)

	type ruleItem struct {
		models.AlertRule
		ServerName string `json:"server_name"`
	}
	allRules := make([]ruleItem, 0)
	for _, sv := range servers {
		rules, _ := h.alertSvc.ListRules(sv.ID)
		for _, r := range rules {
			allRules = append(allRules, ruleItem{AlertRule: r, ServerName: sv.Name})
		}
	}
	return c.JSON(models.MobileOK(allRules))
}

// ─── Activity Log ────────────────────────────────────────────────────────────

// ActivityLog godoc
// GET /api/mobile/v1/activity?page=1&per_page=20&action=&server_id=&site_id=&from=&to=
func (h *MonitoringHandler) ActivityLog(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if perPage > 100 {
		perPage = 100
	}

	action := c.Query("action")
	var serverID, siteID *uint64
	if sid := uint64(c.QueryInt("server_id", 0)); sid > 0 {
		serverID = &sid
	}
	if sid := uint64(c.QueryInt("site_id", 0)); sid > 0 {
		siteID = &sid
	}

	logs, total, err := h.activitySvc.List(nil, serverID, siteID, action, nil, nil, page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.MobileErr("SERVER_ERROR", err.Error()))
	}

	items := make([]models.MobileActivityItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, models.MobileActivityItem{
			ID: l.ID, Action: l.Action, Details: l.Details,
			ServerID: l.ServerID, SiteID: l.SiteID, CreatedAt: l.CreatedAt,
		})
	}

	return c.JSON(models.MobileOKPage(items, page, perPage, total))
}
