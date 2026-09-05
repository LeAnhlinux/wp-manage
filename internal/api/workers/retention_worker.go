package workers

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// RetentionWorker prunes the tables that grow monotonically with normal operation.
// Before this worker, activity_logs was the only table in the schema with any
// retention at all; everything below grew forever:
//
//   - task_queue     — one row per operator action AND per read-only UI endpoint
//     that round-trips to an agent (firewall status, service list,
//     PHP versions, SSL status…). Rows carry an Output LONGTEXT
//     holding log tails and config dumps. This is also the table
//     every agent polls every 3 seconds, so its size directly
//     sets the cost of the hottest query in the system.
//   - server_metrics — 2 rows/minute/server (30s heartbeat) ≈ 1M rows/server/year.
//   - uptime_logs    — 1 row/minute per enabled check ≈ 525k rows/check/year.
//     UptimeService.CleanupOldLogs existed for this but had no
//     callers; it is wired up here.
//
// Deletes are batched: a single unbounded DELETE against a multi-million-row
// table holds row locks long enough to stall the agent poll it is meant to speed
// up. MariaDB frees the row space for reuse, though the file does not shrink
// without OPTIMIZE TABLE — that is left to the operator.
const (
	taskQueueRetentionDays     = 14
	serverMetricsRetentionDays = 30
	uptimeLogRetentionDays     = 30

	// deleteBatchSize caps rows per DELETE. Small enough to keep lock windows
	// short, large enough that a year of backlog clears in minutes.
	deleteBatchSize = 5000
	// maxBatchesPerRun bounds a single sweep so a huge first run cannot hammer the
	// DB indefinitely; the remainder is cleared on the following nights.
	maxBatchesPerRun = 200
)

type RetentionWorker struct {
	db *gorm.DB
}

func NewRetentionWorker(db *gorm.DB) *RetentionWorker {
	return &RetentionWorker{db: db}
}

func (w *RetentionWorker) Start() {
	go w.loop()
	log.Printf("[Retention] Worker started — daily at 03:00 (task_queue %dd, server_metrics %dd, uptime_logs %dd)",
		taskQueueRetentionDays, serverMetricsRetentionDays, uptimeLogRetentionDays)
}

func (w *RetentionWorker) loop() {
	for {
		now := time.Now()
		// 03:00 local — off the midnight slot ActivityCleanupWorker already uses, so
		// the two sweeps do not pile onto the DB at the same time.
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		log.Printf("[Retention] Next sweep in %v (at %s)", time.Until(next).Round(time.Minute), next.Format("2006-01-02 15:04"))
		time.Sleep(time.Until(next))
		w.RunOnce()
	}
}

// RunOnce performs one full sweep. Exported so it can be triggered directly.
func (w *RetentionWorker) RunOnce() {
	// Only finished tasks are eligible — a pending or processing row may still be
	// picked up by an agent.
	w.prune("task_queue",
		"DELETE FROM task_queue WHERE status IN ('completed','failed') AND updated_at < DATE_SUB(NOW(), INTERVAL ? DAY) LIMIT ?",
		taskQueueRetentionDays)

	w.prune("server_metrics",
		"DELETE FROM server_metrics WHERE recorded_at < DATE_SUB(NOW(), INTERVAL ? DAY) LIMIT ?",
		serverMetricsRetentionDays)

	w.prune("uptime_logs",
		"DELETE FROM uptime_logs WHERE created_at < DATE_SUB(NOW(), INTERVAL ? DAY) LIMIT ?",
		uptimeLogRetentionDays)
}

// prune runs a batched DELETE until nothing is left to remove, the batch cap is
// reached, or an error occurs.
func (w *RetentionWorker) prune(table, query string, retentionDays int) {
	var total int64
	for i := 0; i < maxBatchesPerRun; i++ {
		res := w.db.Exec(query, retentionDays, deleteBatchSize)
		if res.Error != nil {
			log.Printf("[Retention] %s sweep failed after %d rows: %v", table, total, res.Error)
			return
		}
		total += res.RowsAffected
		if res.RowsAffected < deleteBatchSize {
			break // fewer rows than requested means the backlog is drained
		}
		// Yield between batches so the agent poll and UI queries get the DB back.
		time.Sleep(200 * time.Millisecond)
	}
	if total > 0 {
		log.Printf("[Retention] %s: deleted %d rows older than %d days", table, total, retentionDays)
	}
}
