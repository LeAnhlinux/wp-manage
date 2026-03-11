package config

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// siteUser derives the Linux username for a WordPress site (matches site/manager.go).
func siteUser(domain string) string {
	return "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
}

type RebuildResult struct {
	ConfigType string `json:"config_type"`
	FilePath   string `json:"file_path"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	BackupPath string `json:"backup_path"`
}

type HealthStatus struct {
	ConfigType string `json:"config_type"`
	FilePath   string `json:"file_path"`
	Status     string `json:"status"` // ok, warning, error
	Message    string `json:"message"`
	Diff       string `json:"diff"`
}

func RebuildNginxVhost(domain string, configData map[string]string) (*RebuildResult, error) {
	filePath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	backupPath := backupConfig(filePath)

	phpVersion := configData["php_version"]
	if phpVersion == "" {
		phpVersion = "8.2"
	}
	sslEnabled := configData["ssl_enabled"] == "true"

	// If ssl_enabled but certbot cert exists, generate SSL config directly.
	// This prevents RebuildNginxVhost from stripping certbot SSL blocks.
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
	if _, err := os.Stat(certPath); err == nil {
		sslEnabled = true // cert exists → always build SSL config
	}

	conf := generateNginxVhost(domain, phpVersion, sslEnabled)

	if err := os.WriteFile(filePath, []byte(conf), 0644); err != nil {
		restoreConfig(filePath, backupPath)
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	// Test nginx config
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		slog.Error("Nginx config test failed, rolling back", "output", string(out))
		restoreConfig(filePath, backupPath)
		return &RebuildResult{
			ConfigType: "nginx_vhost",
			FilePath:   filePath,
			Success:    false,
			Message:    "Config test failed, rolled back: " + string(out),
			BackupPath: backupPath,
		}, nil
	}

	exec.Command("systemctl", "reload", "nginx").Run()

	return &RebuildResult{
		ConfigType: "nginx_vhost",
		FilePath:   filePath,
		Success:    true,
		Message:    "Nginx vhost rebuilt and reloaded successfully",
		BackupPath: backupPath,
	}, nil
}

func RebuildPHPFPMPool(domain, phpVersion string, configData map[string]string) (*RebuildResult, error) {
	filePath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVersion, domain)
	backupPath := backupConfig(filePath)

	maxChildren := configData["max_children"]
	if maxChildren == "" {
		maxChildren = "5"
	}
	memoryLimit := configData["memory_limit"]
	if memoryLimit == "" {
		memoryLimit = "256M"
	}

	// open_basedir is enabled unless explicitly set to "false" in configData.
	// Defaults to true (safe default) if key is absent.
	openBasedir := configData["open_basedir_enabled"] != "false"

	su := siteUser(domain)
	conf := fmt.Sprintf(`[%s]
user = %s
group = %s
listen = /var/run/php/php%s-fpm-%s.sock
listen.owner = www-data
listen.group = www-data
pm = dynamic
pm.max_children = %s
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3
pm.max_requests = 500
php_admin_value[memory_limit] = %s
php_admin_value[upload_max_filesize] = 64M
php_admin_value[post_max_size] = 64M
php_admin_value[max_execution_time] = 300
php_admin_value[max_input_vars] = 3000
`, domain, su, su, phpVersion, domain, maxChildren, memoryLimit)

	if openBasedir {
		conf += fmt.Sprintf("php_admin_value[open_basedir] = /var/www/%s/:/tmp/:/usr/share/php/\n", domain)
	}
	conf += "php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,parse_ini_file,show_source\n"

	if err := os.WriteFile(filePath, []byte(conf), 0644); err != nil {
		restoreConfig(filePath, backupPath)
		return nil, err
	}

	svc := fmt.Sprintf("php%s-fpm", phpVersion)
	testBin := fmt.Sprintf("php-fpm%s", phpVersion)
	if out, err := exec.Command(testBin, "-t").CombinedOutput(); err != nil {
		restoreConfig(filePath, backupPath)
		return &RebuildResult{
			ConfigType: "php_fpm_pool",
			FilePath:   filePath,
			Success:    false,
			Message:    "Config test failed, rolled back: " + string(out),
		}, nil
	}

	exec.Command("systemctl", "restart", svc).Run()

	return &RebuildResult{
		ConfigType: "php_fpm_pool",
		FilePath:   filePath,
		Success:    true,
		Message:    "PHP-FPM pool rebuilt and reloaded",
		BackupPath: backupPath,
	}, nil
}

func RunHealthCheck() ([]HealthStatus, error) {
	var results []HealthStatus

	// Check nginx
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		results = append(results, HealthStatus{
			ConfigType: "nginx",
			FilePath:   "/etc/nginx/nginx.conf",
			Status:     "error",
			Message:    string(out),
		})
	} else {
		results = append(results, HealthStatus{
			ConfigType: "nginx",
			FilePath:   "/etc/nginx/nginx.conf",
			Status:     "ok",
			Message:    "Nginx config is valid",
		})
	}

	// Check PHP-FPM versions
	for _, v := range []string{"7.4", "8.0", "8.1", "8.2", "8.3"} {
		binary := fmt.Sprintf("php-fpm%s", v)
		if _, err := exec.LookPath(binary); err == nil {
			if out, err := exec.Command(binary, "-t").CombinedOutput(); err != nil {
				results = append(results, HealthStatus{
					ConfigType: fmt.Sprintf("php-fpm-%s", v),
					Status:     "error",
					Message:    string(out),
				})
			} else {
				results = append(results, HealthStatus{
					ConfigType: fmt.Sprintf("php-fpm-%s", v),
					Status:     "ok",
					Message:    fmt.Sprintf("PHP-FPM %s config is valid", v),
				})
			}
		}
	}

	// Check MariaDB
	if out, err := exec.Command("mariadb", "-e", "SELECT 1").CombinedOutput(); err != nil {
		results = append(results, HealthStatus{
			ConfigType: "mariadb",
			Status:     "error",
			Message:    string(out),
		})
	} else {
		results = append(results, HealthStatus{
			ConfigType: "mariadb",
			Status:     "ok",
			Message:    "MariaDB is running",
		})
	}

	return results, nil
}

func generateNginxVhost(domain, phpVersion string, ssl bool) string {
	phpBlock := fmt.Sprintf(`    location ~ \.php$ {
        fastcgi_pass unix:/var/run/php/php%s-fpm-%s.sock;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
        fastcgi_read_timeout 300;
    }`, phpVersion, domain)

	commonLocations := fmt.Sprintf(`    location / {
        try_files $uri $uri/ /index.php?$args;
    }

%s

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }

    location ~ /\.ht { deny all; }
    location = /xmlrpc.php { deny all; }
    location ~* /wp-config\.php { deny all; }`, phpBlock)

	if ssl {
		certFile := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
		keyFile := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain)
		// Check if dhparam file exists (created by certbot on first run)
		dhparam := ""
		if _, err := os.Stat("/etc/letsencrypt/ssl-dhparams.pem"); err == nil {
			dhparam = "    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;"
		}
		sslOpts := ""
		if _, err := os.Stat("/etc/letsencrypt/options-ssl-nginx.conf"); err == nil {
			sslOpts = "    include /etc/letsencrypt/options-ssl-nginx.conf;"
		}

		return fmt.Sprintf(`server {
    listen 443 ssl;
    server_name %s www.%s;
    root /var/www/%s/public;
    index index.php index.html;

    access_log /var/www/%s/logs/access.log;
    error_log /var/www/%s/logs/error.log;

    client_max_body_size 64M;

%s

    ssl_certificate %s;
    ssl_certificate_key %s;
%s
%s
}

server {
    listen 80;
    server_name %s www.%s;
    return 301 https://$host$request_uri;
}
`, domain, domain, domain, domain, domain, commonLocations, certFile, keyFile, sslOpts, dhparam, domain, domain)
	}

	// HTTP-only (no cert yet)
	return fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;
    root /var/www/%s/public;
    index index.php index.html;

    access_log /var/www/%s/logs/access.log;
    error_log /var/www/%s/logs/error.log;

    client_max_body_size 64M;

%s
}
`, domain, domain, domain, domain, domain, commonLocations)
}

func backupConfig(filePath string) string {
	backupPath := fmt.Sprintf("%s.bak.%d", filePath, time.Now().Unix())
	exec.Command("cp", filePath, backupPath).Run()
	return backupPath
}

func restoreConfig(filePath, backupPath string) {
	exec.Command("cp", backupPath, filePath).Run()
	slog.Info("Config restored from backup", "path", filePath)
}
