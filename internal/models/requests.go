package models

import "time"

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Name     string `json:"name" validate:"required,min=2"`
}

type CreateServerRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=100"`
	IPAddress string `json:"ip_address" validate:"required,ip"`
}

type CreateSiteRequest struct {
	ServerID     uint64 `json:"server_id" validate:"required"`
	Domain       string `json:"domain" validate:"required,fqdn"`
	PHPVersion   string `json:"php_version" validate:"omitempty,oneof=7.4 8.0 8.1 8.2 8.3"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	RedisEnabled bool   `json:"redis_enabled"`
	InstallType  string `json:"install_type" validate:"omitempty,oneof=auto manual"` // "auto" (default) or "manual"
	WPAdminUser  string `json:"wp_admin_user" validate:"omitempty,min=3"`
	WPAdminEmail string `json:"wp_admin_email" validate:"omitempty,email"`
}

type UpdateSiteRequest struct {
	PHPVersion *string `json:"php_version" validate:"omitempty,oneof=7.4 8.0 8.1 8.2 8.3"`
	SSLEnabled *bool   `json:"ssl_enabled"`
}

type UpdatePHPRequest struct {
	Version string `json:"version" validate:"required,oneof=7.4 8.0 8.1 8.2 8.3"`
}

type AutoTuneRequest struct {
	// empty - server figures it out from hardware
}

type CreateBackupRequest struct {
	Type    string `json:"type" validate:"required,oneof=full database_only"`
	Storage string `json:"storage" validate:"omitempty,oneof=local s3 r2 gdrive backblaze"`
}

type RestoreRequest struct {
	BackupID      uint64 `json:"backup_id" validate:"required"`
	ConfirmDomain string `json:"confirm_domain" validate:"required"`
}

type CreateBackupScheduleRequest struct {
	Type      string `json:"type" validate:"required,oneof=full database_only"`
	Storage   string `json:"storage" validate:"omitempty,oneof=local s3 r2 gdrive backblaze"`
	CronExpr  string `json:"cron_expr" validate:"required"`
	Retention int    `json:"retention" validate:"omitempty,min=1,max=365"`
	Enabled   *bool  `json:"enabled"`
}

type UpdateBackupScheduleRequest struct {
	Type      *string `json:"type" validate:"omitempty,oneof=full database_only"`
	Storage   *string `json:"storage" validate:"omitempty,oneof=local s3 r2 gdrive backblaze"`
	CronExpr  *string `json:"cron_expr"`
	Retention *int    `json:"retention" validate:"omitempty,min=1,max=365"`
	Enabled   *bool   `json:"enabled"`
}

// Agent backup task data
type AgentBackupTask struct {
	BackupID   uint64 `json:"backup_id"`
	Domain     string `json:"domain"`
	Type       string `json:"type"` // full, database_only
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPass     string `json:"db_password"`
	PHPVersion string `json:"php_version"` // e.g. "8.2"
}

type AgentRestoreTask struct {
	BackupID   uint64 `json:"backup_id"`
	Domain     string `json:"domain"`
	Type       string `json:"type"`
	FilePath   string `json:"file_path"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPass     string `json:"db_password"`
	PHPVersion string `json:"php_version"` // fallback if manifest missing
}

type AgentBackupResult struct {
	TaskID    uint64 `json:"task_id"`
	BackupID  uint64 `json:"backup_id"`
	SiteID    uint64 `json:"site_id"`
	FilePath  string `json:"file_path"`
	SizeBytes int64  `json:"size_bytes"`
	Checksum  string `json:"checksum"`
	Status    string `json:"status"` // completed, failed
	Error     string `json:"error,omitempty"`
}

type MigrateRequest struct {
	TargetServerID uint64 `json:"target_server_id" validate:"required"`
	// Mode: "keep_domain" (suspend source, auto DNS update) or "change_domain" (wp search-replace)
	Mode          string `json:"mode"`
	NewDomain     string `json:"new_domain"`     // required when mode=change_domain
	UpdateDNS     bool   `json:"update_dns"`     // auto-update DNS A record after keep_domain migration
	CleanupSource bool   `json:"cleanup_source"` // delete site files, DB, configs from source after migrate
}

// Migration task data for agent
type AgentMigrateTask struct {
	MigrationID    uint64 `json:"migration_id"`
	SiteID         uint64 `json:"site_id"`
	Domain         string `json:"domain"`
	SourceServerIP string `json:"source_server_ip"`
	TargetServerIP string `json:"target_server_ip"`
	DBName         string `json:"db_name"`
	DBUser         string `json:"db_user"`
	DBPass         string `json:"db_password"`
	PHPVersion     string `json:"php_version"`
	SSLEnabled     bool   `json:"ssl_enabled"`
	WPAdminUser    string `json:"wp_admin_user"`
	WPAdminEmail   string `json:"wp_admin_email"`
	LinuxUser      string `json:"linux_user"`           // per-site Linux user for correct chown
	Mode           string `json:"mode"`                 // "keep_domain" | "change_domain"
	NewDomain      string `json:"new_domain,omitempty"` // for change_domain mode
	CleanupSource  bool   `json:"cleanup_source"`       // delete files, DB, configs from source after migrate
}

type AgentMigrationUpdate struct {
	MigrationID uint64 `json:"migration_id"`
	Phase       string `json:"phase"`
	Progress    int    `json:"progress"`
	Status      string `json:"status"` // running, completed, failed, rolled_back
	Error       string `json:"error,omitempty"`
	LogMessage  string `json:"log_message,omitempty"`
	LogLevel    string `json:"log_level,omitempty"` // info, warn, error
}

type RebuildConfigRequest struct {
	ConfigTypes []string `json:"config_types" validate:"required,dive,oneof=nginx_vhost php_fpm_pool mariadb"`
	SiteID      *uint64  `json:"site_id"`
}

type AutoDeployRequest struct {
	ServerID   uint64 `json:"server_id" validate:"required"`
	SSHUser    string `json:"ssh_user" validate:"required"`
	SSHPort    int    `json:"ssh_port" validate:"omitempty,min=1,max=65535"`
	AuthMethod string `json:"auth_method" validate:"required,oneof=password key"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
}

type DeployProgress struct {
	Step    int    `json:"step"`
	Total   int    `json:"total"`
	Message string `json:"message"`
	Status  string `json:"status"` // running, success, error
	Error   string `json:"error,omitempty"`
}

// File Manager types

type FileInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	IsDir       bool   `json:"is_dir"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	ModTime     string `json:"mod_time"`
}

type FileContentRequest struct {
	Path    string `json:"path" validate:"required"`
	Content string `json:"content"`
}

type FileMkdirRequest struct {
	Path string `json:"path" validate:"required"`
}

type FileRenameRequest struct {
	OldPath string `json:"old_path" validate:"required"`
	NewPath string `json:"new_path" validate:"required"`
}

type FileDeleteRequest struct {
	Path string `json:"path" validate:"required"`
}

type FilePermissionsRequest struct {
	Path        string `json:"path" validate:"required"`
	Permissions string `json:"permissions" validate:"required"`
	Recursive   bool   `json:"recursive"`
}

type FileSearchRequest struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern" validate:"required"`
}

type FileSearchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// User management requests
type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Name     string `json:"name" validate:"required,min=2"`
	Role     string `json:"role" validate:"required,oneof=admin manager viewer"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email" validate:"omitempty,email"`
	Password *string `json:"password" validate:"omitempty,min=8"`
	Name     *string `json:"name" validate:"omitempty,min=2"`
	Role     *string `json:"role" validate:"omitempty,oneof=admin manager viewer"`
}

type AssignServerRequest struct {
	ServerID uint64 `json:"server_id" validate:"required"`
}

// API Key requests
type CreateAPIKeyRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=100"`
	RateLimit int    `json:"rate_limit" validate:"omitempty,min=1,max=10000"`
	ExpiresIn int    `json:"expires_in"` // days, 0 = never
}

// Security types
type UpdateWAFRequest struct {
	Enabled        *bool   `json:"enabled"`
	ParanoidLevel  *int    `json:"paranoid_level" validate:"omitempty,min=1,max=4"`
	WhitelistedIPs *string `json:"whitelisted_ips"`
	CustomRules    *string `json:"custom_rules"`
}

type StartMalwareScanRequest struct {
	ScanType string `json:"scan_type" validate:"omitempty,oneof=full quick custom"`
	Engine   string `json:"engine" validate:"omitempty,oneof=wp_signature clamav both"` // default: wp_signature
	Path     string `json:"path"` // custom path for custom scan type
}

type CreateScanScheduleRequest struct {
	ScanType  string `json:"scan_type" validate:"omitempty,oneof=full quick"`
	Engine    string `json:"engine" validate:"omitempty,oneof=wp_signature clamav both"`
	IntervalH int    `json:"interval_h" validate:"required,min=1,max=8760"` // 1h–365d
}

type UpdateScanScheduleRequest struct {
	ScanType  string `json:"scan_type,omitempty"`
	Engine    string `json:"engine,omitempty"`
	IntervalH int    `json:"interval_h,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

type AgentWAFTask struct {
	SiteID         uint64 `json:"site_id"`
	Domain         string `json:"domain"`
	Enabled        bool   `json:"enabled"`
	ParanoidLevel  int    `json:"paranoid_level"`
	WhitelistedIPs string `json:"whitelisted_ips"`
}

type AgentMalwareScanTask struct {
	ScanID   uint64 `json:"scan_id"`
	SiteID   uint64 `json:"site_id"`
	Domain   string `json:"domain"`
	ScanType string `json:"scan_type"`
	Engine   string `json:"engine"` // wp_signature, clamav, both
	Path     string `json:"path"`
}

type AgentClamAVStatusResult struct {
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`     // e.g. "ClamAV 0.103.8"
	DBVersion   string `json:"db_version,omitempty"`  // e.g. "daily.cld 27161"
	LastUpdated string `json:"last_updated,omitempty"` // human readable
	Error       string `json:"error,omitempty"`
}

type AgentMalwareScanResult struct {
	ScanID        uint64 `json:"scan_id"`
	FilesScanned  int    `json:"files_scanned"`
	ThreatsFound  int    `json:"threats_found"`
	ThreatsDetail string `json:"threats_detail"` // JSON array
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

// WordPress Advanced types
type UpdateAutoUpdateConfigRequest struct {
	CoreUpdates   *bool `json:"core_updates"`
	PluginUpdates *bool `json:"plugin_updates"`
	ThemeUpdates  *bool `json:"theme_updates"`
	BackupBefore  *bool `json:"backup_before"`
}

type AgentCheckUpdatesTask struct {
	SiteID     uint64 `json:"site_id"`
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version,omitempty"`
}

type AgentCheckUpdatesResult struct {
	SiteID         uint64         `json:"site_id"`
	CurrentVersion string         `json:"current_version"` // always set, even when no updates
	Core           *WPUpdateInfo  `json:"core"`
	Plugins        []WPUpdateInfo `json:"plugins"`
	Themes         []WPUpdateInfo `json:"themes"`
}

type AgentApplyUpdatesTask struct {
	SiteID        uint64 `json:"site_id"`
	Domain        string `json:"domain"`
	PHPVersion    string `json:"php_version,omitempty"`
	DBName        string `json:"db_name"`
	DBUser        string `json:"db_user"`
	DBPass        string `json:"db_password"`
	CoreUpdates   bool   `json:"core_updates"`
	PluginUpdates bool   `json:"plugin_updates"`
	ThemeUpdates  bool   `json:"theme_updates"`
	BackupBefore  bool   `json:"backup_before"`
}

type AgentApplyUpdatesResult struct {
	SiteID        uint64         `json:"site_id"`
	Domain        string         `json:"domain"`
	NewWPVersion  string         `json:"new_wp_version"`
	CoreUpdated   bool           `json:"core_updated"`
	CoreError     string         `json:"core_error,omitempty"`
	PluginsResult []WPUpdateInfo `json:"plugins_result"`
	ThemesResult  []WPUpdateInfo `json:"themes_result"`
	Status        string         `json:"status"` // success, partial, failed
	Error         string         `json:"error,omitempty"`
}

type WPUpdateInfo struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"` // available, updated, failed
	Error      string `json:"error,omitempty"`
}

type AgentDBOptimizeTask struct {
	SiteID     uint64 `json:"site_id"`
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version,omitempty"`
	DBName     string `json:"db_name"`
	DBUser string `json:"db_user"`
	DBPass string `json:"db_password"`
}

type AgentDBOptimizeResult struct {
	SiteID          uint64 `json:"site_id"`
	TablesOptimized int    `json:"tables_optimized"`
	SpaceSaved      int64  `json:"space_saved"`
	RevisionsClean  int    `json:"revisions_cleaned"`
	TransientsClear int    `json:"transients_cleared"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
}

type AgentHeartbeat struct {
	Version   string  `json:"version"`
	CPUUsage  float64 `json:"cpu_usage"`
	RAMUsage  float64 `json:"ram_usage"`
	DiskUsage float64 `json:"disk_usage"`
	Uptime    int64   `json:"uptime"`
}

type AgentSystemInfo struct {
	Hostname       string   `json:"hostname"`
	OS             string   `json:"os"`
	OSVersion      string   `json:"os_version"`
	CPUCores       int      `json:"cpu_cores"`
	RAMMB          int64    `json:"ram_mb"`
	DiskGB         int64    `json:"disk_gb"`
	PHPVersions    []string `json:"php_versions"`
	NginxVersion   string   `json:"nginx_version"`
	MariaDBVersion string   `json:"mariadb_version"`
}

type AgentTask struct {
	TaskID   uint64      `json:"task_id,omitempty"`
	TaskType string      `json:"task_type"`
	SiteID   uint64      `json:"site_id"`
	Data     interface{} `json:"data"`
}

type AgentCreateSiteTask struct {
	Domain       string `json:"domain"`
	PHPVersion   string `json:"php_version"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	RedisEnabled bool   `json:"redis_enabled"`
	InstallType  string `json:"install_type"` // "auto" or "manual"
	DBName       string `json:"db_name"`
	DBUser       string `json:"db_user"`
	DBPassword   string `json:"db_password"`
	WPAdminUser  string `json:"wp_admin_user"`
	WPAdminEmail string `json:"wp_admin_email"`
	WPAdminPass  string `json:"wp_admin_pass"`
}

// AgentUploadSourceTask carries info for the agent to download and extract a source zip.
// DB credentials are passed so the agent can patch wp-config.php after extraction.
type AgentUploadSourceTask struct {
	Domain      string `json:"domain"`
	DownloadURL string `json:"download_url"`
	FileToken   string `json:"file_token"`
	DBName      string `json:"db_name"`   // used to patch wp-config.php
	DBUser      string `json:"db_user"`
	DBPass      string `json:"db_pass"`
	DBHost      string `json:"db_host"` // defaults to "localhost"
}

// AgentImportDBTask carries info for the agent to download and import a SQL file.
// OldDomain is optional: if set, wp search-replace is run to rewrite URLs after import.
type AgentImportDBTask struct {
	Domain      string `json:"domain"`
	DBName      string `json:"db_name"`
	DownloadURL string `json:"download_url"`
	FileToken   string `json:"file_token"`
	FileExt     string `json:"file_ext"`   // ".sql", ".gz", or ".zip"
	OldDomain   string `json:"old_domain"` // optional: source domain for search-replace
	PHPVersion  string `json:"php_version"` // site's PHP version — used to pick correct WP-CLI binary
}

type AgentTaskResult struct {
	TaskID          uint64     `json:"task_id,omitempty"`
	SiteID          uint64     `json:"site_id"`
	Status          string     `json:"status"`
	Output          string     `json:"output,omitempty"`
	Error           string     `json:"error,omitempty"`
	WPVersion       string     `json:"wp_version,omitempty"`
	AutologinSecret string     `json:"autologin_secret,omitempty"`
	LinuxUser       string     `json:"linux_user,omitempty"`     // Set after site creation (per-site isolation)
	SSLExpiresAt    *time.Time `json:"ssl_expires_at,omitempty"` // Set after SSL issue/renew
	SSLEnabled      *bool      `json:"ssl_enabled,omitempty"`    // Set after SSL force-https toggle

	// Populated when restore_backup completes for an orphaned backup (SiteID==0).
	// The API uses these to recreate the panel site record so the restored site appears in the panel.
	RestoredDomain     string `json:"restored_domain,omitempty"`
	RestoredDBName     string `json:"restored_db_name,omitempty"`
	RestoredDBUser     string `json:"restored_db_user,omitempty"`
	RestoredDBPass     string `json:"restored_db_pass,omitempty"`
	RestoredPHPVersion string `json:"restored_php_version,omitempty"`
	RestoredBackupID   uint64 `json:"restored_backup_id,omitempty"`
	RestoredServerID   uint64 `json:"restored_server_id,omitempty"`
	RestoredHasSSL     *bool  `json:"restored_has_ssl,omitempty"` // nil = unknown, true/false = known after certbot
}

// Rollout types
type CreateReleaseRequest struct {
	Version      string `json:"version" validate:"required"`
	DownloadURL  string `json:"download_url" validate:"required,url"`
	Checksum     string `json:"checksum" validate:"required"`
	ReleaseNotes string `json:"release_notes"`
	FileSize     int64  `json:"file_size"`
}

type CreateRolloutRequest struct {
	ReleaseID    uint64   `json:"release_id" validate:"required"`
	Strategy     string   `json:"strategy" validate:"required,oneof=canary rolling immediate"`
	BatchPercent int      `json:"batch_percent" validate:"omitempty,min=1,max=100"`
	ServerIDs    []uint64 `json:"server_ids"` // optional — empty means all active servers
}

type AgentUpdateTask struct {
	RolloutPlanID uint64 `json:"rollout_plan_id"`
	ServerID      uint64 `json:"server_id"`
	DownloadURL   string `json:"download_url"`
	Checksum      string `json:"checksum"`
	NewVersion    string `json:"new_version"`
}

type AgentUpdateResult struct {
	RolloutPlanID uint64 `json:"rollout_plan_id"`
	ServerID      uint64 `json:"server_id"`
	OldVersion    string `json:"old_version"`
	NewVersion    string `json:"new_version"`
	Status        string `json:"status"` // completed, failed
	Error         string `json:"error,omitempty"`
}

// Performance types
type RunPageSpeedRequest struct {
	DeviceType string `json:"device_type" validate:"omitempty,oneof=mobile desktop"`
}

// Multisite types
type EnableMultisiteRequest struct {
	Type string `json:"type" validate:"required,oneof=subdomain subfolder"`
}

type CreateSubsiteRequest struct {
	Slug  string `json:"slug" validate:"required,min=1,max=100"`
	Title string `json:"title" validate:"required,min=1,max=255"`
}

type AgentEnableMultisiteTask struct {
	SiteID     uint64 `json:"site_id"`
	Domain     string `json:"domain"`
	Type       string `json:"type"` // subdomain, subfolder
	PHPVersion string `json:"php_version"`
}

type AgentEnableMultisiteResult struct {
	SiteID uint64 `json:"site_id"`
	Type   string `json:"type"`   // "subdomain" or "subfolder" — echoed back
	Status string `json:"status"` // completed, failed
	Error  string `json:"error,omitempty"`
}

type AgentCreateSubsiteTask struct {
	SiteID        uint64 `json:"site_id"`
	Domain        string `json:"domain"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	MultisiteType string `json:"multisite_type"` // Fix #10: "subdomain" or "subfolder"
}

type AgentCreateSubsiteResult struct {
	SiteID   uint64 `json:"site_id"`
	WPBlogID int    `json:"wp_blog_id"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Title    string `json:"title"` // blogname from wp site list
	Status   string `json:"status"` // completed, failed
	Error    string `json:"error,omitempty"`
}

type AgentDeleteSubsiteTask struct {
	SiteID   uint64 `json:"site_id"`
	Domain   string `json:"domain"`
	WPBlogID int    `json:"wp_blog_id"`
}

type AgentDeleteSubsiteResult struct {
	SiteID   uint64 `json:"site_id"`
	WPBlogID int    `json:"wp_blog_id"`
	Status   string `json:"status"` // completed, failed
	Error    string `json:"error,omitempty"`
}

type AgentListSubsitesTask struct {
	SiteID uint64 `json:"site_id"`
	Domain string `json:"domain"`
}

type AgentListSubsitesResult struct {
	SiteID   uint64 `json:"site_id"`
	Subsites []struct {
		BlogID int    `json:"blog_id"`
		Domain string `json:"domain"`
		Path   string `json:"path"`
		Title  string `json:"title"`
	} `json:"subsites"`
	Status string `json:"status"` // completed, failed
	Error  string `json:"error,omitempty"`
}

// Image Optimization types
type UpdateImageOptConfigRequest struct {
	Enabled     *bool `json:"enabled"`
	Quality     *int  `json:"quality" validate:"omitempty,min=1,max=100"`
	WebPEnabled *bool `json:"webp_enabled"`
	MaxWidth    *int  `json:"max_width" validate:"omitempty,min=100,max=10000"`
	MaxHeight   *int  `json:"max_height" validate:"omitempty,min=100,max=10000"`
}

type AgentImageOptTask struct {
	SiteID      uint64 `json:"site_id"`
	ResultID    uint64 `json:"result_id"`
	Domain      string `json:"domain"`
	Quality     int    `json:"quality"`
	WebPEnabled bool   `json:"webp_enabled"`
	MaxWidth    int    `json:"max_width"`
	MaxHeight   int    `json:"max_height"`
}

// AgentToggleImageOptTask enables/disables auto-compress on upload (mu-plugin)
type AgentToggleImageOptTask struct {
	Domain      string `json:"domain"`
	Enabled     bool   `json:"enabled"`
	Quality     int    `json:"quality"`
	WebPEnabled bool   `json:"webp_enabled"`
}

// AgentNginxWebPTask enables/disables WebP auto-serving in the site's nginx config
type AgentNginxWebPTask struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

type AgentImageOptResult struct {
	ResultID       uint64 `json:"result_id"`
	SiteID         uint64 `json:"site_id"`
	TotalImages    int    `json:"total_images"`
	OptimizedCount int    `json:"optimized_count"`
	SkippedCount   int    `json:"skipped_count"`
	ErrorCount     int    `json:"error_count"`
	BytesSaved     int64  `json:"bytes_saved"`
	Status         string `json:"status"` // completed, failed
	Error          string `json:"error,omitempty"`
}

// Team management types
type CreateTeamRequest struct {
	Name        string `json:"name" validate:"required,min=2,max=100"`
	Description string `json:"description" validate:"omitempty,max=500"`
}

type UpdateTeamRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}

type AddTeamMemberRequest struct {
	UserID uint64 `json:"user_id" validate:"required"`
	Role   string `json:"role" validate:"required,oneof=admin member"`
}

type UpdateTeamMemberRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member"`
}

type AssignTeamServerRequest struct {
	ServerID uint64 `json:"server_id" validate:"required"`
}

// Infrastructure types (Batch 1)

type CreateFirewallRuleRequest struct {
	RuleType string `json:"rule_type" validate:"required,oneof=allow deny"`
	Protocol string `json:"protocol" validate:"omitempty,oneof=tcp udp both"`
	Port     string `json:"port" validate:"required"`
	Source   string `json:"source"`
	Comment  string `json:"comment"`
}

type CreateNginxConfigRequest struct {
	SiteID     *uint64 `json:"site_id"`
	ConfigType string  `json:"config_type" validate:"required,oneof=redirect custom_location rate_limit security_header"`
	Name       string  `json:"name" validate:"required,min=2,max=100"`
	Content    string  `json:"content" validate:"required"`
	Enabled    *bool   `json:"enabled"`
}

type UpdateNginxConfigRequest struct {
	ConfigType *string `json:"config_type" validate:"omitempty,oneof=redirect custom_location rate_limit security_header"`
	Name       *string `json:"name" validate:"omitempty,min=2,max=100"`
	Content    *string `json:"content"`
	Enabled    *bool   `json:"enabled"`
}

type OPcacheActionRequest struct {
	Action string `json:"action" validate:"required,oneof=enable disable flush"`
}

// Agent infrastructure task types

type AgentFirewallTask struct {
	Action   string `json:"action"` // add, delete, status
	RuleType string `json:"rule_type"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Source   string `json:"source"`
	Comment  string `json:"comment"`
}

type AgentFirewallResult struct {
	TaskID uint64           `json:"task_id"`
	Status string           `json:"status"` // completed, failed
	Error  string           `json:"error,omitempty"`
	Rules  []FirewallStatus `json:"rules,omitempty"`
}

type FirewallStatus struct {
	To     string `json:"to"`
	Action string `json:"action"`
	From   string `json:"from"`
}

type AgentNginxConfigTask struct {
	Action     string `json:"action"` // apply, remove, test
	Domain     string `json:"domain"`
	ConfigType string `json:"config_type"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	Enabled    bool   `json:"enabled"`
}

type AgentOPcacheTask struct {
	Action     string `json:"action"` // enable, disable, flush, stats
	PHPVersion string `json:"php_version"`
}

// ==================== ionCube ====================

type AgentIonCubeTask struct {
	Action     string `json:"action"`      // get_status, enable, disable
	PHPVersion string `json:"php_version"` // single version for enable/disable
}

type AgentOPcacheResult struct {
	TaskID   uint64                 `json:"task_id"`
	Status   string                 `json:"status"`
	Error    string                 `json:"error,omitempty"`
	Stats    map[string]interface{} `json:"stats,omitempty"`
	Enabled  bool                   `json:"enabled"`
}

type AgentRebuildSite struct {
	Domain             string `json:"domain"`
	PHPVersion         string `json:"php_version"`
	OpenBasedirEnabled bool   `json:"open_basedir_enabled"` // false = skip open_basedir in PHP-FPM pool
}

type AgentRebuildConfigTask struct {
	ConfigTypes        []string           `json:"config_types"`
	Domain             string             `json:"domain"`               // single-site mode
	PHPVersion         string             `json:"php_version"`          // single-site mode
	OpenBasedirEnabled bool               `json:"open_basedir_enabled"` // single-site mode
	Sites              []AgentRebuildSite `json:"sites,omitempty"`      // all-sites mode
}

type AgentHealthCheckResult struct {
	TaskID  uint64         `json:"task_id"`
	Status  string         `json:"status"`
	Error   string         `json:"error,omitempty"`
	Results []HealthResult `json:"results"`
}

type HealthResult struct {
	Service string `json:"service"`
	Status  string `json:"status"` // ok, warning, error
	Message string `json:"message"`
}

// ==================== Batch 2: Cron, Domain, Redis, PHP-FPM, MariaDB ====================

type CreateCronJobRequest struct {
	SiteID   *uint64 `json:"site_id"`
	Command  string  `json:"command" validate:"required"`
	Schedule string  `json:"schedule" validate:"required"`
	User     string  `json:"user"`
	Comment  string  `json:"comment"`
}

type UpdateCronJobRequest struct {
	Command  *string `json:"command"`
	Schedule *string `json:"schedule"`
	User     *string `json:"user"`
	Enabled  *bool   `json:"enabled"`
	Comment  *string `json:"comment"`
}

type CreateDomainRequest struct {
	Domain string `json:"domain" validate:"required"`
	Type   string `json:"type"` // primary, alias, subdomain
}

type UpdateDomainRequest struct {
	RedirectTo   string `json:"redirect_to"`   // empty = clear redirect
	RedirectType int    `json:"redirect_type"`  // 301 or 302
}

type DNSCheckResult struct {
	Domain       string   `json:"domain"`
	ServerIP     string   `json:"server_ip"`
	ResolvedIPs  []string `json:"resolved_ips"`
	Pointing     bool     `json:"pointing"`
	Message      string   `json:"message"`
	// Provider info — set when a DNS provider in our DB manages this domain
	ProviderType string   `json:"provider_type,omitempty"` // cloudflare | route53 | godaddy
	ProviderName string   `json:"provider_name,omitempty"` // human-readable: "Cloudflare", "GoDaddy", etc.
	CanAutoPoint bool     `json:"can_auto_point"`          // true if a matching provider found
}

type RedisConfigRequest struct {
	Enabled     *bool `json:"enabled"`
	MaxMemoryMB *int  `json:"max_memory_mb"`
}

type PHPFPMPoolConfig struct {
	MaxChildren     int    `json:"max_children"`
	StartServers    int    `json:"start_servers"`
	MinSpareServers int    `json:"min_spare_servers"`
	MaxSpareServers int    `json:"max_spare_servers"`
	MaxRequests     int    `json:"max_requests"`
	MemoryLimit     string `json:"memory_limit"`
}

type MariaDBUserRequest struct {
	Username   string   `json:"username" validate:"required"`
	Password   string   `json:"password"`
	Database   string   `json:"database"`
	Privileges []string `json:"privileges"` // ALL, SELECT, INSERT, etc.
}

type MariaDBConfigRequest struct {
	MaxConnections  int    `json:"max_connections"`
	InnodbBufferMB  int    `json:"innodb_buffer_mb"`
	QueryCacheMB    int    `json:"query_cache_mb"`
	SlowQueryLog    bool   `json:"slow_query_log"`
	SlowQueryTime   int    `json:"slow_query_time"`
	CustomConfig    string `json:"custom_config"`
}

// Agent task types for Batch 2
type AgentCronTask struct {
	Action   string `json:"action"` // add, remove, run_now, get_logs
	JobID    uint64 `json:"job_id"`
	User     string `json:"user"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Comment  string `json:"comment"`
	Enabled  bool   `json:"enabled"`
	Lines    int    `json:"lines,omitempty"` // for get_logs
}

type AgentDomainTask struct {
	Action       string `json:"action"`                  // add, remove, issue_ssl, set_redirect, remove_redirect
	Domain       string `json:"domain"`                  // alias domain being acted on
	DomainType   string `json:"domain_type"`
	SiteDomain   string `json:"site_domain"`             // primary domain of the site
	PHPVersion   string `json:"php_version"`
	Email        string `json:"email,omitempty"`         // for certbot
	RedirectTo   string `json:"redirect_to,omitempty"`   // set_redirect: target URL
	RedirectType int    `json:"redirect_type,omitempty"` // 301 or 302
}

type AgentRedisTask struct {
	Action      string `json:"action"` // enable, disable, flush
	Domain      string `json:"domain"`
	MaxMemoryMB int    `json:"max_memory_mb"`
	Prefix      string `json:"prefix"`
}

type AgentPHPFPMTask struct {
	Action     string           `json:"action"` // update_pool
	Domain     string           `json:"domain"`
	PHPVersion string           `json:"php_version"`
	PoolConfig PHPFPMPoolConfig `json:"pool_config"`
}

type AgentMariaDBTask struct {
	Action     string   `json:"action"` // create_user, drop_user, update_config, list_databases, create_database, drop_database, import_sql
	Username   string   `json:"username,omitempty"`
	Password   string   `json:"password,omitempty"`
	Database   string   `json:"database,omitempty"`
	Charset    string   `json:"charset,omitempty"`
	Collation  string   `json:"collation,omitempty"`
	Privileges []string `json:"privileges,omitempty"`
	Config     MariaDBConfigRequest `json:"config,omitempty"`
	SQLContent string   `json:"sql_content,omitempty"` // base64-encoded SQL file content for import_sql
}

type CreateDatabaseRequest struct {
	Name      string `json:"name" validate:"required,min=1,max=64"`
	Charset   string `json:"charset,omitempty"`
	Collation string `json:"collation,omitempty"`
}

type MariaDBDatabase struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	Charset   string `json:"charset"`
	Collation string `json:"collation"`
}

type InstalledPHPVersion struct {
	Version        string `json:"version"`
	Installed      bool   `json:"installed"`
	FPMActive      bool   `json:"fpm_active"`
	OPcacheEnabled bool   `json:"opcache_enabled"`
}

// ==================== Batch 4: Security & Authentication ====================

type Enable2FARequest struct{}

type Verify2FARequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type Validate2FARequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	Code     string `json:"code" validate:"required,len=6"`
}

type TwoFactorSetup struct {
	Secret        string   `json:"secret"`
	QRCodeURI     string   `json:"qr_code_uri"`
	RecoveryCodes []string `json:"recovery_codes"`
}

type CreateSSHKeyRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=100"`
	PublicKey string `json:"public_key" validate:"required"`
}

type CreateSFTPRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password"` // auto-generated if empty
}

type HardenWordPressRequest struct {
	DisableXMLRPC     bool `json:"disable_xmlrpc"`
	HideWPVersion     bool `json:"hide_wp_version"`
	SecureFilePerms   bool `json:"secure_file_perms"`
	SecurityHeaders   bool `json:"security_headers"`
	DisableFileEditor bool `json:"disable_file_editor"`
}

// ==================== Batch 5: Monitoring & Alerts ====================

type CreateUptimeCheckRequest struct {
	URL      string `json:"url" validate:"required"`
	Interval int    `json:"interval"`
}

type UpdateUptimeCheckRequest struct {
	URL      string `json:"url"`
	Interval int    `json:"interval"`
	Enabled  *bool  `json:"enabled"`
}

type CreateAlertRuleRequest struct {
	Metric      string  `json:"metric" validate:"required"`
	Threshold   float64 `json:"threshold" validate:"required"`
	Channel     string  `json:"channel" validate:"required"`
	WebhookURL  string  `json:"webhook_url"`
	Email       string  `json:"email"`
	CooldownMin int     `json:"cooldown_min"`
}

type UpdateAlertRuleRequest struct {
	Metric      string   `json:"metric"`
	Threshold   *float64 `json:"threshold"`
	Channel     string   `json:"channel"`
	WebhookURL  string   `json:"webhook_url"`
	Email       string   `json:"email"`
	Enabled     *bool    `json:"enabled"`
	CooldownMin *int     `json:"cooldown_min"`
}

type UpdateNotificationConfigRequest struct {
	TelegramToken string `json:"telegram_token"`
	TelegramChat  string `json:"telegram_chat"`
	SlackWebhook  string `json:"slack_webhook"`
	SMTPHost      string `json:"smtp_host"`
	SMTPPort      int    `json:"smtp_port"`
	SMTPUser      string `json:"smtp_user"`
	SMTPPass      string `json:"smtp_pass"`
	SMTPFrom      string `json:"smtp_from"`
}

type AgentAutotuneTask struct {
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version"`
}

type AgentSSHKeyTask struct {
	Action    string `json:"action"` // add, remove
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

type AgentSFTPTask struct {
	Action   string `json:"action"` // create, delete
	Username string `json:"username"`
	Password string `json:"password"`
	HomeDir  string `json:"home_dir"`
	Domain   string `json:"domain"`
}

type AgentHardenTask struct {
	Domain            string `json:"domain"`
	Revert            bool   `json:"revert"` // true = revert mode
	DisableXMLRPC     bool   `json:"disable_xmlrpc"`
	HideWPVersion     bool   `json:"hide_wp_version"`
	SecureFilePerms   bool   `json:"secure_file_perms"`
	SecurityHeaders   bool   `json:"security_headers"`
	DisableFileEditor bool   `json:"disable_file_editor"`
}

// ==================== Batch 6: WordPress Advanced Operations ====================

type WPCLIRequest struct {
	Command string `json:"command" validate:"required"`
}

type UpdateCacheConfigRequest struct {
	Enabled   *bool   `json:"enabled"`
	CacheTime *string `json:"cache_time"`
	Excludes  *string `json:"excludes"`
}

type ImportExternalRequest struct {
	Domain     string `json:"domain" validate:"required"`
	SourceIP   string `json:"source_ip" validate:"required"`
	SSHUser    string `json:"ssh_user"`
	SSHPort    int    `json:"ssh_port"`
	SSHKey     string `json:"ssh_key"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPass     string `json:"db_pass"`
	PHPVersion string `json:"php_version"`
}

type CreateDNSProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required,oneof=cloudflare route53"`
	APIKey       string `json:"api_key" validate:"required"`
	APIEmail     string `json:"api_email"`
	ZoneID       string `json:"zone_id"`
	Domain       string `json:"domain" validate:"required"`
}

type CreateGlobalDNSProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	APIKey       string `json:"api_key" validate:"required"`
	APIEmail     string `json:"api_email"`
	ZoneID       string `json:"zone_id" validate:"required"`
	Domain       string `json:"domain" validate:"required"`
}

type LookupZoneRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	APIKey       string `json:"api_key" validate:"required"`
	APIEmail     string `json:"api_email"` // Cloudflare: email for Global API Key; Route53: AWS Secret Access Key
	Domain       string `json:"domain" validate:"required"`
}

type LookupZoneResult struct {
	ZoneID   string `json:"zone_id"`
	ZoneName string `json:"zone_name"`
}

type TestDNSProviderRequest struct {
	ProviderType string `json:"provider_type" validate:"required"`
	APIKey       string `json:"api_key" validate:"required"`
	APIEmail     string `json:"api_email"`
	ZoneID       string `json:"zone_id" validate:"required"`
	Domain       string `json:"domain"`
}

type TestDNSProviderResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	ZoneName string `json:"zone_name,omitempty"`
}

type AutoCreateDNSRecordRequest struct {
	Domain   string `json:"domain" validate:"required"`
	ServerIP string `json:"server_ip" validate:"required"`
}

type AutoCreateDNSRecordResult struct {
	RecordID   string `json:"record_id"`
	ProviderID uint64 `json:"provider_id"`
	Domain     string `json:"domain"`
	IP         string `json:"ip"`
	Action     string `json:"action"` // "created" or "updated"
	ZoneName   string `json:"zone_name"`
	Proxied    bool   `json:"proxied"` // true = Cloudflare proxy still on (DNS check will see CF IPs)
}


// Agent task types for Batch 6
type AgentWPCLITask struct {
	HistoryID  uint64 `json:"history_id"`
	SiteID     uint64 `json:"site_id"`
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version,omitempty"`
	Command    string `json:"command"`
}

type AgentWPCLIResult struct {
	HistoryID uint64 `json:"history_id"`
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Status    string `json:"status"` // completed, failed
	Error     string `json:"error,omitempty"`
}

type AgentCacheTask struct {
	Action    string `json:"action"` // enable, disable, purge
	Domain    string `json:"domain"`
	CacheTime string `json:"cache_time"`
	Excludes  string `json:"excludes"`
}

type AgentImportTask struct {
	ImportID   uint64 `json:"import_id"`
	SiteID     uint64 `json:"site_id"`
	Domain     string `json:"domain"`
	SourceIP   string `json:"source_ip"`
	SSHUser    string `json:"ssh_user"`
	SSHPort    int    `json:"ssh_port"`
	SSHKey     string `json:"ssh_key"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPass     string `json:"db_pass"`
	PHPVersion string `json:"php_version"`
}

type AgentImportResult struct {
	ImportID uint64 `json:"import_id"`
	SiteID   uint64 `json:"site_id"`
	Status   string `json:"status"` // completed, failed
	Progress int    `json:"progress"`
	Error    string `json:"error,omitempty"`
}

// ==================== Batch 7: SSL, Storage Credentials, Backup Rotation ====================

type CreateStorageCredentialRequest struct {
	Provider  string `json:"provider" validate:"required,oneof=s3 r2 backblaze gdrive"`
	Bucket    string `json:"bucket" validate:"required"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key" validate:"required"`
	SecretKey string `json:"secret_key" validate:"required"`
	Endpoint  string `json:"endpoint"`
	BasePath  string `json:"base_path"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type PaginatedResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Meta    PaginationMeta `json:"meta"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ==================== PHP Version Management ====================

type InstallPHPVersionRequest struct {
	Version string `json:"version" validate:"required,oneof=7.4 8.0 8.1 8.2 8.3"`
}

type ChangePHPVersionRequest struct {
	NewVersion string `json:"new_version" validate:"required,oneof=7.4 8.0 8.1 8.2 8.3"`
}

type AgentPHPVersionTask struct {
	Action  string `json:"action"` // install, list
	Version string `json:"version"`
}

// MariaDB user list response from agent
type MariaDBUser struct {
	Username  string   `json:"username"`
	Host      string   `json:"host"`
	Databases []string `json:"databases"`
}

// ==================== Batch 8: Fail2Ban ====================

type Fail2BanStatus struct {
	ServiceRunning bool           `json:"service_running"`
	Jails          []Fail2BanJail `json:"jails"`
}

type Fail2BanJail struct {
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Filter    string   `json:"filter"`
	BanTime   int      `json:"ban_time"`
	MaxRetry  int      `json:"max_retry"`
	BannedIPs []string `json:"banned_ips"`
	TotalBans int      `json:"total_bans"`
}

type Fail2BanUnbanRequest struct {
	Jail string `json:"jail" validate:"required"`
	IP   string `json:"ip" validate:"required"`
}

type Fail2BanUpdateJailRequest struct {
	Jail     string `json:"jail" validate:"required"`
	Enabled  *bool  `json:"enabled"`
	BanTime  int    `json:"ban_time"`
	MaxRetry int    `json:"max_retry"`
}

// ==================== Batch 8: Panel Auto-Update ====================

type CreatePanelReleaseRequest struct {
	Version      string `json:"version" validate:"required"`
	DownloadURL  string `json:"download_url" validate:"required,url"`
	Checksum     string `json:"checksum" validate:"required"`
	ReleaseNotes string `json:"release_notes"`
	FileSize     int64  `json:"file_size"`
}
