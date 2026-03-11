package services

import (
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type ActivityService struct {
	db *gorm.DB
}

func NewActivityService(db *gorm.DB) *ActivityService {
	return &ActivityService{db: db}
}

// Log creates a new activity log entry
func (s *ActivityService) Log(userID *uint64, serverID *uint64, siteID *uint64, action, details, ipAddress string) error {
	log := models.ActivityLog{
		UserID:    userID,
		ServerID:  serverID,
		SiteID:    siteID,
		Action:    action,
		Details:   details,
		IPAddress: ipAddress,
	}
	return s.db.Create(&log).Error
}

// List returns paginated activity logs with optional filters
func (s *ActivityService) List(userID *uint64, serverID *uint64, siteID *uint64, action string, from, to *time.Time, page, perPage int) ([]models.ActivityLog, int64, error) {
	query := s.db.Model(&models.ActivityLog{})

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if serverID != nil {
		query = query.Where("server_id = ?", *serverID)
	}
	if siteID != nil {
		query = query.Where("site_id = ?", *siteID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("created_at <= ?", *to)
	}

	var total int64
	query.Count(&total)

	var logs []models.ActivityLog
	err := query.Order("created_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&logs).Error

	return logs, total, err
}

// Cleanup removes activity logs older than the given number of days
func (s *ActivityService) Cleanup(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result := s.db.Where("created_at < ?", cutoff).Delete(&models.ActivityLog{})
	return result.RowsAffected, result.Error
}

// GetActions returns all distinct action types for filtering UI
func (s *ActivityService) GetActions() ([]string, error) {
	var actions []string
	err := s.db.Model(&models.ActivityLog{}).Distinct("action").Pluck("action", &actions).Error
	return actions, err
}
