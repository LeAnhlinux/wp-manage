package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const BackupDir = "/opt/wp-manage/backups"

// Manifest is embedded into every backup archive so a restore is self-contained.
type Manifest struct {
	Version    string    `json:"version"`     // "2"
	Domain     string    `json:"domain"`
	BackupID   uint64    `json:"backup_id"`
	BackupType string    `json:"backup_type"` // full | incremental | database_only
	CreatedAt  time.Time `json:"created_at"`
	DBName     string    `json:"db_name"`
	DBUser     string    `json:"db_user"`
	DBPass     string    `json:"db_pass"`
	DBHost     string    `json:"db_host"`
	PHPVersion string    `json:"php_version"`
	HasSSL     bool      `json:"has_ssl"`
	WebRoot    string    `json:"web_root"` // absolute path, e.g. /var/www/example.com/html
}

type BackupParams struct {
	BackupID   uint64
	Domain     string
	Type       string // full, incremental, database_only
	DBName     string
	DBUser     string
	DBPass     string
	PHPVersion string
}

type BackupResult struct {
	FilePath  string
	SizeBytes int64
	Checksum  string
	Error     error
}

// RunBackup executes a backup based on the type.
func RunBackup(params BackupParams) BackupResult {
	siteBackupDir := filepath.Join(BackupDir, params.Domain)
	os.MkdirAll(siteBackupDir, 0750)

	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s_%s_%s", params.Domain, params.Type, timestamp)

	switch params.Type {
	case "database_only":
		return runDatabaseBackup(params, siteBackupDir, backupName)
	case "full":
		return runFullBackup(params, siteBackupDir, backupName)
	case "incremental":
		return runIncrementalBackup(params, siteBackupDir, backupName)
	default:
		return BackupResult{Error: fmt.Errorf("unknown backup type: %s", params.Type)}
	}
}

// runFullBackup creates a structured tar.gz that includes:
//   - manifest.json  (all metadata needed for restore)
//   - files/html/    (WordPress web root)
//   - database/dump.sql
//   - config/nginx.conf    (nginx vhost)
//   - config/phpfpm.conf   (PHP-FPM pool)
func runFullBackup(params BackupParams, dir, name string) BackupResult {
	slog.Info("Starting full backup", "domain", params.Domain, "backup_id", params.BackupID)

	phpVer := params.PHPVersion
	if phpVer == "" {
		phpVer = detectPHPVersion(params.Domain)
	}

	webRoot := fmt.Sprintf("/var/www/%s/public", params.Domain)

	// Build a staging directory so we can create the structured archive
	stagingDir := filepath.Join(dir, ".staging_"+name)
	defer os.RemoveAll(stagingDir)

	for _, sub := range []string{"files/public", "database", "config"} {
		if err := os.MkdirAll(filepath.Join(stagingDir, sub), 0755); err != nil {
			return BackupResult{Error: fmt.Errorf("create staging dir: %w", err)}
		}
	}

	// 1) Database dump → staging/database/dump.sql
	sqlFile := filepath.Join(stagingDir, "database", "dump.sql")
	if err := mysqldump(params, sqlFile); err != nil {
		return BackupResult{Error: err}
	}

	// 2) Nginx vhost → staging/config/nginx.conf
	nginxSrc := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", params.Domain)
	if err := copyFileIfExists(nginxSrc, filepath.Join(stagingDir, "config", "nginx.conf")); err != nil {
		slog.Warn("Could not copy nginx config", "error", err)
	}

	// 3) PHP-FPM pool → staging/config/phpfpm.conf
	phpfpmSrc := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVer, params.Domain)
	if err := copyFileIfExists(phpfpmSrc, filepath.Join(stagingDir, "config", "phpfpm.conf")); err != nil {
		slog.Warn("Could not copy phpfpm config", "error", err)
	}

	// 4) Site files → staging/files/public/ (rsync preserves permissions)
	filesTarget := filepath.Join(stagingDir, "files", "public")
	rsyncCmd := exec.Command("rsync", "-a",
		"--exclude=*.log",
		"--exclude=wp-content/cache/",
		"--exclude=wp-content/upgrade/",
		webRoot+"/", filesTarget+"/",
	)
	if out, err := rsyncCmd.CombinedOutput(); err != nil {
		// tolerate partial failures (cache files locked etc.)
		slog.Warn("rsync warnings during backup", "output", string(out))
	}

	// 5) manifest.json → staging/manifest.json
	hasSSL := fileExists(fmt.Sprintf("/etc/letsencrypt/live/%s/cert.pem", params.Domain))
	manifest := Manifest{
		Version:    "2",
		Domain:     params.Domain,
		BackupID:   params.BackupID,
		BackupType: "full",
		CreatedAt:  time.Now().UTC(),
		DBName:     params.DBName,
		DBUser:     params.DBUser,
		DBPass:     params.DBPass,
		DBHost:     "localhost",
		PHPVersion: phpVer,
		HasSSL:     hasSSL,
		WebRoot:    webRoot, // /var/www/{domain}/public
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(stagingDir, "manifest.json"), manifestBytes, 0600); err != nil {
		return BackupResult{Error: fmt.Errorf("write manifest: %w", err)}
	}

	// 6) Pack staging dir into final tar.gz
	tarFile := filepath.Join(dir, name+".tar.gz")
	tarCmd := exec.Command("tar", "czf", tarFile,
		"--warning=no-file-changed",
		"-C", filepath.Dir(stagingDir), filepath.Base(stagingDir),
	)
	if out, err := tarCmd.CombinedOutput(); err != nil {
		if tarCmd.ProcessState != nil && tarCmd.ProcessState.ExitCode() > 1 {
			return BackupResult{Error: fmt.Errorf("tar failed: %s %w", string(out), err)}
		}
		slog.Warn("tar completed with warnings", "output", string(out))
	}

	slog.Info("Full backup completed", "domain", params.Domain, "file", tarFile)
	return fileResult(tarFile)
}

func runDatabaseBackup(params BackupParams, dir, name string) BackupResult {
	slog.Info("Starting database-only backup", "domain", params.Domain, "db", params.DBName)

	stagingDir := filepath.Join(dir, ".staging_"+name)
	defer os.RemoveAll(stagingDir)
	os.MkdirAll(filepath.Join(stagingDir, "database"), 0755)

	// Write a minimal manifest so restore can read DB creds
	phpVer := params.PHPVersion
	if phpVer == "" {
		phpVer = detectPHPVersion(params.Domain)
	}
	manifest := Manifest{
		Version:    "2",
		Domain:     params.Domain,
		BackupID:   params.BackupID,
		BackupType: "database_only",
		CreatedAt:  time.Now().UTC(),
		DBName:     params.DBName,
		DBUser:     params.DBUser,
		DBPass:     params.DBPass,
		DBHost:     "localhost",
		PHPVersion: phpVer,
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(stagingDir, "manifest.json"), manifestBytes, 0600)

	sqlFile := filepath.Join(stagingDir, "database", "dump.sql")
	if err := mysqldump(params, sqlFile); err != nil {
		return BackupResult{Error: err}
	}

	tarFile := filepath.Join(dir, name+".sql.tar.gz")
	tarCmd := exec.Command("tar", "czf", tarFile,
		"-C", filepath.Dir(stagingDir), filepath.Base(stagingDir),
	)
	if out, err := tarCmd.CombinedOutput(); err != nil {
		return BackupResult{Error: fmt.Errorf("tar failed: %s %w", string(out), err)}
	}

	return fileResult(tarFile)
}

func runIncrementalBackup(params BackupParams, dir, name string) BackupResult {
	slog.Info("Starting incremental backup", "domain", params.Domain)

	phpVer := params.PHPVersion
	if phpVer == "" {
		phpVer = detectPHPVersion(params.Domain)
	}

	webRoot := fmt.Sprintf("/var/www/%s/public", params.Domain)
	stagingDir := filepath.Join(dir, ".staging_"+name)
	defer os.RemoveAll(stagingDir)

	for _, sub := range []string{"files/public", "database", "config"} {
		os.MkdirAll(filepath.Join(stagingDir, sub), 0755)
	}

	// DB dump (always full for incremental)
	sqlFile := filepath.Join(stagingDir, "database", "dump.sql")
	if err := mysqldump(params, sqlFile); err != nil {
		return BackupResult{Error: err}
	}

	// Configs
	copyFileIfExists(
		fmt.Sprintf("/etc/nginx/sites-available/%s.conf", params.Domain),
		filepath.Join(stagingDir, "config", "nginx.conf"),
	)
	copyFileIfExists(
		fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVer, params.Domain),
		filepath.Join(stagingDir, "config", "phpfpm.conf"),
	)

	// Incremental files: only changed in last 24h
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	filesTarget := filepath.Join(stagingDir, "files", "public")
	rsyncCmd := exec.Command("rsync", "-a",
		"--newer="+yesterday,
		"--exclude=*.log",
		"--exclude=wp-content/cache/",
		webRoot+"/", filesTarget+"/",
	)
	rsyncCmd.CombinedOutput() // tolerate errors

	// Manifest
	hasSSL := fileExists(fmt.Sprintf("/etc/letsencrypt/live/%s/cert.pem", params.Domain))
	manifest := Manifest{
		Version:    "2",
		Domain:     params.Domain,
		BackupID:   params.BackupID,
		BackupType: "incremental",
		CreatedAt:  time.Now().UTC(),
		DBName:     params.DBName,
		DBUser:     params.DBUser,
		DBPass:     params.DBPass,
		DBHost:     "localhost",
		PHPVersion: phpVer,
		HasSSL:     hasSSL,
		WebRoot:    webRoot, // /var/www/{domain}/public
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(stagingDir, "manifest.json"), manifestBytes, 0600)

	tarFile := filepath.Join(dir, name+".tar.gz")
	tarCmd := exec.Command("tar", "czf", tarFile,
		"--warning=no-file-changed",
		"-C", filepath.Dir(stagingDir), filepath.Base(stagingDir),
	)
	if out, err := tarCmd.CombinedOutput(); err != nil {
		if tarCmd.ProcessState != nil && tarCmd.ProcessState.ExitCode() > 1 {
			return BackupResult{Error: fmt.Errorf("tar failed: %s %w", string(out), err)}
		}
	}

	return fileResult(tarFile)
}

// ─── helpers ────────────────────────────────────────────────────────────────

func mysqldump(params BackupParams, destFile string) error {
	f, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("create sql file: %w", err)
	}
	defer f.Close()

	cmd := exec.Command("mysqldump",
		"--user="+params.DBUser,
		"--password="+params.DBPass,
		"--single-transaction",
		"--quick",
		"--lock-tables=false",
		"--routines",
		"--triggers",
		params.DBName,
	)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(destFile)
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return nil
}

func copyFileIfExists(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err // not fatal — caller logs warning
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectPHPVersion(domain string) string {
	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(confPath)
	if err != nil {
		return "8.2"
	}
	content := string(data)
	for _, ver := range []string{"8.4", "8.3", "8.2", "8.1", "8.0", "7.4"} {
		if strings.Contains(content, "php"+ver+"-fpm") {
			return ver
		}
	}
	return "8.2"
}

func fileResult(filePath string) BackupResult {
	info, err := os.Stat(filePath)
	if err != nil {
		return BackupResult{Error: fmt.Errorf("stat backup file: %w", err)}
	}
	checksum, err := sha256File(filePath)
	if err != nil {
		return BackupResult{Error: fmt.Errorf("checksum: %w", err)}
	}
	return BackupResult{
		FilePath:  filePath,
		SizeBytes: info.Size(),
		Checksum:  checksum,
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
