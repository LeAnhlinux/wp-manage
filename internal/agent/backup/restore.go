package backup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/agent/ssl"

	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// siteUser derives the Linux username for a WordPress site (matches site/manager.go).
func siteUser(domain string) string {
	return utils.SiteLinuxUser(domain)
}

// ensureSiteUser creates the site Linux user if it doesn't already exist.
func ensureSiteUser(domain string) {
	user := siteUser(domain)
	siteDir := fmt.Sprintf("/var/www/%s", domain)
	cmd := exec.Command("useradd", "--system", "--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--home-dir", siteDir,
		user,
	)
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		slog.Warn("Could not create site Linux user during restore", "user", user, "error", string(out))
	}
}

type RestoreParams struct {
	BackupID      uint64
	Domain        string
	Type          string // full, database_only
	FilePath      string
	DBName        string
	DBUser        string
	DBPass        string
	PHPVersion    string // fallback if manifest is missing
	EncryptionKey string // agent API key — used to decrypt DBPass stored in manifest
}

// RunRestore restores a site from a backup archive.
//
// Smart restore logic:
//   - If the site's Nginx vhost config exists  → restore only files + DB (site is already configured)
//   - If the vhost config is missing           → full recreate: DB user, dirs, nginx, php-fpm, then restore
//
// Returns the manifest used during restore (contains DB creds, PHP version, etc.)
// so the caller can recreate the panel site record for orphaned backup restores.
func RunRestore(params RestoreParams) (*Manifest, error) {
	if _, err := os.Stat(params.FilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("backup file not found: %s", params.FilePath)
	}

	// Extract archive into a temp directory
	tmpDir := filepath.Join(BackupDir, "tmp_restore_"+params.Domain)
	os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	tarCmd := exec.Command("tar", "xzf", params.FilePath, "-C", tmpDir)
	if out, err := tarCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("extract archive failed: %s %w", string(out), err)
	}

	// The archive root may be a subdirectory (staging dir name)
	extractRoot := findExtractRoot(tmpDir)
	slog.Info("Archive extracted", "root", extractRoot)

	// Read manifest.json
	manifest, err := readManifest(extractRoot)
	if err != nil {
		slog.Warn("manifest.json not found or invalid — using task params as fallback", "error", err)
		// Build a synthetic manifest from task params
		manifest = &Manifest{
			Version:    "1",
			Domain:     params.Domain,
			BackupType: params.Type,
			DBName:     params.DBName,
			DBUser:     params.DBUser,
			DBPass:     params.DBPass,
			DBHost:     "localhost",
			PHPVersion: params.PHPVersion,
			WebRoot:    fmt.Sprintf("/var/www/%s/public", params.Domain),
		}
	} else {
		// Decrypt DBPass stored in the manifest (AES-256-GCM encrypted by backup agent).
		// Falls back gracefully for legacy plaintext manifests.
		if plain, decErr := decryptSecret(manifest.DBPass, params.EncryptionKey); decErr == nil {
			manifest.DBPass = plain
		} else {
			slog.Warn("Could not decrypt manifest DBPass — using raw value", "error", decErr)
		}
	}
	// Allow overriding DB creds from task params (in case they changed after backup)
	if params.DBName != "" {
		manifest.DBName = params.DBName
	}
	if params.DBUser != "" {
		manifest.DBUser = params.DBUser
	}
	if params.DBPass != "" {
		manifest.DBPass = params.DBPass
	}

	phpVer := manifest.PHPVersion
	if phpVer == "" {
		phpVer = detectPHPVersion(manifest.Domain)
	}

	// Determine which paths exist
	nginxConfPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", manifest.Domain)
	siteExists := fileExists(nginxConfPath)

	slog.Info("Restore decision",
		"domain", manifest.Domain,
		"site_exists", siteExists,
		"backup_type", manifest.BackupType,
	)

	if !siteExists {
		slog.Info("Site does not exist — performing full site recreation", "domain", manifest.Domain)
		if err := recreateSite(extractRoot, manifest, phpVer); err != nil {
			return nil, fmt.Errorf("site recreation failed: %w", err)
		}
	}

	// Restore files (for full / incremental)
	if manifest.BackupType != "database_only" {
		if err := restoreFiles(extractRoot, manifest); err != nil {
			return nil, fmt.Errorf("file restore failed: %w", err)
		}
	}

	// Restore database
	if err := restoreDatabase(extractRoot, manifest); err != nil {
		return nil, fmt.Errorf("database restore failed: %w", err)
	}

	// Fix permissions & reload services — use the site's dedicated Linux user
	siteDir := fmt.Sprintf("/var/www/%s", manifest.Domain)
	su := siteUser(manifest.Domain)
	owner := fmt.Sprintf("%s:%s", su, su)
	exec.Command("chown", "-R", owner, siteDir).Run()
	exec.Command("find", siteDir, "-type", "d", "-exec", "chmod", "2755", "{}", "+").Run()
	exec.Command("find", siteDir, "-type", "f", "-exec", "chmod", "644", "{}", "+").Run()
	exec.Command("chmod", "640", fmt.Sprintf("%s/public/wp-config.php", siteDir)).Run()

	// Ensure PHP version is recorded in manifest for caller
	if manifest.PHPVersion == "" {
		manifest.PHPVersion = phpVer
	}

	exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", phpVer)).Run()
	exec.Command("nginx", "-t").Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	// Attempt to re-issue SSL after restore (full/incremental only, not database_only).
	// Reason: cert may not exist on this server (fresh restore, migration target, etc.)
	// and browsers with HSTS cached will refuse HTTP connections.
	//
	// DNS check first: certbot HTTP-01 challenge requires the domain to resolve to
	// this server's IP. If DNS is wrong we skip certbot and leave the site HTTP-only;
	// the ssl_expiry_worker will retry every 12h once DNS propagates.
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", manifest.Domain)
	if manifest.BackupType != "database_only" && !fileExists(certPath) {
		if dnsErr := ssl.CheckDNS(manifest.Domain); dnsErr != nil {
			slog.Warn("SSL skipped after restore: DNS not pointing here yet",
				"domain", manifest.Domain, "reason", dnsErr)
			manifest.HasSSL = false
		} else {
			slog.Info("Attempting SSL certificate issuance after restore", "domain", manifest.Domain)
			if err := runCertbot(manifest.Domain); err != nil {
				slog.Warn("Certbot failed after restore — site accessible via HTTP only",
					"domain", manifest.Domain, "error", err)
				manifest.HasSSL = false
			} else {
				slog.Info("SSL certificate issued successfully after restore", "domain", manifest.Domain)
				manifest.HasSSL = true
				exec.Command("nginx", "-t").Run()
				exec.Command("systemctl", "reload", "nginx").Run()
			}
		}
	}

	slog.Info("Restore completed successfully", "domain", manifest.Domain, "site_existed", siteExists)
	return manifest, nil
}

// recreateSite provisions the site infrastructure when it doesn't exist:
//  1. Create database + MySQL user
//  2. Create web root directories
//  3. Restore Nginx vhost config (from backup or generate minimal)
//  4. Restore PHP-FPM pool (from backup or generate minimal)
//  5. Enable nginx site symlink
func recreateSite(extractRoot string, m *Manifest, phpVer string) error {
	slog.Info("Recreating site infrastructure", "domain", m.Domain)

	// Ensure the site's Linux user exists before setting up PHP-FPM / permissions
	ensureSiteUser(m.Domain)

	// 1) Create database + user (use ALTER USER to reset password if user already exists)
	createDBSQL := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"+
			"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';"+
			"ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';"+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';"+
			"FLUSH PRIVILEGES;",
		m.DBName,
		m.DBUser, m.DBPass,      // CREATE USER IF NOT EXISTS (noop if exists)
		m.DBUser, m.DBPass,      // ALTER USER — always sets correct password
		m.DBName, m.DBUser,
	)
	dbCmd := exec.Command("mysql", "-u", "root", "-e", createDBSQL)
	if out, err := dbCmd.CombinedOutput(); err != nil {
		// Try without -u root (some setups use socket auth)
		dbCmd2 := exec.Command("bash", "-c",
			fmt.Sprintf("mysql -e %q", createDBSQL))
		if out2, err2 := dbCmd2.CombinedOutput(); err2 != nil {
			slog.Warn("DB creation warning", "err1", string(out), "err2", string(out2))
		}
	}
	slog.Info("Database ready", "db", m.DBName, "user", m.DBUser)

	// 2) Create web root directories
	siteDir := fmt.Sprintf("/var/www/%s", m.Domain)
	for _, dir := range []string{siteDir + "/public", siteDir + "/logs"} {
		os.MkdirAll(dir, 0755)
	}
	su := siteUser(m.Domain)
	exec.Command("chown", "-R", fmt.Sprintf("%s:%s", su, su), siteDir).Run()

	// 3) Restore Nginx config from backup, or generate minimal.
	// Strategy:
	//   - If SSL cert exists: restore backup config as-is (cert is valid, no stripping needed)
	//   - If SSL cert missing: always write a fresh minimal HTTP-only config.
	//     Reason: backup may contain certbot-managed SSL directives + redirect server blocks;
	//     trying to strip them surgically is fragile. A minimal config is always valid nginx
	//     syntax and certbot will add SSL to it cleanly after restore.
	nginxConfPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", m.Domain)
	backupNginxConf := filepath.Join(extractRoot, "config", "nginx.conf")
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", m.Domain)
	certExists := fileExists(certPath)

	if certExists && fileExists(backupNginxConf) {
		// Cert already in place — restore full backup config (SSL directives are valid)
		if err := copyFileIfExists(backupNginxConf, nginxConfPath); err != nil {
			slog.Warn("Could not restore nginx config — generating minimal", "error", err)
			writeMinimalNginxConf(nginxConfPath, m.Domain, phpVer)
		} else {
			slog.Info("Nginx config restored from backup (cert present)", "domain", m.Domain)
		}
	} else {
		// No cert yet — write clean HTTP-only minimal config.
		// Certbot will modify it to add SSL after restore completes.
		if !certExists {
			slog.Info("SSL cert missing — writing minimal HTTP-only nginx config (certbot will add SSL)", "domain", m.Domain)
		} else {
			slog.Warn("No nginx config in backup — generating minimal config", "domain", m.Domain)
		}
		writeMinimalNginxConf(nginxConfPath, m.Domain, phpVer)
	}

	// 4) Restore PHP-FPM pool from backup, or generate minimal
	phpfpmConfPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVer, m.Domain)
	backupPhpfpmConf := filepath.Join(extractRoot, "config", "phpfpm.conf")
	if fileExists(backupPhpfpmConf) {
		copyFileIfExists(backupPhpfpmConf, phpfpmConfPath)
		// FIX: rewrite user/group to match the Linux user for THIS domain.
		// The backup was taken on a different server (or with a different domain),
		// so the pool may still have the old user (e.g. wp_test5_wppanel_io_vn).
		// Running PHP-FPM as the wrong user → Permission denied on wp-config.php.
		fixPhpfpmPoolUser(phpfpmConfPath, su)
		slog.Info("PHP-FPM pool restored from backup", "domain", m.Domain)
	} else {
		slog.Warn("No phpfpm config in backup — generating minimal pool", "domain", m.Domain)
		writeMinimalPhpfpmConf(phpfpmConfPath, m.Domain, phpVer)
	}

	// 5) Enable nginx site
	symlinkPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", m.Domain)
	if !fileExists(symlinkPath) {
		os.Symlink(nginxConfPath, symlinkPath)
	}

	slog.Info("Site infrastructure recreated", "domain", m.Domain)
	return nil
}

// restoreFiles syncs the backed-up files/ tree into the live web root.
func restoreFiles(extractRoot string, m *Manifest) error {
	// Detection order (newest → oldest backup format):
	//   1. files/public/   — current format (v2, after html→public fix)
	//   2. files/html/     — intermediate format (v2, before html→public fix)
	//   3. public/         — old format: extractRoot IS the domain dir, has public/ directly
	//   4. {domain}/       — very old format
	filesDir := ""

	for _, candidate := range []string{
		filepath.Join(extractRoot, "files", "public"),  // current
		filepath.Join(extractRoot, "files", "html"),    // intermediate legacy
		filepath.Join(extractRoot, "public"),           // old direct-rsync format
		filepath.Join(extractRoot, m.Domain),           // very old format
	} {
		if _, err := os.Stat(candidate); err == nil {
			filesDir = candidate
			slog.Info("Found files directory in backup", "path", candidate)
			break
		}
	}

	if filesDir == "" {
		slog.Warn("No files directory found in backup — skipping file restore")
		return nil
	}

	webRoot := fmt.Sprintf("/var/www/%s/public", m.Domain)
	os.MkdirAll(webRoot, 0755)

	rsyncCmd := exec.Command("rsync", "-a", "--delete",
		filesDir+"/", webRoot+"/",
	)
	if out, err := rsyncCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync restore failed: %s %w", string(out), err)
	}
	slog.Info("Files restored", "domain", m.Domain, "destination", webRoot)
	return nil
}

// restoreDatabase imports the SQL dump from the archive.
func restoreDatabase(extractRoot string, m *Manifest) error {
	sqlFile := filepath.Join(extractRoot, "database", "dump.sql")
	if _, err := os.Stat(sqlFile); err != nil {
		// Try all known legacy locations for the SQL dump:
		// 1. extractRoot/*_db.sql  (intermediate legacy)
		// 2. tmpDir/*_db.sql       (very old format: SQL was placed at archive root, one level above extractRoot)
		tmpDir := filepath.Dir(extractRoot) // parent of extractRoot is the tmpDir
		candidates := []string{
			extractRoot,
			tmpDir,
		}
		sqlFile = ""
		for _, dir := range candidates {
			matches, _ := filepath.Glob(filepath.Join(dir, "*_db.sql"))
			if len(matches) > 0 {
				sqlFile = matches[0]
				break
			}
		}
		if sqlFile == "" {
			slog.Warn("No SQL dump found in backup — skipping database restore")
			return nil
		}
	}

	slog.Info("Restoring database", "domain", m.Domain, "db", m.DBName, "file", sqlFile)

	// Drop + recreate DB to ensure clean state (avoids duplicate key errors)
	dropRecreateSQL := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
		m.DBName, m.DBName,
	)
	exec.Command("mysql", "-u", "root", "-e", dropRecreateSQL).Run()

	// Import SQL dump
	cmd := fmt.Sprintf("mysql --binary-mode=1 --user=%s --password=%s %s < %s",
		m.DBUser, m.DBPass, m.DBName, sqlFile)
	shellCmd := exec.Command("bash", "-c", cmd)
	if out, err := shellCmd.CombinedOutput(); err != nil {
		// Fallback: try as root
		cmd2 := fmt.Sprintf("mysql --binary-mode=1 %s < %s", m.DBName, sqlFile)
		if out2, err2 := exec.Command("bash", "-c", cmd2).CombinedOutput(); err2 != nil {
			return fmt.Errorf("database import failed: %s / %s", string(out), string(out2))
		}
	}
	slog.Info("Database restored", "domain", m.Domain, "db", m.DBName)
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// findExtractRoot returns the first child DIRECTORY of tmpDir if there is exactly
// one directory entry (ignoring files like *_db.sql at the top level).
// Otherwise returns tmpDir itself (for flat archives).
func findExtractRoot(tmpDir string) string {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return tmpDir
	}
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	if len(dirs) == 1 {
		return filepath.Join(tmpDir, dirs[0].Name())
	}
	return tmpDir
}

func readManifest(root string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// restoreNginxConf copies the backup nginx config but strips SSL blocks if the
// Let's Encrypt cert no longer exists (site was deleted, cert was cleaned up).
// Without this, a restored nginx.conf with SSL directives pointing to missing
// certs would break nginx -t and prevent ANY site from loading.
func restoreNginxConf(src, dst, domain string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
	if !fileExists(certPath) {
		// Strip SSL — rewrite to HTTP-only config
		slog.Warn("SSL cert missing for domain — stripping SSL from restored nginx config", "domain", domain)
		content := string(data)
		// Remove SSL-related lines added by certbot / our own provisioner
		stripped := stripNginxSSL(content)
		return os.WriteFile(dst, []byte(stripped), 0644)
	}

	// Cert exists — use config as-is
	return copyFileIfExists(src, dst)
}

// stripNginxSSL rewrites a nginx server block to HTTP-only by removing SSL directives
// and changing listen 443 ssl → listen 80, and removing certbot redirect blocks.
func stripNginxSSL(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	skipBlock := false
	braceDepth := 0
	inCertbotRedirectBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect certbot redirect server block: server { if ($host = ...) { return 301 https...
		if strings.HasPrefix(trimmed, "server {") {
			// Count how many server blocks we've seen; the redirect block has "return 301 https"
			// We'll use a simple heuristic: once we see a line with "return 301 https" inside a server block, skip to closing
		}

		// Skip certbot SSL directive lines
		if strings.Contains(trimmed, "# managed by Certbot") {
			continue
		}
		if strings.Contains(trimmed, "ssl_certificate") ||
			strings.Contains(trimmed, "ssl_certificate_key") ||
			strings.Contains(trimmed, "options-ssl-nginx.conf") ||
			strings.Contains(trimmed, "ssl-dhparams.pem") {
			continue
		}

		// Change "listen 443 ssl" → "listen 80"
		if strings.Contains(trimmed, "listen 443") {
			out = append(out, strings.ReplaceAll(line, "listen 443 ssl;", "listen 80;"))
			continue
		}

		// Detect and skip certbot's HTTP→HTTPS redirect server block
		// It looks like: server { if ($host = domain) { return 301 https...; } ... return 404; }
		if !inCertbotRedirectBlock && strings.HasPrefix(trimmed, "server {") {
			// peek ahead is hard, so track brace depth
			_ = skipBlock
			_ = braceDepth
		}
		if strings.Contains(trimmed, "return 301 https://") {
			inCertbotRedirectBlock = true
		}
		if inCertbotRedirectBlock {
			// Count braces to find end of this server block
			for _, ch := range line {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}
			if braceDepth <= 0 {
				inCertbotRedirectBlock = false
				braceDepth = 0
			}
			continue // skip all lines in certbot redirect block
		}

		// Ensure listen 80 exists if we stripped all listen directives
		out = append(out, line)
	}

	result := strings.Join(out, "\n")

	// If there's no "listen 80" left in the config, inject it
	if !strings.Contains(result, "listen 80") {
		result = strings.Replace(result, "server_name", "listen 80;\n    server_name", 1)
	}

	return result
}

// writeMinimalNginxConf generates a basic Nginx vhost when no backup config exists.
func writeMinimalNginxConf(path, domain, phpVer string) {
	cfg := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;

    root /var/www/%s/public;
    index index.php index.html;

    access_log /var/www/%s/logs/access.log;
    error_log  /var/www/%s/logs/error.log;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php%s-fpm-%s.sock;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }

    location ~ /\.ht {
        deny all;
    }
}
`, domain, domain, domain, domain, domain, phpVer, domain)
	os.WriteFile(path, []byte(cfg), 0644)
}

// runCertbot attempts to issue/renew a Let's Encrypt SSL certificate for the domain.
// It uses an existing certbot account if available, otherwise registers unsafely without email.
func runCertbot(domain string) error {
	// Read email from config file if available (written by provisioner on server setup)
	email := ""
	if data, err := os.ReadFile("/opt/wp-manage/certbot-email"); err == nil {
		email = strings.TrimSpace(string(data))
	}

	args := []string{
		"--nginx",
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
	}
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	cmd := exec.Command("certbot", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("certbot: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// writeMinimalPhpfpmConf generates a basic PHP-FPM pool when no backup config exists.
func writeMinimalPhpfpmConf(path, domain, phpVer string) {
	su := siteUser(domain)

	cfg := fmt.Sprintf(`[%s]
user  = %s
group = %s

listen = /run/php/php%s-fpm-%s.sock
listen.owner = www-data
listen.group = www-data
listen.mode  = 0660

pm                   = dynamic
pm.max_children      = 10
pm.start_servers     = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
pm.max_requests      = 500

php_admin_value[error_log]    = /var/www/%s/logs/php-error.log
php_admin_flag[log_errors]    = on
php_admin_value[memory_limit] = 256M
php_admin_value[open_basedir] = /var/www/%s/:/tmp/:/usr/share/php/
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,parse_ini_file,show_source
`, domain, su, su, phpVer, domain, domain, domain)
	os.WriteFile(path, []byte(cfg), 0644)
}

// fixPhpfpmPoolUser rewrites the user and group directives in a PHP-FPM pool
// config to match the correct Linux user for the target domain. This is needed
// after restoring a pool config from backup: the backup may have been taken on
// a different server where the site user had a different name (e.g. the source
// domain was test5 but the target domain is test6, so the user differs).
func fixPhpfpmPoolUser(poolPath, correctUser string) {
	raw, err := os.ReadFile(poolPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "user") && strings.Contains(trimmed, "=") {
			lines[i] = "user = " + correctUser
		} else if strings.HasPrefix(trimmed, "group") && strings.Contains(trimmed, "=") {
			lines[i] = "group = " + correctUser
		}
	}
	os.WriteFile(poolPath, []byte(strings.Join(lines, "\n")), 0644)
}
