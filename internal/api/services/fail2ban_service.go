package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type Fail2BanService struct {
	db *gorm.DB
}

func NewFail2BanService(db *gorm.DB) *Fail2BanService {
	return &Fail2BanService{db: db}
}

// GetStatus queries fail2ban status from agent (synchronous — waits for result).
func (s *Fail2BanService) GetStatus(serverID uint64) (*models.Fail2BanStatus, error) {
	dataJSON, _ := json.Marshal(map[string]string{})
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "fail2ban_status",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to queue task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("agent error: %s", result.Error)
	}

	var status models.Fail2BanStatus
	if err := json.Unmarshal([]byte(result.Output), &status); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}
	return &status, nil
}

// UnbanIP removes an IP from a fail2ban jail (synchronous).
func (s *Fail2BanService) UnbanIP(serverID uint64, jail, ip string) error {
	dataJSON, _ := json.Marshal(map[string]string{"jail": jail, "ip": ip})
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "fail2ban_unban",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 15*time.Second)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}

// SetupDefaultJails installs nginx + WordPress jail configs and reloads fail2ban (synchronous).
func (s *Fail2BanService) SetupDefaultJails(serverID uint64) error {
	dataJSON, _ := json.Marshal(map[string]string{})
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "fail2ban_setup_jails",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}

// ToggleService enables or disables the fail2ban systemd service (synchronous).
func (s *Fail2BanService) ToggleService(serverID uint64, enable bool) error {
	dataJSON, _ := json.Marshal(map[string]interface{}{"enable": enable})
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "fail2ban_toggle_service",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 20*time.Second)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}

// UpdateJail updates fail2ban jail settings (synchronous).
func (s *Fail2BanService) UpdateJail(serverID uint64, req models.Fail2BanUpdateJailRequest) error {
	data := map[string]interface{}{"jail": req.Jail}
	if req.Enabled != nil {
		data["enabled"] = *req.Enabled
	}
	if req.BanTime > 0 {
		data["ban_time"] = req.BanTime
	}
	if req.MaxRetry > 0 {
		data["max_retry"] = req.MaxRetry
	}

	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "fail2ban_update_jail",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 15*time.Second)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}
