package site

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// SiteUser returns the Linux username for a WordPress site.
// Convention: "wp_" + domain with dots and hyphens replaced by underscores.
// e.g. "example.com" → "wp_example_com", "my-site.co.uk" → "wp_my_site_co_uk"
func SiteUser(domain string) string {
	return "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
}

// createSiteUser creates a system user for the site (no login shell, no home dir creation).
func createSiteUser(domain string) error {
	user := SiteUser(domain)
	cmd := exec.Command("useradd",
		"--system",
		"--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--home-dir", fmt.Sprintf("/var/www/%s", domain),
		user,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "already exists") {
			return nil // idempotent
		}
		return fmt.Errorf("useradd failed for %s: %s", user, string(out))
	}
	slog.Info("Created site Linux user", "user", user, "domain", domain)
	return nil
}

// deleteSiteUser removes the site's Linux user and group.
func deleteSiteUser(domain string) {
	user := SiteUser(domain)
	exec.Command("userdel", user).Run()
	exec.Command("groupdel", user).Run()
	slog.Info("Deleted site Linux user", "user", user, "domain", domain)
}

type CreateParams struct {
	Domain          string
	PHPVersion      string
	SSLEnabled      bool
	RedisEnabled    bool
	DBName          string
	DBUser          string
	DBPassword      string
	WPAdminUser     string
	WPAdminEmail    string
	WPAdminPass     string
	AutologinSecret string // generated during creation, reported back to API
}

type ProgressFunc func(step string, progress int, message string)

type Manager struct {
	onProgress ProgressFunc
}

func NewManager(onProgress ProgressFunc) *Manager {
	return &Manager{onProgress: onProgress}
}

// GetWPVersion returns the WordPress version installed at the given webroot.
func GetWPVersion(domain string) string {
	webRoot := fmt.Sprintf("/var/www/%s/public", domain)
	out, err := exec.Command("wp", "core", "version", "--path="+webRoot, "--allow-root").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *Manager) CreateSite(params CreateParams) error {
	siteRoot := fmt.Sprintf("/var/www/%s", params.Domain)
	webRoot := siteRoot + "/public"
	phpVer := params.PHPVersion
	if phpVer == "" {
		phpVer = "8.2"
	}

	// Validate that the requested PHP version is actually installed on this server.
	// Check for the FPM directory — if it doesn't exist, PHP is not installed.
	fpmDir := fmt.Sprintf("/etc/php/%s/fpm", phpVer)
	if _, err := os.Stat(fpmDir); err != nil {
		// List what IS installed to give a helpful error message
		allVersions := []string{"7.4", "8.0", "8.1", "8.2", "8.3"}
		var installedVersions []string
		for _, v := range allVersions {
			if _, e := os.Stat(fmt.Sprintf("/etc/php/%s/fpm", v)); e == nil {
				installedVersions = append(installedVersions, "PHP "+v)
			}
		}
		msg := fmt.Sprintf("PHP %s chưa được cài trên server này", phpVer)
		if len(installedVersions) > 0 {
			msg += fmt.Sprintf(". Các version đã cài: %s", strings.Join(installedVersions, ", "))
		} else {
			msg += ". Không tìm thấy PHP version nào được cài. Hãy cài PHP từ tab Server > PHP trước."
		}
		return fmt.Errorf("%s", msg)
	}

	steps := []struct {
		name string
		pct  int
		fn   func() error
	}{
		{"create_linux_user", 5, func() error { return createSiteUser(params.Domain) }},
		{"create_directories", 10, func() error { return m.createDirs(siteRoot, webRoot) }},
		{"create_database", 20, func() error { return m.createDatabase(params) }},
		{"configure_nginx", 35, func() error { return m.configureNginx(params, webRoot, phpVer) }},
		{"configure_php_fpm", 45, func() error { return m.configurePHPPool(params, phpVer) }},
		{"download_wordpress", 60, func() error { return m.downloadWordPress(webRoot) }},
		{"configure_wordpress", 70, func() error { return m.configureWordPress(params, webRoot) }},
		{"install_wordpress", 80, func() error { return m.installWordPress(params, webRoot) }},
		{"set_permissions", 85, func() error { return m.setPermissions(siteRoot, params.Domain) }},
		{"issue_ssl", 90, func() error {
			if params.SSLEnabled {
				return m.issueSSL(params.Domain, params.WPAdminEmail)
			}
			return nil
		}},
		{"security_hardening", 92, func() error { return m.hardenWordPress(webRoot) }},
		{"install_autologin", 96, func() error {
			secret, err := m.installAutologin(webRoot, params.Domain)
			if err != nil {
				slog.Warn("Autologin install failed (non-critical)", "error", err)
				return nil // not critical
			}
			params.AutologinSecret = secret
			return nil
		}},
		{"finalize", 100, func() error { return m.reload() }},
	}

	for _, step := range steps {
		m.onProgress(step.name, step.pct, fmt.Sprintf("Creating site: %s", step.name))
		if err := step.fn(); err != nil {
			return fmt.Errorf("step %s failed: %w", step.name, err)
		}
	}
	return nil
}

func (m *Manager) DeleteSite(domain, dbName, dbUser string) error {
	siteRoot := fmt.Sprintf("/var/www/%s", domain)

	// Remove nginx config
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))

	// Remove PHP-FPM pool for all versions
	phpVersions := []string{"7.4", "8.0", "8.1", "8.2", "8.3"}
	for _, v := range phpVersions {
		poolFile := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", v, domain)
		os.Remove(poolFile)
	}

	// Revoke & delete SSL certificate (certbot)
	exec.Command("certbot", "delete", "--cert-name", domain, "--non-interactive").Run()

	// Drop database and user
	runSQL(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
	runSQL(fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser))
	runSQL("FLUSH PRIVILEGES")

	// Remove all site files
	os.RemoveAll(siteRoot)

	// Remove the site's Linux user + group
	deleteSiteUser(domain)

	// Reload services
	exec.Command("nginx", "-t").Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("Site deleted", "domain", domain)
	return nil
}

// CreateSiteManual sets up server infrastructure (dirs, DB, Nginx, PHP-FPM) WITHOUT installing WordPress.
// Used for manual/custom deployments where the user will upload their own source code.
func (m *Manager) CreateSiteManual(params CreateParams) error {
	siteRoot := fmt.Sprintf("/var/www/%s", params.Domain)
	webRoot := siteRoot + "/public"
	phpVer := params.PHPVersion
	if phpVer == "" {
		phpVer = "8.2"
	}

	steps := []struct {
		name string
		pct  int
		fn   func() error
	}{
		{"create_linux_user", 5, func() error { return createSiteUser(params.Domain) }},
		{"create_directories", 20, func() error { return m.createDirs(siteRoot, webRoot) }},
		{"create_database", 40, func() error { return m.createDatabase(params) }},
		{"configure_nginx", 60, func() error { return m.configureNginx(params, webRoot, phpVer) }},
		{"configure_php_fpm", 75, func() error { return m.configurePHPPool(params, phpVer) }},
		{"set_permissions", 90, func() error { return m.setPermissions(siteRoot, params.Domain) }},
		{"finalize", 100, func() error { return m.reload() }},
	}

	for _, step := range steps {
		m.onProgress(step.name, step.pct, fmt.Sprintf("Setting up site infrastructure: %s", step.name))
		if err := step.fn(); err != nil {
			return fmt.Errorf("step %s failed: %w", step.name, err)
		}
	}
	return nil
}

// ExtractSourceZip extracts a zip archive to the site's webroot.
// Handles the common case where the zip has a single top-level folder (moves its contents up).
// If DB credentials are provided, wp-config.php is patched with the correct values so the
// site connects to the panel-managed database immediately after deploy.
func (m *Manager) ExtractSourceZip(domain, zipPath, dbName, dbUser, dbPass, dbHost string) error {
	webRoot := fmt.Sprintf("/var/www/%s/public", domain)
	tmpExtract := fmt.Sprintf("/tmp/wp-source-extract-%s", domain)

	// Clean up any previous temp dir
	os.RemoveAll(tmpExtract)
	os.MkdirAll(tmpExtract, 0755)
	defer os.RemoveAll(tmpExtract)
	defer os.Remove(zipPath)

	// Extract zip using Go's native archive/zip (no unzip binary needed)
	if err := extractZipNative(zipPath, tmpExtract); err != nil {
		return fmt.Errorf("failed to extract zip: %w", err)
	}

	// Detect if there's a single top-level directory
	entries, err := os.ReadDir(tmpExtract)
	if err != nil {
		return fmt.Errorf("failed to read extract dir: %w", err)
	}

	srcDir := tmpExtract
	if len(entries) == 1 && entries[0].IsDir() {
		// Single folder in zip — use its contents directly
		srcDir = fmt.Sprintf("%s/%s", tmpExtract, entries[0].Name())
	}

	// Move contents to webroot (preserve existing files not in the zip)
	if out, err := exec.Command("rsync", "-a", "--delete", srcDir+"/", webRoot+"/").CombinedOutput(); err != nil {
		// Fallback: cp -r
		if out2, err2 := exec.Command("sh", "-c", fmt.Sprintf("cp -r %s/. %s/", srcDir, webRoot)).CombinedOutput(); err2 != nil {
			return fmt.Errorf("failed to copy source: %s / %s", string(out), string(out2))
		}
	}

	// Patch wp-config.php with correct DB credentials for this server.
	// Without this, the site would try to connect to the old server's DB.
	if dbName != "" {
		wpConfigPath := webRoot + "/wp-config.php"
		if err := patchWPConfig(wpConfigPath, dbName, dbUser, dbPass, dbHost); err != nil {
			slog.Warn("Could not patch wp-config.php", "domain", domain, "error", err)
		} else {
			slog.Info("wp-config.php patched with new DB credentials", "domain", domain, "db_name", dbName)
		}
	}

	// Fix permissions — use the site's dedicated Linux user
	siteUser := SiteUser(domain)
	owner := fmt.Sprintf("%s:%s", siteUser, siteUser)
	exec.Command("chown", "-R", owner, webRoot).Run()
	exec.Command("find", webRoot, "-type", "d", "-exec", "chmod", "2755", "{}", "+").Run()
	exec.Command("find", webRoot, "-type", "f", "-exec", "chmod", "644", "{}", "+").Run()
	exec.Command("chmod", "640", webRoot+"/wp-config.php").Run()

	slog.Info("Source zip extracted", "domain", domain, "webroot", webRoot)
	return nil
}

// patchWPConfig rewrites DB_NAME, DB_USER, DB_PASSWORD, DB_HOST values inside wp-config.php.
// Handles both define('KEY', 'value') and define("KEY", "value") forms.
func patchWPConfig(configPath, dbName, dbUser, dbPass, dbHost string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return err // wp-config.php doesn't exist — not an error for non-WP sites
	}
	if dbHost == "" {
		dbHost = "localhost"
	}
	content := string(raw)
	content = replaceWPDefine(content, "DB_NAME", dbName)
	content = replaceWPDefine(content, "DB_USER", dbUser)
	content = replaceWPDefine(content, "DB_PASSWORD", dbPass)
	content = replaceWPDefine(content, "DB_HOST", dbHost)
	return os.WriteFile(configPath, []byte(content), 0640)
}

// replaceWPDefine replaces  define( 'KEY', 'old' );  →  define( 'KEY', 'new' );
// Handles single/double quotes and optional spaces around the parentheses.
func replaceWPDefine(content, key, value string) string {
	// Escape single quotes in value to avoid breaking the PHP string
	safeVal := strings.ReplaceAll(value, "'", "\\'")
	re := regexp.MustCompile(`(?i)define\s*\(\s*['"]` + regexp.QuoteMeta(key) + `['"]\s*,\s*['"][^'"]*['"]\s*\)`)
	return re.ReplaceAllString(content, fmt.Sprintf("define( '%s', '%s' )", key, safeVal))
}

// extractZipNative extracts a zip file to destDir using Go's archive/zip (no external tools needed).
func extractZipNative(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Sanitize path to prevent zip-slip
		fpath := filepath.Join(destDir, filepath.Clean("/"+f.Name))
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue // skip unsafe paths
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.Create(fpath)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// ImportDatabase imports a SQL file into the site's database.
// After import, wp-cli is used to set the correct siteurl/home for `domain`.
// If oldDomain is non-empty, a full wp search-replace is run first to rewrite all URLs.
func (m *Manager) ImportDatabase(dbName, sqlPath, domain, oldDomain string) error {
	defer os.Remove(sqlPath)

	// Support .sql, .gz, .sql.gz, and .zip
	if strings.HasSuffix(sqlPath, ".gz") {
		if out, err := exec.Command("gunzip", "-f", sqlPath).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to decompress SQL: %s", string(out))
		}
		sqlPath = strings.TrimSuffix(sqlPath, ".gz")
		defer os.Remove(sqlPath)
	} else if strings.HasSuffix(sqlPath, ".zip") {
		// Extract zip using Go's built-in archive/zip (no external unzip binary needed)
		extracted, err := extractSQLFromZip(sqlPath)
		if err != nil {
			return fmt.Errorf("failed to extract zip: %s", err.Error())
		}
		defer os.Remove(extracted)
		sqlPath = extracted
	}

	// Validate SQL file is not empty before importing
	if info, err := os.Stat(sqlPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("SQL file is empty (0 bytes) — the zip may not contain a valid database dump")
	}

	cmd := exec.Command("sh", "-c", fmt.Sprintf("mariadb '%s' < '%s'", dbName, sqlPath))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("import failed: %s", string(out))
	}
	slog.Info("Database imported", "db_name", dbName, "file", sqlPath, "size_bytes", func() int64 {
		if info, err := os.Stat(sqlPath); err == nil {
			return info.Size()
		}
		return -1
	}())

	// ── Post-import: fix WordPress site URL via WP-CLI ────────────────────────
	// This ensures the site works on the current server/domain regardless of
	// what siteurl/home values were stored in the exported database.
	if domain == "" {
		return nil
	}
	webRoot := fmt.Sprintf("/var/www/%s/public", domain)

	// Detect whether SSL cert exists for this domain → use https or http
	scheme := "http"
	if _, err := os.Stat(fmt.Sprintf("/etc/letsencrypt/live/%s", domain)); err == nil {
		scheme = "https"
	}
	siteURL := fmt.Sprintf("%s://%s", scheme, domain)

	// If old domain is provided, rewrite all occurrences (posts, meta, options, etc.)
	if oldDomain != "" && oldDomain != domain {
		slog.Info("Running wp search-replace for domain change", "old", oldDomain, "new", domain)
		// Replace plain domain references
		exec.Command("bash", "-c", fmt.Sprintf(
			`cd %s && wp search-replace "%s" "%s" --all-tables --allow-root 2>/dev/null`,
			webRoot, oldDomain, domain)).Run()
		// Normalise http→https if SSL is enabled
		if scheme == "https" {
			exec.Command("bash", "-c", fmt.Sprintf(
				`cd %s && wp search-replace "http://%s" "https://%s" --all-tables --allow-root 2>/dev/null`,
				webRoot, domain, domain)).Run()
		}
	}

	// Always force siteurl and home to the correct value (handles cases where
	// old_domain was not supplied but the DB had a different URL).
	out2, err2 := exec.Command("bash", "-c", fmt.Sprintf(
		`cd %s && wp option update siteurl "%s" --allow-root 2>&1 && wp option update home "%s" --allow-root 2>&1`,
		webRoot, siteURL, siteURL)).CombinedOutput()
	if err2 != nil {
		slog.Warn("wp option update failed (wp-cli not available or not a WP site)", "output", string(out2))
	} else {
		slog.Info("WordPress site URL updated", "domain", domain, "url", siteURL)
	}

	return nil
}

// extractSQLFromZip opens a zip archive using Go's built-in archive/zip,
// finds the first .sql file (any depth), extracts it to a temp file, and
// returns the temp file path. Caller is responsible for os.Remove(path).
func extractSQLFromZip(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("cannot open zip: %w", err)
	}
	defer r.Close()

	// Find first .sql file (sorted by name for determinism)
	var sqlFile *zip.File
	for _, f := range r.File {
		if !f.FileInfo().IsDir() && strings.HasSuffix(strings.ToLower(f.Name), ".sql") {
			if sqlFile == nil || f.Name < sqlFile.Name {
				sqlFile = f
			}
		}
	}
	if sqlFile == nil {
		// List files in zip for a helpful error message
		var names []string
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				names = append(names, f.Name)
			}
		}
		return "", fmt.Errorf("no .sql file found in zip (files: %s) — make sure the zip contains a SQL database dump", strings.Join(names, ", "))
	}

	// Extract to a temp file
	tmpFile, err := os.CreateTemp("", "wp-import-*.sql")
	if err != nil {
		return "", fmt.Errorf("cannot create temp file: %w", err)
	}
	defer tmpFile.Close()

	rc, err := sqlFile.Open()
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("cannot read %s from zip: %w", sqlFile.Name, err)
	}
	defer rc.Close()

	if _, err := io.Copy(tmpFile, rc); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("cannot extract %s: %w", sqlFile.Name, err)
	}

	slog.Info("Extracted SQL from zip", "zip", zipPath, "sql_file", sqlFile.Name, "dest", tmpFile.Name())
	return tmpFile.Name(), nil
}

func (m *Manager) createDirs(siteRoot, webRoot string) error {
	dirs := []string{webRoot, siteRoot + "/logs", siteRoot + "/backups", siteRoot + "/tmp"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) createDatabase(p CreateParams) error {
	cmds := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", p.DBName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", p.DBUser, p.DBPassword),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", p.DBName, p.DBUser),
		"FLUSH PRIVILEGES",
	}
	for _, sql := range cmds {
		if err := runSQL(sql); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) configureNginx(p CreateParams, webRoot, phpVer string) error {
	tmpl := `server {
    listen 80;
    server_name {{.Domain}};
    root {{.WebRoot}};
    index index.php index.html;

    access_log /var/www/{{.Domain}}/logs/access.log;
    error_log /var/www/{{.Domain}}/logs/error.log;

    client_max_body_size 64M;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        fastcgi_pass unix:/var/run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_read_timeout 300;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }

    location ~ /\.ht {
        deny all;
    }

    location = /xmlrpc.php {
        deny all;
    }

    location ~* /wp-config\.php {
        deny all;
    }
}
`
	t, err := template.New("nginx").Parse(tmpl)
	if err != nil {
		return err
	}

	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", p.Domain)
	f, err := os.Create(confPath)
	if err != nil {
		return err
	}
	defer f.Close()

	data := map[string]string{
		"Domain":     p.Domain,
		"WebRoot":    webRoot,
		"PHPVersion": phpVer,
	}
	if err := t.Execute(f, data); err != nil {
		return err
	}

	// Enable site
	enabledPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", p.Domain)
	os.Symlink(confPath, enabledPath)

	// Test config
	return exec.Command("nginx", "-t").Run()
}

func (m *Manager) configurePHPPool(p CreateParams, phpVer string) error {
	siteUser := SiteUser(p.Domain)
	poolConf := fmt.Sprintf(`[%s]
user = %s
group = %s
listen = /var/run/php/php%s-fpm-%s.sock
listen.owner = www-data
listen.group = www-data
pm = dynamic
pm.max_children = 5
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
pm.max_requests = 500
php_admin_value[memory_limit] = 256M
php_admin_value[upload_max_filesize] = 64M
php_admin_value[post_max_size] = 64M
php_admin_value[max_execution_time] = 300
php_admin_value[max_input_vars] = 3000
php_admin_value[open_basedir] = /var/www/%s/:/tmp/:/usr/share/php/
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,parse_ini_file,show_source
`, p.Domain, siteUser, siteUser, phpVer, p.Domain, p.Domain)

	poolPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVer, p.Domain)
	if err := os.WriteFile(poolPath, []byte(poolConf), 0644); err != nil {
		return err
	}

	return exec.Command("systemctl", "reload", fmt.Sprintf("php%s-fpm", phpVer)).Run()
}

func (m *Manager) downloadWordPress(webRoot string) error {
	return exec.Command("wp", "core", "download", "--path="+webRoot, "--locale=vi", "--allow-root").Run()
}

func (m *Manager) configureWordPress(p CreateParams, webRoot string) error {
	args := []string{
		"config", "create",
		"--path=" + webRoot,
		"--dbname=" + p.DBName,
		"--dbuser=" + p.DBUser,
		"--dbpass=" + p.DBPassword,
		"--dbhost=localhost",
		"--dbcharset=utf8mb4",
		"--allow-root",
	}
	return exec.Command("wp", args...).Run()
}

func (m *Manager) installWordPress(p CreateParams, webRoot string) error {
	url := "http://" + p.Domain
	if p.SSLEnabled {
		url = "https://" + p.Domain
	}
	args := []string{
		"core", "install",
		"--path=" + webRoot,
		"--url=" + url,
		"--title=" + p.Domain,
		"--admin_user=" + p.WPAdminUser,
		"--admin_email=" + p.WPAdminEmail,
		"--admin_password=" + p.WPAdminPass,
		"--skip-email",
		"--allow-root",
	}
	return exec.Command("wp", args...).Run()
}

func (m *Manager) setPermissions(siteRoot, domain string) error {
	siteUser := SiteUser(domain)
	owner := fmt.Sprintf("%s:%s", siteUser, siteUser)
	exec.Command("chown", "-R", owner, siteRoot).Run()
	exec.Command("find", siteRoot, "-type", "d", "-exec", "chmod", "2755", "{}", ";").Run()
	exec.Command("find", siteRoot, "-type", "f", "-exec", "chmod", "644", "{}", ";").Run()
	// wp-config.php must not be world-readable (contains DB credentials)
	exec.Command("chmod", "640", siteRoot+"/public/wp-config.php").Run()
	return nil
}

func (m *Manager) issueSSL(domain, email string) error {
	return exec.Command("certbot", "--nginx", "-d", domain, "--non-interactive",
		"--agree-tos", "-m", email, "--redirect").Run()
}

func (m *Manager) hardenWordPress(webRoot string) error {
	// Disable file editing in WP admin
	wpConfig := webRoot + "/wp-config.php"
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		return nil // not critical
	}
	content := string(data)
	if !strings.Contains(content, "DISALLOW_FILE_EDIT") {
		content = strings.Replace(content, "/* That's all",
			"define('DISALLOW_FILE_EDIT', true);\ndefine('WP_AUTO_UPDATE_CORE', 'minor');\n/* That's all", 1)
		os.WriteFile(wpConfig, []byte(content), 0644)
	}
	return nil
}

// generateSecret creates a 32-byte hex secret for HMAC autologin.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// installAutologin installs the WPM autologin mu-plugin and adds a secret to wp-config.php.
// Returns the generated secret.
func (m *Manager) installAutologin(webRoot, domain string) (string, error) {
	secret, err := generateSecret()
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}

	// 1) Add/sync WPM_AUTOLOGIN_SECRET in wp-config.php
	wpConfig := webRoot + "/wp-config.php"
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		return "", fmt.Errorf("read wp-config: %w", err)
	}
	content := string(data)
	if strings.Contains(content, "WPM_AUTOLOGIN_SECRET") {
		// Secret already in wp-config.php — read it back so DB stays in sync.
		// (Re-generating a new secret here and storing in DB would cause a mismatch
		// because wp-config.php still has the old value.)
		re := regexp.MustCompile(`define\('WPM_AUTOLOGIN_SECRET',\s*'([^']+)'\)`)
		if m := re.FindStringSubmatch(content); len(m) > 1 {
			secret = m[1]
		}
		// else: define exists but can't parse — fall through with newly generated secret
		// and overwrite below
	} else {
		content = strings.Replace(content, "/* That's all",
			fmt.Sprintf("define('WPM_AUTOLOGIN_SECRET', '%s');\n/* That's all", secret), 1)
		if err := os.WriteFile(wpConfig, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write wp-config: %w", err)
		}
	}

	// 2) Install mu-plugin
	muDir := webRoot + "/wp-content/mu-plugins"
	os.MkdirAll(muDir, 0755)

	muPlugin := `<?php
/**
 * WP Manage - Auto Login
 * Allows panel administrators to auto-login via HMAC-signed URL.
 */
if (!defined('ABSPATH')) exit;

add_action('init', function() {
    if (!isset($_GET['wpm_autologin']) || !isset($_GET['wpm_ts']) || !isset($_GET['wpm_sig'])) return;

    $secret = defined('WPM_AUTOLOGIN_SECRET') ? WPM_AUTOLOGIN_SECRET : '';
    if (empty($secret)) return;

    $ts  = intval($_GET['wpm_ts']);
    $sig = $_GET['wpm_sig'];

    // Token expires after 60 seconds
    if (abs(time() - $ts) > 60) {
        wp_die('Auto-login link has expired. Please try again from the panel.', 'Link Expired', array('response' => 403));
    }

    // Verify HMAC signature
    $expected = hash_hmac('sha256', strval($ts), $secret);
    if (!hash_equals($expected, $sig)) {
        wp_die('Invalid auto-login signature.', 'Unauthorized', array('response' => 403));
    }

    // Find admin user (ID 1 or first administrator)
    $user = get_user_by('ID', 1);
    if (!$user || !user_can($user, 'administrator')) {
        $admins = get_users(array('role' => 'administrator', 'number' => 1, 'orderby' => 'ID'));
        $user = !empty($admins) ? $admins[0] : null;
    }
    if (!$user) {
        wp_die('Admin user not found.', 'Error', array('response' => 500));
    }

    // Auto-login
    wp_set_current_user($user->ID);
    wp_set_auth_cookie($user->ID, true);
    do_action('wp_login', $user->user_login, $user);

    wp_safe_redirect(admin_url());
    exit;
}, 1);
`
	muPath := muDir + "/wpm-autologin.php"
	if err := os.WriteFile(muPath, []byte(muPlugin), 0644); err != nil {
		return "", fmt.Errorf("write mu-plugin: %w", err)
	}

	// Fix ownership — mu-plugin must be owned by the site user so PHP-FPM can read it
	siteUser := SiteUser(domain)
	owner := fmt.Sprintf("%s:%s", siteUser, siteUser)
	exec.Command("chown", owner, muPath).Run()
	exec.Command("chown", owner, muDir).Run()

	slog.Info("Autologin mu-plugin installed", "domain", domain)
	return secret, nil
}

// InstallAutologinOnExisting installs autologin on an already existing site.
// Called via a task from the panel.
func InstallAutologinOnExisting(domain string) (string, error) {
	webRoot := fmt.Sprintf("/var/www/%s/public", domain)
	mgr := NewManager(func(s string, p int, m string) {})
	return mgr.installAutologin(webRoot, domain)
}

func (m *Manager) reload() error {
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

func runSQL(query string) error {
	return exec.Command("mariadb", "-e", query).Run()
}
