package ssl

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CertInfo struct {
	Domain    string    `json:"domain"`
	Issuer    string    `json:"issuer"`
	ExpiresAt time.Time `json:"expires_at"`
	AutoRenew bool     `json:"auto_renew"`
}

func IssueCertificate(domain, email string) error {
	args := []string{
		"--nginx",
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
		"-m", email,
		"--redirect",
	}
	out, err := exec.Command("certbot", args...).CombinedOutput()
	if err != nil {
		slog.Error("SSL issue failed", "domain", domain, "output", string(out))
		return fmt.Errorf("certbot failed: %s", string(out))
	}
	slog.Info("SSL certificate issued", "domain", domain)
	return nil
}

func RenewCertificate(domain string) error {
	// Step 1: Force-renew the certificate
	out, err := exec.Command("certbot", "renew", "--cert-name", domain, "--force-renewal").CombinedOutput()
	if err != nil {
		return fmt.Errorf("renewal failed: %s", string(out))
	}

	// Step 2: Redeploy the renewed cert to nginx (--reinstall deploys without issuing new cert)
	if deployOut, deployErr := exec.Command(
		"certbot", "--nginx", "-d", domain,
		"--non-interactive", "--reinstall",
	).CombinedOutput(); deployErr != nil {
		slog.Warn("Certbot nginx redeploy after renewal failed, trying reload", "domain", domain, "output", string(deployOut))
	}

	// Step 3: Reload nginx to pick up renewed cert regardless
	exec.Command("systemctl", "reload", "nginx").Run()
	return nil
}

// InstallCustomCert installs a custom SSL certificate for the domain.
// certPEM = domain certificate PEM, keyPEM = private key PEM, caPEM = CA bundle PEM (optional).
// It builds a fullchain (cert + CA), saves to /etc/nginx/ssl/{domain}/, then
// rewrites the nginx vhost to serve HTTPS using the new cert files.
func InstallCustomCert(domain, certPEM, keyPEM, caPEM string) error {
	sslDir := fmt.Sprintf("/etc/nginx/ssl/%s", domain)
	if err := os.MkdirAll(sslDir, 0700); err != nil {
		return fmt.Errorf("create ssl dir: %w", err)
	}

	// Build fullchain = cert + CA bundle (nginx needs the full chain in ssl_certificate)
	fullchain := certPEM
	if caPEM != "" {
		if fullchain[len(fullchain)-1] != '\n' {
			fullchain += "\n"
		}
		fullchain += caPEM
	}

	fullchainPath := sslDir + "/fullchain.pem"
	privkeyPath := sslDir + "/privkey.pem"

	if err := os.WriteFile(fullchainPath, []byte(fullchain), 0644); err != nil {
		return fmt.Errorf("write fullchain: %w", err)
	}
	if err := os.WriteFile(privkeyPath, []byte(keyPEM), 0600); err != nil {
		return fmt.Errorf("write privkey: %w", err)
	}

	// Detect PHP version from existing nginx config
	phpVer := detectPHPFromNginx(domain)

	// Write a fresh nginx vhost that uses the custom cert
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	cfg := buildCustomSSLNginxConf(domain, phpVer, fullchainPath, privkeyPath)
	if err := os.WriteFile(nginxConf, []byte(cfg), 0644); err != nil {
		return fmt.Errorf("write nginx config: %w", err)
	}

	// Ensure symlink
	symlinkPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain)
	if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
		os.Symlink(nginxConf, symlinkPath)
	}

	// Validate and reload nginx
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config invalid: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("Custom SSL certificate installed", "domain", domain)
	return nil
}

func buildCustomSSLNginxConf(domain, phpVer, fullchainPath, privkeyPath string) string {
	return fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name %s www.%s;

    ssl_certificate     %s;
    ssl_certificate_key %s;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-CHACHA20-POLY1305;
    ssl_prefer_server_ciphers on;
    ssl_session_cache   shared:SSL:10m;
    ssl_session_timeout 10m;

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
`, domain, domain, domain, domain, fullchainPath, privkeyPath, domain, domain, domain, phpVer, domain)
}

// DisableSSL removes the Let's Encrypt certificate and rewrites a minimal
// HTTP-only nginx config so the site continues to serve on port 80.
func DisableSSL(domain, phpVer string) error {
	if phpVer == "" {
		phpVer = detectPHPFromNginx(domain)
	}

	// 1. Delete the Let's Encrypt certificate (non-fatal — cert may already be gone)
	out, err := exec.Command("certbot", "delete", "--cert-name", domain, "--non-interactive").CombinedOutput()
	if err != nil {
		slog.Warn("certbot delete warning (cert may not exist)", "domain", domain, "output", string(out))
	} else {
		slog.Info("certbot cert deleted", "domain", domain)
	}

	// 2. Write a clean HTTP-only nginx config (replaces any SSL/certbot-managed config)
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	if err := os.WriteFile(nginxConf, []byte(buildMinimalHTTPNginxConf(domain, phpVer)), 0644); err != nil {
		return fmt.Errorf("write nginx config: %w", err)
	}

	// 3. Ensure symlink in sites-enabled
	symlinkPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain)
	if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
		os.Symlink(nginxConf, symlinkPath)
	}

	// 4. Validate and reload nginx
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config invalid: %s", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()

	slog.Info("SSL disabled — site now HTTP only", "domain", domain)
	return nil
}

func detectPHPFromNginx(domain string) string {
	data, err := os.ReadFile(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))
	if err != nil {
		return "8.2"
	}
	content := string(data)
	for _, ver := range []string{"8.3", "8.2", "8.1", "8.0", "7.4"} {
		if strings.Contains(content, "php"+ver+"-fpm") {
			return ver
		}
	}
	return "8.2"
}

func buildMinimalHTTPNginxConf(domain, phpVer string) string {
	return fmt.Sprintf(`server {
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
}

func GetCertExpiry(domain string) (*time.Time, error) {
	out, err := exec.Command("certbot", "certificates", "-d", domain).CombinedOutput()
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Expiry Date:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				dateStr := strings.TrimSpace(strings.Split(parts[1], "(")[0])
				t, err := time.Parse("2006-01-02 15:04:05+00:00", dateStr)
				if err == nil {
					return &t, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("could not parse expiry for %s", domain)
}
