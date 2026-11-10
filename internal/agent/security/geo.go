package security

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// Country access: nginx decides per request from three shared variables
// ($wpm_geo_country, $wpm_geo_provider, $wpm_geo_exempt — one file per
// server) and three small per-site maps.
const (
	geoDBPath         = "/var/lib/wp-manage/geoip/country.mmdb"
	geoBaseConfPath   = "/etc/nginx/conf.d/wpm-geo-base.conf"
	geoRealIPConfPath = "/etc/nginx/conf.d/cloudflare-real-ip.conf" // shared with the Cloudflare firewall + login hardening
	geoDBMaxAge       = 35 * 24 * time.Hour
	geoModulePackage  = "libnginx-mod-http-geoip2"
	geoSelf           = "self"
)

func geoSiteConfPath(siteID uint64) string {
	return fmt.Sprintf("/etc/nginx/conf.d/wpm-geo-site-%d.conf", siteID)
}

// normalizeCIDR returns the canonical form of a network, or "" if it is not one.
// Everything that reaches an nginx file goes through this (or ValidCountryCode).
func normalizeCIDR(raw string) string {
	raw = strings.TrimSpace(raw)
	if ip := net.ParseIP(raw); ip != nil {
		if ip.To4() != nil {
			return ip.String() + "/32"
		}
		return ip.String() + "/128"
	}
	_, network, err := net.ParseCIDR(raw)
	if err != nil {
		return ""
	}
	return network.String()
}

// BuildGeoBaseConfig renders the server-wide file. providers maps a catalog key
// to its CIDRs; selfIPs are this server's own addresses, which must always be
// allowed because WordPress calls itself (cron, site health, REST loopback).
func BuildGeoBaseConfig(providers map[string][]string, selfIPs []string) string {
	var b strings.Builder
	b.WriteString("# Managed by WP Manage — country access (shared by all sites). Do not edit.\n")
	fmt.Fprintf(&b, "geoip2 %s {\n    auto_reload 60m;\n    $wpm_geo_country country iso_code;\n}\n", geoDBPath)

	b.WriteString("geo $wpm_geo_provider {\n    default \"\";\n")
	seen := map[string]bool{}
	add := func(cidr, value string) {
		n := normalizeCIDR(cidr)
		if n == "" || seen[n] {
			return // first writer wins: self, then catalog order
		}
		seen[n] = true
		fmt.Fprintf(&b, "    %s %s;\n", n, value)
	}
	add("127.0.0.0/8", geoSelf)
	add("::1", geoSelf)
	for _, ip := range selfIPs {
		add(ip, geoSelf)
	}
	for _, p := range models.GeoProviders {
		cidrs := append([]string(nil), providers[p.Key]...)
		sort.Strings(cidrs)
		for _, c := range cidrs {
			add(c, p.Key)
		}
	}
	b.WriteString("}\n")

	// Let's Encrypt validates from several countries; blocking it breaks renewals.
	b.WriteString("map $request_uri $wpm_geo_exempt {\n    default 0;\n    \"~^/\\.well-known/acme-challenge/\" 1;\n}\n")
	return b.String()
}

// validateGeoSite checks everything that ends up in a path or in nginx config.
func validateGeoSite(siteID uint64, domain string, rule models.GeoRuleSet) error {
	if siteID == 0 {
		return fmt.Errorf("geo rules: missing site id for %q", domain)
	}
	if err := utils.ValidateDomain(domain); err != nil {
		return fmt.Errorf("geo rules: %w", err)
	}
	if !models.ValidGeoMode(rule.Mode) {
		return fmt.Errorf("geo rules: invalid mode %q", rule.Mode)
	}
	for _, c := range rule.Countries {
		if !models.ValidCountryCode(c) {
			return fmt.Errorf("geo rules: invalid country code %q", c)
		}
	}
	if rule.Mode != models.GeoModeOff && len(rule.Countries) == 0 {
		return fmt.Errorf("geo rules: %s mode needs at least one country", rule.Mode)
	}
	for key, action := range rule.Providers {
		if _, ok := models.FindGeoProvider(key); !ok {
			return fmt.Errorf("geo rules: unknown provider %q", key)
		}
		if action != models.GeoActionAllow && action != models.GeoActionDeny {
			return fmt.Errorf("geo rules: invalid action %q for %s", action, key)
		}
	}
	return nil
}

func geoRuleEnforced(rule models.GeoRuleSet) bool {
	if rule.Mode != models.GeoModeOff {
		return true
	}
	for _, action := range rule.Providers {
		if action == models.GeoActionDeny {
			return true
		}
	}
	return false
}

// BuildGeoSiteConfig renders one site's http-context maps and its
// server-context snippet. Both are empty when the rules can block nothing,
// meaning the site's files should be removed.
//
// Decision order, encoded in the final map: exempt path → provider deny →
// provider allow / this server → country.
func BuildGeoSiteConfig(siteID uint64, domain string, rule models.GeoRuleSet) (httpConf, snippet string, err error) {
	if err := validateGeoSite(siteID, domain, rule); err != nil {
		return "", "", err
	}
	if !geoRuleEnforced(rule) {
		return "", "", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Managed by WP Manage — country access for site %d (%s). Do not edit.\n", siteID, domain)

	// 1 = blocked by country. An unknown country ("") is always allowed.
	countries := append([]string(nil), rule.Countries...)
	sort.Strings(countries)
	fmt.Fprintf(&b, "map $wpm_geo_country $wpm_geo_c_%d {\n", siteID)
	switch rule.Mode {
	case models.GeoModeWhitelist:
		b.WriteString("    default 1;\n    \"\" 0;\n")
		for _, c := range countries {
			fmt.Fprintf(&b, "    %s 0;\n", c)
		}
	case models.GeoModeBlacklist:
		b.WriteString("    default 0;\n")
		for _, c := range countries {
			fmt.Fprintf(&b, "    %s 1;\n", c)
		}
	default:
		b.WriteString("    default 0;\n")
	}
	b.WriteString("}\n")

	keys := make([]string, 0, len(rule.Providers))
	for k := range rule.Providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(&b, "map $wpm_geo_provider $wpm_geo_p_%d {\n    default \"\";\n    %s allow;\n", siteID, geoSelf)
	for _, k := range keys {
		fmt.Fprintf(&b, "    %s %s;\n", k, rule.Providers[k])
	}
	b.WriteString("}\n")

	fmt.Fprintf(&b, "map \"$wpm_geo_exempt:$wpm_geo_p_%d:$wpm_geo_c_%d\" $wpm_geo_block_%d {\n", siteID, siteID, siteID)
	b.WriteString("    default 0;\n    \"0:deny:0\" 1;\n    \"0:deny:1\" 1;\n    \"0::1\" 1;\n}\n")

	snippet = fmt.Sprintf("# Managed by WP Manage — country access. Do not edit.\nif ($wpm_geo_block_%d) { return 403; }\n", siteID)
	return b.String(), snippet, nil
}

// LocalIPs returns this server's global unicast addresses.
func LocalIPs() []string {
	var out []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || !ipnet.IP.IsGlobalUnicast() {
			continue
		}
		out = append(out, ipnet.IP.String())
	}
	sort.Strings(out)
	return out
}

// ensureGeoModule installs the nginx geoip2 module if it is not loaded yet.
// On Debian/Ubuntu the package enables itself and a plain reload activates it.
func ensureGeoModule() error {
	if matches, _ := filepath.Glob("/etc/nginx/modules-enabled/*geoip2*"); len(matches) > 0 {
		return nil
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("the nginx geoip2 module is not installed and automatic install is only supported on Debian/Ubuntu (package %s)", geoModulePackage)
	}
	slog.Info("Installing nginx geoip2 module", "package", geoModulePackage)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", "install", "-y", "-q", geoModulePackage)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if out, err := cmd.CombinedOutput(); err != nil {
		tail := strings.TrimSpace(string(out))
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		return fmt.Errorf("could not install %s: %s", geoModulePackage, tail)
	}
	if matches, _ := filepath.Glob("/etc/nginx/modules-enabled/*geoip2*"); len(matches) == 0 {
		return fmt.Errorf("%s was installed but nginx does not load it", geoModulePackage)
	}
	return nil
}

// downloadGeoDB fetches one month's DB-IP Lite country database to dest.
func downloadGeoDB(month time.Time, dest string) error {
	url := fmt.Sprintf("https://download.db-ip.com/free/dbip-country-lite-%s.mmdb.gz", month.Format("2006-01"))
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(gz, 200<<20))
	if err != nil {
		return err
	}
	// Every MaxMind-format database ends with this metadata marker.
	if len(data) < 1<<20 || !bytes.Contains(data[len(data)-(128<<10):], []byte("\xab\xcd\xefMaxMind.com")) {
		return fmt.Errorf("%s is not a valid country database", url)
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// ensureGeoDB makes sure a reasonably fresh country database is on disk. A
// stale database is kept (with a warning) when the download fails: stale
// country data is far better than taking the rules down.
func ensureGeoDB() error {
	info, statErr := os.Stat(geoDBPath)
	if statErr == nil && time.Since(info.ModTime()) < geoDBMaxAge {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(geoDBPath), 0755); err != nil {
		return err
	}
	now := time.Now().UTC()
	var lastErr error
	// The new month's file can appear a few days late: fall back one month.
	for _, month := range []time.Time{now, now.AddDate(0, 0, -now.Day())} {
		if lastErr = downloadGeoDB(month, geoDBPath); lastErr == nil {
			slog.Info("Country database updated", "month", month.Format("2006-01"))
			return nil
		}
	}
	if statErr == nil {
		slog.Warn("Country database refresh failed, keeping the existing file", "error", lastErr)
		return nil
	}
	return fmt.Errorf("could not download the country database: %w", lastErr)
}

// buildRealIPConfig renders the Cloudflare real-IP file, so sites proxied by
// Cloudflare are judged by the visitor's address rather than Cloudflare's.
func buildRealIPConfig(cloudflareCIDRs []string) string {
	var b strings.Builder
	b.WriteString("# Cloudflare Real IP — auto-generated by wp-manage, do not edit\n")
	n := 0
	for _, c := range cloudflareCIDRs {
		if cidr := normalizeCIDR(c); cidr != "" {
			fmt.Fprintf(&b, "set_real_ip_from %s;\n", cidr)
			n++
		}
	}
	if n == 0 {
		return ""
	}
	b.WriteString("real_ip_header CF-Connecting-IP;\n")
	return b.String()
}

// GeoDeps is the data the agent gathers before applying country access.
// Providers is nil when the lists could not be fetched from the panel.
type GeoDeps struct {
	Providers map[string][]string
	SelfIPs   []string
}

// writeGeoBase (re)writes the shared files, recording backups for rollback.
// With no provider data it keeps an existing base file untouched.
func writeGeoBase(deps GeoDeps, needProviders bool, backups *[]fileBackup) error {
	_, statErr := os.Stat(geoBaseConfPath)
	if deps.Providers == nil {
		if statErr == nil {
			return nil
		}
		if needProviders {
			return fmt.Errorf("provider IP lists are not available from the panel yet — try again in a minute")
		}
	}
	*backups = append(*backups, backupFile(geoBaseConfPath))
	if err := os.WriteFile(geoBaseConfPath, []byte(BuildGeoBaseConfig(deps.Providers, deps.SelfIPs)), 0644); err != nil {
		return fmt.Errorf("write %s: %w", geoBaseConfPath, err)
	}
	// Only create the real-IP file; when it exists another feature owns it.
	if _, err := os.Stat(geoRealIPConfPath); os.IsNotExist(err) {
		if conf := buildRealIPConfig(deps.Providers["cloudflare"]); conf != "" {
			*backups = append(*backups, backupFile(geoRealIPConfPath))
			if err := os.WriteFile(geoRealIPConfPath, []byte(conf), 0644); err != nil {
				return fmt.Errorf("write %s: %w", geoRealIPConfPath, err)
			}
		}
	}
	return nil
}

// ApplyGeoRules writes country-access config for every site in the task, then
// tests and reloads nginx once. All-or-nothing: on any error every file
// touched is restored.
func ApplyGeoRules(task models.AgentGeoRulesTask, deps GeoDeps) error {
	type rendered struct {
		site              models.AgentGeoSite
		httpConf, snippet string
	}
	items := make([]rendered, 0, len(task.Sites))
	anyEnforced, needProviders := false, false
	for _, site := range task.Sites {
		rule, err := task.RuleFor(site)
		if err != nil {
			return err
		}
		httpConf, snippet, err := BuildGeoSiteConfig(site.SiteID, site.Domain, rule)
		if err != nil {
			return err
		}
		if httpConf != "" {
			anyEnforced = true
			if len(rule.Providers) > 0 {
				needProviders = true
			}
		}
		items = append(items, rendered{site, httpConf, snippet})
	}
	if len(items) == 0 {
		return nil
	}

	var backups []fileBackup
	rollback := func() {
		for i := len(backups) - 1; i >= 0; i-- {
			backups[i].restore()
		}
	}

	if anyEnforced {
		if err := ensureGeoModule(); err != nil {
			return err
		}
		if err := ensureGeoDB(); err != nil {
			return err
		}
		if err := os.MkdirAll(geoInclude.dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", geoInclude.dir, err)
		}
		if err := writeGeoBase(deps, needProviders, &backups); err != nil {
			rollback()
			return err
		}
	}

	for _, it := range items {
		vPath := vhostPath(it.site.Domain)
		vhostData, err := os.ReadFile(vPath)
		if err != nil {
			rollback()
			return fmt.Errorf("nginx vhost not found for %s", it.site.Domain)
		}
		enforce := it.httpConf != ""
		newVhost := geoInclude.set(string(vhostData), it.site.SiteID, enforce)
		if enforce && !strings.Contains(newVhost, geoInclude.line(it.site.SiteID)) {
			rollback()
			return fmt.Errorf("could not find a server block in the nginx vhost for %s", it.site.Domain)
		}
		writes := []struct{ path, content string }{
			{geoSiteConfPath(it.site.SiteID), it.httpConf},
			{geoInclude.snippetPath(it.site.SiteID), it.snippet},
			{vPath, newVhost},
		}
		for _, w := range writes {
			backups = append(backups, backupFile(w.path))
			if err := writeOrRemoveFile(w.path, w.content); err != nil {
				rollback()
				return fmt.Errorf("write %s: %w", w.path, err)
			}
		}
	}

	if err := nginxTestAndReload(rollback); err != nil {
		return err
	}
	slog.Info("Country access rules applied", "sites", len(items))
	return nil
}

// RefreshGeoData updates the country database and the provider ranges on a
// server that already uses country access. No-op elsewhere.
func RefreshGeoData(deps GeoDeps) error {
	if _, err := os.Stat(geoBaseConfPath); err != nil {
		return nil
	}
	if err := ensureGeoDB(); err != nil {
		return err
	}
	if deps.Providers == nil {
		return fmt.Errorf("provider IP lists are not available from the panel")
	}
	var backups []fileBackup
	if err := writeGeoBase(deps, false, &backups); err != nil {
		return err
	}
	return nginxTestAndReload(func() {
		for i := len(backups) - 1; i >= 0; i-- {
			backups[i].restore()
		}
	})
}
