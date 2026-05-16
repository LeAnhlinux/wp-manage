package workers

import (
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// CloudflareIPWorker syncs Cloudflare IP ranges to all servers that have
// Cloudflare-only firewall mode enabled. Runs every Monday at 02:00.
type CloudflareIPWorker struct {
	db  *gorm.DB
	svc *services.InfrastructureService
}

func NewCloudflareIPWorker(db *gorm.DB, svc *services.InfrastructureService) *CloudflareIPWorker {
	return &CloudflareIPWorker{db: db, svc: svc}
}

func (w *CloudflareIPWorker) Start() {
	slog.Info("[CloudflareIP] Worker started — syncs every Monday 02:00")
	go w.loop()
}

func (w *CloudflareIPWorker) loop() {
	// Check every hour whether it's time to sync
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		if now.Weekday() == time.Monday && now.Hour() == 2 {
			w.syncAll()
		}
	}
}

func (w *CloudflareIPWorker) syncAll() {
	slog.Info("[CloudflareIP] Starting weekly Cloudflare IP sync")

	var configs []models.CloudflareFirewallConfig
	if err := w.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		slog.Error("[CloudflareIP] Failed to query enabled configs", "error", err)
		return
	}

	if len(configs) == 0 {
		slog.Info("[CloudflareIP] No servers with Cloudflare protection enabled")
		return
	}

	for _, cfg := range configs {
		slog.Info("[CloudflareIP] Syncing server", "server_id", cfg.ServerID)
		if err := w.svc.SyncCloudflareFirewall(cfg.ServerID); err != nil {
			slog.Error("[CloudflareIP] Sync failed", "server_id", cfg.ServerID, "error", err)
		} else {
			slog.Info("[CloudflareIP] Sync completed", "server_id", cfg.ServerID)
		}
	}

	slog.Info("[CloudflareIP] Weekly sync done", "servers", len(configs))
}
