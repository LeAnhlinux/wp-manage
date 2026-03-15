package workers

import (
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// ScanScheduleWorker checks every minute for due malware scans and triggers them.
type ScanScheduleWorker struct {
	db  *gorm.DB
	svc *services.SecurityService
}

func NewScanScheduleWorker(db *gorm.DB, svc *services.SecurityService) *ScanScheduleWorker {
	return &ScanScheduleWorker{db: db, svc: svc}
}

func (w *ScanScheduleWorker) Start() {
	// Randomise first tick slightly to avoid thundering herd with other workers
	time.Sleep(15 * time.Second)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.runDueScans()
	}
}

func (w *ScanScheduleWorker) runDueScans() {
	var schedules []models.MalwareScanSchedule
	now := time.Now()
	if err := w.db.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).Find(&schedules).Error; err != nil {
		slog.Warn("ScanScheduleWorker: failed to query schedules", "err", err)
		return
	}

	for _, sched := range schedules {
		slog.Info("ScanScheduleWorker: triggering scheduled malware scan",
			"schedule_id", sched.ID, "site_id", sched.SiteID,
			"scan_type", sched.ScanType, "engine", sched.Engine)

		_, err := w.svc.StartMalwareScan(sched.SiteID, models.StartMalwareScanRequest{
			ScanType: sched.ScanType,
			Engine:   sched.Engine,
		})
		if err != nil {
			slog.Warn("ScanScheduleWorker: failed to start scan", "site_id", sched.SiteID, "err", err)
		}

		// Advance next_run_at regardless of scan start outcome, to avoid hammering failed sites
		if err := w.svc.MarkScheduleRan(sched.ID, sched.IntervalH); err != nil {
			slog.Warn("ScanScheduleWorker: failed to advance next_run_at", "id", sched.ID, "err", err)
		}
	}
}
