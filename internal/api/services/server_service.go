package services

import (
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

type ServerService struct {
	db *gorm.DB
}

func NewServerService(db *gorm.DB) *ServerService {
	return &ServerService{db: db}
}

func (s *ServerService) Create(userID uint64, req models.CreateServerRequest) (*models.Server, string, error) {
	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	server := models.Server{
		UserID:      userID,
		Name:        req.Name,
		IPAddress:   req.IPAddress,
		Status:      "provisioning",
		AgentAPIKey: apiKey,
	}

	if err := s.db.Create(&server).Error; err != nil {
		return nil, "", err
	}

	return &server, apiKey, nil
}

func (s *ServerService) List(userID uint64, role string, page, perPage int) ([]models.Server, int64, error) {
	var servers []models.Server
	var total int64

	query := s.db.Model(&models.Server{})
	if role != "admin" {
		// Non-admin: show servers from team membership + direct UserServer assignments
		query = query.Where("id IN (?) OR id IN (?)",
			s.db.Model(&models.TeamServer{}).Select("server_id").Where("team_id IN (?)",
				s.db.Model(&models.TeamMember{}).Select("team_id").Where("user_id = ?", userID),
			),
			s.db.Model(&models.UserServer{}).Select("server_id").Where("user_id = ?", userID),
		)
	}
	query.Count(&total)

	err := query.Offset((page - 1) * perPage).Limit(perPage).
		Preload("Sites").
		Order("created_at DESC").
		Find(&servers).Error

	return servers, total, err
}

func (s *ServerService) GetByID(id uint64) (*models.Server, error) {
	var server models.Server
	err := s.db.Where("id = ?", id).
		Preload("Sites").
		First(&server).Error
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func (s *ServerService) Delete(id uint64) error {
	var count int64
	s.db.Model(&models.Site{}).Where("server_id = ?", id).Count(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete server with %d active sites", count)
	}
	return s.db.Where("id = ?", id).Delete(&models.Server{}).Error
}

func (s *ServerService) GetByAPIKey(apiKey string) (*models.Server, error) {
	var server models.Server
	err := s.db.Where("agent_api_key = ?", apiKey).First(&server).Error
	return &server, err
}

func (s *ServerService) UpdateHeartbeat(serverID uint64, hb models.AgentHeartbeat) error {
	now := time.Now()
	return s.db.Model(&models.Server{}).Where("id = ?", serverID).Updates(map[string]interface{}{
		"last_heartbeat": &now,
		"agent_version":  hb.Version,
		"status":         "active",
		// Cache latest metrics so Dashboard can show them immediately on load
		// without waiting for the next heartbeat (up to 30s).
		"cpu_usage":  hb.CPUUsage,
		"ram_usage":  hb.RAMUsage,
		"disk_usage": hb.DiskUsage,
	}).Error
}

func (s *ServerService) UpdateSystemInfo(serverID uint64, ramMB, cpuCores, diskGB int, os string) error {
	return s.db.Model(&models.Server{}).Where("id = ?", serverID).Updates(map[string]interface{}{
		"ram_mb":    ramMB,
		"cpu_cores": cpuCores,
		"disk_gb":   diskGB,
		"os":        os,
	}).Error
}

// CreateTask creates a task queue entry
func (s *ServerService) CreateTask(task *models.TaskQueue) error {
	return s.db.Create(task).Error
}

func (s *ServerService) GenerateInstallCommand(server *models.Server, apiKey, panelDomain string) string {
	return fmt.Sprintf(
		`curl -sSL https://install.wpm.vn | bash -s -- --api-url=https://%s --api-key=%s`,
		panelDomain, apiKey,
	)
}
