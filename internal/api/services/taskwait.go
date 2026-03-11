package services

import (
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

const (
	taskPollInterval = 500 * time.Millisecond
	taskTimeout      = 60 * time.Second
)

// WaitForTaskComplete polls the TaskQueue until the task is no longer pending/processing
func WaitForTaskComplete(db *gorm.DB, taskID uint64) (*models.TaskQueue, error) {
	deadline := time.Now().Add(taskTimeout)
	for time.Now().Before(deadline) {
		var task models.TaskQueue
		if err := db.First(&task, taskID).Error; err != nil {
			return nil, err
		}
		if task.Status != "pending" && task.Status != "processing" {
			return &task, nil
		}
		time.Sleep(taskPollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for task to complete")
}

// WaitForTaskCompleteWithTimeout polls the TaskQueue with a custom timeout
func WaitForTaskCompleteWithTimeout(db *gorm.DB, taskID uint64, timeout time.Duration) (*models.TaskQueue, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var task models.TaskQueue
		if err := db.First(&task, taskID).Error; err != nil {
			return nil, err
		}
		if task.Status != "pending" && task.Status != "processing" {
			return &task, nil
		}
		time.Sleep(taskPollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for task to complete")
}

// WaitForWPCLIResult polls WPCLIHistory until status != "running"
func WaitForWPCLIResult(db *gorm.DB, historyID uint64) (*models.WPCLIHistory, error) {
	deadline := time.Now().Add(taskTimeout)
	for time.Now().Before(deadline) {
		var h models.WPCLIHistory
		if err := db.First(&h, historyID).Error; err != nil {
			return nil, err
		}
		if h.Status != "running" {
			return &h, nil
		}
		time.Sleep(taskPollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for WP-CLI result")
}

// WaitForImageOptResult polls ImageOptResult until status != "running"
func WaitForImageOptResult(db *gorm.DB, resultID uint64) (*models.ImageOptResult, error) {
	deadline := time.Now().Add(taskTimeout)
	for time.Now().Before(deadline) {
		var r models.ImageOptResult
		if err := db.First(&r, resultID).Error; err != nil {
			return nil, err
		}
		if r.Status != "running" {
			return &r, nil
		}
		time.Sleep(taskPollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for image optimization result")
}

// WaitForUpdateLogs polls UpdateLog until at least one record exists for the site created after startTime
func WaitForUpdateLogs(db *gorm.DB, siteID uint64, startTime time.Time) ([]models.UpdateLog, error) {
	deadline := time.Now().Add(taskTimeout)
	for time.Now().Before(deadline) {
		var logs []models.UpdateLog
		db.Where("site_id = ? AND created_at >= ?", siteID, startTime).
			Order("created_at DESC").Find(&logs)
		if len(logs) > 0 {
			return logs, nil
		}
		time.Sleep(taskPollInterval)
	}
	// No updates found — that's ok, it means everything is up to date
	return nil, nil
}
