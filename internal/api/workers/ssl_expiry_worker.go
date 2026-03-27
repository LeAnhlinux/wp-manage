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

// autoRenewThresholdDays: queue auto-renewal when cert has ≤ this many days left.
// Let's Encrypt certs last 90 days; renew at 30 days = standard certbot behaviour.
const autoRenewThresholdDays = 30

type SSLExpiryWorker struct {
	db       *gorm.DB
	alertSvc *services.AlertService
}

func NewSSLExpiryWorker(db *gorm.DB, alertSvc *services.AlertService) *SSLExpiryWorker {
	return &SSLExpiryWorker{db: db, alertSvc: alertSvc}
}

func (w *SSLExpiryWorker) Start() {
	// Run once at startup, then every 12 hours (more responsive than daily)
	w.checkAll()

	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	slog.Info("SSL expiry worker started", "auto_renew_threshold_days", autoRenewThresholdDays)
	for range ticker.C {
		w.checkAll()
	}
}

func (w *SSLExpiryWorker) checkAll() {
	var sites []models.Site
	if err := w.db.Where("ssl_enabled = ? AND ssl_expires_at IS NOT NULL", true).Find(&sites).Error; err != nil {
		slog.Error("SSL expiry worker: failed to get sites", "error", err)
		return
	}

	now := time.Now()
	warningDays := []int{14, 7, 3, 1}

	for _, site := range sites {
		if site.SSLExpiresAt == nil {
			continue
		}

		daysLeft := int(site.SSLExpiresAt.Sub(now).Hours() / 24)

		// ── Auto-renew when ≤ 30 days remain ────────────────────────────────
		// Queue a renew_ssl task only if no pending/running renewal already exists
		// for this site (avoid duplicate tasks on every 12h tick).
		if daysLeft <= autoRenewThresholdDays {
			w.queueAutoRenew(site, daysLeft)
		}

		// ── Warning notifications (best-effort, non-blocking) ─────────────
		for _, warnDay := range warningDays {
			if daysLeft == warnDay {
				w.sendSSLWarning(site, daysLeft)
				break
			}
		}

		// Also warn if already expired
		if daysLeft <= 0 {
			w.sendSSLExpiredNotification(site)
		}
	}
}

// queueAutoRenew queues a renew_ssl task for the site, deduplicating against
// any task already pending or running for the same site.
func (w *SSLExpiryWorker) queueAutoRenew(site models.Site, daysLeft int) {
	// Dedup: skip if a renew_ssl task is already pending/running for this site
	var count int64
	w.db.Model(&models.TaskQueue{}).
		Where("site_id = ? AND task_type = ? AND status IN ('pending','running')", site.ID, "renew_ssl").
		Count(&count)
	if count > 0 {
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{"domain": site.Domain})
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   site.ID,
		TaskType: "renew_ssl",
		Data:     string(payload),
		Status:   "pending",
	}
	if err := w.db.Create(&task).Error; err != nil {
		slog.Error("SSL auto-renew: failed to queue task", "domain", site.Domain, "error", err)
		return
	}
	slog.Info("SSL auto-renew queued", "domain", site.Domain, "days_left", daysLeft, "task_id", task.ID)

	// Notify so user knows auto-renewal is happening
	w.sendAutoRenewNotification(site, daysLeft, task.ID)
}

func (w *SSLExpiryWorker) sendAutoRenewNotification(site models.Site, daysLeft int, taskID uint64) {
	var msg string
	if daysLeft <= 0 {
		msg = fmt.Sprintf("🔄 <b>SSL Auto-Renew</b> (hết hạn)\nDomain: <code>%s</code>\nĐã queue gia hạn SSL tự động (task #%d)", site.Domain, taskID)
	} else {
		msg = fmt.Sprintf("🔄 <b>SSL Auto-Renew</b>\nDomain: <code>%s</code>\nCòn %d ngày — đã queue gia hạn tự động (task #%d)", site.Domain, daysLeft, taskID)
	}
	w.sendNotification(msg)
}

func (w *SSLExpiryWorker) sendSSLWarning(site models.Site, daysLeft int) {
	message := fmt.Sprintf("🔒 <b>SSL Certificate Expiring</b>\nDomain: %s\nExpires in: %d day(s)\nExpiry date: %s",
		site.Domain, daysLeft, site.SSLExpiresAt.Format("2006-01-02"))

	slog.Warn("SSL certificate expiring", "domain", site.Domain, "days_left", daysLeft)
	w.sendNotification(message)
}

func (w *SSLExpiryWorker) sendSSLExpiredNotification(site models.Site) {
	message := fmt.Sprintf("🔴 <b>SSL Certificate EXPIRED</b>\nDomain: %s\nExpired at: %s\nPlease renew immediately!",
		site.Domain, site.SSLExpiresAt.Format("2006-01-02"))

	slog.Error("SSL certificate expired", "domain", site.Domain)
	w.sendNotification(message)
}

func (w *SSLExpiryWorker) sendNotification(message string) {
	cfg, err := w.alertSvc.GetNotificationConfig()
	if err != nil || cfg == nil {
		return
	}

	if cfg.TelegramToken != "" && cfg.TelegramChat != "" {
		if err := notify.SendTelegram(cfg.TelegramToken, cfg.TelegramChat, message); err != nil {
			slog.Error("SSL expiry: telegram failed", "error", err)
		}
	}

	if cfg.SlackWebhook != "" {
		if err := notify.SendSlack(cfg.SlackWebhook, message); err != nil {
			slog.Error("SSL expiry: slack failed", "error", err)
		}
	}

	if cfg.SMTPFrom != "" && cfg.SMTPHost != "" {
		smtpCfg := notify.SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			User: cfg.SMTPUser, Password: cfg.SMTPPass, From: cfg.SMTPFrom,
		}
		// Send to admin email
		if err := notify.SendEmail(smtpCfg, cfg.SMTPFrom, "SSL Certificate Alert", message); err != nil {
			slog.Error("SSL expiry: email failed", "error", err)
		}
	}
}
