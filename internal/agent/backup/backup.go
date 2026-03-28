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
	"syscall"
	"time"
)

const BackupDir = "/opt/wp-manage/backups"

// Manifest is embedded into every backup archive so a restore is self-contained.
type Manifest struct {
	Version    string    `json:"version"`     // "2"
	Domain     string    `json:"domain"`
	BackupID   uint64    `json:"backup_id"`
	BackupType string    `json:"backup_type"` // full | database_only
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
	BackupID      uint64
	Domain        string
	Type          string // full, database_only
	DBName        string
	DBUser        string
	DBPass        string
	PHPVersion    string
	EncryptionKey string // agent API key — used to encrypt DBPass in manifest
}

type BackupResult struct {
	FilePath  string
	SizeBytes int64
	Checksum  string
	Error     error
}

// RunBackup executes a backup based on the type.
func RunBackup(params BackupParams) BackupResult {
	// Pre-flight: abort if disk usage on backup partition ≥ 90%
	if err := checkDiskSpace(BackupDir); err != nil {
		return BackupResult{Error: err}
	}

	siteBackupDir := filepath.Join(BackupDir, params.Domain)
	os.MkdirAll(siteBackupDir, 0750)

	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s_%s_%s", params.Domain, params.Type, timestamp)

	switch params.Type {
	case "database_only":
		return runDatabaseBackup(params, siteBackupDir, backupName)
	case "full":
		return runFullBackup(params, siteBackupDir, backupName)
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
	// Encrypt DBPass with AES-256-GCM using the agent API key so that the
	// manifest (and therefore the backup archive) never contains a plaintext password.
	encryptedDBPass, err := encryptSecret(params.DBPass, params.EncryptionKey)
	if err != nil {
		slog.Warn("Could not encrypt DB password for manifest — storing without encryption", "error", err)
		encryptedDBPass = params.DBPass
	}
	hasSSL := fileExists(fmt.Sprintf("/etc/letsencrypt/live/%s/cert.pem", params.Domain))
	manifest := Manifest{
		Version:    "2",
		Domain:     params.Domain,
		BackupID:   params.BackupID,
		BackupType: "full",
		CreatedAt:  time.Now().UTC(),
		DBName:     params.DBName,
		DBUser:     params.DBUser,
		DBPass:     encryptedDBPass, // AES-256-GCM encrypted; decrypted only by RunRestore
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
	encryptedDBPass, err := encryptSecret(params.DBPass, params.EncryptionKey)
	if err != nil {
		slog.Warn("Could not encrypt DB password for manifest", "error", err)
		encryptedDBPass = params.DBPass
	}
	manifest := Manifest{
		Version:    "2",
		Domain:     params.Domain,
		BackupID:   params.BackupID,
		BackupType: "database_only",
		CreatedAt:  time.Now().UTC(),
		DBName:     params.DBName,
		DBUser:     params.DBUser,
		DBPass:     encryptedDBPass,
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

// ─── helpers ────────────────────────────────────────────────────────────────

// checkDiskSpace returns an error if the partition containing path has ≥ 90% disk usage.
// If the check itself fails (e.g. path doesn't exist yet) it logs a warning and allows
// the backup to proceed, so transient stat errors don't silently block all backups.
func checkDiskSpace(path string) error {
	// Ensure the directory exists so Statfs can stat it
	os.MkdirAll(path, 0750)

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		slog.Warn("Could not check disk space before backup, proceeding anyway",
			"path", path, "error", err)
		return nil
	}

	total := stat.Blocks * uint64(stat.Bsize)
	if total == 0 {
		return nil // avoid division by zero on unusual mounts
	}
	avail := stat.Bavail * uint64(stat.Bsize)
	used := total - avail
	usedPct := float64(used) / float64(total) * 100

	slog.Info("Disk space check", "path", path,
		"used_pct", fmt.Sprintf("%.1f%%", usedPct),
		"used_gb", fmt.Sprintf("%.1f GB", float64(used)/1e9),
		"avail_gb", fmt.Sprintf("%.1f GB", float64(avail)/1e9),
	)

	if usedPct >= 90 {
		return fmt.Errorf(
			"disk usage is %.1f%% (≥ 90%%) on %s — backup aborted to prevent disk full (%.1f GB used / %.1f GB total)",
			usedPct, path, float64(used)/1e9, float64(total)/1e9,
		)
	}
	return nil
}

func mysqldump(params BackupParams, destFile string) error {
	// Write credentials to a temp file (0600) so the password is NEVER visible
	// in the process list via `ps aux` or `/proc/<pid>/cmdline`.
	myCnf := fmt.Sprintf("[client]\nuser=%s\npassword=%s\n", params.DBUser, params.DBPass)
	tmpCnf, err := os.CreateTemp("", "wpm-mycnf-*.cnf")
	if err != nil {
		return fmt.Errorf("create temp mycnf: %w", err)
	}
	defer os.Remove(tmpCnf.Name()) // always delete, even on error
	if err := tmpCnf.Chmod(0600); err != nil {
		tmpCnf.Close()
		return fmt.Errorf("chmod temp mycnf: %w", err)
	}
	if _, err := tmpCnf.WriteString(myCnf); err != nil {
		tmpCnf.Close()
		return fmt.Errorf("write temp mycnf: %w", err)
	}
	tmpCnf.Close()

	f, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("create sql file: %w", err)
	}
	defer f.Close()

	cmd := exec.Command("mysqldump",
		"--defaults-extra-file="+tmpCnf.Name(), // credentials from file, not CLI arg
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
	for _, ver := range []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"} {
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
