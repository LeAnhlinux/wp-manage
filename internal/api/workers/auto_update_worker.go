package workers

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// AutoUpdateWorker checks once per day (at ~3am) for sites that have auto-update
// enabled and queues apply_updates tasks accordingly.
type AutoUpdateWorker struct {
	db *gorm.DB
}

func NewAutoUpdateWorker(db *gorm.DB) *AutoUpdateWorker {
	return &AutoUpdateWorker{db: db}
}

func (w *AutoUpdateWorker) Start() {
	go func() {
		// Wait until the next 3:00 AM before first run, then repeat every 24h
		now := time.Now()
		next3am := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if now.After(next3am) {
			next3am = next3am.Add(24 * time.Hour)
		}
		delay := time.Until(next3am)
		log.Printf("[AutoUpdate] Worker started — first run in %v (at 03:00)", delay.Round(time.Minute))

		timer := time.NewTimer(delay)
		<-timer.C
		w.run()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			w.run()
		}
	}()
}

func (w *AutoUpdateWorker) run() {
	log.Println("[AutoUpdate] Running daily auto-update check...")

	// Find all auto-update configs with at least one update type enabled
	var configs []models.AutoUpdateConfig
	w.db.Preload("Site").Preload("Site.Server").
		Where("(core_updates = true OR plugin_updates = true OR theme_updates = true)").
		Find(&configs)

	queued := 0
	for _, cfg := range configs {
		if cfg.Site == nil || cfg.Site.Status != "active" {
			continue
		}
		site := cfg.Site

		taskData := models.AgentApplyUpdatesTask{
			SiteID:        site.ID,
			Domain:        site.Domain,
			PHPVersion:    site.PHPVersion,
			DBName:        site.DBName,
			DBUser:        site.DBUser,
			DBPass:        site.DBPassword,
			CoreUpdates:   cfg.CoreUpdates,
			PluginUpdates: cfg.PluginUpdates,
			ThemeUpdates:  cfg.ThemeUpdates,
			BackupBefore:  cfg.BackupBefore,
		}
		dataJSON, _ := json.Marshal(taskData)
		task := models.TaskQueue{
			ServerID: site.ServerID,
			SiteID:   site.ID,
			TaskType: "apply_updates",
			Data:     string(dataJSON),
			Status:   "pending",
		}
		if err := w.db.Create(&task).Error; err != nil {
			log.Printf("[AutoUpdate] Failed to queue task for site %s: %v", site.Domain, err)
			continue
		}
		queued++
		log.Printf("[AutoUpdate] Queued apply_updates for %s (core=%v plugins=%v themes=%v)",
			site.Domain, cfg.CoreUpdates, cfg.PluginUpdates, cfg.ThemeUpdates)
	}

	log.Printf("[AutoUpdate] Done — queued %d sites", queued)
}
