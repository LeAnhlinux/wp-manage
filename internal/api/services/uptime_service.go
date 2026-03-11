package services

import (
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type UptimeService struct {
	db *gorm.DB
}

func NewUptimeService(db *gorm.DB) *UptimeService {
	return &UptimeService{db: db}
}

func (s *UptimeService) List(siteID uint64) ([]models.UptimeCheck, error) {
	var checks []models.UptimeCheck
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Find(&checks).Error
	return checks, err
}

func (s *UptimeService) Get(id uint64) (*models.UptimeCheck, error) {
	var check models.UptimeCheck
	err := s.db.First(&check, id).Error
	return &check, err
}

func (s *UptimeService) Create(siteID uint64, req models.CreateUptimeCheckRequest) (*models.UptimeCheck, error) {
	interval := req.Interval
	if interval <= 0 {
		interval = 60
	}

	check := models.UptimeCheck{
		SiteID:   siteID,
		URL:      req.URL,
		Interval: interval,
		Enabled:  true,
	}
	err := s.db.Create(&check).Error
	return &check, err
}

func (s *UptimeService) Update(id uint64, req models.UpdateUptimeCheckRequest) (*models.UptimeCheck, error) {
	var check models.UptimeCheck
	if err := s.db.First(&check, id).Error; err != nil {
		return nil, err
	}

	if req.URL != "" {
		check.URL = req.URL
	}
	if req.Interval > 0 {
		check.Interval = req.Interval
	}
	if req.Enabled != nil {
		check.Enabled = *req.Enabled
	}

	err := s.db.Save(&check).Error
	return &check, err
}

func (s *UptimeService) Delete(id uint64) error {
	// Delete logs first
	s.db.Where("check_id = ?", id).Delete(&models.UptimeLog{})
	return s.db.Delete(&models.UptimeCheck{}, id).Error
}

func (s *UptimeService) GetHistory(checkID uint64, limit int) ([]models.UptimeLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var logs []models.UptimeLog
	err := s.db.Where("check_id = ?", checkID).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (s *UptimeService) GetAllEnabled() ([]models.UptimeCheck, error) {
	var checks []models.UptimeCheck
	err := s.db.Where("enabled = ?", true).Find(&checks).Error
	return checks, err
}

func (s *UptimeService) LogResult(checkID uint64, statusCode, responseMs int, errMsg string) error {
	log := models.UptimeLog{
		CheckID:    checkID,
		StatusCode: statusCode,
		ResponseMs: responseMs,
		Error:      errMsg,
	}
	if err := s.db.Create(&log).Error; err != nil {
		return err
	}

	// Update check's last status
	now := s.db.NowFunc()
	return s.db.Model(&models.UptimeCheck{}).Where("id = ?", checkID).Updates(map[string]interface{}{
		"last_status": statusCode,
		"last_check":  now,
	}).Error
}

// CleanupOldLogs removes uptime logs older than the given number of days
func (s *UptimeService) CleanupOldLogs(days int) error {
	return s.db.Where("created_at < DATE_SUB(NOW(), INTERVAL ? DAY)", days).Delete(&models.UptimeLog{}).Error
}
