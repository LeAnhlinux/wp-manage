package services

import (
	"bytes"
	"fmt"
	"io"
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
func (s *DeployService) Deploy(req models.AutoDeployRequest, progress ProgressFunc) error {
	totalSteps := 7

	// Step 1: Validate server exists and get API key
	progress(models.DeployProgress{Step: 1, Total: totalSteps, Message: "Kiểm tra thông tin server...", Status: "running"})

	var server models.Server
	if err := s.db.First(&server, req.ServerID).Error; err != nil {
		return fmt.Errorf("server không tồn tại: %w", err)
	}

	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	// Step 2: Establish SSH connection
	progress(models.DeployProgress{Step: 2, Total: totalSteps, Message: fmt.Sprintf("Kết nối SSH tới %s:%d...", server.IPAddress, sshPort), Status: "running"})

	client, err := s.connectSSH(server.IPAddress, sshPort, req)
	if err != nil {
		return fmt.Errorf("kết nối SSH thất bại: %w", err)
	}
	defer client.Close()

	// Step 3: Check OS compatibility
	progress(models.DeployProgress{Step: 3, Total: totalSteps, Message: "Kiểm tra hệ điều hành...", Status: "running"})

	osInfo, err := s.runCommand(client, "cat /etc/os-release 2>/dev/null | grep -E '^(ID|VERSION_ID)=' | head -2")
	if err != nil {
		return fmt.Errorf("không thể kiểm tra OS: %w", err)
	}
	if !strings.Contains(strings.ToLower(osInfo), "ubuntu") {
		return fmt.Errorf("OS không được hỗ trợ. Yêu cầu Ubuntu 20.04+")
	}

	// Detect architecture
	arch, err := s.runCommand(client, "uname -m")
	if err != nil {
		return fmt.Errorf("không thể xác định kiến trúc CPU: %w", err)
	}
	arch = strings.TrimSpace(arch)
	goarch := "amd64"
	if arch == "aarch64" || arch == "arm64" {
		goarch = "arm64"
	}

	// Step 4: Upload agent binary
	progress(models.DeployProgress{Step: 4, Total: totalSteps, Message: "Upload agent binary...", Status: "running"})

	binaryName := fmt.Sprintf("wp-manage-agent-linux-%s", goarch)
	localBinary := s.findAgentBinary(binaryName)
	if localBinary == "" {
		return fmt.Errorf("không tìm thấy agent binary: %s. Chạy 'make build-linux' trước", binaryName)
	}

	if err := s.uploadFile(client, localBinary, "/usr/local/bin/wp-manage-agent"); err != nil {
		return fmt.Errorf("upload agent binary thất bại: %w", err)
	}

	if _, err := s.runCommand(client, "chmod +x /usr/local/bin/wp-manage-agent"); err != nil {
		return fmt.Errorf("chmod thất bại: %w", err)
	}

	// Step 5: Create systemd service
	progress(models.DeployProgress{Step: 5, Total: totalSteps, Message: "Cấu hình systemd service...", Status: "running"})

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
		return fmt.Errorf("tạo systemd service thất bại: %w", err)
	}

	// Step 6: Start agent service
	progress(models.DeployProgress{Step: 6, Total: totalSteps, Message: "Khởi động agent service...", Status: "running"})

	commands := []string{
		"systemctl daemon-reload",
		"systemctl enable wp-manage-agent",
		"systemctl restart wp-manage-agent",
	}
	for _, cmd := range commands {
		if _, err := s.runCommand(client, cmd); err != nil {
			return fmt.Errorf("lệnh '%s' thất bại: %w", cmd, err)
		}
	}

	// Wait for agent to start and verify
	time.Sleep(3 * time.Second)

	status, err := s.runCommand(client, "systemctl is-active wp-manage-agent")
	if err != nil || strings.TrimSpace(status) != "active" {
		// Get logs for debugging
		logs, _ := s.runCommand(client, "journalctl -u wp-manage-agent --no-pager -n 20")
		return fmt.Errorf("agent không khởi động được. Logs:\n%s", logs)
	}

	// Step 7: Update server status
	progress(models.DeployProgress{Step: 7, Total: totalSteps, Message: "Hoàn tất deploy agent!", Status: "running"})

	// Detect OS info from remote
	osVersion, _ := s.runCommand(client, `grep PRETTY_NAME /etc/os-release | cut -d'"' -f2`)
	osVersion = strings.TrimSpace(osVersion)

	s.db.Model(&models.Server{}).Where("id = ?", server.ID).Updates(map[string]interface{}{
		"status": "active",
		"os":     osVersion,
	})

	progress(models.DeployProgress{Step: 7, Total: totalSteps, Message: "Deploy agent thành công!", Status: "success"})

	slog.Info("Auto deploy completed", "server_id", server.ID, "ip", server.IPAddress)
	return nil
}

func (s *DeployService) connectSSH(host string, port int, req models.AutoDeployRequest) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	switch req.AuthMethod {
	case "password":
		if req.Password == "" {
			return nil, fmt.Errorf("password không được để trống")
		}
		authMethods = append(authMethods, ssh.Password(req.Password))
	case "key":
		if req.PrivateKey == "" {
			return nil, fmt.Errorf("SSH private key không được để trống")
		}
		signer, err := ssh.ParsePrivateKey([]byte(req.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("SSH key không hợp lệ: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("auth method không hợp lệ: %s", req.AuthMethod)
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
			return nil, fmt.Errorf("timeout - port %d có thể bị firewall chặn", port)
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("xác thực thất bại - kiểm tra lại thông tin đăng nhập")
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

func (s *DeployService) uploadFile(client *ssh.Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("không thể mở file %s: %w", localPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C0755 %d %s\n", stat.Size(), filepath.Base(remotePath))
		io.Copy(w, f)
		fmt.Fprint(w, "\x00")
	}()

	dir := filepath.Dir(remotePath)
	return session.Run(fmt.Sprintf("scp -t %s", dir))
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
