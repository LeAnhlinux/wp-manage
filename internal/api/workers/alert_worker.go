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

type AlertWorker struct {
	db       *gorm.DB
	alertSvc *services.AlertService
}

func NewAlertWorker(db *gorm.DB, alertSvc *services.AlertService) *AlertWorker {
	return &AlertWorker{db: db, alertSvc: alertSvc}
}

// CheckServerMetrics is called on each heartbeat to evaluate alert rules
func (w *AlertWorker) CheckServerMetrics(serverID uint64, cpuUsage, ramUsage, diskUsage float64) {
	rules, err := w.alertSvc.ListRules(serverID)
	if err != nil {
		slog.Error("Alert worker: failed to get rules", "server_id", serverID, "error", err)
		return
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		var currentValue float64
		switch rule.Metric {
		case "cpu":
			currentValue = cpuUsage
		case "ram":
			currentValue = ramUsage
		case "disk":
			currentValue = diskUsage
		default:
			continue
		}

		if currentValue >= rule.Threshold {
			// Check cooldown
			if rule.LastFired != nil {
				cooldownDuration := time.Duration(rule.CooldownMin) * time.Minute
				if time.Since(*rule.LastFired) < cooldownDuration {
					continue
				}
			}

			w.fireAlert(rule, currentValue)
		}
	}
}

func (w *AlertWorker) fireAlert(rule models.AlertRule, value float64) {
	serverName := "Unknown"
	if rule.Server != nil {
		serverName = rule.Server.Name
	} else {
		var server models.Server
		if err := w.db.First(&server, rule.ServerID).Error; err == nil {
			serverName = server.Name
		}
	}

	message := fmt.Sprintf("⚠️ Alert: %s usage at %.1f%% (threshold: %.1f%%)\nServer: %s",
		rule.Metric, value, rule.Threshold, serverName)

	sent := w.sendNotification(rule, message)

	if err := w.alertSvc.RecordAlert(rule.ID, rule.ServerID, rule.Metric, value, message, rule.Channel, sent); err != nil {
		slog.Error("Failed to record alert", "rule_id", rule.ID, "error", err)
	}

	slog.Warn("Alert fired", "server", serverName, "metric", rule.Metric, "value", value, "threshold", rule.Threshold)
}

func (w *AlertWorker) sendNotification(rule models.AlertRule, message string) bool {
	cfg, err := w.alertSvc.GetNotificationConfig()
	if err != nil {
		slog.Error("Failed to get notification config", "error", err)
		return false
	}

	switch rule.Channel {
	case "telegram":
		token := cfg.TelegramToken
		chatID := cfg.TelegramChat
		if err := notify.SendTelegram(token, chatID, message); err != nil {
			slog.Error("Alert telegram failed", "error", err)
			return false
		}
		return true

	case "slack":
		webhookURL := rule.WebhookURL
		if webhookURL == "" {
			webhookURL = cfg.SlackWebhook
		}
		if err := notify.SendSlack(webhookURL, message); err != nil {
			slog.Error("Alert slack failed", "error", err)
			return false
		}
		return true

	case "email":
		if rule.Email == "" {
			return false
		}
		smtpCfg := notify.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPass,
			From:     cfg.SMTPFrom,
		}
		if err := notify.SendEmail(smtpCfg, rule.Email, "WP Manage Alert", message); err != nil {
			slog.Error("Alert email failed", "error", err)
			return false
		}
		return true
	}

	return false
}
