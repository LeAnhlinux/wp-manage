package mobile

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// DashboardHandler handles GET /api/mobile/v1/dashboard.
// It fans out to multiple services concurrently and returns a single
// combined response, replacing ~8 separate web-panel API calls.
type DashboardHandler struct {
	db          *gorm.DB
	serverSvc   *services.ServerService
	siteSvc     *services.SiteService
	uptimeSvc   *services.UptimeService
	alertSvc    *services.AlertService
	activitySvc *services.ActivityService
	backupSvc   *services.BackupService
	migSvc      *services.MigrationService
	scanSvc     *services.SecurityService
}

func NewDashboardHandler(
	db *gorm.DB,
	serverSvc *services.ServerService,
	siteSvc *services.SiteService,
	uptimeSvc *services.UptimeService,
	alertSvc *services.AlertService,
	activitySvc *services.ActivityService,
	backupSvc *services.BackupService,
	migSvc *services.MigrationService,
	scanSvc *services.SecurityService,
) *DashboardHandler {
	return &DashboardHandler{
		db: db, serverSvc: serverSvc, siteSvc: siteSvc,
		uptimeSvc: uptimeSvc, alertSvc: alertSvc,
		activitySvc: activitySvc, backupSvc: backupSvc,
		migSvc: migSvc, scanSvc: scanSvc,
	}
}

// Overview godoc
// GET /api/mobile/v1/dashboard
// Returns a combined dashboard object with summary, servers, alerts, uptime down, recent activity.
func (h *DashboardHandler) Overview(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	var (
		servers        []models.Server
		allSites       []models.Site
		allUptime      []models.UptimeCheck
		alertHistory   []models.AlertHistory
		activity       []models.ActivityLog
		backupsFailed  int64
		threats7d      int64
		migsRunning    int64
		wg             sync.WaitGroup
	)

	wg.Add(6)

	go func() {
		defer wg.Done()
		servers, _, _ = h.serverSvc.List(userID, role, 1, 200)
	}()

	go func() {
		defer wg.Done()
		allSites, _, _ = h.siteSvc.List(userID, role, 0, 1, 500)
	}()

	go func() {
		defer wg.Done()
		allUptime, _ = h.uptimeSvc.ListAll()
	}()

	go func() {
		defer wg.Done()
		alertHistory, _ = h.alertSvc.GetHistory(0, 5) // 0 = all servers, last 5
	}()

	go func() {
		defer wg.Done()
		activity, _, _ = h.activitySvc.List(nil, nil, nil, "", nil, nil, 1, 10)
	}()

	go func() {
		defer wg.Done()
		since24h := time.Now().Add(-24 * time.Hour)
		h.db.Model(&models.Backup{}).Where("status = ? AND created_at > ?", "failed", since24h).Count(&backupsFailed)

		since7d := time.Now().Add(-7 * 24 * time.Hour)
		h.db.Model(&models.MalwareScan{}).Where("threats_found > 0 AND started_at > ?", since7d).Count(&threats7d)

		h.db.Model(&models.Migration{}).Where("status = ?", "running").Count(&migsRunning)
	}()

	wg.Wait()

	// ── Summary ──
	summary := buildSummary(servers, allSites, allUptime, int(backupsFailed), int(threats7d), int(migsRunning))

	// ── Server items ──
	serverItems := make([]models.MobileServerItem, 0, len(servers))
	for _, s := range servers {
		item := models.MobileServerItem{
			ID:           s.ID,
			Name:         s.Name,
			IPAddress:    s.IPAddress,
			Status:       s.Status,
			AgentVersion: s.AgentVersion,
			OS:           s.OS,
			LastHeartbeat: s.LastHeartbeat,
			SitesCount:   len(s.Sites),
		}
		// Metrics come straight off the server row: UpdateHeartbeat caches the
		// latest cpu/ram/disk there on every heartbeat, precisely so the dashboard
		// does not have to touch the time-series table. This used to run one
		// ORDER BY ... LIMIT 1 query per server against server_metrics — an N+1 that
		// returned these exact numbers from a table that grows by ~1M rows/server/year.
		item.Metrics = models.MobileServerMetrics{
			CPUUsage:  s.CPUUsage,
			RAMUsage:  s.RAMUsage,
			DiskUsage: s.DiskUsage,
		}
		serverItems = append(serverItems, item)
	}

	// ── Alerts ──
	alertItems := make([]models.MobileAlertItem, 0, len(alertHistory))
	for _, a := range alertHistory {
		serverName := ""
		for _, sv := range servers {
			if sv.ID == a.ServerID {
				serverName = sv.Name
				break
			}
		}
		alertItems = append(alertItems, models.MobileAlertItem{
			ID: a.ID, ServerID: a.ServerID, ServerName: serverName,
			Metric: a.Metric, Value: a.Value, Message: a.Message, CreatedAt: a.CreatedAt,
		})
	}

	// ── Uptime down ──
	uptimeDown := make([]models.MobileUptimeItem, 0)
	for _, u := range allUptime {
		if u.LastStatus == 0 || (u.LastStatus >= 400) || u.LastStatus == -1 {
			item := models.MobileUptimeItem{
				ID: u.ID, URL: u.URL, SiteID: u.SiteID,
				LastStatus: u.LastStatus, LastCheck: u.LastCheck,
			}
			if u.Site != nil {
				item.SiteDomain = u.Site.Domain
			}
			// avg response from last 10 logs
			var avgMs float64
			h.db.Raw("SELECT AVG(response_ms) FROM uptime_logs WHERE check_id = ? ORDER BY created_at DESC LIMIT 10", u.ID).Scan(&avgMs)
			item.ResponseMsAvg = int(avgMs)
			uptimeDown = append(uptimeDown, item)
		}
	}

	// ── Recent activity ──
	actItems := make([]models.MobileActivityItem, 0, len(activity))
	for _, a := range activity {
		actItems = append(actItems, models.MobileActivityItem{
			ID: a.ID, Action: a.Action, Details: a.Details,
			ServerID: a.ServerID, SiteID: a.SiteID, CreatedAt: a.CreatedAt,
		})
	}

	resp := models.MobileDashboardResponse{
		Summary:        summary,
		Servers:        serverItems,
		Alerts:         alertItems,
		UptimeDown:     uptimeDown,
		RecentActivity: actItems,
	}

	return c.JSON(models.MobileOK(resp))
}

func buildSummary(
	servers []models.Server,
	sites []models.Site,
	uptime []models.UptimeCheck,
	backupsFailed, threats7d, migsRunning int,
) models.MobileDashboardSummary {
	s := models.MobileDashboardSummary{
		ServersTotal:      len(servers),
		SitesTotal:        len(sites),
		BackupsFailed24h:  backupsFailed,
		ThreatsFound7d:    threats7d,
		MigrationsRunning: migsRunning,
	}
	for _, sv := range servers {
		if sv.Status == "active" {
			s.ServersOnline++
		} else {
			s.ServersOffline++
		}
	}
	for _, si := range sites {
		switch si.Status {
		case "active":
			s.SitesActive++
		case "suspended":
			s.SitesSuspended++
		case "error":
			s.SitesError++
		}
	}
	for _, u := range uptime {
		if u.Enabled && (u.LastStatus == 0 || u.LastStatus >= 400) {
			s.UptimeChecksDown++
		}
	}
	return s
}
