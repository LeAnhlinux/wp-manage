package services

import (
	"encoding/json"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type Fail2BanService struct {
	db *gorm.DB
}

func NewFail2BanService(db *gorm.DB) *Fail2BanService {
	return &Fail2BanService{db: db}
}

func (s *Fail2BanService) GetStatus(serverID uint64) error {
	s.queueTask(serverID, 0, "fail2ban_status", map[string]string{})
	return nil
}

func (s *Fail2BanService) UnbanIP(serverID uint64, jail, ip string) error {
	s.queueTask(serverID, 0, "fail2ban_unban", map[string]string{
		"jail": jail,
		"ip":   ip,
	})
	return nil
}

func (s *Fail2BanService) UpdateJail(serverID uint64, req models.Fail2BanUpdateJailRequest) error {
	data := map[string]interface{}{
		"jail": req.Jail,
	}
	if req.Enabled != nil {
		data["enabled"] = *req.Enabled
	}
	if req.BanTime > 0 {
		data["ban_time"] = req.BanTime
	}
	if req.MaxRetry > 0 {
		data["max_retry"] = req.MaxRetry
	}

	s.queueTask(serverID, 0, "fail2ban_update_jail", data)
	return nil
}

func (s *Fail2BanService) queueTask(serverID, siteID uint64, taskType string, data interface{}) {
	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: taskType,
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)
}
