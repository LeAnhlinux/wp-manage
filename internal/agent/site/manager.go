package site

import (
	"archive/zip"
	"compress/gzip"
	"context"
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
	"time"

	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// SiteUser returns the Linux username for a WordPress site.
// Convention: "wp_" + domain with dots and hyphens replaced by underscores.
// e.g. "example.com" → "wp_example_com", "my-site.co.uk" → "wp_my_site_co_uk"
func SiteUser(domain string) string {
	return "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
}

// SiteRoot returns the site's root directory path.
// Single source of truth — always use this instead of SiteRoot(domain).
func SiteRoot(domain string) string {
	return "/var/www/" + domain
}

// SiteWebRoot returns the web-accessible directory (served by Nginx).
// Single source of truth — always use this instead of siteRoot + "/public".
func SiteWebRoot(domain string) string {
	return SiteRoot(domain) + "/public"
}

// createSiteUser creates a system user for the site (no login shell, no home dir creation).
func createSiteUser(domain string) error {
	if err := utils.ValidateDomain(domain); err != nil {
		return fmt.Errorf("createSiteUser: %w", err)
	}
	user := SiteUser(domain)
	cmd := exec.Command("useradd",
		"--system",
		"--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--home-dir", SiteRoot(domain),
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
	if err := utils.ValidateDomain(domain); err != nil {
		slog.Error("deleteSiteUser: invalid domain, aborting", "domain", domain, "error", err)
		return
	}
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

// GetWPVersion reads the WordPress version directly from wp-includes/version.php.
// This avoids WP-CLI/PHP entirely — works with any PHP version, ionCube themes, etc.
func GetWPVersion(domain string) string {
	versionFile := SiteWebRoot(domain) + "/wp-includes/version.php"
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$wp_version") && strings.Contains(line, "=") {
			if s := strings.Index(line, "'"); s >= 0 {
				if e := strings.LastIndex(line, "'"); e > s {
					return line[s+1 : e]
				}
			}
			if s := strings.Index(line, `"`); s >= 0 {
				if e := strings.LastIndex(line, `"`); e > s {
					return line[s+1 : e]
				}
			}
		}
	}
	return ""
}

// cleanupFailedSite removes all resources created during a failed site creation.
// Called automatically when any CreateSite step fails.
func (m *Manager) cleanupFailedSite(domain, dbName, dbUser string) {
	slog.Info("Rolling back failed site creation", "domain", domain)
	siteRoot := SiteRoot(domain)

	// Remove nginx configs
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))

	// Remove PHP-FPM pool for all versions
	for _, v := range []string{"7.4", "8.0", "8.1", "8.2", "8.3"} {
		os.Remove(fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", v, domain))
	}

	// Kill connections + drop DB/user
	if dbName != "" {
		killDBConnections(dbName)
		runSQL(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
	}
	if dbUser != "" {
		runSQL(fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser))
		runSQL("FLUSH PRIVILEGES")
	}

	// Remove site files
	os.RemoveAll(siteRoot)

	// Remove Linux user
	deleteSiteUser(domain)

	// Reload nginx (ignore errors)
	exec.Command("nginx", "-t").Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("Rollback complete", "domain", domain)
}

func (m *Manager) CreateSite(params CreateParams) error {
	// Validate domain before any filesystem or system-user operations (H9/H10/H17)
	if err := utils.ValidateDomain(params.Domain); err != nil {
		return fmt.Errorf("CreateSite: %w", err)
	}

	siteRoot := SiteRoot(params.Domain)
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
				if err := m.issueSSL(params.Domain, params.WPAdminEmail); err != nil {
					// SSL failure is non-fatal in auto mode — site works over HTTP.
					// DNS may not be pointing here yet. User can issue SSL later from panel.
					slog.Warn("SSL issue failed during auto site create (non-fatal)", "domain", params.Domain, "error", err)
				}
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
			slog.Error("Create site step failed, rolling back", "domain", params.Domain, "step", step.name, "error", err)
			m.cleanupFailedSite(params.Domain, params.DBName, params.DBUser)
			return fmt.Errorf("step %s failed: %w", step.name, err)
		}
	}
	return nil
}

func (m *Manager) DeleteSite(domain, dbName, dbUser string) error {
	// Validate domain before any filesystem operations (H9/H17)
	if err := utils.ValidateDomain(domain); err != nil {
		return fmt.Errorf("DeleteSite: %w", err)
	}

	siteRoot := SiteRoot(domain)

	// Remove nginx config
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))

	// Remove PHP-FPM pool for all versions and reload each affected FPM service
	// so that worker processes exit before we try to remove the Linux user.
	phpVersions := []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}
	for _, v := range phpVersions {
		poolFile := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", v, domain)
		if err := os.Remove(poolFile); err == nil {
			// Pool file existed — reload this FPM version so workers stop now.
			svc := fmt.Sprintf("php%s-fpm", v)
			if out, err := exec.Command("systemctl", "reload", svc).CombinedOutput(); err != nil {
				// reload failed — try restart as fallback
				exec.Command("systemctl", "restart", svc).Run()
				slog.Warn("PHP-FPM reload failed, restarted", "version", v, "output", string(out))
			}
		}
	}

	// Revoke & delete SSL certificate (certbot)
	exec.Command("certbot", "delete", "--cert-name", domain, "--non-interactive").Run()

	// Kill any active connections to the DB before dropping (prevents metadata lock hang)
	killDBConnections(dbName)

	// Drop database and user
	runSQL(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
	runSQL(fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", dbUser))
	runSQL("FLUSH PRIVILEGES")

	// Remove all site files
	os.RemoveAll(siteRoot)

	// Remove the site's Linux user + group
	deleteSiteUser(domain)

	// Reload nginx (PHP-FPM already reloaded above per version)
	exec.Command("nginx", "-t").Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("Site deleted", "domain", domain)
	return nil
}

// CreateSiteManual sets up server infrastructure (dirs, DB, Nginx, PHP-FPM) WITHOUT installing WordPress.
// Used for manual/custom deployments where the user will upload their own source code.
func (m *Manager) CreateSiteManual(params CreateParams) error {
	siteRoot := SiteRoot(params.Domain)
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
		{"finalize", 95, func() error { return m.reload() }},
		{"issue_ssl", 100, func() error {
			if !params.SSLEnabled {
				return nil
			}
			if err := m.issueSSL(params.Domain, params.WPAdminEmail); err != nil {
				// SSL failure is non-fatal — site still works over HTTP.
				// Log and continue; user can issue SSL later via the panel.
				slog.Warn("SSL issue failed during manual site create (non-fatal)", "domain", params.Domain, "error", err)
			}
			return nil
		}},
	}

	for _, step := range steps {
		m.onProgress(step.name, step.pct, fmt.Sprintf("Setting up site infrastructure: %s", step.name))
		if err := step.fn(); err != nil {
			slog.Error("Create site (manual) step failed, rolling back", "domain", params.Domain, "step", step.name, "error", err)
			m.cleanupFailedSite(params.Domain, params.DBName, params.DBUser)
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
	webRoot := SiteWebRoot(domain)
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
	// Use ReplaceAllLiteralString (NOT ReplaceAllString) so that $ in the replacement
	// value is treated literally, not as a capture-group reference ($1, $j, etc.).
	// Example: password "abc$j" would have "$j" silently dropped by ReplaceAllString.
	replacement := fmt.Sprintf("define( '%s', '%s' )", key, safeVal)
	return re.ReplaceAllLiteralString(content, replacement)
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
// phpVersion (e.g. "8.1") selects the correct PHP binary for WP-CLI — critical for sites
// using ionCube-encoded themes/plugins where the default system PHP may lack the loader.
func (m *Manager) ImportDatabase(dbName, sqlPath, domain, oldDomain, phpVersion string) error {
	defer os.Remove(sqlPath)

	// Support .sql, .gz, .sql.gz, and .zip — all decompressed using Go builtins (no external binaries)
	if strings.HasSuffix(sqlPath, ".gz") {
		decompressed := strings.TrimSuffix(sqlPath, ".gz")
		if err := decompressGzip(sqlPath, decompressed); err != nil {
			return fmt.Errorf("failed to decompress .gz: %w", err)
		}
		defer os.Remove(decompressed)
		sqlPath = decompressed
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

	// Kill existing connections before importing to prevent metadata lock issues
	killDBConnections(dbName)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("mariadb --binary-mode=1 '%s' < '%s'", dbName, sqlPath))
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
	webRoot := SiteWebRoot(domain)

	// Detect whether SSL cert exists for this domain → use https or http
	scheme := "http"
	if _, err := os.Stat(fmt.Sprintf("/etc/letsencrypt/live/%s", domain)); err == nil {
		scheme = "https"
	}
	siteURL := fmt.Sprintf("%s://%s", scheme, domain)

	// Build WP-CLI invocation using the correct PHP binary for this site.
	// This is critical for sites using ionCube-encoded themes/plugins: the
	// system default PHP (e.g. 8.3) may not have the ionCube loader for the
	// version the theme was encoded with (e.g. 8.1).
	phpBin := phpBinaryForVersion(phpVersion)
	wpBin := wpcliBinary()
	// wp <cmd> --path=<webRoot> --allow-root
	wpRun := func(args ...string) *exec.Cmd {
		cmdArgs := append([]string{phpBin, wpBin, "--path=" + webRoot, "--allow-root"}, args...)
		return exec.Command(cmdArgs[0], cmdArgs[1:]...)
	}

	// ── Step 1: search-replace old domain across all tables ─────────────────────
	// Use --skip-themes --skip-plugins to avoid fatal errors from incompatible
	// themes/plugins (e.g. create_function() removed in PHP 8.0, ionCube files).
	if oldDomain != "" && oldDomain != domain {
		slog.Info("Running wp search-replace for domain change", "old", oldDomain, "new", domain, "php", phpBin)
		wpRun("search-replace", oldDomain, domain, "--all-tables", "--skip-themes", "--skip-plugins").Run()
		if scheme == "https" {
			wpRun("search-replace", "http://"+domain, "https://"+domain, "--all-tables", "--skip-themes", "--skip-plugins").Run()
		}
	}

	// ── Step 2: Force siteurl + home via direct SQL (100% reliable) ───────────
	// WP-CLI requires loading WordPress (theme + plugins) which can fatal-error
	// on PHP version mismatches. Direct SQL bypasses PHP entirely and always works.
	// Detect the options table dynamically — handles any table prefix (wp_, motphim_, etc.)
	tableOut, tableErr := exec.Command("mariadb", "-sN", dbName, "-e",
		"SELECT table_name FROM information_schema.columns WHERE table_schema = DATABASE() "+
			"AND column_name = 'option_name' AND table_name LIKE '%options' LIMIT 1;",
	).Output()
	if tableErr == nil && len(strings.TrimSpace(string(tableOut))) > 0 {
		optTable := strings.TrimSpace(string(tableOut))
		sqlUpdate := fmt.Sprintf(
			"UPDATE `%s` SET option_value = '%s' WHERE option_name IN ('siteurl', 'home');",
			optTable, siteURL,
		)
		if err := exec.Command("mariadb", dbName, "-e", sqlUpdate).Run(); err != nil {
			slog.Warn("SQL siteurl/home update failed", "table", optTable, "error", err)
		} else {
			slog.Info("WordPress siteurl/home updated via SQL", "domain", domain, "url", siteURL, "table", optTable)
		}
	} else {
		// Fallback: WP-CLI with skip flags
		slog.Warn("Could not detect options table via SQL, falling back to wp-cli", "db", dbName)
		wpRun("option", "update", "siteurl", siteURL, "--skip-themes", "--skip-plugins").Run()
		wpRun("option", "update", "home", siteURL, "--skip-themes", "--skip-plugins").Run()
	}

	return nil
}

// phpBinaryForVersion returns /usr/bin/phpX.Y if it exists, else falls back to "php".
func phpBinaryForVersion(phpVersion string) string {
	if phpVersion == "" {
		return "php"
	}
	bin := fmt.Sprintf("/usr/bin/php%s", phpVersion)
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	return "php"
}

// wpcliBinary returns the path to the wp-cli binary.
func wpcliBinary() string {
	for _, p := range []string{"/usr/local/bin/wp", "/usr/bin/wp"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "wp"
}

// extractSQLFromZip opens a zip archive using Go's built-in archive/zip,
// finds the first SQL-containing file (.sql, .sql.gz, or .gz), extracts and
// decompresses it if needed, and returns the temp .sql file path.
// Caller is responsible for os.Remove(path).
// decompressGzip decompresses src (.gz) into dst using Go's compress/gzip.
func decompressGzip(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open gz: %w", err)
	}
	defer in.Close()

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, gzr); err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	return nil
}

func extractSQLFromZip(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("cannot open zip: %w", err)
	}
	defer r.Close()

	// Priority: .sql > .sql.gz / -db.gz (UpdraftPlus) > any .gz
	var sqlFile *zip.File
	var sqlGzFile *zip.File
	var allNames []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip macOS metadata files (__MACOSX/ and ._* resource forks)
		base := filepath.Base(f.Name)
		if strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(base, "._") {
			continue
		}
		allNames = append(allNames, f.Name)
		lower := strings.ToLower(f.Name)
		if strings.HasSuffix(lower, ".sql") {
			if sqlFile == nil || f.UncompressedSize64 > sqlFile.UncompressedSize64 {
				// prefer the largest .sql file (most likely the real dump, not a stub)
				sqlFile = f
			}
		} else if strings.HasSuffix(lower, ".sql.gz") || strings.HasSuffix(lower, "-db.gz") || strings.HasSuffix(lower, "_db.gz") {
			if sqlGzFile == nil || f.UncompressedSize64 > sqlGzFile.UncompressedSize64 {
				sqlGzFile = f
			}
		}
	}

	chosen := sqlFile
	if chosen == nil && sqlGzFile != nil {
		chosen = sqlGzFile
	}
	if chosen == nil {
		return "", fmt.Errorf("no SQL dump found in zip (files: %s) — zip must contain a .sql or .sql.gz database dump", strings.Join(allNames, ", "))
	}

	// Step 1: extract entry bytes to a raw temp file (no streaming decompression)
	rawTmp, err := os.CreateTemp("", "wp-import-raw-*")
	if err != nil {
		return "", fmt.Errorf("cannot create temp file: %w", err)
	}
	rawPath := rawTmp.Name()

	rc, err := chosen.Open()
	if err != nil {
		rawTmp.Close()
		os.Remove(rawPath)
		return "", fmt.Errorf("cannot open zip entry %s: %w", chosen.Name, err)
	}
	_, copyErr := io.Copy(rawTmp, rc)
	rc.Close()
	rawTmp.Close()
	if copyErr != nil {
		os.Remove(rawPath)
		return "", fmt.Errorf("cannot extract %s: %w", chosen.Name, copyErr)
	}

	// Step 2: check first 2 bytes on disk for gzip magic \x1f\x8b
	needsDecompress := strings.HasSuffix(strings.ToLower(chosen.Name), ".gz")
	if !needsDecompress {
		if magic, merr := readFirstBytes(rawPath, 2); merr == nil && len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
			needsDecompress = true
			slog.Info("Detected gzip content despite .sql extension", "entry", chosen.Name)
		}
	}

	if needsDecompress {
		// Step 3a: decompress gzip to a proper .sql temp file
		os.Remove(rawPath) // remove raw gz file after decompression
		sqlTmp, err := os.CreateTemp("", "wp-import-*.sql")
		if err != nil {
			return "", fmt.Errorf("cannot create sql temp: %w", err)
		}
		sqlPath := sqlTmp.Name()
		sqlTmp.Close()
		if err := decompressGzip(rawPath, sqlPath); err != nil {
			os.Remove(sqlPath)
			return "", fmt.Errorf("cannot decompress %s: %w", chosen.Name, err)
		}
		slog.Info("Extracted+decompressed SQL from zip", "entry", chosen.Name, "dest", sqlPath)
		return sqlPath, nil
	}

	// Step 3b: plain .sql — rename raw temp to a .sql-named temp
	sqlTmp, err := os.CreateTemp("", "wp-import-*.sql")
	if err != nil {
		os.Remove(rawPath)
		return "", fmt.Errorf("cannot create sql temp: %w", err)
	}
	sqlPath := sqlTmp.Name()
	sqlTmp.Close()
	os.Remove(sqlPath) // remove empty placeholder so Rename can overwrite
	if err := os.Rename(rawPath, sqlPath); err != nil {
		// cross-device rename — fall back to copy+delete
		if err2 := copyFile(rawPath, sqlPath); err2 != nil {
			os.Remove(rawPath)
			return "", fmt.Errorf("cannot move extracted sql: %w", err2)
		}
		os.Remove(rawPath)
	}
	slog.Info("Extracted SQL from zip", "entry", chosen.Name, "dest", sqlPath)
	return sqlPath, nil
}

func readFirstBytes(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, _ := f.Read(buf)
	return buf[:read], nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
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

    # Serve WebP: rewrite URI when browser supports it and .webp file exists
    # Requires: map $http_accept $webp_accept in /etc/nginx/conf.d/webp.conf
    location ~* \.(jpe?g|png)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header Vary Accept;
        set $do_webp "";
        if ($webp_accept) { set $do_webp "A"; }
        if (-f $request_filename.webp) { set $do_webp "${do_webp}B"; }
        if ($do_webp = "AB") { rewrite ^(.+)$ $1.webp last; }
    }
    # Serve *.jpg.webp / *.png.webp with correct content-type
    location ~* \.(jpg|jpeg|png)\.webp$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header Vary Accept;
        default_type image/webp;
    }

    location ~* \.(js|css|gif|ico|svg|woff|woff2|ttf|eot)$ {
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

	// Ensure /etc/nginx/conf.d/webp.conf exists (defines $webp_accept variable)
	// Must be created before nginx -t or it will fail with "unknown variable"
	ensureWebPConfd()

	// Enable site
	enabledPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", p.Domain)
	os.Symlink(confPath, enabledPath)

	// Test config — roll back if broken so nginx stays healthy
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		os.Remove(enabledPath)
		os.Remove(confPath)
		return fmt.Errorf("nginx -t failed: %s", string(out))
	}
	return nil
}

// ensureWebPConfd creates /etc/nginx/conf.d/webp.conf with the http-level map directive.
// This must exist for the per-site WebP location blocks to work.
func ensureWebPConfd() {
	confPath := "/etc/nginx/conf.d/webp.conf"
	if _, err := os.Stat(confPath); err == nil {
		return // already exists
	}
	content := "# WebP auto-serving: detect browser support via Accept header\n" +
		"map $http_accept $webp_accept {\n" +
		"    default   0;\n" +
		"    \"~*webp\" 1;\n" +
		"}\n"
	os.WriteFile(confPath, []byte(content), 0644)
}

const webpLocationBlock = `    # Serve WebP: rewrite URI when browser supports it and .webp file exists
    # Requires: map $http_accept $webp_accept in /etc/nginx/conf.d/webp.conf
    location ~* \.(jpe?g|png)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header Vary Accept;
        set $do_webp "";
        if ($webp_accept) { set $do_webp "A"; }
        if (-f $request_filename.webp) { set $do_webp "${do_webp}B"; }
        if ($do_webp = "AB") { rewrite ^(.+)$ $1.webp last; }
    }
    # Serve *.jpg.webp / *.png.webp with correct content-type
    location ~* \.(jpg|jpeg|png)\.webp$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header Vary Accept;
        default_type image/webp;
    }
`

// EnsureWebPConfd creates /etc/nginx/conf.d/webp.conf if it doesn't exist.
// Exported wrapper so other packages (e.g. staging) can call it.
func EnsureWebPConfd() {
	ensureWebPConfd()
}

// EnsureWebPNginx patches an existing site's nginx config to serve WebP automatically.
// Also creates /etc/nginx/conf.d/webp.conf with the http-level map if missing.
// Safe to call multiple times (idempotent).
func EnsureWebPNginx(domain string) error {
	ensureWebPConfd()

	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("nginx config not found: %w", err)
	}
	content := string(data)

	// Already patched?
	if strings.Contains(content, "do_webp") {
		return nil
	}

	// Replace old static assets block with new WebP-aware blocks
	oldBlock := "    location ~* \\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {\n        expires 30d;\n        add_header Cache-Control \"public, immutable\";\n    }"
	newBlock := webpLocationBlock + "\n    location ~* \\.(js|css|gif|ico|svg|woff|woff2|ttf|eot)$ {\n        expires 30d;\n        add_header Cache-Control \"public, immutable\";\n    }"

	if strings.Contains(content, oldBlock) {
		content = strings.Replace(content, oldBlock, newBlock, 1)
	} else if !strings.Contains(content, "do_webp") {
		// Fallback: inject before the closing brace of the FIRST server block.
		// Using LastIndex would target the last server block (HTTP redirect),
		// which is wrong for configs managed by Certbot that have two server blocks.
		idx := firstServerBlockEnd(content)
		if idx < 0 {
			return fmt.Errorf("cannot locate end of server block")
		}
		content = content[:idx] + "\n" + webpLocationBlock + content[idx:]
	}

	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		os.WriteFile(confPath, data, 0644) // revert
		return fmt.Errorf("nginx -t failed after webp patch: %s", out)
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

// firstServerBlockEnd returns the index of "\n}" that closes the FIRST top-level
// server block in an nginx config. Configs managed by Certbot often have two
// server blocks: the primary HTTPS block followed by an HTTP→HTTPS redirect block.
// Using strings.LastIndex would wrongly target the redirect block.
func firstServerBlockEnd(content string) int {
	depth := 0
	inFirstServer := false
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
			if depth == 1 {
				inFirstServer = true
			}
		case '}':
			depth--
			if inFirstServer && depth == 0 {
				// Find the \n before this }
				if i > 0 && content[i-1] == '\n' {
					return i - 1
				}
				return i
			}
		}
	}
	return -1
}

// RevertWebPNginx removes WebP location blocks from a site's nginx config.
func RevertWebPNginx(domain string) error {
	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("nginx config not found: %w", err)
	}
	content := string(data)

	if !strings.Contains(content, "do_webp") {
		return nil // already clean
	}

	// Remove WebP location blocks and the js/css block, replace with original combined block
	// Find start of WebP comment block
	webpMarker := "    # Serve WebP: rewrite URI"
	jsMarker := "    location ~* \\.(js|css|gif|ico|svg"
	closingJs := "    }"

	start := strings.Index(content, webpMarker)
	if start < 0 {
		return nil
	}
	// Find the end of the js/css block (after webp blocks)
	jsStart := strings.Index(content[start:], jsMarker)
	if jsStart < 0 {
		return nil
	}
	jsStart += start
	jsEnd := strings.Index(content[jsStart:], closingJs)
	if jsEnd < 0 {
		return nil
	}
	jsEnd += jsStart + len(closingJs)

	// Replace everything from webp marker to end of js block with combined original block
	originalBlock := "    location ~* \\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {\n        expires 30d;\n        add_header Cache-Control \"public, immutable\";\n    }"
	content = content[:start] + originalBlock + content[jsEnd:]

	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		os.WriteFile(confPath, data, 0644) // revert
		return fmt.Errorf("nginx -t failed after webp revert: %s", out)
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
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
request_terminate_timeout = 300s
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wp", "core", "download", "--path="+webRoot, "--locale=vi", "--allow-root")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out downloading WordPress (3 min limit)")
		}
		return fmt.Errorf("wp core download failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wp", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("wp config create failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
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
		"--skip-themes",
		"--skip-plugins",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wp", args...).CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out installing WordPress (2 min limit)")
		}
		return fmt.Errorf("wp core install failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
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
	webRoot := SiteWebRoot(domain)
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

// killDBConnections kills all active MariaDB connections to the given database.
// This prevents "Waiting for metadata lock" when dropping a busy database.
func killDBConnections(dbName string) {
	if dbName == "" {
		return
	}
	// Get all connection IDs using this DB
	out, err := exec.Command("mariadb", "-sN", "-e",
		fmt.Sprintf("SELECT id FROM information_schema.processlist WHERE db='%s' AND id != CONNECTION_ID();", dbName),
	).Output()
	if err != nil || len(out) == 0 {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		id := strings.TrimSpace(line)
		if id != "" {
			exec.Command("mariadb", "-e", fmt.Sprintf("KILL %s;", id)).Run()
		}
	}
	slog.Info("Killed DB connections before drop", "db", dbName)
}
