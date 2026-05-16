package providers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// AAPanelProvider implements PanelProvider for aaPanel (宝塔面板).
// Sites live in /www/wwwroot/. Panel SQLite DB: /www/server/panel/data/default.db.
type AAPanelProvider struct {
	detectedVersion string
}

func (p *AAPanelProvider) Name() string {
	if p.detectedVersion != "" {
		return "aaPanel " + p.detectedVersion
	}
	return "aaPanel"
}

// Detect checks for aaPanel presence via multiple indicators.
// Returns true if ANY indicator matches — do not rely on a single file
// because file paths change between aaPanel versions.
func (p *AAPanelProvider) Detect(ssh SSHClient) (bool, error) {
	// Each entry is a shell test; we run them in one compound command for speed.
	// Any "yes" output means aaPanel is installed.
	indicators := []string{
		"test -f /usr/bin/bt",                                 // bt CLI — present on all versions
		"test -f /www/server/panel/data/default.db",           // SQLite DB
		"test -d /www/server/panel/BTPanel",                   // web UI directory
		"test -f /www/server/panel/BT-Panel",                  // marker file (older versions)
		"test -f /www/server/panel/class/panelPlugin.py",      // class file (some versions)
		"test -d /www/server/panel/config",                    // config directory
	}

	for _, check := range indicators {
		if ssh.RunQuiet(check+" && echo yes || echo no") == "yes" {
			p.tryDetectVersion(ssh)
			return true, nil
		}
	}
	return false, nil
}

// tryDetectVersion populates p.detectedVersion from multiple possible sources.
func (p *AAPanelProvider) tryDetectVersion(ssh SSHClient) {
	// Try various version file locations across aaPanel versions
	sources := []string{
		// Newer versions store version in config JSON
		`cat /www/server/panel/config/config.json 2>/dev/null | grep -oP '"version"\s*:\s*"\K[^"]+'`,
		// Class file in older versions
		`cat /www/server/panel/class/common.py 2>/dev/null | grep -oP "(?<=version = ')[^']+"`,
		// Version file
		`cat /www/server/panel/data/version.pl 2>/dev/null | head -1`,
		// bt command itself reports version
		`bt 14 2>/dev/null | grep -oP '(?i)version[:\s]+\K[\d.]+'  | head -1`,
	}
	for _, cmd := range sources {
		ver := ssh.RunQuiet(cmd)
		if ver != "" {
			p.detectedVersion = ver
			return
		}
	}
}

// Discover finds all WordPress sites on an aaPanel server.
// Discovery is web-server agnostic: we scan for wp-config.php files under /www/wwwroot.
// PHP version is detected via 4-layer fallback (A→B→C→empty).
// Sites are processed in parallel (up to 8 concurrent goroutines) to minimise SSH round-trips.
func (p *AAPanelProvider) Discover(ssh SSHClient) ([]models.DiscoveredSiteInfo, error) {
	slog.Info("aaPanel Discover: scanning for wp-config.php files")

	// ── Step 1: Find all wp-config.php files (max depth 3, exclude wp-content) ──
	findOut, err := ssh.Run(
		`find /www/wwwroot -maxdepth 3 -name wp-config.php -not -path '*/wp-content/*' 2>/dev/null`,
	)
	if err != nil || findOut == "" {
		return nil, nil // no WP sites found
	}

	configPaths := strings.Split(strings.TrimSpace(findOut), "\n")
	var validPaths []string
	for _, p := range configPaths {
		if p := strings.TrimSpace(p); p != "" {
			validPaths = append(validPaths, p)
		}
	}
	if len(validPaths) == 0 {
		return nil, nil
	}

	// ── Step 2: Load aaPanel SQLite for domain+PHP data (once) ──
	panelSites, panelDomains := loadAAPanelSQLite(ssh)

	// ── Step 3: Detect web server once for all sites ──
	webServer := detectWebServer(ssh)

	// ── Step 4: Start batchDirSize concurrently — du can be slow on large dirs ──
	docRoots := make([]string, 0, len(validPaths))
	for _, cfgPath := range validPaths {
		docRoots = append(docRoots, cfgPath[:strings.LastIndex(cfgPath, "/")])
	}
	sizeReady := make(chan map[string]int64, 1)
	go func() { sizeReady <- batchDirSize(ssh, docRoots) }()

	// ── Step 5: Process each site in parallel (max 8 concurrent) ──
	type result struct {
		idx  int
		site *models.DiscoveredSiteInfo
		err  error
	}

	sem := make(chan struct{}, 8)
	resultCh := make(chan result, len(validPaths))
	var wg sync.WaitGroup

	for i, cfgPath := range validPaths {
		wg.Add(1)
		go func(idx int, cfgPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Pass nil sizeMap — sizes will be merged after du completes
			site, err := p.discoverOneSiteWithSize(ssh, cfgPath, panelSites, panelDomains, webServer, nil)
			resultCh <- result{idx: idx, site: site, err: err}
		}(i, cfgPath)
	}

	wg.Wait()
	close(resultCh)

	// Wait for du to finish and merge sizes
	sizeMap := <-sizeReady

	// Collect results preserving original order, then fill sizes from du
	ordered := make([]*models.DiscoveredSiteInfo, len(validPaths))
	for r := range resultCh {
		if r.err != nil {
			slog.Warn("aaPanel Discover: skip site", "config", validPaths[r.idx], "error", r.err)
			continue
		}
		ordered[r.idx] = r.site
	}

	var results []models.DiscoveredSiteInfo
	for _, s := range ordered {
		if s == nil {
			continue
		}
		if s.SizeBytes == 0 {
			s.SizeBytes = sizeMap[s.DocRoot]
		}
		results = append(results, *s)
	}

	slog.Info("aaPanel Discover: done", "found", len(results))
	return results, nil
}

// batchDirSize runs a single `du -sb dir1 dir2 ...` on the remote host and returns
// a map of dir → bytes. Falls back gracefully if du fails.
func batchDirSize(ssh SSHClient, dirs []string) map[string]int64 {
	sizes := make(map[string]int64, len(dirs))
	if len(dirs) == 0 {
		return sizes
	}
	quotedDirs := make([]string, len(dirs))
	for i, d := range dirs {
		quotedDirs[i] = shellQuote(d)
	}
	out := ssh.RunDirSize("du -sb " + strings.Join(quotedDirs, " ") + " 2>/dev/null")
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			continue
		}
		sizes[strings.TrimSpace(parts[1])] = n
	}
	return sizes
}

// discoverOneSite extracts all metadata for one wp-config.php (legacy, single-site path).
func (p *AAPanelProvider) discoverOneSite(
	ssh SSHClient,
	cfgPath string,
	panelSites map[string]aaPanelSite,
	panelDomains map[string][]string,
) (*models.DiscoveredSiteInfo, error) {
	return p.discoverOneSiteWithSize(ssh, cfgPath, panelSites, panelDomains, detectWebServer(ssh), nil)
}

// discoverOneSiteWithSize is the parallel-friendly variant that accepts pre-computed
// webServer type and sizeMap (from batchDirSize) to avoid redundant SSH calls.
func (p *AAPanelProvider) discoverOneSiteWithSize(
	ssh SSHClient,
	cfgPath string,
	panelSites map[string]aaPanelSite,
	panelDomains map[string][]string,
	webServer string,
	sizeMap map[string]int64,
) (*models.DiscoveredSiteInfo, error) {

	// ── Parse wp-config.php ──
	cfgContent, err := ssh.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read wp-config: %w", err)
	}

	dbName := extractPHPDefine(cfgContent, "DB_NAME")
	dbUser := extractPHPDefine(cfgContent, "DB_USER")
	dbPass := extractPHPDefine(cfgContent, "DB_PASSWORD")
	dbHost := extractPHPDefine(cfgContent, "DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	tablePrefix := extractPHPDefine(cfgContent, "table_prefix")

	// Require at least DB_NAME and table_prefix to be a valid WP install
	if dbName == "" || tablePrefix == "" {
		return nil, fmt.Errorf("not a valid wp-config.php (missing DB_NAME or table_prefix)")
	}

	// Doc root = directory of wp-config.php
	docRoot := cfgPath[:strings.LastIndex(cfgPath, "/")]

	// ── Get WP siteurl + home from DB ──
	wpSiteURL, wpHomeURL := queryWPOptions(ssh, dbHost, dbUser, dbPass, dbName, tablePrefix)

	// ── Domain merge (3 sources) ──
	siteName := extractSiteNameFromDocRoot(docRoot)
	domains, primary := mergeDomains(ssh, siteName, docRoot, wpSiteURL, panelSites, panelDomains)

	// ── PHP version detect (4-layer A→B→C→empty) ──
	phpVersion, phpMethod := detectPHPVersion(ssh, siteName, docRoot, primary, panelSites)

	// ── Disk size — use pre-computed batch value if available ──
	var size int64
	if sizeMap != nil {
		size = sizeMap[docRoot]
	}
	if size == 0 {
		size = ssh.DirSize(docRoot) // fallback for single-site path
	}

	// ── SSL info ──
	sslInfo := detectSSL(ssh, siteName, primary)

	site := &models.DiscoveredSiteInfo{
		SiteName:        siteName,
		DocRoot:         docRoot,
		PrimaryDomain:   primary,
		Aliases:         domains,
		WPSiteURL:       wpSiteURL,
		WPHomeURL:       wpHomeURL,
		DBName:          dbName,
		DBUser:          dbUser,
		DBPass:          dbPass,
		DBHost:          dbHost,
		SizeBytes:       size,
		PHPVersion:      phpVersion,
		PHPDetectMethod: phpMethod,
		WebServerType:   webServer,
		SSLInfo:         sslInfo,
	}
	return site, nil
}

// ── aaPanel SQLite helpers ──────────────────────────────────────────────────

type aaPanelSite struct {
	Name       string
	Path       string
	PHPVersion string
}

// loadAAPanelSQLite reads site + domain data from the aaPanel SQLite database.
// We copy the DB to /tmp first to avoid SQLITE_BUSY when aapanel holds a lock.
// Tries sqlite3 CLI first; falls back to python3 if sqlite3 is not installed.
func loadAAPanelSQLite(ssh SSHClient) (sites map[string]aaPanelSite, domains map[string][]string) {
	sites = make(map[string]aaPanelSite)
	domains = make(map[string][]string)

	dbPath := "/www/server/panel/data/default.db"
	tmpPath := fmt.Sprintf("/tmp/wpm_aapanel_%d.db", time.Now().UnixNano())

	// Copy DB to avoid lock conflicts; delete after query
	if _, err := ssh.Run(fmt.Sprintf("cp %s %s 2>/dev/null", dbPath, tmpPath)); err != nil {
		return
	}
	defer ssh.Run("rm -f " + tmpPath) //nolint:errcheck

	// NOTE: aaPanel's `sites` table does NOT have a `php_version` column.
	// PHP version is stored only in the nginx vhost via `include enable-php-84.conf;`.
	// This query fetches site name/path and domain mappings only.
	query := `SELECT s.id, s.name, s.path, '' AS php_version, COALESCE(d.name,'') AS domain FROM sites s LEFT JOIN domain d ON d.pid = s.id`

	out := ssh.RunQuiet(fmt.Sprintf(
		`sqlite3 -separator '|' %s %s 2>/dev/null`,
		tmpPath, shellQuote(query+";"),
	))

	if out == "" {
		// Fallback: python3 (always present on aaPanel servers)
		pyScript := fmt.Sprintf(
			`python3 -c "import sqlite3,sys; db=sqlite3.connect('%s'); [print('|'.join(str(x) if x is not None else '' for x in r)) for r in db.execute('%s')]" 2>/dev/null`,
			tmpPath, query,
		)
		out = ssh.RunQuiet(pyScript)
	}

	if out == "" {
		return
	}

	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		path := strings.TrimSpace(parts[2])
		domain := strings.TrimSpace(parts[4])

		if name == "" {
			continue
		}
		// Only set site entry once (first row wins; subsequent rows are extra domains)
		if _, ok := sites[name]; !ok {
			sites[name] = aaPanelSite{Name: name, Path: path, PHPVersion: ""}
		}
		if domain != "" {
			domains[name] = append(domains[name], domain)
		}
	}
	return
}

// ── Domain merge ──────────────────────────────────────────────────────────────

func mergeDomains(
	ssh SSHClient,
	siteName, docRoot, wpSiteURL string,
	panelSites map[string]aaPanelSite,
	panelDomains map[string][]string,
) (aliases []string, primary string) {

	seen := make(map[string]bool)
	var all []string

	// Source A: aaPanel SQLite domains
	if ds, ok := panelDomains[siteName]; ok {
		for _, d := range ds {
			d = strings.TrimSpace(d)
			if d != "" && !seen[d] {
				seen[d] = true
				all = append(all, d)
			}
		}
	}

	// Source B: webserver vhost (grep server_name in nginx)
	vhostDomains := parseNginxServerNames(ssh, siteName)
	for _, d := range vhostDomains {
		if !seen[d] {
			seen[d] = true
			all = append(all, d)
		}
	}

	// Source C (primary): WP siteurl host
	primary = ""
	if wpSiteURL != "" {
		primary = extractHost(wpSiteURL)
	}

	// Fall back to first domain from SQLite/vhost
	if primary == "" && len(all) > 0 {
		primary = all[0]
	}

	// Fall back to docroot basename
	if primary == "" {
		primary = extractSiteNameFromDocRoot(docRoot)
	}

	// aliases = all minus primary (always return non-nil slice so JSON is [] not null)
	aliases = []string{}
	for _, d := range all {
		if d != primary {
			aliases = append(aliases, d)
		}
	}
	return aliases, primary
}

// ── PHP detection (5-layer) ──────────────────────────────────────────────────

func detectPHPVersion(
	ssh SSHClient,
	siteName, docRoot, primaryDomain string,
	panelSites map[string]aaPanelSite,
) (version, method string) {

	// Layer A: aaPanel SQLite php_version column
	if s, ok := panelSites[siteName]; ok && s.PHPVersion != "" {
		v := normalizePhpVersion(s.PHPVersion)
		if v != "" {
			return v, "panel_db"
		}
	}

	// Layer B: parse named nginx vhost conf for PHP socket
	v := parseNginxPHPVersion(ssh, siteName)
	if v != "" {
		return v, "vhost_config"
	}

	// Layer B2: grep ALL nginx conf files for this docRoot, then parse PHP socket
	v = scanNginxConfsForDocRoot(ssh, docRoot)
	if v != "" {
		return v, "vhost_config"
	}

	// Layer C: aaPanel PHP-FPM pool conf file (/www/server/php/<ver>/etc/php-fpm.d/<site>.conf)
	v = detectPHPFromFPMPool(ssh, siteName)
	if v != "" {
		return v, "vhost_config"
	}

	// Layer D: HTTP probe X-Powered-By header
	v = probePHPHeader(primaryDomain)
	if v != "" {
		return v, "http_header"
	}

	// Layer E: unknown — caller will prompt user to select manually
	return "", "unknown"
}

// phpSocketPatterns matches PHP version references in nginx/vhost config files.
// Order matters: more specific patterns first.
var phpSocketPatterns = []*regexp.Regexp{
	// aaPanel-style: include enable-php-84.conf; → "84" → "8.4"
	regexp.MustCompile(`enable-php-(\d{2,3})\.conf`),
	// Direct socket paths
	regexp.MustCompile(`php-cgi-(\d+)\.sock`),            // /tmp/php-cgi-74.sock
	regexp.MustCompile(`php(\d)(\d)-fpm\.sock`),           // php74-fpm.sock
	regexp.MustCompile(`php(\d+\.\d+)-fpm\.sock`),         // php7.4-fpm.sock
	regexp.MustCompile(`unix:/run/php/php(\d+\.\d+)-fpm`), // Debian-style
	regexp.MustCompile(`lsphp(\d+)`),                      // OpenLiteSpeed
	regexp.MustCompile(`php(\d)(\d+)-fpm`),                // php74-fpm (no .sock)
	regexp.MustCompile(`php-fpm(\d+)\.(\d+)`),             // php-fpm7.4
	regexp.MustCompile(`/php/(\d+)/`),                     // /www/server/php/74/ paths
}

func extractPHPFromContent(content string) string {
	for _, re := range phpSocketPatterns {
		m := re.FindStringSubmatch(content)
		if m == nil {
			continue
		}
		raw := strings.Join(m[1:], ".")
		v := normalizePhpVersion(raw)
		if v != "" {
			return v
		}
	}
	return ""
}

func parseNginxPHPVersion(ssh SSHClient, siteName string) string {
	// aaPanel stores vhosts in /www/server/panel/vhost/nginx/<site>.conf
	paths := []string{
		fmt.Sprintf("/www/server/panel/vhost/nginx/%s.conf", siteName),
		fmt.Sprintf("/www/server/panel/vhost/nginx/%s.conf_bk", siteName),
		fmt.Sprintf("/etc/nginx/sites-available/%s.conf", siteName),
		fmt.Sprintf("/etc/nginx/conf.d/%s.conf", siteName),
	}
	for _, p := range paths {
		content := ssh.RunQuiet("cat " + shellQuote(p) + " 2>/dev/null")
		if v := extractPHPFromContent(content); v != "" {
			return v
		}
	}
	return ""
}

// scanNginxConfsForDocRoot finds which nginx conf has `root <docRoot>` and
// extracts the PHP version from the PHP-FPM socket reference in that conf.
func scanNginxConfsForDocRoot(ssh SSHClient, docRoot string) string {
	if docRoot == "" {
		return ""
	}
	// Find conf files referencing this docRoot
	confFile := strings.TrimSpace(ssh.RunQuiet(fmt.Sprintf(
		`grep -rl "root %s" /www/server/panel/vhost/nginx/ /etc/nginx/ 2>/dev/null | grep -v '\.bk$' | head -1`,
		docRoot,
	)))
	if confFile == "" {
		return ""
	}
	content := ssh.RunQuiet("cat " + shellQuote(confFile) + " 2>/dev/null")
	return extractPHPFromContent(content)
}

// detectPHPFromFPMPool checks aaPanel PHP-FPM pool conf directories for a
// per-site pool file, extracting the PHP version from the directory path.
// aaPanel path: /www/server/php/<version>/etc/php-fpm.d/<siteName>.conf
func detectPHPFromFPMPool(ssh SSHClient, siteName string) string {
	if siteName == "" {
		return ""
	}
	out := strings.TrimSpace(ssh.RunQuiet(fmt.Sprintf(
		`ls /www/server/php/*/etc/php-fpm.d/%s.conf 2>/dev/null | head -3`,
		shellQuote(siteName),
	)))
	if out == "" {
		return ""
	}
	// Path: /www/server/php/74/etc/php-fpm.d/site.conf → "74"
	re := regexp.MustCompile(`/www/server/php/(\d+)/`)
	m := re.FindStringSubmatch(out)
	if m != nil {
		return normalizePhpVersion(m[1])
	}
	return ""
}

func probePHPHeader(domain string) string {
	if domain == "" {
		return ""
	}
	client := &http.Client{Timeout: 5 * time.Second}
	urls := []string{"https://" + domain, "http://" + domain}
	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		resp.Body.Close()
		powered := resp.Header.Get("X-Powered-By")
		if strings.HasPrefix(powered, "PHP/") {
			// "PHP/8.1.27" → "8.1"
			ver := strings.TrimPrefix(powered, "PHP/")
			parts := strings.Split(ver, ".")
			if len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
		}
	}
	return ""
}

// normalizePhpVersion converts "74", "80", "81", "8.1", "8.1.27" → "8.1"
func normalizePhpVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, ".") {
		parts := strings.Split(raw, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return raw
	}
	// "74" → "7.4", "81" → "8.1"
	if len(raw) == 2 {
		return string(raw[0]) + "." + string(raw[1])
	}
	return raw
}

// ── SSL detection ────────────────────────────────────────────────────────────

func detectSSL(ssh SSHClient, siteName, primaryDomain string) *models.TransferSSLInfo {
	// Look for cert paths in common aaPanel locations
	certLocations := []struct{ cert, key string }{
		{
			fmt.Sprintf("/www/server/panel/vhost/cert/%s/fullchain.pem", siteName),
			fmt.Sprintf("/www/server/panel/vhost/cert/%s/privkey.pem", siteName),
		},
		{
			fmt.Sprintf("/www/server/panel/vhost/cert/%s/fullchain.pem", primaryDomain),
			fmt.Sprintf("/www/server/panel/vhost/cert/%s/privkey.pem", primaryDomain),
		},
		{
			fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", primaryDomain),
			fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", primaryDomain),
		},
	}

	// Also try to extract cert paths from nginx vhost
	vhostCertPath := extractNginxCertPath(ssh, siteName)
	if vhostCertPath != "" {
		keyPath := strings.Replace(vhostCertPath, "fullchain.pem", "privkey.pem", 1)
		keyPath = strings.Replace(keyPath, "certificate.crt", "private.key", 1)
		certLocations = append([]struct{ cert, key string }{{vhostCertPath, keyPath}}, certLocations...)
	}

	for _, loc := range certLocations {
		if !ssh.FileExists(loc.cert) {
			continue
		}
		// Parse cert with openssl
		out := ssh.RunQuiet(fmt.Sprintf(
			"openssl x509 -in %s -noout -enddate -issuer -text 2>/dev/null",
			shellQuote(loc.cert),
		))
		if out == "" {
			continue
		}

		info := &models.TransferSSLInfo{
			CertPath: loc.cert,
			KeyPath:  loc.key,
		}

		// Parse expiry
		if m := regexp.MustCompile(`notAfter=(.+)`).FindStringSubmatch(out); m != nil {
			if t, err := time.Parse("Jan  2 15:04:05 2006 MST", strings.TrimSpace(m[1])); err == nil {
				info.NotAfter = t.Format(time.RFC3339)
			}
		}

		// Parse issuer
		lowerOut := strings.ToLower(out)
		switch {
		case strings.Contains(lowerOut, "let's encrypt") || strings.Contains(lowerOut, "letsencrypt"):
			info.Issuer = "letsencrypt"
		case strings.Contains(lowerOut, "zerossl"):
			info.Issuer = "zerossl"
		case strings.Contains(lowerOut, "self-signed") || strings.Contains(lowerOut, "self signed"):
			info.Issuer = "selfsigned"
		default:
			info.Issuer = "paid"
		}

		// Parse SANs
		sanOut := ssh.RunQuiet(fmt.Sprintf(
			"openssl x509 -in %s -text -noout 2>/dev/null | grep -oP '(?<=DNS:)[^,]+'",
			shellQuote(loc.cert),
		))
		if sanOut != "" {
			for _, san := range strings.Split(sanOut, "\n") {
				san = strings.TrimSpace(san)
				if san != "" {
					info.SANs = append(info.SANs, san)
				}
			}
		}
		info.IsWildcard = strings.Contains(sanOut, "*.")
		return info
	}
	return nil
}

func extractNginxCertPath(ssh SSHClient, siteName string) string {
	confPaths := []string{
		fmt.Sprintf("/www/server/panel/vhost/nginx/%s.conf", siteName),
		fmt.Sprintf("/etc/nginx/sites-available/%s.conf", siteName),
	}
	for _, p := range confPaths {
		out := ssh.RunQuiet(fmt.Sprintf(
			"awk '/ssl_certificate[^_]/{print $2}' %s 2>/dev/null | head -1 | tr -d ';'",
			shellQuote(p),
		))
		if out != "" {
			return strings.TrimSpace(out)
		}
	}
	return ""
}

// ── StreamDB: mysqldump → stream to writer ────────────────────────────────────

// StreamDB dumps the site's MySQL database over SSH and writes to dst.
// No temp file is created on the source server.
func (p *AAPanelProvider) StreamDB(ssh SSHClient, site *models.DiscoveredSiteInfo, dst io.Writer) error {
	cmd := fmt.Sprintf(
		`mysqldump --single-transaction --quick --skip-comments --no-tablespaces `+
			`-h %s -u %s -p%s %s 2>/dev/null`,
		shellQuote(site.DBHost),
		shellQuote(site.DBUser),
		shellQuote(site.DBPass),
		shellQuote(site.DBName),
	)
	return ssh.StreamPipe(cmd, dst)
}

// ── FetchSSL ─────────────────────────────────────────────────────────────────

// FetchSSL downloads the certificate and key files from the source server.
func (p *AAPanelProvider) FetchSSL(ssh SSHClient, ssl *models.TransferSSLInfo, localCertPath, localKeyPath string) error {
	if ssl == nil {
		return fmt.Errorf("no SSL info available")
	}
	if err := ssh.DownloadFile(ssl.CertPath, localCertPath); err != nil {
		return fmt.Errorf("download cert: %w", err)
	}
	if err := ssh.DownloadFile(ssl.KeyPath, localKeyPath); err != nil {
		return fmt.Errorf("download key: %w", err)
	}
	return nil
}

// ── Nginx helpers ─────────────────────────────────────────────────────────────

func parseNginxServerNames(ssh SSHClient, siteName string) []string {
	paths := []string{
		fmt.Sprintf("/www/server/panel/vhost/nginx/%s.conf", siteName),
		fmt.Sprintf("/etc/nginx/sites-available/%s.conf", siteName),
		fmt.Sprintf("/etc/nginx/conf.d/%s.conf", siteName),
	}
	seen := make(map[string]bool)
	var domains []string
	for _, p := range paths {
		out := ssh.RunQuiet(fmt.Sprintf(
			`grep -oP '(?<=server_name\s)[^;]+' %s 2>/dev/null`,
			shellQuote(p),
		))
		if out == "" {
			continue
		}
		for _, d := range strings.Fields(out) {
			d = strings.TrimSpace(d)
			if d != "" && d != "_" && !seen[d] {
				seen[d] = true
				domains = append(domains, d)
			}
		}
	}
	return domains
}

// ── WP DB helpers ─────────────────────────────────────────────────────────────

func queryWPOptions(ssh SSHClient, dbHost, dbUser, dbPass, dbName, tablePrefix string) (siteURL, homeURL string) {
	// We query via mysql CLI on the remote server (simpler than tunnelling)
	query := fmt.Sprintf(
		`SELECT option_name, option_value FROM %soptions WHERE option_name IN ('siteurl','home') LIMIT 2;`,
		tablePrefix,
	)
	cmd := fmt.Sprintf(
		`mysql -h %s -u %s -p%s %s -N -e %s 2>/dev/null`,
		shellQuote(dbHost),
		shellQuote(dbUser),
		shellQuote(dbPass),
		shellQuote(dbName),
		shellQuote(query),
	)
	out := ssh.RunQuiet(cmd)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "siteurl":
			siteURL = parts[1]
		case "home":
			homeURL = parts[1]
		}
	}
	return
}

// ── Shell helpers (local to providers package) ────────────────────────────────

// shellQuote wraps a string in single quotes for shell safety.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ── Misc helpers ──────────────────────────────────────────────────────────────

func extractPHPDefine(content, name string) string {
	re := regexp.MustCompile(`define\s*\(\s*['"]` + regexp.QuoteMeta(name) + `['"]\s*,\s*['"]([^'"]+)['"]`)
	m := re.FindStringSubmatch(content)
	if m != nil {
		return m[1]
	}
	// Also handle $table_prefix = 'wp_';
	if name == "table_prefix" {
		re2 := regexp.MustCompile(`\$table_prefix\s*=\s*['"]([^'"]+)['"]`)
		m2 := re2.FindStringSubmatch(content)
		if m2 != nil {
			return m2[1]
		}
	}
	return ""
}

func extractSiteNameFromDocRoot(docRoot string) string {
	parts := strings.Split(docRoot, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return docRoot
}

func extractHost(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	if idx := strings.Index(rawURL, "/"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	return strings.TrimSpace(rawURL)
}

func detectWebServer(ssh SSHClient) string {
	if out := ssh.RunQuiet("which nginx 2>/dev/null"); out != "" {
		return "nginx"
	}
	if out := ssh.RunQuiet("which lsws 2>/dev/null"); out != "" {
		return "openlitespeed"
	}
	if out := ssh.RunQuiet("which apache2 2>/dev/null || which httpd 2>/dev/null"); out != "" {
		return "apache"
	}
	return "unknown"
}

