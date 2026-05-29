package workers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/notify"
	"gorm.io/gorm"
)

const diskScanInterval = 6 * time.Hour

type DiskUsageWorker struct {
	db       *gorm.DB
	siteSvc  *services.SiteService
	alertSvc *services.AlertService
}

func NewDiskUsageWorker(db *gorm.DB, siteSvc *services.SiteService, alertSvc *services.AlertService) *DiskUsageWorker {
	return &DiskUsageWorker{db: db, siteSvc: siteSvc, alertSvc: alertSvc}
}

func (w *DiskUsageWorker) Start() {
	// Stagger the first run so startup isn't hammered by a full du sweep.
	time.Sleep(2 * time.Minute)
	w.scanAll()

	ticker := time.NewTicker(diskScanInterval)
	defer ticker.Stop()
	slog.Info("Disk usage worker started", "interval", diskScanInterval.String())
	for range ticker.C {
		w.scanAll()
	}
}

// scanAll groups active sites by server, runs one batch scan per server, then
// updates usage + handles limit-breach transitions.
func (w *DiskUsageWorker) scanAll() {
	var sites []models.Site
	if err := w.db.Where("status = ?", "active").Find(&sites).Error; err != nil {
		slog.Error("disk worker: list sites failed", "error", err)
		return
	}

	byServer := map[uint64][]models.Site{}
	for _, s := range sites {
		byServer[s.ServerID] = append(byServer[s.ServerID], s)
	}

	for serverID, srvSites := range byServer {
		w.scanServer(serverID, srvSites)
	}
}

func (w *DiskUsageWorker) scanServer(serverID uint64, sites []models.Site) {
	scanList := make([]map[string]string, 0, len(sites))
	for _, s := range sites {
		scanList = append(scanList, map[string]string{"domain": s.Domain, "db_name": s.DBName})
	}

	task, err := w.siteSvc.CreateJSONTaskReturn(serverID, 0, "scan_disk_usage",
		map[string]interface{}{"sites": scanList})
	if err != nil {
		slog.Error("disk worker: queue scan failed", "server", serverID, "error", err)
		return
	}

	result, err := services.WaitForTaskCompleteWithTimeout(w.db, task.ID, 10*time.Minute)
	if err != nil {
		slog.Warn("disk worker: scan timed out", "server", serverID, "error", err)
		return
	}
	if result.Error != "" {
		slog.Warn("disk worker: scan failed on agent", "server", serverID, "error", result.Error)
		return
	}

	var scan []models.DiskScanSiteResult
	if err := json.Unmarshal([]byte(result.Output), &scan); err != nil {
		slog.Error("disk worker: bad scan output", "server", serverID, "error", err)
		return
	}

	// Index sites by domain for quick lookup.
	byDomain := map[string]models.Site{}
	for _, s := range sites {
		byDomain[s.Domain] = s
	}

	for _, r := range scan {
		site, ok := byDomain[r.Domain]
		if !ok {
			continue
		}
		usedMB := uint64((r.FilesBytes + r.DBBytes) / (1024 * 1024))
		oldState := site.DiskState
		newState := w.siteSvc.ApplyDiskUsage(site.ID, usedMB, site.DiskLimitMB)
		if newState != oldState {
			w.handleTransition(site, oldState, newState, usedMB)
		}
	}
}

// handleTransition acts only on state changes (no spam). warning/over → notify;
// over + action=suspend → suspend the site.
func (w *DiskUsageWorker) handleTransition(site models.Site, oldState, newState string, usedMB uint64) {
	limit := uint64(0)
	if site.DiskLimitMB != nil {
		limit = *site.DiskLimitMB
	}

	switch newState {
	case "warning":
		w.notify(fmt.Sprintf("⚠️ Disk warning: %s đang dùng %d/%d MB (≥80%%).", site.Domain, usedMB, limit))
	case "over":
		w.notify(fmt.Sprintf("🔴 Disk OVER LIMIT: %s dùng %d/%d MB.", site.Domain, usedMB, limit))
		if site.OverLimitAction == "suspend" {
			if err := w.siteSvc.CreateTask(site.ServerID, site.ID, "suspend_site",
				map[string]string{"domain": site.Domain}); err == nil {
				w.siteSvc.UpdateStatus(site.ID, "suspended")
				w.notify(fmt.Sprintf("⛔ %s đã bị tự động SUSPEND do vượt giới hạn disk.", site.Domain))
			}
		}
	}
}

func (w *DiskUsageWorker) notify(message string) {
	cfg, err := w.alertSvc.GetNotificationConfig()
	if err != nil || cfg == nil {
		return
	}
	if cfg.TelegramToken != "" && cfg.TelegramChat != "" {
		if err := notify.SendTelegram(cfg.TelegramToken, cfg.TelegramChat, message); err != nil {
			slog.Error("disk worker: telegram failed", "error", err)
		}
	}
	if cfg.SlackWebhook != "" {
		notify.SendSlack(cfg.SlackWebhook, message)
	}
	if cfg.SMTPFrom != "" && cfg.SMTPHost != "" {
		smtpCfg := notify.SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			User: cfg.SMTPUser, Password: cfg.SMTPPass, From: cfg.SMTPFrom,
		}
		notify.SendEmail(smtpCfg, cfg.SMTPFrom, "Disk Limit Alert", message)
	}
}
