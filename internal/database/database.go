package database

import (
	"fmt"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"log"
	"log/slog"

	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(cfg config.DatabaseConfig) (*gorm.DB, error) {
	logLevel := logger.Warn
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	DB = db
	return db, nil
}

// migrate runs one manual migration statement and logs a failure instead of
// discarding it. Every statement here is written to be idempotent, so an error
// means something is genuinely wrong — a typo'd table name, a bad column, a
// permission problem. Bare db.Exec() swallows that silently: an ALTER naming a
// table that does not exist looked exactly like success, and the index it was
// supposed to create simply never appeared.
//
// Failures are logged, not fatal: a broken migration should not stop the API from
// booting, but it must be visible in the log.
func migrate(db *gorm.DB, desc, query string) {
	if err := db.Exec(query).Error; err != nil {
		log.Printf("[migration] %s FAILED: %v", desc, err)
	}
}

// RunManualMigrations applies schema changes that GORM AutoMigrate cannot handle
// (e.g. changing column nullability, adding columns to existing tables safely).
//
// Note on table names: GORM pluralises model names, so models.TaskQueue lives in
// `task_queues`, not `task_queue`. Raw SQL here must use the pluralised name.
func RunManualMigrations(db *gorm.DB) {
	log.Println("Running manual schema migrations...")

	// Make backups.site_id nullable so backup records survive site deletion.
	// AutoMigrate will not do this — it only adds columns, never changes nullability.
	migrate(db, "ALTER TABLE backups MODIFY", "ALTER TABLE backups MODIFY COLUMN site_id BIGINT UNSIGNED NULL")

	// Add domain column to backups (captured at backup time, survives site deletion).
	// Use IF NOT EXISTS via a safe pattern: GORM AutoMigrate handles new columns,
	// but we use raw SQL here to ensure it runs AFTER the nullability change.
	migrate(db, "ALTER TABLE backups ADD", "ALTER TABLE backups ADD COLUMN IF NOT EXISTS domain VARCHAR(255) NOT NULL DEFAULT ''")
	migrate(db, "ALTER TABLE backups ADD", "ALTER TABLE backups ADD INDEX IF NOT EXISTS idx_backups_domain (domain)")

	// Backfill domain from the sites table for existing backup records.
	migrate(db, "UPDATE backups b INNER", "UPDATE backups b INNER JOIN sites s ON b.site_id = s.id SET b.domain = s.domain WHERE b.domain = ''")

	// Add server_id column to backups (allows restore dispatch even after site deletion).
	migrate(db, "ALTER TABLE backups ADD", "ALTER TABLE backups ADD COLUMN IF NOT EXISTS server_id BIGINT UNSIGNED NOT NULL DEFAULT 0")
	migrate(db, "ALTER TABLE backups ADD", "ALTER TABLE backups ADD INDEX IF NOT EXISTS idx_backups_server_id (server_id)")

	// Backfill server_id from sites for existing records.
	migrate(db, "UPDATE backups b INNER", "UPDATE backups b INNER JOIN sites s ON b.site_id = s.id SET b.server_id = s.server_id WHERE b.server_id = 0")

	// Add open_basedir_enabled to sites (per-site PHP open_basedir toggle).
	// Default 1 = enabled (most secure). Existing sites get open_basedir enabled.
	migrate(db, "ALTER TABLE sites ADD", "ALTER TABLE sites ADD COLUMN IF NOT EXISTS open_basedir_enabled TINYINT(1) NOT NULL DEFAULT 1")

	// Transfer Tool: change JSON columns → LONGTEXT to remove MariaDB's implicit
	// JSON_VALID() CHECK constraint which rejects empty string on INSERT.
	// MariaDB stores JSON and LONGTEXT identically on disk; no data loss.
	migrate(db, "ALTER TABLE transfer_jobs MODIFY", "ALTER TABLE transfer_jobs MODIFY COLUMN IF EXISTS discovered_sites LONGTEXT")
	migrate(db, "ALTER TABLE transfer_jobs MODIFY", "ALTER TABLE transfer_jobs MODIFY COLUMN IF EXISTS target_php_versions LONGTEXT")
	migrate(db, "ALTER TABLE transfer_jobs MODIFY", "ALTER TABLE transfer_jobs MODIFY COLUMN IF EXISTS selected_sites LONGTEXT")

	// --- Composite indexes for the hot query paths -------------------------------
	// GORM's `index` tag only ever produces single-column indexes, so before this
	// block the schema had none. Each query below filters on several columns and/or
	// sorts by one, which a single-column index cannot serve: MariaDB range-scans
	// one column then filters the rest row by row, and sorts with a filesort.
	// These are pure additions — no column or data changes.

	// Agent task dispatch (SiteService.GetPendingTasks). The hottest query in the
	// system: every agent polls it every 3 seconds, and task_queue grows with
	// operator activity, so an unindexed scan gets slower without bound.
	migrate(db, "ALTER TABLE task_queues ADD", "ALTER TABLE task_queues ADD INDEX IF NOT EXISTS idx_tq_dispatch (server_id, status, created_at)")
	// Stale-task requeue in the same function, and the retention sweep.
	migrate(db, "ALTER TABLE task_queues ADD", "ALTER TABLE task_queues ADD INDEX IF NOT EXISTS idx_tq_stale (status, updated_at)")

	// Dashboard metrics chart: WHERE server_id = ? AND recorded_at >= ?
	// (mobile.MetricsHistory), the latest-metric lookup, and the retention sweep.
	migrate(db, "ALTER TABLE server_metrics ADD", "ALTER TABLE server_metrics ADD INDEX IF NOT EXISTS idx_sm_server_time (server_id, recorded_at)")

	// Uptime history: WHERE check_id = ? ORDER BY created_at DESC LIMIT 100,
	// plus CleanupOldLogs' WHERE created_at < ?.
	migrate(db, "ALTER TABLE uptime_logs ADD", "ALTER TABLE uptime_logs ADD INDEX IF NOT EXISTS idx_ul_check_time (check_id, created_at)")

	// Activity log list (ORDER BY created_at DESC with no filter — the default
	// panel view) and the daily retention sweep.
	migrate(db, "ALTER TABLE activity_logs ADD", "ALTER TABLE activity_logs ADD INDEX IF NOT EXISTS idx_al_created (created_at)")

	// Malware scan history: WHERE site_id = ? ORDER BY started_at DESC.
	migrate(db, "ALTER TABLE malware_scans ADD", "ALTER TABLE malware_scans ADD INDEX IF NOT EXISTS idx_ms_site_started (site_id, started_at)")
}

func AutoMigrate(db *gorm.DB) error {
	log.Println("Running database auto-migration...")
	return db.AutoMigrate(
		&models.User{},
		&models.Server{},
		&models.Site{},
		&models.Domain{},
		&models.Backup{},
		&models.BackupSchedule{},
		&models.Migration{},
		&models.ActivityLog{},
		&models.ServerConfig{},
		&models.TaskQueue{},
		&models.UserServer{},
		&models.APIKeyModel{},
		&models.MigrationLog{},
		&models.WAFConfig{},
		&models.BotConfig{},
		&models.BotRule{},
		&models.GeoConfig{},
		&models.IPProvider{},
		&models.MalwareScan{},
		&models.MalwareScanSchedule{},
		&models.ThreatWhitelist{},
		&models.QuarantinedFile{},
		&models.StagingSite{},
		&models.AutoUpdateConfig{},
		&models.UpdateLog{},
		&models.AgentRelease{},
		&models.RolloutPlan{},
		&models.RolloutServer{},
		&models.PerformanceMetric{},
		&models.Subsite{},
		&models.ImageOptConfig{},
		&models.ImageOptResult{},
		&models.Team{},
		&models.TeamMember{},
		&models.TeamServer{},
		&models.FirewallRule{},
		&models.NginxConfig{},
		&models.CronJob{},
		&models.RedisConfig{},
		&models.SSHKey{},
		&models.SFTPAccount{},
		&models.RecoveryCode{},
		&models.UptimeCheck{},
		&models.UptimeLog{},
		&models.AlertRule{},
		&models.AlertHistory{},
		&models.NotificationConfig{},
		&models.IntegrationConfig{},
		&models.WPCLIHistory{},
		&models.CacheConfig{},
		&models.ExternalImport{},
		&models.DNSProvider{},
		&models.StorageCredential{},
		&models.PanelRelease{},
		// Mobile API tables
		&models.MobileRefreshToken{},
		&models.MobileNotification{},
		&models.MobilePushLog{},
		// Metrics history (mobile + dashboard)
		&models.ServerMetric{},
		// Transfer Tool
		&models.TransferJob{},
		&models.TransferSiteResult{},
		&models.CloudflareFirewallConfig{},
	)
}

// storedSecretColumns lists every column that holds a third-party credential
// encrypted at rest. It must match the fields tagged `serializer:secret` in
// internal/models (TestStoredSecretColumnsMatchModels checks this), using the
// real column names — GORM names GooglePageSpeedKey "google_page_speed_key".
var storedSecretColumns = map[string][]string{
	"dns_providers":        {"api_key", "api_email"},
	"storage_credentials":  {"secret_key"},
	"notification_configs": {"telegram_token", "slack_webhook", "smtp_pass"},
	"integration_configs":  {"google_page_speed_key"},
}

func dbString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	}
	return ""
}

// EncryptStoredSecrets encrypts credentials that older versions stored as
// plaintext. It is idempotent: values that are already encrypted are skipped,
// so it runs on every start. It works on the raw tables (no models) on purpose
// — a model's serializer hands back decrypted values and would hide which rows
// still need converting.
func EncryptStoredSecrets(db *gorm.DB, key []byte) {
	converted := 0
	for table, columns := range storedSecretColumns {
		var rows []map[string]interface{}
		if err := db.Table(table).Select(append([]string{"id"}, columns...)).Find(&rows).Error; err != nil {
			slog.Error("EncryptStoredSecrets: could not read table", "table", table, "error", err)
			continue
		}
		for _, row := range rows {
			updates := map[string]interface{}{}
			for _, column := range columns {
				value := dbString(row[column])
				if value == "" || utils.IsEncryptedSecret(value) {
					continue
				}
				enc, err := utils.EncryptSecret(key, value)
				if err != nil {
					slog.Error("EncryptStoredSecrets: encryption failed", "table", table, "column", column, "error", err)
					continue
				}
				updates[column] = enc
			}
			if len(updates) == 0 {
				continue
			}
			if err := db.Table(table).Where("id = ?", row["id"]).Updates(updates).Error; err != nil {
				slog.Error("EncryptStoredSecrets: update failed", "table", table, "id", row["id"], "error", err)
				continue
			}
			converted += len(updates)
		}
	}
	if converted > 0 {
		slog.Info("Encrypted stored third-party credentials", "values", converted)
	}
}
