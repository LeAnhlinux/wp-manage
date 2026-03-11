package models

import "time"

type User struct {
	ID              uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Email           string    `json:"email" gorm:"uniqueIndex;size:255;not null"`
	PasswordHash    string    `json:"-" gorm:"size:255;not null"`
	Name            string    `json:"name" gorm:"size:100;not null"`
	Role            string    `json:"role" gorm:"type:enum('admin','manager','viewer');default:'viewer';not null"`
	APIKey          string    `json:"-" gorm:"size:64"`
	TwoFactorSecret string   `json:"-" gorm:"size:100"`
	TwoFactorEnabled bool    `json:"two_factor_enabled" gorm:"default:false"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Server struct {
	ID             uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID         uint64    `json:"user_id" gorm:"index;not null"`
	Name           string    `json:"name" gorm:"size:100;not null"`
	IPAddress      string    `json:"ip_address" gorm:"uniqueIndex;size:45;not null"`
	OS             string    `json:"os" gorm:"size:50"`
	RAMMB          int       `json:"ram_mb"`
	CPUCores       int       `json:"cpu_cores"`
	DiskGB         int       `json:"disk_gb"`
	AgentVersion   string    `json:"agent_version" gorm:"size:20"`
	AgentAPIKey    string    `json:"-" gorm:"uniqueIndex;size:64"`
	Status         string    `json:"status" gorm:"type:enum('active','provisioning','offline','error');default:'provisioning';not null"`
	LastHeartbeat  *time.Time `json:"last_heartbeat"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Sites          []Site     `json:"sites,omitempty" gorm:"foreignKey:ServerID"`
	User           *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

type Site struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID     uint64    `json:"server_id" gorm:"index;not null"`
	Domain       string    `json:"domain" gorm:"uniqueIndex;size:255;not null"`
	PHPVersion   string    `json:"php_version" gorm:"size:10;default:'8.2'"`
	SSLEnabled   bool       `json:"ssl_enabled" gorm:"default:false"`
	SSLExpiresAt *time.Time `json:"ssl_expires_at"`
	ForceHTTPS   bool       `json:"force_https" gorm:"column:force_https;default:false"`
	DBName       string    `json:"db_name" gorm:"size:64"`
	DBUser       string    `json:"db_user" gorm:"size:64"`
	DBPassword   string    `json:"-" gorm:"size:255"`
	WPVersion    string    `json:"wp_version" gorm:"size:20"`
	WPAdminUser  string    `json:"wp_admin_user" gorm:"size:100"`
	WPAdminEmail string    `json:"wp_admin_email" gorm:"size:255"`
	WPAdminPass     string    `json:"-" gorm:"size:255"`
	AutologinSecret string    `json:"-" gorm:"size:64"`
	IsMultisite     bool      `json:"is_multisite" gorm:"default:false"`
	MultisiteType   string    `json:"multisite_type" gorm:"size:20"` // subdomain, subfolder
	InstallType     string    `json:"install_type" gorm:"size:20;default:'auto'"` // auto, manual
	LinuxUser       string    `json:"linux_user" gorm:"size:64"`                  // e.g. "wp_example_com" (per-site Linux isolation)
	Status          string    `json:"status" gorm:"type:enum('active','suspended','creating','error','deleting');default:'creating';not null"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Server       *Server    `json:"server,omitempty" gorm:"foreignKey:ServerID"`
}

type Backup struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	// SiteID is nullable — set to NULL when a site is deleted so backups are preserved as orphaned records.
	SiteID      *uint64    `json:"site_id" gorm:"index"`
	// ServerID is captured at creation time; allows dispatching restore tasks even after the site is deleted.
	ServerID    uint64     `json:"server_id" gorm:"index;not null;default:0"`
	// Domain is captured at backup-creation time and survives site deletion.
	Domain      string     `json:"domain" gorm:"size:255;index"`
	Type        string     `json:"type" gorm:"type:enum('full','incremental','database_only');not null"`
	Storage     string     `json:"storage" gorm:"type:enum('local','s3','r2','gdrive','backblaze');default:'local'"`
	FilePath    string     `json:"file_path" gorm:"size:500"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum" gorm:"size:64"`
	Status      string     `json:"status" gorm:"type:enum('running','completed','failed');default:'running';not null"`
	Error       string     `json:"error,omitempty" gorm:"type:text"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Site        *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

type Migration struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID         uint64     `json:"site_id" gorm:"index;not null"`
	SourceServerID uint64     `json:"source_server_id" gorm:"index;not null"`
	TargetServerID uint64     `json:"target_server_id" gorm:"index;not null"`
	Phase          string     `json:"phase" gorm:"type:enum('preparation','initial_sync','delta_sync','switchover','cleanup');default:'preparation'"`
	Status         string     `json:"status" gorm:"type:enum('running','completed','failed','rolled_back');default:'running';not null"`
	Progress       int        `json:"progress" gorm:"default:0"`
	Error          string     `json:"error,omitempty" gorm:"type:text"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Site           *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
	SourceServer   *Server    `json:"source_server,omitempty" gorm:"foreignKey:SourceServerID"`
	TargetServer   *Server    `json:"target_server,omitempty" gorm:"foreignKey:TargetServerID"`
}

type Domain struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"index;not null"`
	Domain    string    `json:"domain" gorm:"uniqueIndex;size:255;not null"`
	Type      string    `json:"type" gorm:"type:enum('primary','alias','subdomain');default:'primary';not null"`
	CreatedAt time.Time `json:"created_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

type ActivityLog struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    *uint64   `json:"user_id" gorm:"index"`
	ServerID  *uint64   `json:"server_id" gorm:"index"`
	SiteID    *uint64   `json:"site_id" gorm:"index"`
	Action    string    `json:"action" gorm:"size:100;not null"`
	Details   string    `json:"details" gorm:"type:text"`
	IPAddress string    `json:"ip_address" gorm:"size:45"`
	CreatedAt time.Time `json:"created_at"`
}

type BackupSchedule struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"index;not null"`
	Type      string    `json:"type" gorm:"type:enum('full','incremental','database_only');not null"`
	Storage   string    `json:"storage" gorm:"type:enum('local','s3','r2','gdrive','backblaze');default:'local'"`
	CronExpr  string    `json:"cron_expr" gorm:"size:100;not null"`
	Retention int       `json:"retention" gorm:"default:7"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

type ServerConfig struct {
	ID         uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID   uint64    `json:"server_id" gorm:"index;not null"`
	ConfigType string   `json:"config_type" gorm:"type:enum('nginx_main','nginx_vhost','php_fpm_pool','mariadb');not null"`
	SiteID     *uint64   `json:"site_id" gorm:"index"`
	Content    string    `json:"content" gorm:"type:longtext;not null"`
	Version    int       `json:"version" gorm:"default:1"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TaskQueue struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID  uint64    `json:"server_id" gorm:"index;not null"`
	SiteID    uint64    `json:"site_id" gorm:"index"`
	TaskType  string    `json:"task_type" gorm:"size:50;not null"`
	Data      string    `json:"data" gorm:"type:text"`
	Status    string    `json:"status" gorm:"size:20;default:'pending';not null"`
	Output    string    `json:"output,omitempty" gorm:"type:longtext"`
	Error     string    `json:"error,omitempty" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MigrationLog stores per-phase log entries for a migration
type MigrationLog struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	MigrationID uint64   `json:"migration_id" gorm:"index;not null"`
	Phase       string    `json:"phase" gorm:"size:50;not null"`
	Message     string    `json:"message" gorm:"type:text;not null"`
	Level       string    `json:"level" gorm:"size:10;default:'info';not null"` // info, warn, error
	CreatedAt   time.Time `json:"created_at"`
}

// UserServer is the junction table for RBAC — assigns servers to non-admin users
type UserServer struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `json:"user_id" gorm:"uniqueIndex:idx_user_server;not null"`
	ServerID  uint64    `json:"server_id" gorm:"uniqueIndex:idx_user_server;not null"`
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Server    *Server   `json:"server,omitempty" gorm:"foreignKey:ServerID"`
}

// WAFConfig stores WAF (ModSecurity) configuration per site
type WAFConfig struct {
	ID              uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID          uint64    `json:"site_id" gorm:"uniqueIndex;not null"`
	Enabled         bool      `json:"enabled" gorm:"default:false"`
	RuleSet         string    `json:"rule_set" gorm:"size:50;default:'owasp-crs'"`
	ParanoidLevel   int       `json:"paranoid_level" gorm:"default:1"`
	WhitelistedIPs  string    `json:"whitelisted_ips" gorm:"type:text"` // comma-separated
	CustomRules     string    `json:"custom_rules" gorm:"type:text"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Site            *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// MalwareScan stores scan results per site
type MalwareScan struct {
	ID            uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID        uint64     `json:"site_id" gorm:"index;not null"`
	ScanType      string     `json:"scan_type" gorm:"size:30;default:'full'"` // full, quick, custom
	Status        string     `json:"status" gorm:"size:20;default:'running';not null"` // running, completed, failed
	FilesScanned  int        `json:"files_scanned"`
	ThreatsFound  int        `json:"threats_found"`
	ThreatsDetail string     `json:"threats_detail" gorm:"type:text"` // JSON array of threat details
	Error         string     `json:"error,omitempty" gorm:"type:text"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	Site          *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// StagingSite tracks staging environments
type StagingSite struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ProductionID uint64    `json:"production_id" gorm:"index;not null"` // parent site ID
	StagingSiteID uint64   `json:"staging_site_id" gorm:"index;not null"` // actual staging Site ID
	Status       string    `json:"status" gorm:"size:30;default:'creating'"` // creating, active, pushing, deleted
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Production   *Site     `json:"production,omitempty" gorm:"foreignKey:ProductionID"`
	StagingSite2 *Site     `json:"staging_site,omitempty" gorm:"foreignKey:StagingSiteID"`
}

// AutoUpdateConfig stores auto-update settings per site
type AutoUpdateConfig struct {
	ID              uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID          uint64    `json:"site_id" gorm:"uniqueIndex;not null"`
	CoreUpdates     bool      `json:"core_updates" gorm:"default:false"`
	PluginUpdates   bool      `json:"plugin_updates" gorm:"default:false"`
	ThemeUpdates    bool      `json:"theme_updates" gorm:"default:false"`
	BackupBefore    bool      `json:"backup_before" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Site            *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// UpdateLog records auto-update results
type UpdateLog struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID      uint64     `json:"site_id" gorm:"index;not null"`
	UpdateType  string     `json:"update_type" gorm:"size:20"` // core, plugin, theme
	ItemName    string     `json:"item_name" gorm:"size:200"`
	FromVersion string     `json:"from_version" gorm:"size:30"`
	ToVersion   string     `json:"to_version" gorm:"size:30"`
	Status      string     `json:"status" gorm:"size:20"` // success, failed
	Error       string     `json:"error,omitempty" gorm:"type:text"`
	CreatedAt   time.Time  `json:"created_at"`
	Site        *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// AgentRelease tracks agent binary releases
type AgentRelease struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Version     string    `json:"version" gorm:"size:30;uniqueIndex;not null"`
	DownloadURL string    `json:"download_url" gorm:"size:500;not null"`
	Checksum    string    `json:"checksum" gorm:"size:64;not null"` // SHA256
	ReleaseNotes string  `json:"release_notes" gorm:"type:text"`
	FileSize    int64     `json:"file_size"`
	CreatedAt   time.Time `json:"created_at"`
}

// RolloutPlan tracks a rollout of an agent release across servers
type RolloutPlan struct {
	ID           uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ReleaseID    uint64     `json:"release_id" gorm:"index;not null"`
	Strategy     string     `json:"strategy" gorm:"size:20;not null"` // canary, rolling, immediate
	BatchPercent int        `json:"batch_percent" gorm:"default:20"` // % of servers per batch
	Status       string     `json:"status" gorm:"size:20;default:'pending';not null"` // pending, running, completed, failed, paused
	TotalServers int        `json:"total_servers"`
	UpdatedCount int        `json:"updated_count"`
	FailedCount  int        `json:"failed_count"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Release      *AgentRelease  `json:"release,omitempty" gorm:"foreignKey:ReleaseID"`
	Servers      []RolloutServer `json:"servers,omitempty" gorm:"foreignKey:RolloutPlanID"`
}

// RolloutServer tracks per-server update status within a rollout
type RolloutServer struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	RolloutPlanID  uint64     `json:"rollout_plan_id" gorm:"index;not null"`
	ServerID       uint64     `json:"server_id" gorm:"index;not null"`
	BatchNumber    int        `json:"batch_number"` // which batch this server is in
	OldVersion     string     `json:"old_version" gorm:"size:30"`
	NewVersion     string     `json:"new_version" gorm:"size:30"`
	Status         string     `json:"status" gorm:"size:20;default:'pending'"` // pending, updating, completed, failed, rolled_back
	Error          string     `json:"error,omitempty" gorm:"type:text"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Server         *Server    `json:"server,omitempty" gorm:"foreignKey:ServerID"`
}

// PerformanceMetric stores PageSpeed Insights results per site
type PerformanceMetric struct {
	ID                 uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID             uint64    `json:"site_id" gorm:"index;not null"`
	DeviceType         string    `json:"device_type" gorm:"size:10;not null"` // mobile, desktop
	PerformanceScore   float64   `json:"performance_score"`
	AccessibilityScore float64   `json:"accessibility_score"`
	LCPMS              float64   `json:"lcp_ms"`
	CLS                float64   `json:"cls"`
	FIDMS              float64   `json:"fid_ms"`
	TTFBMS             float64   `json:"ttfb_ms"`
	MeasuredAt         time.Time `json:"measured_at"`
	Site               *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// ImageOptConfig stores image optimization settings per site
type ImageOptConfig struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID      uint64    `json:"site_id" gorm:"uniqueIndex;not null"`
	Enabled     bool      `json:"enabled" gorm:"default:false"`
	Quality     int       `json:"quality" gorm:"default:80"`
	WebPEnabled bool      `json:"webp_enabled" gorm:"default:true"`
	MaxWidth    int       `json:"max_width" gorm:"default:2560"`
	MaxHeight   int       `json:"max_height" gorm:"default:2560"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Site        *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// ImageOptResult stores image optimization run results
type ImageOptResult struct {
	ID             uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID         uint64     `json:"site_id" gorm:"index;not null"`
	TotalImages    int        `json:"total_images"`
	OptimizedCount int        `json:"optimized_count"`
	SkippedCount   int        `json:"skipped_count"`
	ErrorCount     int        `json:"error_count"`
	BytesSaved     int64      `json:"bytes_saved"`
	Status         string     `json:"status" gorm:"size:20;default:'running'"` // running, completed, failed
	Error          string     `json:"error,omitempty" gorm:"type:text"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Site           *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// Subsite tracks WordPress Multisite subsites
type Subsite struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"index;not null"`
	WPBlogID  int       `json:"wp_blog_id"`
	Domain    string    `json:"domain" gorm:"size:255"`
	Path      string    `json:"path" gorm:"size:255"`
	Title     string    `json:"title" gorm:"size:255"`
	Status    string    `json:"status" gorm:"size:30;default:'active'"` // active, archived, deleted
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// Team represents a group of users with shared server access
type Team struct {
	ID          uint64       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string       `json:"name" gorm:"size:100;not null"`
	Description string       `json:"description" gorm:"size:500"`
	OwnerID     uint64       `json:"owner_id" gorm:"index;not null"`
	Owner       *User        `json:"owner,omitempty" gorm:"foreignKey:OwnerID"`
	Members     []TeamMember `json:"members,omitempty" gorm:"foreignKey:TeamID"`
	Servers     []TeamServer `json:"servers,omitempty" gorm:"foreignKey:TeamID"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// TeamMember is the junction table linking users to teams
type TeamMember struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TeamID    uint64    `json:"team_id" gorm:"uniqueIndex:idx_team_member;not null"`
	UserID    uint64    `json:"user_id" gorm:"uniqueIndex:idx_team_member;not null"`
	Role      string    `json:"role" gorm:"size:20;default:'member'"` // admin, member
	Team      *Team     `json:"team,omitempty" gorm:"foreignKey:TeamID"`
	User      *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamServer is the junction table assigning servers to teams
type TeamServer struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TeamID    uint64    `json:"team_id" gorm:"uniqueIndex:idx_team_server;not null"`
	ServerID  uint64    `json:"server_id" gorm:"uniqueIndex:idx_team_server;not null"`
	Team      *Team     `json:"team,omitempty" gorm:"foreignKey:TeamID"`
	Server    *Server   `json:"server,omitempty" gorm:"foreignKey:ServerID"`
	CreatedAt time.Time `json:"created_at"`
}

// FirewallRule stores UFW firewall rules per server
type FirewallRule struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID  uint64    `json:"server_id" gorm:"index;not null"`
	RuleType  string    `json:"rule_type" gorm:"size:10;not null"` // allow, deny
	Protocol  string    `json:"protocol" gorm:"size:10;default:'tcp'"`
	Port      string    `json:"port" gorm:"size:20;not null"` // e.g. "80", "443", "3000:3100"
	Source    string    `json:"source" gorm:"size:50"` // IP or CIDR, empty = anywhere
	Comment   string    `json:"comment" gorm:"size:200"`
	CreatedAt time.Time `json:"created_at"`
}

// NginxConfig stores custom Nginx configurations per server/site
type NginxConfig struct {
	ID         uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID   uint64    `json:"server_id" gorm:"index;not null"`
	SiteID     *uint64   `json:"site_id" gorm:"index"`
	ConfigType string    `json:"config_type" gorm:"size:30;not null"` // redirect, custom_location, rate_limit, security_header
	Name       string    `json:"name" gorm:"size:100;not null"`
	Content    string    `json:"content" gorm:"type:text;not null"`
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CronJob stores cron job entries managed per server/site
type CronJob struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID  uint64    `json:"server_id" gorm:"index;not null"`
	SiteID    *uint64   `json:"site_id" gorm:"index"`
	Command   string    `json:"command" gorm:"size:500;not null"`
	Schedule  string    `json:"schedule" gorm:"size:50;not null"` // cron expression
	User      string    `json:"user" gorm:"size:50;default:'root'"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	Comment   string    `json:"comment" gorm:"size:200"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RedisConfig stores per-site Redis object cache configuration
type RedisConfig struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID      uint64    `json:"site_id" gorm:"uniqueIndex;not null"`
	Enabled     bool      `json:"enabled" gorm:"default:false"`
	MaxMemoryMB int       `json:"max_memory_mb" gorm:"default:64"`
	Prefix      string    `json:"prefix" gorm:"size:50"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Site        *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// SSHKey stores SSH public keys per server
type SSHKey struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID    uint64    `json:"server_id" gorm:"index;not null"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	PublicKey   string    `json:"public_key" gorm:"type:text;not null"`
	Fingerprint string    `json:"fingerprint" gorm:"size:100"`
	CreatedAt   time.Time `json:"created_at"`
}

// SFTPAccount stores SFTP-only user accounts per site
type SFTPAccount struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"index;not null"`
	Username  string    `json:"username" gorm:"size:50;not null"`
	HomeDir   string    `json:"home_dir" gorm:"size:255"`
	CreatedAt time.Time `json:"created_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// RecoveryCode stores hashed recovery codes for 2FA backup
type RecoveryCode struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `json:"user_id" gorm:"index;not null"`
	CodeHash  string    `json:"-" gorm:"size:64;not null"`
	Used      bool      `json:"used" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
}

// UptimeCheck stores uptime monitoring configuration per site
type UptimeCheck struct {
	ID         uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID     uint64     `json:"site_id" gorm:"index;not null"`
	URL        string     `json:"url" gorm:"size:500;not null"`
	Interval   int        `json:"interval" gorm:"default:60"` // seconds
	Enabled    bool       `json:"enabled" gorm:"default:true"`
	LastStatus int        `json:"last_status"`
	LastCheck  *time.Time `json:"last_check"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Site       *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// UptimeLog stores individual uptime check results
type UptimeLog struct {
	ID         uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	CheckID    uint64    `json:"check_id" gorm:"index;not null"`
	StatusCode int       `json:"status_code"`
	ResponseMs int       `json:"response_ms"`
	Error      string    `json:"error,omitempty" gorm:"size:500"`
	CreatedAt  time.Time `json:"created_at"`
}

// AlertRule stores alert threshold configurations per server
type AlertRule struct {
	ID         uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID   uint64    `json:"server_id" gorm:"index;not null"`
	Metric     string    `json:"metric" gorm:"size:20;not null"` // cpu, ram, disk
	Threshold  float64   `json:"threshold" gorm:"not null"`      // percentage
	Channel    string    `json:"channel" gorm:"size:20;not null"` // telegram, slack, email
	WebhookURL string    `json:"webhook_url,omitempty" gorm:"size:500"`
	Email      string    `json:"email,omitempty" gorm:"size:255"`
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	CooldownMin int      `json:"cooldown_min" gorm:"default:30"` // minutes between alerts
	LastFired  *time.Time `json:"last_fired"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Server     *Server    `json:"server,omitempty" gorm:"foreignKey:ServerID"`
}

// AlertHistory stores fired alert records
type AlertHistory struct {
	ID       uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleID   uint64    `json:"rule_id" gorm:"index;not null"`
	ServerID uint64    `json:"server_id" gorm:"index;not null"`
	Metric   string    `json:"metric" gorm:"size:20"`
	Value    float64   `json:"value"`
	Message  string    `json:"message" gorm:"size:500"`
	Channel  string    `json:"channel" gorm:"size:20"`
	Sent     bool      `json:"sent" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationConfig stores global notification settings
type NotificationConfig struct {
	ID            uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	TelegramToken string `json:"telegram_token,omitempty" gorm:"size:255"`
	TelegramChat  string `json:"telegram_chat,omitempty" gorm:"size:50"`
	SlackWebhook  string `json:"slack_webhook,omitempty" gorm:"size:500"`
	SMTPHost      string `json:"smtp_host,omitempty" gorm:"size:255"`
	SMTPPort      int    `json:"smtp_port,omitempty"`
	SMTPUser      string `json:"smtp_user,omitempty" gorm:"size:255"`
	SMTPPass      string `json:"-" gorm:"size:255"`
	SMTPFrom      string `json:"smtp_from,omitempty" gorm:"size:255"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// WPCLIHistory stores WP-CLI command execution history
type WPCLIHistory struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"index;not null"`
	Command   string    `json:"command" gorm:"size:1000;not null"`
	Output    string    `json:"output" gorm:"type:mediumtext"`
	ExitCode  int       `json:"exit_code"`
	Status    string    `json:"status" gorm:"size:20;default:'running'"` // running, completed, failed
	CreatedAt time.Time `json:"created_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// CacheConfig stores Nginx FastCGI cache configuration per site
type CacheConfig struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64    `json:"site_id" gorm:"uniqueIndex;not null"`
	Enabled   bool      `json:"enabled" gorm:"default:false"`
	CacheTime string    `json:"cache_time" gorm:"size:20;default:'60m'"` // nginx time format
	Excludes  string    `json:"excludes" gorm:"type:text"`               // comma-separated paths
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Site      *Site     `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// ExternalImport tracks import-from-external-server jobs
type ExternalImport struct {
	ID        uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	SiteID    uint64     `json:"site_id" gorm:"index;not null"`
	Domain    string     `json:"domain" gorm:"size:255;not null"`
	SourceIP  string     `json:"source_ip" gorm:"size:45;not null"`
	SSHUser   string     `json:"ssh_user" gorm:"size:50;default:'root'"`
	SSHPort   int        `json:"ssh_port" gorm:"default:22"`
	Status    string     `json:"status" gorm:"size:30;default:'pending'"` // pending, running, completed, failed
	Progress  int        `json:"progress" gorm:"default:0"`
	Error     string     `json:"error,omitempty" gorm:"type:text"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Site      *Site      `json:"site,omitempty" gorm:"foreignKey:SiteID"`
}

// DNSProvider stores DNS provider credentials per server
type DNSProvider struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID     uint64    `json:"server_id" gorm:"index;not null"`
	ProviderType string    `json:"provider_type" gorm:"size:20;not null"` // cloudflare, route53
	APIKey       string    `json:"-" gorm:"size:500"`
	APIEmail     string    `json:"api_email,omitempty" gorm:"size:255"`
	ZoneID       string    `json:"zone_id,omitempty" gorm:"size:100"`
	Domain       string    `json:"domain" gorm:"size:255"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Server       *Server   `json:"server,omitempty" gorm:"foreignKey:ServerID"`
}

// StorageCredential stores remote backup storage credentials
type StorageCredential struct {
	ID        uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerID  uint64    `json:"server_id" gorm:"index"`
	Provider  string    `json:"provider"`                // s3, r2, backblaze, gdrive
	Bucket    string    `json:"bucket"`
	Region    string    `json:"region"`
	AccessKey string    `json:"access_key"`
	SecretKey string    `json:"-"`                        // hidden from JSON
	Endpoint  string    `json:"endpoint"`                 // for S3-compatible
	BasePath  string    `json:"base_path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PanelRelease tracks available panel releases
type PanelRelease struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Version      string    `json:"version" gorm:"uniqueIndex;size:50"`
	DownloadURL  string    `json:"download_url" gorm:"size:500"`
	Checksum     string    `json:"checksum" gorm:"size:64"`
	ReleaseNotes string    `json:"release_notes" gorm:"type:text"`
	FileSize     int64     `json:"file_size"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIKeyModel stores hashed API keys for programmatic access
type APIKeyModel struct {
	ID         uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID     uint64     `json:"user_id" gorm:"index;not null"`
	Name       string     `json:"name" gorm:"size:100;not null"`
	KeyHash    string     `json:"-" gorm:"uniqueIndex;size:64;not null"`
	KeyPrefix  string     `json:"key_prefix" gorm:"size:12;not null"`
	RateLimit  int        `json:"rate_limit" gorm:"default:100"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	User       *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}
