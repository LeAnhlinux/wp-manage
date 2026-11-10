package services

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
)

// agentMirrorURL is the public release mirror the agent binary is fetched from.
const agentMirrorURL = "https://mirrors.bucloud.com"

type DeployService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewDeployService(db *gorm.DB, cfg *config.Config) *DeployService {
	return &DeployService{db: db, cfg: cfg}
}

// ProgressFunc is called for each deployment step
type ProgressFunc func(progress models.DeployProgress)

// Deploy connects to VPS via SSH and installs the agent binary + systemd service.
// SSH credentials are used only during this call and never persisted.
// Services (Nginx, PHP-FPM, MariaDB, etc.) are installed separately via the panel after deploy.
func (s *DeployService) Deploy(req models.AutoDeployRequest, progress ProgressFunc) error {
	totalSteps := 7

	// Step 1: Validate server exists and get API key
	progress(models.DeployProgress{Step: 1, Total: totalSteps, Message: "Checking server info...", Status: "running"})

	var server models.Server
	if err := s.db.First(&server, req.ServerID).Error; err != nil {
		return fmt.Errorf("server not found: %w", err)
	}

	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	// Step 2: Establish SSH connection
	progress(models.DeployProgress{Step: 2, Total: totalSteps, Message: fmt.Sprintf("Connecting via SSH to %s:%d...", server.IPAddress, sshPort), Status: "running"})

	client, err := s.connectSSH(server.IPAddress, sshPort, req)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Step 3: Check OS compatibility
	progress(models.DeployProgress{Step: 3, Total: totalSteps, Message: "Checking operating system...", Status: "running"})

	osInfo, err := s.runCommand(client, "cat /etc/os-release 2>/dev/null | grep -E '^(ID|VERSION_ID)=' | head -2")
	if err != nil {
		return fmt.Errorf("cannot check OS: %w", err)
	}
	if !strings.Contains(strings.ToLower(osInfo), "ubuntu") {
		return fmt.Errorf("unsupported OS — Ubuntu 20.04+ required")
	}

	// Detect architecture
	arch, err := s.runCommand(client, "uname -m")
	if err != nil {
		return fmt.Errorf("cannot detect CPU architecture: %w", err)
	}
	arch = strings.TrimSpace(arch)
	goarch := "amd64"
	if arch == "aarch64" || arch == "arm64" {
		goarch = "arm64"
	}

	// Step 4: Upload agent binary
	progress(models.DeployProgress{Step: 4, Total: totalSteps, Message: "Uploading agent binary...", Status: "running"})

	// The binary is written next to the target and renamed into place: writing
	// straight over a running agent fails with "Text file busy" on a re-deploy.
	const agentPath = "/usr/local/bin/wp-manage-agent"
	const agentTmp = agentPath + ".new"
	binaryName := fmt.Sprintf("wp-manage-agent-linux-%s", goarch)

	// Prefer the current release from the mirror. The copy next to the API
	// binary dates from when the panel was installed and is not refreshed by
	// panel updates, so it is only the fallback for servers without outbound
	// access to the mirror.
	dlCmd := fmt.Sprintf("curl -fsSL --max-time 120 %s/%s -o %s && test -s %s",
		agentMirrorURL, binaryName, agentTmp, agentTmp)
	if _, err := s.runCommand(client, dlCmd); err != nil {
		localBinary := s.findAgentBinary(binaryName)
		if localBinary == "" {
			return fmt.Errorf("agent binary could not be downloaded from the mirror and no local copy was found: %s", binaryName)
		}
		if err := s.uploadFile(client, localBinary, agentTmp); err != nil {
			return fmt.Errorf("failed to upload agent binary: %w", err)
		}
	}

	if _, err := s.runCommand(client, fmt.Sprintf("chmod +x %s && mv -f %s %s", agentTmp, agentTmp, agentPath)); err != nil {
		return fmt.Errorf("failed to install agent binary: %w", err)
	}

	// Step 5: Create systemd service
	progress(models.DeployProgress{Step: 5, Total: totalSteps, Message: "Configuring systemd service...", Status: "running"})

	apiURL := fmt.Sprintf("https://%s", s.cfg.Panel.Domain)
	serviceContent := fmt.Sprintf(`[Unit]
Description=WP Manage Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/wp-manage-agent --api-url=%s --api-key=%s
Restart=always
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=3
StandardOutput=journal
StandardError=journal
SyslogIdentifier=wp-manage-agent

[Install]
WantedBy=multi-user.target
`, apiURL, server.AgentAPIKey)


	if err := s.writeRemoteFile(client, "/etc/systemd/system/wp-manage-agent.service", serviceContent); err != nil {
		return fmt.Errorf("failed to create systemd service: %w", err)
	}

	// Step 6: Start agent service
	progress(models.DeployProgress{Step: 6, Total: totalSteps, Message: "Starting agent service...", Status: "running"})

	commands := []string{
		"systemctl daemon-reload",
		"systemctl enable wp-manage-agent",
		"systemctl restart wp-manage-agent",
	}
	for _, cmd := range commands {
		if _, err := s.runCommand(client, cmd); err != nil {
			return fmt.Errorf("command '%s' failed: %w", cmd, err)
		}
	}

	// Wait for agent to start and verify
	time.Sleep(3 * time.Second)

	status, err := s.runCommand(client, "systemctl is-active wp-manage-agent")
	if err != nil || strings.TrimSpace(status) != "active" {
		// Get logs for debugging
		logs, _ := s.runCommand(client, "journalctl -u wp-manage-agent --no-pager -n 20")
		return fmt.Errorf("agent failed to start. Logs:\n%s", logs)
	}

	// Step 7: Finalize
	progress(models.DeployProgress{Step: 7, Total: totalSteps, Message: "Finalizing...", Status: "running"})

	osVersion, _ := s.runCommand(client, `grep PRETTY_NAME /etc/os-release | cut -d'"' -f2`)
	osVersion = strings.TrimSpace(osVersion)

	s.db.Model(&models.Server{}).Where("id = ?", server.ID).Updates(map[string]interface{}{
		"status": "active",
		"os":     osVersion,
	})

	// Auto-queue install tasks for all services once the agent is ready.
	// Order matters: ufw first, php last (it takes the longest).
	autoInstallServices := []string{"ufw", "nginx", "mariadb", "redis", "certbot", "fail2ban", "cron", "wpcli", "php"}
	for _, svc := range autoInstallServices {
		task := models.TaskQueue{
			ServerID: server.ID,
			TaskType: "install_service",
			Data:     fmt.Sprintf(`{"service":"%s"}`, svc),
			Status:   "pending",
		}
		if err := s.db.Create(&task).Error; err != nil {
			slog.Warn("Failed to queue auto-install task", "service", svc, "error", err)
		}
	}
	slog.Info("Queued auto-install tasks", "server_id", server.ID, "services", autoInstallServices)

	progress(models.DeployProgress{Step: 7, Total: totalSteps, Message: "Agent deployed successfully! Auto-installing all services (Nginx, PHP, MariaDB, Redis...)...", Status: "success"})

	slog.Info("Auto deploy completed", "server_id", server.ID, "ip", server.IPAddress)
	return nil
}

func (s *DeployService) connectSSH(host string, port int, req models.AutoDeployRequest) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	switch req.AuthMethod {
	case "password":
		if req.Password == "" {
			return nil, fmt.Errorf("password must not be empty")
		}
		authMethods = append(authMethods, ssh.Password(req.Password))
	case "key":
		if req.PrivateKey == "" {
			return nil, fmt.Errorf("SSH private key must not be empty")
		}
		signer, err := ssh.ParsePrivateKey([]byte(req.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("invalid SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("invalid auth method: %s", req.AuthMethod)
	}

	config := &ssh.ClientConfig{
		User:            req.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("timeout - port %d may be blocked by a firewall", port)
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("authentication failed - check your credentials")
		}
		return nil, err
	}

	return client, nil
}

func (s *DeployService) runCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return stdout.String(), fmt.Errorf("%s: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// uploadFile streams a local file to the remote host by piping it into
// `cat > <path>` over an SSH session.
//
// We intentionally do NOT use the legacy `scp -t` sink protocol: OpenSSH 9.0+
// (Ubuntu 22.10+, all of 24.04) removed the old SCP transfer protocol, so
// `scp -t` now fails with "protocol error: unexpected <newline>" and the
// session exits with status 1. Piping through `cat` works on every OpenSSH
// version and needs no extra dependency.
func (s *DeployService) uploadFile(client *ssh.Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %w", localPath, err)
	}
	defer f.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = f
	var stderr bytes.Buffer
	session.Stderr = &stderr

	// shell-quote the remote path to be safe
	quoted := "'" + strings.ReplaceAll(remotePath, "'", `'\''`) + "'"
	if err := session.Run("cat > " + quoted); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *DeployService) writeRemoteFile(client *ssh.Client, path, content string) error {
	// Use heredoc via cat to write file content
	cmd := fmt.Sprintf("cat > %s << 'WPMANAGE_EOF'\n%sWPMANAGE_EOF", path, content)
	_, err := s.runCommand(client, cmd)
	return err
}

func (s *DeployService) findAgentBinary(name string) string {
	// Search with full name (wp-manage-agent-linux-amd64) and short name (wp-manage-agent)
	names := []string{name, "wp-manage-agent"}

	var searchPaths []string
	for _, n := range names {
		searchPaths = append(searchPaths,
			filepath.Join(".", n),
			filepath.Join(".", "bin", n),
			filepath.Join("..", n),
		)
	}

	// Also check next to the running API binary (e.g. /opt/wp-manage/bin/)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, n := range names {
			searchPaths = append(searchPaths, filepath.Join(dir, n))
		}
	}

	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return ""
}
