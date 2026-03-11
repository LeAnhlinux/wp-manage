package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type RedisService struct {
	db *gorm.DB
}

func NewRedisService(db *gorm.DB) *RedisService {
	return &RedisService{db: db}
}

func (s *RedisService) GetConfig(siteID uint64) (*models.RedisConfig, error) {
	var config models.RedisConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return &models.RedisConfig{
			SiteID:      siteID,
			Enabled:     false,
			MaxMemoryMB: 64,
		}, nil
	}
	return &config, err
}

func (s *RedisService) UpdateConfig(siteID uint64, req models.RedisConfigRequest) (*models.RedisConfig, error) {
	var config models.RedisConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error

	if err == gorm.ErrRecordNotFound {
		config = models.RedisConfig{
			SiteID:      siteID,
			Enabled:     false,
			MaxMemoryMB: 64,
		}
	}

	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}
	if req.MaxMemoryMB != nil {
		config.MaxMemoryMB = *req.MaxMemoryMB
	}

	if config.ID == 0 {
		err = s.db.Create(&config).Error
	} else {
		err = s.db.Save(&config).Error
	}
	if err != nil {
		return nil, err
	}

	// Queue agent task (fire-and-forget for config updates)
	s.queueRedisTask(siteID, config)
	return &config, nil
}

// Toggle enables/disables Redis object cache for a site (synchronous — waits for result)
func (s *RedisService) Toggle(siteID uint64, enable bool) (*models.RedisConfig, error) {
	var config models.RedisConfig
	err := s.db.Where("site_id = ?", siteID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		config = models.RedisConfig{SiteID: siteID, MaxMemoryMB: 64}
	}

	config.Enabled = enable
	if config.ID == 0 {
		s.db.Create(&config)
	} else {
		s.db.Save(&config)
	}

	// Get site info for domain + server
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	action := "disable"
	if enable {
		action = "enable"
	}
	prefix := config.Prefix
	if prefix == "" {
		prefix = fmt.Sprintf("wp_%d_", siteID)
	}

	task := models.AgentRedisTask{
		Action:      action,
		Domain:      site.Domain,
		MaxMemoryMB: config.MaxMemoryMB,
		Prefix:      prefix,
	}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return &config, nil // Still return updated config even if queue fails
	}

	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return &config, fmt.Errorf("task failed: %w", err)
	}
	if result.Error != "" {
		return &config, fmt.Errorf("%s", result.Error)
	}

	return &config, nil
}

// GetStats retrieves Redis server statistics from the agent (synchronous)
func (s *RedisService) GetStats(siteID uint64) (string, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return "", fmt.Errorf("site not found")
	}

	task := models.AgentRedisTask{
		Action: "get_stats",
		Domain: site.Domain,
	}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return "", err
	}

	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// Flush flushes the Redis cache for a site (synchronous)
func (s *RedisService) Flush(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	task := models.AgentRedisTask{
		Action: "flush",
		Domain: site.Domain,
	}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return err
	}

	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// ─── Server-level Redis operations ───────────────────────────────────────────

// GetServerStats retrieves Redis INFO from the agent using a serverID directly.
func (s *RedisService) GetServerStats(serverID uint64) (string, error) {
	task := models.AgentRedisTask{Action: "get_stats"}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: serverID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return "", err
	}
	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// FlushServer flushes all Redis cache on a server directly by serverID.
func (s *RedisService) FlushServer(serverID uint64) error {
	task := models.AgentRedisTask{Action: "flush"}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: serverID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return err
	}
	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// RedisSiteStatus is the DTO returned for the per-site status list.
type RedisSiteStatus struct {
	SiteID      uint64 `json:"site_id"`
	ConfigID    uint64 `json:"config_id,omitempty"`
	Domain      string `json:"domain"`
	Enabled     bool   `json:"enabled"`
	MaxMemoryMB int    `json:"max_memory_mb"`
	Prefix      string `json:"prefix,omitempty"`
}

// GetServerSiteStatuses lists all sites on a server with their Redis enable/disable status.
func (s *RedisService) GetServerSiteStatuses(serverID uint64) ([]RedisSiteStatus, error) {
	var sites []models.Site
	if err := s.db.Where("server_id = ?", serverID).Find(&sites).Error; err != nil {
		return nil, err
	}

	siteIDs := make([]uint64, len(sites))
	siteMap := make(map[uint64]models.Site)
	for i, site := range sites {
		siteIDs[i] = site.ID
		siteMap[site.ID] = site
	}

	var configs []models.RedisConfig
	if len(siteIDs) > 0 {
		s.db.Where("site_id IN ?", siteIDs).Find(&configs)
	}

	configMap := make(map[uint64]models.RedisConfig)
	for _, c := range configs {
		configMap[c.SiteID] = c
	}

	result := make([]RedisSiteStatus, 0, len(sites))
	for _, site := range sites {
		c := configMap[site.ID]
		mm := c.MaxMemoryMB
		if mm == 0 {
			mm = 64
		}
		result = append(result, RedisSiteStatus{
			SiteID:      site.ID,
			ConfigID:    c.ID,
			Domain:      site.Domain,
			Enabled:     c.Enabled,
			MaxMemoryMB: mm,
			Prefix:      c.Prefix,
		})
	}
	return result, nil
}

func (s *RedisService) queueRedisTask(siteID uint64, config models.RedisConfig) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return
	}

	action := "disable"
	if config.Enabled {
		action = "enable"
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = fmt.Sprintf("wp_%d_", siteID)
	}

	task := models.AgentRedisTask{
		Action:      action,
		Domain:      site.Domain,
		MaxMemoryMB: config.MaxMemoryMB,
		Prefix:      prefix,
	}
	data, _ := json.Marshal(task)
	s.db.Create(&models.TaskQueue{
		ServerID: site.ServerID,
		TaskType: "toggle_redis",
		Data:     string(data),
		Status:   "pending",
	})
}
