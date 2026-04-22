package workers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	ws "github.com/gofiber-team/wp-manage/internal/api/websocket"
	"github.com/gofiber-team/wp-manage/pkg/notify"
)

type UptimeWorker struct {
	uptimeSvc *services.UptimeService
	alertSvc  *services.AlertService
	hub       *ws.Hub
}

func NewUptimeWorker(uptimeSvc *services.UptimeService, alertSvc *services.AlertService, hub *ws.Hub) *UptimeWorker {
	return &UptimeWorker{uptimeSvc: uptimeSvc, alertSvc: alertSvc, hub: hub}
}

func (w *UptimeWorker) Start() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	slog.Info("Uptime worker started")
	for range ticker.C {
		w.checkAll()
	}
}

func (w *UptimeWorker) checkAll() {
	checks, err := w.uptimeSvc.GetAllEnabled()
	if err != nil {
		slog.Error("Uptime worker: failed to get checks", "error", err)
		return
	}

	for _, check := range checks {
		go w.performCheck(check.ID, check.URL, check.SiteID)
	}
}

func (w *UptimeWorker) performCheck(checkID uint64, url string, siteID uint64) {
	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(url)
	responseMs := int(time.Since(start).Milliseconds())

	var statusCode int
	var errMsg string

	if err != nil {
		statusCode = 0
		errMsg = err.Error()
	} else {
		statusCode = resp.StatusCode
		resp.Body.Close()
	}

	// Log result
	if logErr := w.uptimeSvc.LogResult(checkID, statusCode, responseMs, errMsg); logErr != nil {
		slog.Error("Failed to log uptime result", "check_id", checkID, "error", logErr)
	}

	// Broadcast via WebSocket if down
	if statusCode == 0 || statusCode >= 500 {
		w.hub.Broadcast("monitoring", "uptime_alert", map[string]interface{}{
			"check_id":    checkID,
			"site_id":     siteID,
			"url":         url,
			"status_code": statusCode,
			"error":       errMsg,
			"response_ms": responseMs,
		})

		// Send notification
		w.sendDownNotification(url, statusCode, errMsg)
	}
}

func (w *UptimeWorker) sendDownNotification(url string, statusCode int, errMsg string) {
	cfg, err := w.alertSvc.GetNotificationConfig()
	if err != nil || cfg == nil {
		return
	}

	message := fmt.Sprintf("🔴 <b>Site Down Alert</b>\nURL: %s\nStatus: %d\nError: %s\nTime: %s",
		url, statusCode, errMsg, time.Now().Format("2006-01-02 15:04:05"))

	if cfg.TelegramToken != "" && cfg.TelegramChat != "" {
		if err := notify.SendTelegram(cfg.TelegramToken, cfg.TelegramChat, message); err != nil {
			slog.Error("Uptime: telegram notification failed", "error", err)
		}
	}

}
