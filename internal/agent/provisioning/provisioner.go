package provisioning

import (
	"fmt"
	"log/slog"
	"os/exec"
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
		"php8.2-fpm", "php8.2-cli", "php8.2-common",
		"php8.2-mysql", "php8.2-xml", "php8.2-curl",
		"php8.2-gd", "php8.2-mbstring", "php8.2-zip",
		"php8.2-bcmath", "php8.2-intl", "php8.2-soap",
		"php8.2-redis", "php8.2-imagick", "php8.2-opcache",
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
	return runCmd("apt-get", "install", "-y", "fail2ban")
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
	return runCmd("systemctl", "restart", "php8.2-fpm")
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
	services := []string{"nginx", "php8.2-fpm", "mariadb", "redis-server", "fail2ban"}
	for _, svc := range services {
		runCmd("systemctl", "enable", svc)
	}
	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Command failed", "cmd", name, "args", args, "output", string(output), "error", err)
		return fmt.Errorf("%s: %s", name, string(output))
	}
	return nil
}
