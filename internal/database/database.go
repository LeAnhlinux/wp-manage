package database

import (
	"fmt"
	"log"

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

// RunManualMigrations applies schema changes that GORM AutoMigrate cannot handle
// (e.g. changing column nullability, adding columns to existing tables safely).
func RunManualMigrations(db *gorm.DB) {
	log.Println("Running manual schema migrations...")

	// Make backups.site_id nullable so backup records survive site deletion.
	// AutoMigrate will not do this — it only adds columns, never changes nullability.
	db.Exec("ALTER TABLE backups MODIFY COLUMN site_id BIGINT UNSIGNED NULL")

	// Add domain column to backups (captured at backup time, survives site deletion).
	// Use IF NOT EXISTS via a safe pattern: GORM AutoMigrate handles new columns,
	// but we use raw SQL here to ensure it runs AFTER the nullability change.
	db.Exec("ALTER TABLE backups ADD COLUMN IF NOT EXISTS domain VARCHAR(255) NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE backups ADD INDEX IF NOT EXISTS idx_backups_domain (domain)")

	// Backfill domain from the sites table for existing backup records.
	db.Exec("UPDATE backups b INNER JOIN sites s ON b.site_id = s.id SET b.domain = s.domain WHERE b.domain = ''")

	// Add server_id column to backups (allows restore dispatch even after site deletion).
	db.Exec("ALTER TABLE backups ADD COLUMN IF NOT EXISTS server_id BIGINT UNSIGNED NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE backups ADD INDEX IF NOT EXISTS idx_backups_server_id (server_id)")

	// Backfill server_id from sites for existing records.
	db.Exec("UPDATE backups b INNER JOIN sites s ON b.site_id = s.id SET b.server_id = s.server_id WHERE b.server_id = 0")

	// Add open_basedir_enabled to sites (per-site PHP open_basedir toggle).
	// Default 1 = enabled (most secure). Existing sites get open_basedir enabled.
	db.Exec("ALTER TABLE sites ADD COLUMN IF NOT EXISTS open_basedir_enabled TINYINT(1) NOT NULL DEFAULT 1")
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
		&models.MalwareScan{},
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
		&models.WPCLIHistory{},
		&models.CacheConfig{},
		&models.ExternalImport{},
		&models.DNSProvider{},
		&models.StorageCredential{},
		&models.PanelRelease{},
	)
}
