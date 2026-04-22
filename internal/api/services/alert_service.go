package services

import (
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type AlertService struct {
	db *gorm.DB
}

func NewAlertService(db *gorm.DB) *AlertService {
	return &AlertService{db: db}
}

func (s *AlertService) ListRules(serverID uint64) ([]models.AlertRule, error) {
	var rules []models.AlertRule
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (s *AlertService) GetRule(id uint64) (*models.AlertRule, error) {
	var rule models.AlertRule
	err := s.db.First(&rule, id).Error
	return &rule, err
}

func (s *AlertService) CreateRule(serverID uint64, req models.CreateAlertRuleRequest) (*models.AlertRule, error) {
	cooldown := req.CooldownMin
	if cooldown <= 0 {
		cooldown = 30
	}

	rule := models.AlertRule{
		ServerID:    serverID,
		Metric:      req.Metric,
		Threshold:   req.Threshold,
		Channel:     req.Channel,
		WebhookURL:  req.WebhookURL,
		Email:       req.Email,
		CooldownMin: cooldown,
		Enabled:     true,
	}
	err := s.db.Create(&rule).Error
	return &rule, err
}

func (s *AlertService) UpdateRule(id uint64, req models.UpdateAlertRuleRequest) (*models.AlertRule, error) {
	var rule models.AlertRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}

	if req.Metric != "" {
		rule.Metric = req.Metric
	}
	if req.Threshold != nil {
		rule.Threshold = *req.Threshold
	}
	if req.Channel != "" {
		rule.Channel = req.Channel
	}
	if req.WebhookURL != "" {
		rule.WebhookURL = req.WebhookURL
	}
	if req.Email != "" {
		rule.Email = req.Email
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.CooldownMin != nil {
		rule.CooldownMin = *req.CooldownMin
	}

	err := s.db.Save(&rule).Error
	return &rule, err
}

func (s *AlertService) DeleteRule(id uint64) error {
	s.db.Where("rule_id = ?", id).Delete(&models.AlertHistory{})
	return s.db.Delete(&models.AlertRule{}, id).Error
}

func (s *AlertService) GetHistory(serverID uint64, limit int) ([]models.AlertHistory, error) {
	if limit <= 0 {
		limit = 50
	}
	var history []models.AlertHistory
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Limit(limit).Find(&history).Error
	return history, err
}

func (s *AlertService) GetAllEnabledRules() ([]models.AlertRule, error) {
	var rules []models.AlertRule
	err := s.db.Where("enabled = ?", true).Preload("Server").Find(&rules).Error
	return rules, err
}

func (s *AlertService) RecordAlert(ruleID, serverID uint64, metric string, value float64, message, channel string, sent bool) error {
	history := models.AlertHistory{
		RuleID:   ruleID,
		ServerID: serverID,
		Metric:   metric,
		Value:    value,
		Message:  message,
		Channel:  channel,
		Sent:     sent,
	}
	if err := s.db.Create(&history).Error; err != nil {
		return err
	}

	// Update last fired time
	now := s.db.NowFunc()
	return s.db.Model(&models.AlertRule{}).Where("id = ?", ruleID).Update("last_fired", now).Error
}

// GetNotificationConfig returns the global notification configuration
func (s *AlertService) GetNotificationConfig() (*models.NotificationConfig, error) {
	var cfg models.NotificationConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// Create default config
		cfg = models.NotificationConfig{}
		s.db.Create(&cfg)
		return &cfg, nil
	}
	return &cfg, err
}

// UpdateNotificationConfig updates or creates the notification config
func (s *AlertService) UpdateNotificationConfig(req models.UpdateNotificationConfigRequest) (*models.NotificationConfig, error) {
	var cfg models.NotificationConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = models.NotificationConfig{}
		s.db.Create(&cfg)
	}

	if req.TelegramToken != "" {
		cfg.TelegramToken = req.TelegramToken
	}
	if req.TelegramChat != "" {
		cfg.TelegramChat = req.TelegramChat
	}
	if req.SMTPHost != "" {
		cfg.SMTPHost = req.SMTPHost
	}
	if req.SMTPPort > 0 {
		cfg.SMTPPort = req.SMTPPort
	}
	if req.SMTPUser != "" {
		cfg.SMTPUser = req.SMTPUser
	}
	if req.SMTPPass != "" {
		cfg.SMTPPass = req.SMTPPass
	}
	if req.SMTPFrom != "" {
		cfg.SMTPFrom = req.SMTPFrom
	}

	err = s.db.Save(&cfg).Error
	return &cfg, err
}

// GetIntegrationConfig returns the global integration config
func (s *AlertService) GetIntegrationConfig() (*models.IntegrationConfig, error) {
	var cfg models.IntegrationConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = models.IntegrationConfig{}
		s.db.Create(&cfg)
		return &cfg, nil
	}
	return &cfg, err
}

// UpdateIntegrationConfig updates or creates the integration config
func (s *AlertService) UpdateIntegrationConfig(req models.UpdateIntegrationConfigRequest) (*models.IntegrationConfig, error) {
	var cfg models.IntegrationConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = models.IntegrationConfig{}
		s.db.Create(&cfg)
	}
	cfg.GooglePageSpeedKey = req.GooglePageSpeedKey
	err = s.db.Save(&cfg).Error
	return &cfg, err
}

// PHPFPMAutotune queues an autotune task for the server
func (s *AlertService) PHPFPMAutotune(serverID uint64, siteID uint64, domain, phpVersion string) error {
	taskData := `{"domain":"` + domain + `","php_version":"` + phpVersion + `"}`
	return s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: "phpfpm_autotune",
		Data:     taskData,
		Status:   "pending",
	}).Error
}
