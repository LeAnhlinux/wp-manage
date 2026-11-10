package services

import (
	"fmt"
	"strings"
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
		if strings.Contains(err.Error(), "Duplicate entry") && strings.Contains(err.Error(), "ip_address") {
			return nil, "", fmt.Errorf("IP address %s is already registered. Each server can only be added once", req.IPAddress)
		}
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

	// Cascade delete all dependent records to avoid FK constraint errors
	tables := []interface{}{
		&models.TaskQueue{},
		&models.Backup{},
		&models.ServerConfig{},
		&models.ActivityLog{},
		&models.CronJob{},
		&models.SSHKey{},
		&models.FirewallRule{},
		&models.NginxConfig{},
		&models.AlertRule{},
		&models.AlertHistory{},
		&models.StorageCredential{},
		&models.DNSProvider{},
		&models.RolloutServer{},
		&models.UserServer{},
		&models.TeamServer{},
	}
	for _, model := range tables {
		s.db.Where("server_id = ?", id).Delete(model)
	}
	// Bot-control rows are keyed by (scope_type, scope_id), not server_id.
	s.db.Where("scope_type = ? AND scope_id = ?", "server", id).Delete(&models.BotRule{})
	s.db.Where("scope_type = ? AND scope_id = ?", "server", id).Delete(&models.BotConfig{})

	return s.db.Where("id = ?", id).Delete(&models.Server{}).Error
}

func (s *ServerService) GetByAPIKey(apiKey string) (*models.Server, error) {
	var server models.Server
	err := s.db.Where("agent_api_key = ?", apiKey).First(&server).Error
	return &server, err
}

func (s *ServerService) UpdateHeartbeat(serverID uint64, hb models.AgentHeartbeat) error {
	now := time.Now()
	if err := s.db.Model(&models.Server{}).Where("id = ?", serverID).Updates(map[string]interface{}{
		"last_heartbeat": &now,
		"agent_version":  hb.Version,
		"status":         "active",
		// Cache latest metrics so Dashboard can show them immediately on load
		// without waiting for the next heartbeat (up to 30s).
		"cpu_usage":  hb.CPUUsage,
		"ram_usage":  hb.RAMUsage,
		"disk_usage": hb.DiskUsage,
	}).Error; err != nil {
		return err
	}
	// Record time-series snapshot for mobile metrics history endpoint.
	// Fire-and-forget in goroutine to avoid adding latency to the heartbeat response.
	go s.db.Create(&models.ServerMetric{
		ServerID:   serverID,
		CPUUsage:   hb.CPUUsage,
		RAMUsage:   hb.RAMUsage,
		DiskUsage:  hb.DiskUsage,
		UptimeSec:  int64(hb.Uptime),
		RecordedAt: now,
	})
	return nil
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

// GenerateInstallCommand returns a one-line command the user pastes on a fresh
// VPS to install ONLY the agent and connect it back to this panel. It mirrors
// what the Auto-Deploy SSH flow does (download agent binary → systemd service →
// start), but runs locally on the VPS so no SSH credentials leave the machine.
//
// The agent binary is pulled from the public mirror; the service connects to
// this panel via --api-url / --api-key.
func (s *ServerService) GenerateInstallCommand(server *models.Server, apiKey, panelDomain string) string {
	apiURL := "https://" + panelDomain
	const mirror = "https://mirrors.bucloud.com"
	// Single root shell command: arch-detect → download → install → systemd → start.
	return fmt.Sprintf(`curl -fsSL %s/wp-manage-agent-linux-$(case $(uname -m) in aarch64|arm64) echo arm64;; *) echo amd64;; esac) -o /usr/local/bin/wp-manage-agent && chmod +x /usr/local/bin/wp-manage-agent && printf '[Unit]\nDescription=WP Manage Agent\nAfter=network.target\n[Service]\nExecStart=/usr/local/bin/wp-manage-agent --api-url=%s --api-key=%s\nRestart=always\nRestartSec=5\n[Install]\nWantedBy=multi-user.target\n' > /etc/systemd/system/wp-manage-agent.service && systemctl daemon-reload && systemctl enable --now wp-manage-agent`,
		mirror, apiURL, apiKey,
	)
}
