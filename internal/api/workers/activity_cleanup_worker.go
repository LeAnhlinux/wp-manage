package workers

import (
	"log"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
)

// ActivityCleanupWorker runs daily to remove old activity logs.
// Default retention: 90 days.
const defaultActivityRetentionDays = 90

type ActivityCleanupWorker struct {
	svc *services.ActivityService
}

func NewActivityCleanupWorker(svc *services.ActivityService) *ActivityCleanupWorker {
	return &ActivityCleanupWorker{svc: svc}
}

func (w *ActivityCleanupWorker) Start() {
	go func() {
		// Wait until next midnight before the first run, then repeat every 24h
		w.waitUntilMidnightAndRun()
	}()
	log.Printf("[ActivityCleanup] Worker started — runs daily at midnight (retention: %d days)", defaultActivityRetentionDays)
}

func (w *ActivityCleanupWorker) waitUntilMidnightAndRun() {
	for {
		now := time.Now()
		// Calculate duration until next midnight (local time)
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		sleepDuration := time.Until(nextMidnight)

		log.Printf("[ActivityCleanup] Next cleanup in %v (at %s)", sleepDuration.Round(time.Minute), nextMidnight.Format("2006-01-02 00:00:00"))
		time.Sleep(sleepDuration)

		w.runCleanup()
	}
}

func (w *ActivityCleanupWorker) runCleanup() {
	deleted, err := w.svc.Cleanup(defaultActivityRetentionDays)
	if err != nil {
		log.Printf("[ActivityCleanup] Cleanup failed: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("[ActivityCleanup] Deleted %d activity logs older than %d days", deleted, defaultActivityRetentionDays)
	}
}
