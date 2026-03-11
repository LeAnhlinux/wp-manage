package workers

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/notify"
	"gorm.io/gorm"
)

type SSLExpiryWorker struct {
	db       *gorm.DB
	alertSvc *services.AlertService
}

func NewSSLExpiryWorker(db *gorm.DB, alertSvc *services.AlertService) *SSLExpiryWorker {
	return &SSLExpiryWorker{db: db, alertSvc: alertSvc}
}

func (w *SSLExpiryWorker) Start() {
	// Run once at startup, then daily
	w.checkAll()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	slog.Info("SSL expiry worker started")
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
