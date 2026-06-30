package provisioning

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	agentfail2ban "github.com/gofiber-team/wp-manage/internal/agent/fail2ban"
)

type ProgressFunc func(step string, progress int, message string)

type Provisioner struct {
	onProgress ProgressFunc
}

func New(onProgress ProgressFunc) *Provisioner {
	return &Provisioner{onProgress: onProgress}
}

func (p *Provisioner) Run() error {
	steps := []struct {
		name    string
		pct     int
		fn      func() error
	}{
		{"update_system", 5, p.updateSystem},
		{"install_nginx", 15, p.installNginx},
		{"install_php", 35, p.installPHP},
		{"install_mariadb", 55, p.installMariaDB},
		{"install_redis", 65, p.installRedis},
		{"install_wpcli", 68, p.installWPCLI},
		{"install_certbot", 72, p.installCertbot},
		{"install_fail2ban", 76, p.installFail2Ban},
		{"configure_ufw", 80, p.configureUFW},
		{"configure_nginx", 85, p.configureNginx},
		{"configure_php", 90, p.configurePHP},
		{"configure_mariadb", 95, p.configureMariaDB},
		{"finalize", 100, p.finalize},
	}

	for _, step := range steps {
		p.onProgress(step.name, step.pct, fmt.Sprintf("Running: %s", step.name))
		slog.Info("Provisioning step", "step", step.name)
		if err := step.fn(); err != nil {
			return fmt.Errorf("step %s failed: %w", step.name, err)
		}
	}
	return nil
}

func (p *Provisioner) updateSystem() error {
	return runCmd("apt-get", "update", "-y")
}

func (p *Provisioner) installNginx() error {
	return runCmd("apt-get", "install", "-y", "nginx")
}

func (p *Provisioner) installPHP() error {
	if err := runCmd("apt-get", "install", "-y", "software-properties-common"); err != nil {
		return err
	}
	runCmd("add-apt-repository", "-y", "ppa:ondrej/php")
	runCmd("apt-get", "update", "-y")

	packages := []string{
		"php8.3-fpm", "php8.3-cli", "php8.3-common",
		"php8.3-mysql", "php8.3-xml", "php8.3-curl",
		"php8.3-gd", "php8.3-mbstring", "php8.3-zip",
		"php8.3-bcmath", "php8.3-intl", "php8.3-soap",
		"php8.3-redis", "php8.3-imagick", "php8.3-opcache",
	}
	args := append([]string{"install", "-y"}, packages...)
	return runCmd("apt-get", args...)
}

func (p *Provisioner) installMariaDB() error {
	return runCmd("apt-get", "install", "-y", "mariadb-server", "mariadb-client")
}

func (p *Provisioner) installRedis() error {
	return runCmd("apt-get", "install", "-y", "redis-server")
}

func (p *Provisioner) installWPCLI() error {
	if err := runCmd("sh", "-c", "curl -sO https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar && chmod +x wp-cli.phar && mv wp-cli.phar /usr/local/bin/wp"); err != nil {
		return err
	}
	return nil
}

func (p *Provisioner) installCertbot() error {
	return runCmd("apt-get", "install", "-y", "certbot", "python3-certbot-nginx")
}

func (p *Provisioner) installFail2Ban() error {
	if err := runCmd("apt-get", "install", "-y", "fail2ban"); err != nil {
		return err
	}
	// Auto-configure default jails (nginx + wordpress) after install.
	// Errors are non-fatal — jails can be set up manually via panel later.
	if err := agentfail2ban.SetupDefaultJails(); err != nil {
		slog.Warn("fail2ban default jails setup failed (non-fatal)", "error", err)
	}
	return nil
}

func (p *Provisioner) configureUFW() error {
	cmds := [][]string{
		{"ufw", "default", "deny", "incoming"},
		{"ufw", "default", "allow", "outgoing"},
		{"ufw", "allow", "22/tcp"},
		{"ufw", "allow", "80/tcp"},
		{"ufw", "allow", "443/tcp"},
		{"ufw", "--force", "enable"},
	}
	for _, cmd := range cmds {
		if err := runCmd(cmd[0], cmd[1:]...); err != nil {
			slog.Warn("UFW command failed", "cmd", cmd, "error", err)
		}
	}
	return nil
}

func (p *Provisioner) configureNginx() error {
	// Remove default site
	runCmd("rm", "-f", "/etc/nginx/sites-enabled/default")
	// Test and reload
	if err := runCmd("nginx", "-t"); err != nil {
		return err
	}
	return runCmd("systemctl", "reload", "nginx")
}

func (p *Provisioner) configurePHP() error {
	return runCmd("systemctl", "restart", "php8.3-fpm")
}

func (p *Provisioner) configureMariaDB() error {
	cmds := [][]string{
		{"systemctl", "enable", "mariadb"},
		{"systemctl", "start", "mariadb"},
	}
	for _, cmd := range cmds {
		runCmd(cmd[0], cmd[1:]...)
	}
	return nil
}

func (p *Provisioner) finalize() error {
	services := []string{"nginx", "php8.3-fpm", "mariadb", "redis-server", "fail2ban"}
	for _, svc := range services {
		runCmd("systemctl", "enable", svc)
	}
	return nil
}

// InstallService installs and configures a single named service.
// Valid names: nginx, php, mariadb, redis, certbot, fail2ban, ufw
func (p *Provisioner) InstallService(name string) error {
	// wpcli không cần apt
	if name != "wpcli" {
		runCmd("apt-get", "update", "-y")
	}

	switch name {
	case "nginx":
		if err := p.installNginx(); err != nil {
			return err
		}
		runCmd("systemctl", "enable", "nginx")
		if err := p.configureNginx(); err != nil {
			return err
		}
		return runCmd("systemctl", "restart", "nginx")
	case "php":
		if err := p.installPHP(); err != nil {
			return err
		}
		runCmd("systemctl", "enable", "php8.3-fpm")
		if err := p.configurePHP(); err != nil {
			return err
		}
		return runCmd("systemctl", "restart", "php8.3-fpm")
	case "mariadb":
		if err := p.installMariaDB(); err != nil {
			return err
		}
		runCmd("systemctl", "enable", "mariadb")
		if err := p.configureMariaDB(); err != nil {
			return err
		}
		return runCmd("systemctl", "restart", "mariadb")
	case "redis":
		if err := p.installRedis(); err != nil {
			return err
		}
		runCmd("systemctl", "enable", "redis-server")
		return runCmd("systemctl", "restart", "redis-server")
	case "certbot":
		return p.installCertbot()
	case "fail2ban":
		if err := p.installFail2Ban(); err != nil {
			return err
		}
		runCmd("systemctl", "enable", "fail2ban")
		runCmd("systemctl", "restart", "fail2ban")
		// fail2ban exits (status 255) shortly after start if any jail config is
		// bad, but `systemctl restart` still returns 0. Verify it's actually up so
		// the install task reports the real outcome instead of a false success.
		if out, _ := exec.Command("systemctl", "is-active", "fail2ban").Output(); strings.TrimSpace(string(out)) != "active" {
			jout, _ := exec.Command("journalctl", "-u", "fail2ban", "--no-pager", "-n", "8").CombinedOutput()
			return fmt.Errorf("fail2ban installed but failed to start:\n%s", strings.TrimSpace(string(jout)))
		}
		return nil
	case "ufw":
		// Install UFW trước nếu chưa có
		if _, err := exec.LookPath("ufw"); err != nil {
			out, err := exec.Command("apt-get", "install", "-y", "ufw").CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to install ufw: %s: %w", string(out), err)
			}
		}
		if err := p.configureUFW(); err != nil {
			return err
		}
		return runCmd("ufw", "--force", "enable")
	case "cron":
		out, err := exec.Command("apt-get", "install", "-y", "cron").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install cron: %s: %w", string(out), err)
		}
		runCmd("systemctl", "enable", "cron")
		return runCmd("systemctl", "restart", "cron")
	case "wpcli":
		return p.installWPCLI()
	default:
		return fmt.Errorf("unknown service: %s", name)
	}
}

// waitDPKG waits until the dpkg/apt lock is free (max 5 minutes).
// Ubuntu mới thường chạy unattended-upgrades ngay sau boot.
func waitDPKG() {
	locks := []string{
		"/var/lib/dpkg/lock-frontend",
		"/var/lib/dpkg/lock",
		"/var/cache/apt/archives/lock",
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		locked := false
		for _, f := range locks {
			out, _ := exec.Command("fuser", f).Output()
			if len(strings.TrimSpace(string(out))) > 0 {
				locked = true
				break
			}
		}
		if !locked {
			return
		}
		slog.Info("Waiting for dpkg lock to be released...")
		time.Sleep(5 * time.Second)
	}
	slog.Warn("dpkg lock wait timed out after 5 minutes, proceeding anyway")
}

func runCmd(name string, args ...string) error {
	// Nếu là apt-get hoặc apt, đợi dpkg lock được giải phóng trước
	if name == "apt-get" || name == "apt" || name == "add-apt-repository" {
		waitDPKG()
	}
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Command failed", "cmd", name, "args", args, "output", string(output), "error", err)
		return fmt.Errorf("%s: %s", name, string(output))
	}
	return nil
}
