package nginx

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// EnableFastCGICache enables Nginx FastCGI cache for a domain
func EnableFastCGICache(domain, cacheTime, excludes string) error {
	if cacheTime == "" {
		cacheTime = "60m"
	}

	vhostFile := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(vhostFile)
	if err != nil {
		return fmt.Errorf("failed to read vhost: %w", err)
	}

	content := string(data)

	// Check if cache is already configured
	if strings.Contains(content, "fastcgi_cache_path") {
		slog.Info("FastCGI cache already configured", "domain", domain)
		return testAndReload()
	}

	// Đảm bảo fastcgi_cache_key có trong nginx.conf (http block) — chỉ 1 lần toàn server.
	// Nginx 1.24+ không cho phép khai báo fastcgi_cache_key nhiều lần trong http context.
	ensureFastCGICacheKey()

	// Add cache zone definition before server block — chỉ fastcgi_cache_path (site-specific)
	cacheZone := fmt.Sprintf(`# FastCGI Cache for %s
fastcgi_cache_path /var/cache/nginx/%s levels=1:2 keys_zone=%s:10m max_size=256m inactive=%s;

`, domain, domain, domain, cacheTime)

	// Build exclude conditions
	excludeBlock := `
    # Cache bypass conditions
    set $skip_cache 0;
    if ($request_method = POST) { set $skip_cache 1; }
    if ($query_string != "") { set $skip_cache 1; }
    if ($request_uri ~* "/wp-admin/|/wp-json/|/xmlrpc.php|wp-.*.php|/feed/|index.php|sitemap(_index)?.xml") { set $skip_cache 1; }
    if ($http_cookie ~* "comment_author|wordpress_[a-f0-9]+|wp-postpass|wordpress_no_cache|wordpress_logged_in") { set $skip_cache 1; }
`
	if excludes != "" {
		for _, path := range strings.Split(excludes, ",") {
			path = strings.TrimSpace(path)
			if path != "" {
				excludeBlock += fmt.Sprintf("    if ($request_uri ~* \"%s\") { set $skip_cache 1; }\n", path)
			}
		}
	}

	// Add cache directives to fastcgi_pass location
	cacheDirectives := fmt.Sprintf(`
    fastcgi_cache %s;
    fastcgi_cache_valid 200 301 302 %s;
    fastcgi_cache_bypass $skip_cache;
    fastcgi_no_cache $skip_cache;
    add_header X-FastCGI-Cache $upstream_cache_status;
`, domain, cacheTime)

	// Insert cache zone before first "server {"
	content = cacheZone + content

	// Insert exclude block + cache directives after "location ~ \.php$" block opening
	content = strings.Replace(content, "fastcgi_pass unix:", excludeBlock+"\n    fastcgi_pass unix:", 1)
	content = strings.Replace(content, "fastcgi_param SCRIPT_FILENAME", cacheDirectives+"    fastcgi_param SCRIPT_FILENAME", 1)

	// Create cache directory
	os.MkdirAll(fmt.Sprintf("/var/cache/nginx/%s", domain), 0755)

	if err := os.WriteFile(vhostFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write vhost: %w", err)
	}

	slog.Info("FastCGI cache enabled", "domain", domain, "cache_time", cacheTime)
	return testAndReload()
}

// DisableFastCGICache removes cache config from vhost
func DisableFastCGICache(domain string) error {
	vhostFile := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	data, err := os.ReadFile(vhostFile)
	if err != nil {
		return fmt.Errorf("failed to read vhost: %w", err)
	}

	content := string(data)

	// Remove cache zone definition
	lines := strings.Split(content, "\n")
	var filtered []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, "# FastCGI Cache for") {
			skip = true
			continue
		}
		if skip && line == "" {
			skip = false
			continue
		}
		if skip && (strings.Contains(line, "fastcgi_cache_path") || strings.Contains(line, "fastcgi_cache_key")) {
			continue
		}
		skip = false

		// Remove inline cache directives
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "fastcgi_cache ") ||
			strings.HasPrefix(trimmed, "fastcgi_cache_valid") ||
			strings.HasPrefix(trimmed, "fastcgi_cache_bypass") ||
			strings.HasPrefix(trimmed, "fastcgi_no_cache") ||
			strings.Contains(trimmed, "X-FastCGI-Cache") ||
			strings.Contains(trimmed, "# Cache bypass conditions") ||
			(strings.Contains(trimmed, "set $skip_cache") && !strings.Contains(trimmed, "map")) {
			continue
		}

		filtered = append(filtered, line)
	}

	if err := os.WriteFile(vhostFile, []byte(strings.Join(filtered, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write vhost: %w", err)
	}

	// Remove cache directory
	os.RemoveAll(fmt.Sprintf("/var/cache/nginx/%s", domain))

	slog.Info("FastCGI cache disabled", "domain", domain)
	return testAndReload()
}

// ensureFastCGICacheKey đảm bảo fastcgi_cache_key xuất hiện đúng 1 lần trong nginx.conf http block.
// Nginx 1.24+ không cho phép duplicate directive này trong http context.
func ensureFastCGICacheKey() {
	const cacheKeyLine = `fastcgi_cache_key "$scheme$request_method$host$request_uri";`
	nginxConf := "/etc/nginx/nginx.conf"
	data, err := os.ReadFile(nginxConf)
	if err != nil {
		return
	}
	if strings.Contains(string(data), "fastcgi_cache_key") {
		return // đã có rồi, không thêm nữa
	}
	// Chèn vào cuối http block, trước dòng include sites-enabled
	content := strings.Replace(string(data),
		"include /etc/nginx/sites-enabled",
		cacheKeyLine+"\n\tinclude /etc/nginx/sites-enabled",
		1)
	os.WriteFile(nginxConf, []byte(content), 0644)
}

// PurgeCache clears the FastCGI cache for a domain
func PurgeCache(domain string) error {
	cachePath := fmt.Sprintf("/var/cache/nginx/%s", domain)
	if err := os.RemoveAll(cachePath); err != nil {
		return fmt.Errorf("failed to purge cache: %w", err)
	}
	os.MkdirAll(cachePath, 0755)
	slog.Info("FastCGI cache purged", "domain", domain)
	return nil
}

