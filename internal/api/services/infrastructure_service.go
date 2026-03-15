package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type InfrastructureService struct {
	db *gorm.DB
}

func NewInfrastructureService(db *gorm.DB) *InfrastructureService {
	return &InfrastructureService{db: db}
}

// ==================== Firewall Rules ====================

func (s *InfrastructureService) ListFirewallRules(serverID uint64) ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (s *InfrastructureService) CreateFirewallRule(serverID uint64, req models.CreateFirewallRuleRequest) (*models.FirewallRule, error) {
	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	rule := models.FirewallRule{
		ServerID: serverID,
		RuleType: req.RuleType,
		Protocol: protocol,
		Port:     req.Port,
		Source:   req.Source,
		Comment:  req.Comment,
	}
	if err := s.db.Create(&rule).Error; err != nil {
		return nil, err
	}

	// Queue task for agent
	taskData := models.AgentFirewallTask{
		Action:   "add",
		RuleType: req.RuleType,
		Protocol: protocol,
		Port:     req.Port,
		Source:   req.Source,
		Comment:  req.Comment,
	}
	s.queueTask(serverID, 0, "manage_firewall", taskData)

	return &rule, nil
}

func (s *InfrastructureService) DeleteFirewallRule(serverID, ruleID uint64) error {
	var rule models.FirewallRule
	if err := s.db.Where("id = ? AND server_id = ?", ruleID, serverID).First(&rule).Error; err != nil {
		return fmt.Errorf("firewall rule not found")
	}

	// Queue delete task for agent
	taskData := models.AgentFirewallTask{
		Action:   "delete",
		RuleType: rule.RuleType,
		Protocol: rule.Protocol,
		Port:     rule.Port,
		Source:   rule.Source,
	}
	s.queueTask(serverID, 0, "manage_firewall", taskData)

	return s.db.Delete(&rule).Error
}

// GetFirewallStatus retrieves live UFW status from the agent (synchronous)
func (s *InfrastructureService) GetFirewallStatus(serverID uint64) (string, error) {
	taskData := models.AgentFirewallTask{Action: "status"}
	task := s.queueTaskReturn(serverID, 0, "manage_firewall", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// ==================== Nginx Configs ====================

func (s *InfrastructureService) ListNginxConfigs(serverID uint64) ([]models.NginxConfig, error) {
	var configs []models.NginxConfig
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&configs).Error
	return configs, err
}

func (s *InfrastructureService) GetNginxConfig(serverID, configID uint64) (*models.NginxConfig, error) {
	var config models.NginxConfig
	err := s.db.Where("id = ? AND server_id = ?", configID, serverID).First(&config).Error
	if err != nil {
		return nil, fmt.Errorf("nginx config not found")
	}
	return &config, nil
}

func (s *InfrastructureService) CreateNginxConfig(serverID uint64, req models.CreateNginxConfigRequest) (*models.NginxConfig, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	config := models.NginxConfig{
		ServerID:   serverID,
		SiteID:     req.SiteID,
		ConfigType: req.ConfigType,
		Name:       req.Name,
		Content:    req.Content,
		Enabled:    enabled,
	}
	if err := s.db.Create(&config).Error; err != nil {
		return nil, err
	}

	// Queue apply task — need domain if site-specific
	domain := s.getDomainForSite(req.SiteID)
	taskData := models.AgentNginxConfigTask{
		Action:     "apply",
		Domain:     domain,
		ConfigType: req.ConfigType,
		Name:       req.Name,
		Content:    req.Content,
		Enabled:    enabled,
	}
	s.queueTask(serverID, 0, "manage_nginx_config", taskData)

	return &config, nil
}

func (s *InfrastructureService) UpdateNginxConfig(serverID, configID uint64, req models.UpdateNginxConfigRequest) (*models.NginxConfig, error) {
	config, err := s.GetNginxConfig(serverID, configID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.ConfigType != nil {
		updates["config_type"] = *req.ConfigType
		config.ConfigType = *req.ConfigType
	}
	if req.Name != nil {
		updates["name"] = *req.Name
		config.Name = *req.Name
	}
	if req.Content != nil {
		updates["content"] = *req.Content
		config.Content = *req.Content
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
		config.Enabled = *req.Enabled
	}

	if len(updates) > 0 {
		s.db.Model(&models.NginxConfig{}).Where("id = ?", configID).Updates(updates)
	}

	// Queue apply task
	domain := s.getDomainForSite(config.SiteID)
	taskData := models.AgentNginxConfigTask{
		Action:     "apply",
		Domain:     domain,
		ConfigType: config.ConfigType,
		Name:       config.Name,
		Content:    config.Content,
		Enabled:    config.Enabled,
	}
	s.queueTask(serverID, 0, "manage_nginx_config", taskData)

	return config, nil
}

func (s *InfrastructureService) DeleteNginxConfig(serverID, configID uint64) error {
	config, err := s.GetNginxConfig(serverID, configID)
	if err != nil {
		return err
	}

	domain := s.getDomainForSite(config.SiteID)
	taskData := models.AgentNginxConfigTask{
		Action:     "remove",
		Domain:     domain,
		ConfigType: config.ConfigType,
		Name:       config.Name,
	}
	s.queueTask(serverID, 0, "manage_nginx_config", taskData)

	return s.db.Delete(config).Error
}

// ==================== OPcache ====================

func (s *InfrastructureService) OPcacheAction(serverID uint64, action string, phpVersion string) error {
	if phpVersion == "" {
		phpVersion = "8.2"
	}
	taskData := models.AgentOPcacheTask{
		Action:     action,
		PHPVersion: phpVersion,
	}
	task := s.queueTaskReturn(serverID, 0, "opcache_action", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// ==================== Rebuild Config ====================

func (s *InfrastructureService) RebuildConfig(serverID uint64, req models.RebuildConfigRequest) (string, error) {
	var taskData models.AgentRebuildConfigTask
	taskData.ConfigTypes = req.ConfigTypes

	if req.SiteID != nil {
		// Single-site mode: rebuild only the specified site
		var site models.Site
		if err := s.db.First(&site, *req.SiteID).Error; err != nil {
			return "", fmt.Errorf("site not found")
		}
		phpVersion := site.PHPVersion
		if phpVersion == "" {
			phpVersion = "8.2"
		}
		taskData.Domain = site.Domain
		taskData.PHPVersion = phpVersion
		taskData.OpenBasedirEnabled = site.OpenBasedirEnabled
	} else {
		// All-sites mode: collect all sites on this server
		var sites []models.Site
		if err := s.db.Where("server_id = ?", serverID).Find(&sites).Error; err != nil {
			return "", fmt.Errorf("failed to list sites: %w", err)
		}
		for _, site := range sites {
			phpVersion := site.PHPVersion
			if phpVersion == "" {
				phpVersion = "8.2"
			}
			taskData.Sites = append(taskData.Sites, models.AgentRebuildSite{
				Domain:             site.Domain,
				PHPVersion:         phpVersion,
				OpenBasedirEnabled: site.OpenBasedirEnabled,
			})
		}
	}

	task := s.queueTaskReturn(serverID, 0, "rebuild_config", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return result.Output, fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// ==================== Services Status ====================

func (s *InfrastructureService) InstallService(serverID uint64, service string) error {
	task := s.queueTaskReturn(serverID, 0, "install_service", map[string]string{"service": service})
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) CheckServices(serverID uint64) (string, error) {
	task := s.queueTaskReturn(serverID, 0, "check_services", map[string]string{})
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// ==================== Health Check ====================

func (s *InfrastructureService) RunHealthCheck(serverID uint64) (string, error) {
	task := s.queueTaskReturn(serverID, 0, "health_check", map[string]string{})
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// ==================== Server Logs ====================

func (s *InfrastructureService) GetServerLogs(serverID uint64, logType string, lines int) (string, error) {
	taskData := map[string]interface{}{
		"log_type": logType,
		"lines":    lines,
	}
	task := s.queueTaskReturn(serverID, 0, "tail_logs", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// ==================== PHP Version Management ====================

func (s *InfrastructureService) InstallPHPVersion(serverID uint64, version string) error {
	taskData := models.AgentPHPVersionTask{
		Action:  "install",
		Version: version,
	}
	task := s.queueTaskReturn(serverID, 0, "install_php_version", taskData)
	// PHP install can take 2-5 minutes
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 300*1000000000) // 300s
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) ChangeSitePHPVersion(serverID, siteID uint64, newVersion string) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	oldVersion := site.PHPVersion
	if oldVersion == "" {
		oldVersion = "8.2"
	}
	if oldVersion == newVersion {
		return fmt.Errorf("site is already using PHP %s", newVersion)
	}

	// Use existing update_php task type
	taskData := map[string]string{
		"domain":          site.Domain,
		"old_php_version": oldVersion,
		"new_php_version": newVersion,
	}
	task := s.queueTaskReturn(serverID, siteID, "update_php", taskData)

	// PHP version change can take 2-5 minutes (may need to install new version)
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 300*1000000000) // 300s
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}

	// Update site's PHP version in DB
	s.db.Model(&site).Update("php_version", newVersion)
	return nil
}

// ==================== PHP-FPM Pool ====================

func (s *InfrastructureService) GetPHPFPMPool(serverID, siteID uint64) (*models.PHPFPMPoolConfig, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	// Return defaults — actual config is on the agent
	return &models.PHPFPMPoolConfig{
		MaxChildren:     10,
		StartServers:    2,
		MinSpareServers: 1,
		MaxSpareServers: 3,
		MaxRequests:     500,
		MemoryLimit:     "256M",
	}, nil
}

func (s *InfrastructureService) UpdatePHPFPMPool(serverID, siteID uint64, req models.PHPFPMPoolConfig) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	taskData := models.AgentPHPFPMTask{
		Action:     "update_pool",
		Domain:     site.Domain,
		PHPVersion: site.PHPVersion,
		PoolConfig: req,
	}
	s.queueTask(serverID, siteID, "update_phpfpm_pool", taskData)
	return nil
}

// ==================== MariaDB Management ====================

func (s *InfrastructureService) ListMariaDBUsers(serverID uint64) ([]models.MariaDBUser, error) {
	taskData := models.AgentMariaDBTask{Action: "list_users"}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	// Parse JSON output
	var users []models.MariaDBUser
	if result.Output != "" {
		if err := json.Unmarshal([]byte(result.Output), &users); err != nil {
			return nil, fmt.Errorf("failed to parse user list: %w", err)
		}
	}
	return users, nil
}

func (s *InfrastructureService) CreateMariaDBUser(serverID uint64, req models.MariaDBUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}

	taskData := models.AgentMariaDBTask{
		Action:     "create_user",
		Username:   req.Username,
		Password:   req.Password,
		Database:   req.Database,
		Privileges: req.Privileges,
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) DeleteMariaDBUser(serverID uint64, username string) error {
	taskData := models.AgentMariaDBTask{
		Action:   "drop_user",
		Username: username,
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) GetMariaDBConfig(serverID uint64) (*models.MariaDBConfigRequest, error) {
	taskData := models.AgentMariaDBTask{Action: "get_config"}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		// Fall back to defaults on error
		return &models.MariaDBConfigRequest{
			MaxConnections: 150, InnodbBufferMB: 256, QueryCacheMB: 64,
			SlowQueryLog: true, SlowQueryTime: 2,
		}, nil
	}
	if result.Error != "" || result.Output == "" {
		return &models.MariaDBConfigRequest{
			MaxConnections: 150, InnodbBufferMB: 256, QueryCacheMB: 64,
			SlowQueryLog: true, SlowQueryTime: 2,
		}, nil
	}

	var cfg models.MariaDBConfigRequest
	if err := json.Unmarshal([]byte(result.Output), &cfg); err != nil {
		return &models.MariaDBConfigRequest{
			MaxConnections: 150, InnodbBufferMB: 256, QueryCacheMB: 64,
			SlowQueryLog: true, SlowQueryTime: 2,
		}, nil
	}
	return &cfg, nil
}

func (s *InfrastructureService) UpdateMariaDBConfig(serverID uint64, req models.MariaDBConfigRequest) error {
	taskData := models.AgentMariaDBTask{
		Action: "update_config",
		Config: req,
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// ==================== MariaDB Databases ====================

func (s *InfrastructureService) ListMariaDBDatabases(serverID uint64) ([]models.MariaDBDatabase, error) {
	taskData := models.AgentMariaDBTask{Action: "list_databases"}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	var dbs []models.MariaDBDatabase
	if result.Output != "" {
		if err := json.Unmarshal([]byte(result.Output), &dbs); err != nil {
			return nil, fmt.Errorf("failed to parse database list: %w", err)
		}
	}
	return dbs, nil
}

func (s *InfrastructureService) CreateMariaDBDatabase(serverID uint64, req models.CreateDatabaseRequest) error {
	if req.Name == "" {
		return fmt.Errorf("database name is required")
	}
	taskData := models.AgentMariaDBTask{
		Action:    "create_database",
		Database:  req.Name,
		Charset:   req.Charset,
		Collation: req.Collation,
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) DeleteMariaDBDatabase(serverID uint64, dbName string) error {
	taskData := models.AgentMariaDBTask{
		Action:   "drop_database",
		Database: dbName,
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *InfrastructureService) ImportMariaDBDatabase(serverID uint64, dbName string, sqlData []byte) error {
	if dbName == "" {
		return fmt.Errorf("database name is required")
	}
	taskData := models.AgentMariaDBTask{
		Action:     "import_sql",
		Database:   dbName,
		SQLContent: base64.StdEncoding.EncodeToString(sqlData),
	}
	task := s.queueTaskReturn(serverID, 0, "manage_mariadb", taskData)
	// Use longer timeout for large SQL files (5 minutes)
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 300*1000000000)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// ==================== Installed PHP Versions ====================

func (s *InfrastructureService) ListInstalledPHPVersions(serverID uint64) ([]models.InstalledPHPVersion, error) {
	task := s.queueTaskReturn(serverID, 0, "list_installed_php", nil)
	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	var versions []models.InstalledPHPVersion
	if result.Output != "" {
		if err := json.Unmarshal([]byte(result.Output), &versions); err != nil {
			return nil, fmt.Errorf("failed to parse PHP versions: %w", err)
		}
	}
	return versions, nil
}

// ==================== Nginx Vhost ====================

func (s *InfrastructureService) ReadVhost(siteID uint64) (string, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return "", fmt.Errorf("site not found")
	}

	taskData := map[string]string{"domain": site.Domain}
	task := s.queueTaskReturn(site.ServerID, siteID, "read_vhost", taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

func (s *InfrastructureService) WriteVhost(siteID uint64, content string) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	taskData := map[string]string{"domain": site.Domain, "content": content}
	task := s.queueTaskReturn(site.ServerID, siteID, "write_vhost", taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// ==================== Helpers ====================

func (s *InfrastructureService) queueTask(serverID, siteID uint64, taskType string, data interface{}) {
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

func (s *InfrastructureService) queueTaskReturn(serverID, siteID uint64, taskType string, data interface{}) *models.TaskQueue {
	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: taskType,
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)
	return &task
}

func (s *InfrastructureService) getDomainForSite(siteID *uint64) string {
	if siteID == nil || *siteID == 0 {
		return ""
	}
	var site models.Site
	if err := s.db.First(&site, *siteID).Error; err != nil {
		return ""
	}
	return site.Domain
}
