package workers

import (
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/cron"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// BackupScheduleWorker checks every minute for due backup schedules and queues them.
// It uses the next_run_at field to avoid parsing cron expressions on every tick.
type BackupScheduleWorker struct {
	db  *gorm.DB
	svc *services.BackupService
}

func NewBackupScheduleWorker(db *gorm.DB, svc *services.BackupService) *BackupScheduleWorker {
	return &BackupScheduleWorker{db: db, svc: svc}
}

func (w *BackupScheduleWorker) Start() {
	go func() {
		// On startup: seed next_run_at for schedules that don't have it set yet
		w.seedNextRunAt()

		time.Sleep(20 * time.Second) // stagger from other workers
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			w.runDue()
		}
	}()
	slog.Info("[BackupSchedule] Worker started (every 60s)")
}

// seedNextRunAt sets next_run_at for any schedule where it's NULL.
// Called once on startup so existing schedules get scheduled immediately.
func (w *BackupScheduleWorker) seedNextRunAt() {
	var schedules []models.BackupSchedule
	w.db.Where("enabled = ? AND next_run_at IS NULL", true).Find(&schedules)
	now := time.Now()
	for _, s := range schedules {
		next, err := cron.Next(s.CronExpr, now)
		if err != nil {
			slog.Warn("[BackupSchedule] Invalid cron_expr on seed", "id", s.ID, "expr", s.CronExpr, "err", err)
			continue
		}
		w.db.Model(&s).Update("next_run_at", next)
		slog.Info("[BackupSchedule] Seeded next_run_at", "schedule_id", s.ID, "site_id", s.SiteID, "next", next.Format(time.RFC3339))
	}
}

// runDue finds all schedules whose next_run_at has passed and queues a backup task.
func (w *BackupScheduleWorker) runDue() {
	var schedules []models.BackupSchedule
	now := time.Now()
	if err := w.db.
		Preload("Site").
		Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).
		Find(&schedules).Error; err != nil {
		slog.Warn("[BackupSchedule] DB query failed", "err", err)
		return
	}

	for _, sched := range schedules {
		w.triggerBackup(sched, now)
	}
}

func (w *BackupScheduleWorker) triggerBackup(sched models.BackupSchedule, now time.Time) {
	site := sched.Site
	if site == nil {
		// Lazy load site if Preload missed it
		var s models.Site
		if err := w.db.First(&s, sched.SiteID).Error; err != nil {
			slog.Warn("[BackupSchedule] Site not found", "schedule_id", sched.ID, "site_id", sched.SiteID)
			w.advanceNextRun(sched, now)
			return
		}
		site = &s
	}

	if site.Status != "active" {
		slog.Info("[BackupSchedule] Skipping — site not active", "site_id", site.ID, "status", site.Status)
		w.advanceNextRun(sched, now)
		return
	}

	// Dedup: skip if a backup of the same type is already running for this site
	var inFlight int64
	w.db.Model(&models.Backup{}).
		Where("site_id = ? AND type = ? AND status = ?", sched.SiteID, sched.Type, "running").
		Count(&inFlight)
	if inFlight > 0 {
		slog.Info("[BackupSchedule] Skipping — backup already running", "site_id", sched.SiteID, "type", sched.Type)
		// Don't advance — retry on next tick (backup may finish by then)
		return
	}

	// Create backup DB record + queue agent task
	payload := models.CreateBackupRequest{
		Type:    sched.Type,
		Storage: sched.Storage,
	}
	backup, err := w.svc.CreateBackup(sched.SiteID, payload)
	if err != nil {
		slog.Error("[BackupSchedule] Failed to queue backup", "schedule_id", sched.ID, "site_id", sched.SiteID, "err", err)
		w.advanceNextRun(sched, now)
		return
	}

	slog.Info("[BackupSchedule] Backup queued",
		"schedule_id", sched.ID, "site_id", sched.SiteID,
		"backup_id", backup.ID, "type", sched.Type)

	w.advanceNextRun(sched, now)
}

// advanceNextRun calculates the next occurrence after now and saves it.
func (w *BackupScheduleWorker) advanceNextRun(sched models.BackupSchedule, now time.Time) {
	next, err := cron.Next(sched.CronExpr, now)
	if err != nil {
		slog.Warn("[BackupSchedule] Cannot advance next_run_at — invalid cron", "id", sched.ID, "expr", sched.CronExpr, "err", err)
		return
	}
	w.db.Model(&sched).Updates(map[string]interface{}{
		"last_run_at": now,
		"next_run_at": next,
	})
}

