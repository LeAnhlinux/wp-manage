package workers

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type BackupRotationWorker struct {
	db *gorm.DB
}

func NewBackupRotationWorker(db *gorm.DB) *BackupRotationWorker {
	return &BackupRotationWorker{db: db}
}

func (w *BackupRotationWorker) Start() {
	go func() {
		// Run immediately on start, then every 6 hours
		w.cleanup()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			w.cleanup()
		}
	}()
	log.Println("[BackupRotation] Worker started (every 6 hours)")
}

func (w *BackupRotationWorker) cleanup() {
	log.Println("[BackupRotation] Running backup rotation cleanup...")

	// Get all schedules with retention
	var schedules []models.BackupSchedule
	w.db.Where("enabled = ? AND retention > 0", true).Find(&schedules)

	totalDeleted := 0

	for _, schedule := range schedules {
		// Find completed backups for this site that exceed retention
		var backups []models.Backup
		w.db.Where("site_id = ? AND type = ? AND status = ?",
			schedule.SiteID, schedule.Type, "completed").
			Order("created_at DESC").
			Find(&backups)

		if len(backups) <= schedule.Retention {
			continue
		}

		// Delete backups beyond retention count
		toDelete := backups[schedule.Retention:]
		for _, backup := range toDelete {
			// Use backup.ServerID directly — SiteID may be nil (orphaned backup)
			// and looking up the site is unnecessary since we already have the server.
			if backup.FilePath != "" && backup.ServerID > 0 {
				payload := map[string]interface{}{
					"file_path": backup.FilePath,
				}
				dataJSON, _ := json.Marshal(payload)
				var taskSiteID uint64
				if backup.SiteID != nil {
					taskSiteID = *backup.SiteID
				}
				task := models.TaskQueue{
					ServerID: backup.ServerID,
					SiteID:   taskSiteID,
					TaskType: "delete_backup_file",
					Data:     string(dataJSON),
					Status:   "pending",
				}
				w.db.Create(&task)
			}

			// Remove DB record
			w.db.Delete(&backup)
			totalDeleted++
		}
	}

	if totalDeleted > 0 {
		log.Printf("[BackupRotation] Cleaned up %d old backups", totalDeleted)
	}
}
