package services

import (
	"encoding/json"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type WPCLIService struct {
	db *gorm.DB
}

func NewWPCLIService(db *gorm.DB) *WPCLIService {
	return &WPCLIService{db: db}
}

func (s *WPCLIService) Execute(siteID uint64, command string) (*models.WPCLIHistory, error) {
	history := &models.WPCLIHistory{
		SiteID:  siteID,
		Command: command,
		Status:  "running",
	}
	if err := s.db.Create(history).Error; err != nil {
		return nil, err
	}

	// Get site for domain + server info
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, err
	}

	// Queue task for agent
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "wpcli_execute",
		Status:   "pending",
	}
	taskData := models.AgentWPCLITask{
		HistoryID:  history.ID,
		SiteID:     siteID,
		Domain:     site.Domain,
		PHPVersion: site.PHPVersion,
		Command:    command,
	}
	data, _ := json.Marshal(taskData)
	task.Data = string(data)
	s.db.Create(&task)

	return history, nil
}

func (s *WPCLIService) WaitForResult(historyID uint64) (*models.WPCLIHistory, error) {
	return WaitForWPCLIResult(s.db, historyID)
}

func (s *WPCLIService) GetHistory(siteID uint64, limit int) ([]models.WPCLIHistory, error) {
	if limit <= 0 {
		limit = 50
	}
	var history []models.WPCLIHistory
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Limit(limit).Find(&history).Error
	return history, err
}

func (s *WPCLIService) UpdateResult(historyID uint64, output string, exitCode int, status string) error {
	return s.db.Model(&models.WPCLIHistory{}).Where("id = ?", historyID).
		Updates(map[string]interface{}{"output": output, "exit_code": exitCode, "status": status}).Error
}
