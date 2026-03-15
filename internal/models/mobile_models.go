package models

import "time"

// ─── Mobile Auth ─────────────────────────────────────────────────────────────

// MobileRefreshToken stores long-lived refresh tokens for mobile sessions.
// Access tokens (JWT) expire in 15 min; refresh tokens last 30 days and are
// stored hashed so that a DB breach does not expose live tokens.
type MobileRefreshToken struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      uint64     `json:"user_id" gorm:"index;not null"`
	TokenHash   string     `json:"-" gorm:"uniqueIndex;size:64;not null"` // SHA-256 hex of raw token
	DeviceID    string     `json:"device_id" gorm:"size:255;index"`
	DeviceType  string     `json:"device_type" gorm:"type:enum('ios','android','other');default:'other'"`
	DeviceName  string     `json:"device_name" gorm:"size:255"`
	FCMToken    string     `json:"-" gorm:"size:500"` // Firebase Cloud Messaging
	APNSToken   string     `json:"-" gorm:"size:500"` // Apple Push Notification Service
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	User        *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// MobileNotification stores push notification history for the in-app notification center.
type MobileNotification struct {
	ID        uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    uint64     `json:"user_id" gorm:"index;not null"`
	Type      string     `json:"type" gorm:"size:50;not null"` // e.g. "server_offline", "backup_failed"
	Title     string     `json:"title" gorm:"size:255;not null"`
	Body      string     `json:"body" gorm:"size:500;not null"`
	Payload   string     `json:"payload" gorm:"type:text"` // JSON extra data
	SentAt    time.Time  `json:"sent_at"`
	ReadAt    *time.Time `json:"read_at"`
	User      *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// MobilePushLog records every FCM/APNS delivery attempt for debugging.
type MobilePushLog struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       uint64    `json:"user_id" gorm:"index;not null"`
	DeviceID     string    `json:"device_id" gorm:"size:255"`
	Type         string    `json:"type" gorm:"size:50"`
	Status       string    `json:"status" gorm:"type:enum('sent','failed','skipped');default:'sent'"`
	FCMMessageID string    `json:"fcm_message_id" gorm:"size:255"`
	Error        string    `json:"error,omitempty" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── Mobile Request / Response DTOs ──────────────────────────────────────────

// MobileLoginRequest is the payload for POST /api/mobile/v1/auth/login
type MobileLoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Mobile2FACompleteRequest completes a 2FA challenge started during login.
type Mobile2FACompleteRequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	Code      string `json:"code" validate:"required,len=6"`
}

// MobileRefreshRequest exchanges a refresh token for a new access token.
type MobileRefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// MobileDeviceRequest registers or updates a device for push notifications.
type MobileDeviceRequest struct {
	DeviceID   string `json:"device_id" validate:"required"`
	DeviceType string `json:"device_type" validate:"required,oneof=ios android other"`
	DeviceName string `json:"device_name"`
	FCMToken   string `json:"fcm_token"`
	APNSToken  string `json:"apns_token"`
}

// MobileLogoutRequest invalidates a specific refresh token on logout.
type MobileLogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// MobileTokenResponse is returned after a successful login or 2FA completion.
type MobileTokenResponse struct {
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	TokenType        string   `json:"token_type"`
	ExpiresIn        int      `json:"expires_in"`         // seconds
	RefreshExpiresIn int      `json:"refresh_expires_in"` // seconds
	User             MobileMe `json:"user"`
}

// MobileMe is the current-user profile returned by /auth/me and embedded in token responses.
type MobileMe struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
}

// ─── Mobile Dashboard ─────────────────────────────────────────────────────────

// MobileDashboardResponse is the single combined response for GET /api/mobile/v1/dashboard.
// It replaces ~8 separate web-panel calls with one mobile-optimised endpoint.
type MobileDashboardResponse struct {
	Summary        MobileDashboardSummary `json:"summary"`
	Servers        []MobileServerItem     `json:"servers"`
	Alerts         []MobileAlertItem      `json:"alerts"`          // last 5 fired alerts
	UptimeDown     []MobileUptimeItem     `json:"uptime_down"`     // all currently-down checks
	RecentActivity []MobileActivityItem   `json:"recent_activity"` // last 10 entries
}

type MobileDashboardSummary struct {
	ServersTotal      int `json:"servers_total"`
	ServersOnline     int `json:"servers_online"`
	ServersOffline    int `json:"servers_offline"`
	SitesTotal        int `json:"sites_total"`
	SitesActive       int `json:"sites_active"`
	SitesSuspended    int `json:"sites_suspended"`
	SitesError        int `json:"sites_error"`
	BackupsFailed24h  int `json:"backups_failed_24h"`
	ThreatsFound7d    int `json:"threats_found_7d"`
	UptimeChecksDown  int `json:"uptime_checks_down"`
	MigrationsRunning int `json:"migrations_running"`
}

// MobileServerItem is the lightweight server representation used in dashboard + list.
type MobileServerItem struct {
	ID            uint64              `json:"id"`
	Name          string              `json:"name"`
	IPAddress     string              `json:"ip_address"`
	Status        string              `json:"status"`
	AgentVersion  string              `json:"agent_version"`
	OS            string              `json:"os"`
	LastHeartbeat *time.Time          `json:"last_heartbeat"`
	Metrics       MobileServerMetrics `json:"metrics"`
	SitesCount    int                 `json:"sites_count"`
}

type MobileServerMetrics struct {
	CPUUsage  float64 `json:"cpu_usage"`
	RAMUsage  float64 `json:"ram_usage"`
	DiskUsage float64 `json:"disk_usage"`
	UptimeSec int64   `json:"uptime_sec"`
}

type MobileAlertItem struct {
	ID         uint64    `json:"id"`
	ServerID   uint64    `json:"server_id"`
	ServerName string    `json:"server_name"`
	Metric     string    `json:"metric"`
	Value      float64   `json:"value"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

type MobileUptimeItem struct {
	ID           uint64     `json:"id"`
	URL          string     `json:"url"`
	SiteID       uint64     `json:"site_id"`
	SiteDomain   string     `json:"site_domain"`
	LastStatus   int        `json:"last_status"`
	LastCheck    *time.Time `json:"last_check"`
	ResponseMsAvg int       `json:"response_ms_avg"`
}

type MobileActivityItem struct {
	ID        uint64    `json:"id"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	ServerID  *uint64   `json:"server_id,omitempty"`
	SiteID    *uint64   `json:"site_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Mobile Site Detail ───────────────────────────────────────────────────────

// MobileSiteDetail is the combined site detail response for GET /api/mobile/v1/sites/:id.
// Merges Site + WAFConfig + RedisConfig + UptimeChecks + latest Backup + SSL info.
type MobileSiteDetail struct {
	ID                 uint64              `json:"id"`
	Domain             string              `json:"domain"`
	Status             string              `json:"status"`
	PHPVersion         string              `json:"php_version"`
	WPVersion          string              `json:"wp_version"`
	SSLEnabled         bool                `json:"ssl_enabled"`
	SSLExpiresAt       *time.Time          `json:"ssl_expires_at"`
	DaysUntilExpiry    int                 `json:"days_until_expiry"`
	SSLWarning         bool                `json:"ssl_warning"` // true when < 14 days
	ForceHTTPS         bool                `json:"force_https"`
	IsMultisite        bool                `json:"is_multisite"`
	OpenBasedirEnabled bool                `json:"open_basedir_enabled"`
	LinuxUser          string              `json:"linux_user"`
	Server             MobileSiteServer    `json:"server"`
	WAF                MobileSiteWAF       `json:"waf"`
	Redis              MobileSiteRedis     `json:"redis"`
	Cache              MobileSiteCache     `json:"cache"`
	UptimeChecks       []MobileUptimeItem  `json:"uptime_checks"`
	LatestBackup       *MobileBackupItem   `json:"latest_backup"`
	CreatedAt          time.Time           `json:"created_at"`
}

type MobileSiteServer struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
}

type MobileSiteWAF struct {
	Enabled       bool `json:"enabled"`
	ParanoidLevel int  `json:"paranoid_level"`
}

type MobileSiteRedis struct {
	Enabled     bool `json:"enabled"`
	MaxMemoryMB int  `json:"max_memory_mb"`
}

type MobileSiteCache struct {
	Enabled   bool   `json:"enabled"`
	CacheTime string `json:"cache_time"`
}

// ─── Mobile Backup ────────────────────────────────────────────────────────────

type MobileBackupItem struct {
	ID          uint64     `json:"id"`
	SiteID      *uint64    `json:"site_id"`
	Domain      string     `json:"domain"`
	Type        string     `json:"type"`
	Storage     string     `json:"storage"`
	Status      string     `json:"status"`
	SizeBytes   int64      `json:"size_bytes"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// ─── Mobile SSL ───────────────────────────────────────────────────────────────

type MobileSSLStatus struct {
	SiteID          uint64     `json:"site_id"`
	Domain          string     `json:"domain"`
	Enabled         bool       `json:"ssl_enabled"`
	ExpiresAt       *time.Time `json:"ssl_expires_at"`
	DaysUntilExpiry int        `json:"days_until_expiry"`
	Warning         bool       `json:"warning"` // < 14 days
	ForceHTTPS      bool       `json:"force_https"`
}

// ─── Mobile Scan ──────────────────────────────────────────────────────────────

type MobileScanItem struct {
	ID           uint64     `json:"id"`
	SiteID       uint64     `json:"site_id"`
	Domain       string     `json:"domain,omitempty"`
	ScanType     string     `json:"scan_type"`
	Engine       string     `json:"engine"`
	Status       string     `json:"status"`
	FilesScanned int        `json:"files_scanned"`
	ThreatsFound int        `json:"threats_found"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

type MobileScanDetail struct {
	MobileScanItem
	Threats []MobileThreat `json:"threats"`
}

type MobileThreat struct {
	File        string `json:"file"`
	ThreatType  string `json:"threat_type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ─── Mobile Migration ─────────────────────────────────────────────────────────

type MobileMigrationItem struct {
	ID           uint64     `json:"id"`
	SiteID       uint64     `json:"site_id"`
	Domain       string     `json:"domain,omitempty"`
	Mode         string     `json:"mode"`
	Phase        string     `json:"phase"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	SourceServer string     `json:"source_server"`
	TargetServer string     `json:"target_server"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

// ─── Mobile WP-CLI ────────────────────────────────────────────────────────────

type MobileWPCLIRequest struct {
	Command string `json:"command" validate:"required"`
}

// ─── Mobile API Response envelope ────────────────────────────────────────────

// MobileResponse is the standard envelope for all /api/mobile/v1/ responses.
// Unlike the internal APIResponse it always uses "ok" (not "success") and
// includes a "meta" field for pagination / etag info.
type MobileResponse struct {
	OK    bool         `json:"ok"`
	Data  interface{}  `json:"data,omitempty"`
	Error *MobileError `json:"error,omitempty"`
	Meta  *MobileMeta  `json:"meta,omitempty"`
}

type MobileError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type MobileMeta struct {
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	Total      int64  `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	ETag       string `json:"etag,omitempty"`
}

// MobileOK returns a successful MobileResponse with data.
func MobileOK(data interface{}) MobileResponse {
	return MobileResponse{OK: true, Data: data}
}

// MobileOKPage returns a successful paginated MobileResponse.
func MobileOKPage(data interface{}, page, perPage int, total int64) MobileResponse {
	totalPages := int(total) / perPage
	if int(total)%perPage != 0 {
		totalPages++
	}
	return MobileResponse{
		OK:   true,
		Data: data,
		Meta: &MobileMeta{
			Page: page, PerPage: perPage,
			Total: total, TotalPages: totalPages,
		},
	}
}

// MobileErr returns an error MobileResponse.
func MobileErr(code, message string) MobileResponse {
	return MobileResponse{OK: false, Error: &MobileError{Code: code, Message: message}}
}
