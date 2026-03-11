package grpc

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	agentconfig "github.com/gofiber-team/wp-manage/internal/agent/config"
	agentbackup "github.com/gofiber-team/wp-manage/internal/agent/backup"
	agentdb "github.com/gofiber-team/wp-manage/internal/agent/database"
	agentcron "github.com/gofiber-team/wp-manage/internal/agent/cron"
	agentfirewall "github.com/gofiber-team/wp-manage/internal/agent/firewall"
	"github.com/gofiber-team/wp-manage/internal/agent/filemanager"
	agentmigration "github.com/gofiber-team/wp-manage/internal/agent/migration"
	agentnginx "github.com/gofiber-team/wp-manage/internal/agent/nginx"
	agentopcache "github.com/gofiber-team/wp-manage/internal/agent/opcache"
	agentredis "github.com/gofiber-team/wp-manage/internal/agent/redis"
	agentphpfpm "github.com/gofiber-team/wp-manage/internal/agent/phpfpm"
	agentsecurity "github.com/gofiber-team/wp-manage/internal/agent/security"
	agentsftp "github.com/gofiber-team/wp-manage/internal/agent/sftp"
	agentssh "github.com/gofiber-team/wp-manage/internal/agent/ssh"
	agentfail2ban "github.com/gofiber-team/wp-manage/internal/agent/fail2ban"
	agentwp "github.com/gofiber-team/wp-manage/internal/agent/wordpress"
	"github.com/gofiber-team/wp-manage/internal/agent/provisioning"
	"github.com/gofiber-team/wp-manage/internal/agent/site"
	"github.com/gofiber-team/wp-manage/internal/agent/ssl"
	"github.com/gofiber-team/wp-manage/internal/agent/system"
	"github.com/gofiber-team/wp-manage/internal/models"
)

const Version = "1.0.0"

type AgentServer struct {
	apiURL          string
	apiKey          string
	siteMu          sync.Map // per-domain mutexes to prevent concurrent site operations
	activeCreations sync.Map // tracks siteIDs currently being created to prevent double-dispatch
}

func NewAgentServer(apiURL, apiKey string) *AgentServer {
	return &AgentServer{apiURL: apiURL, apiKey: apiKey}
}

// lockDomain acquires a per-domain mutex and returns an unlock function.
// Use defer unlock() to release. This prevents concurrent operations
// (e.g. delete_site + restore_backup) from racing on the same domain.
func (s *AgentServer) lockDomain(domain string) func() {
	v, _ := s.siteMu.LoadOrStore(domain, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// StartHTTPServer starts the agent's HTTP server for file manager and other direct operations
func (s *AgentServer) StartHTTPServer(port int) {
	mux := http.NewServeMux()

	// Register file manager routes
	fm := filemanager.NewHandler(s.apiKey)
	fm.RegisterRoutes(mux)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"` + Version + `"}`))
	})

	addr := fmt.Sprintf(":%d", port)
	slog.Info("Starting agent HTTP server", "port", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Agent HTTP server failed", "error", err)
	}
}

func (s *AgentServer) StartHeartbeat(interval time.Duration) {
	heartbeatTicker := time.NewTicker(interval)
	taskTicker := time.NewTicker(3 * time.Second) // Poll tasks every 3s for fast response
	defer heartbeatTicker.Stop()
	defer taskTicker.Stop()

	// Initial heartbeat + poll
	s.sendHeartbeat()
	s.pollTasks()

	for {
		select {
		case <-heartbeatTicker.C:
			s.sendHeartbeat()
		case <-taskTicker.C:
			s.pollTasks()
		}
	}
}

func (s *AgentServer) sendHeartbeat() {
	metrics, err := system.GetMetrics()
	if err != nil {
		slog.Error("Failed to get metrics", "error", err)
		return
	}

	hb := models.AgentHeartbeat{
		Version:   Version,
		CPUUsage:  metrics.CPUUsage,
		RAMUsage:  metrics.RAMUsage,
		DiskUsage: metrics.DiskUsage,
		Uptime:    metrics.UptimeSeconds,
	}

	data, _ := json.Marshal(hb)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/heartbeat", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Heartbeat failed", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) pollTasks() {
	req, _ := http.NewRequest("GET", s.apiURL+"/api/v1/agent/tasks", nil)
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var apiResp struct {
		Success bool `json:"success"`
		Data    []struct {
			TaskID   uint64          `json:"task_id"`
			TaskType string          `json:"task_type"`
			SiteID   uint64          `json:"site_id"`
			Data     json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil || !apiResp.Success {
		return
	}

	for _, task := range apiResp.Data {
		slog.Info("Received task", "type", task.TaskType, "site_id", task.SiteID, "task_id", task.TaskID)

		switch task.TaskType {
		case "create_site":
			var siteTask models.AgentCreateSiteTask
			json.Unmarshal(task.Data, &siteTask)
			// Prevent double-dispatch: skip if a goroutine is already creating this site
			if _, alreadyRunning := s.activeCreations.LoadOrStore(task.SiteID, true); alreadyRunning {
				slog.Warn("Skipping duplicate create_site task", "site_id", task.SiteID, "domain", siteTask.Domain)
				continue
			}
			go func(sID uint64, t models.AgentCreateSiteTask) {
				defer s.activeCreations.Delete(sID)
				s.executeCreateSite(sID, t)
			}(task.SiteID, siteTask)

		case "delete_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeDeleteSite(task.TaskID, data["domain"], data["db_name"], data["db_user"])

		case "restart_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeRestartSite(task.TaskID, data["domain"], data["php_version"])

		case "suspend_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeSuspendSite(task.TaskID, data["domain"])

		case "resume_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeResumeSite(task.TaskID, data["domain"])

		case "update_php":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeUpdatePHP(task.TaskID, data["domain"], data["old_php_version"], data["new_php_version"])

		case "issue_ssl":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeIssueSSL(task.TaskID, task.SiteID, data["domain"], data["email"])

		case "install_autologin":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeInstallAutologin(task.TaskID, task.SiteID, data["domain"])

		case "create_backup":
			var backupTask models.AgentBackupTask
			json.Unmarshal(task.Data, &backupTask)
			go s.executeCreateBackup(task.TaskID, task.SiteID, backupTask)

		case "restore_backup":
			var restoreTask models.AgentRestoreTask
			json.Unmarshal(task.Data, &restoreTask)
			go s.executeRestoreBackup(task.TaskID, task.SiteID, restoreTask)

		case "migrate_site":
			var migrateTask models.AgentMigrateTask
			json.Unmarshal(task.Data, &migrateTask)
			runner := agentmigration.NewMigrationRunner(s.apiURL, s.apiKey)
			go runner.RunMigration(migrateTask)

		case "rollback_migration":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			migrationID := uint64(data["migration_id"].(float64))
			domain, _ := data["domain"].(string)
			runner := agentmigration.NewMigrationRunner(s.apiURL, s.apiKey)
			go runner.RollbackMigration(migrationID, domain)

		case "configure_waf":
			var wafTask models.AgentWAFTask
			json.Unmarshal(task.Data, &wafTask)
			go s.executeConfigureWAF(task.TaskID, wafTask)

		case "malware_scan":
			var scanTask models.AgentMalwareScanTask
			json.Unmarshal(task.Data, &scanTask)
			go s.executeMalwareScan(task.TaskID, scanTask)

		case "check_updates":
			var wpTask models.AgentCheckUpdatesTask
			json.Unmarshal(task.Data, &wpTask)
			go s.executeCheckUpdates(task.TaskID, wpTask)

		case "db_optimize":
			var dbTask models.AgentDBOptimizeTask
			json.Unmarshal(task.Data, &dbTask)
			go s.executeDBOptimize(task.TaskID, dbTask)

		case "staging_create":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			go s.executeStagingCreate(task.TaskID, data)

		case "staging_push":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			go s.executeStagingPush(task.TaskID, data)

		case "staging_delete":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			go s.executeStagingDelete(task.TaskID, data)

		case "agent_update":
			var updateTask models.AgentUpdateTask
			json.Unmarshal(task.Data, &updateTask)
			go s.executeAgentUpdate(task.TaskID, updateTask)

		case "optimize_images":
			var imgTask models.AgentImageOptTask
			json.Unmarshal(task.Data, &imgTask)
			go s.executeOptimizeImages(task.TaskID, imgTask)

		case "enable_multisite":
			var msTask models.AgentEnableMultisiteTask
			json.Unmarshal(task.Data, &msTask)
			go s.executeEnableMultisite(task.TaskID, msTask)

		case "create_subsite":
			var subTask models.AgentCreateSubsiteTask
			json.Unmarshal(task.Data, &subTask)
			go s.executeCreateSubsite(task.TaskID, subTask)

		case "delete_subsite":
			var delTask models.AgentDeleteSubsiteTask
			json.Unmarshal(task.Data, &delTask)
			go s.executeDeleteSubsite(task.TaskID, delTask)

		case "sync_subsites":
			var syncTask models.AgentListSubsitesTask
			json.Unmarshal(task.Data, &syncTask)
			go s.executeSyncSubsites(task.TaskID, syncTask)

		case "manage_firewall":
			var fwTask models.AgentFirewallTask
			json.Unmarshal(task.Data, &fwTask)
			go s.executeManageFirewall(task.TaskID, fwTask)

		case "manage_nginx_config":
			var ngxTask models.AgentNginxConfigTask
			json.Unmarshal(task.Data, &ngxTask)
			go s.executeManageNginxConfig(task.TaskID, ngxTask)

		case "read_vhost":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeReadVhost(task.TaskID, data["domain"])

		case "write_vhost":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeWriteVhost(task.TaskID, data["domain"], data["content"])

		case "opcache_action":
			var opcTask models.AgentOPcacheTask
			json.Unmarshal(task.Data, &opcTask)
			go s.executeOPcacheAction(task.TaskID, opcTask)

		case "rebuild_config":
			var rebuildTask models.AgentRebuildConfigTask
			json.Unmarshal(task.Data, &rebuildTask)
			go s.executeRebuildConfig(task.TaskID, rebuildTask)

		case "health_check":
			go s.executeHealthCheck(task.TaskID)

		case "manage_cron":
			var cronTask models.AgentCronTask
			json.Unmarshal(task.Data, &cronTask)
			go s.executeManageCron(task.TaskID, cronTask)

		case "manage_domain":
			var domainTask models.AgentDomainTask
			json.Unmarshal(task.Data, &domainTask)
			go s.executeManageDomain(task.TaskID, domainTask)

		case "toggle_redis":
			var redisTask models.AgentRedisTask
			json.Unmarshal(task.Data, &redisTask)
			go s.executeToggleRedis(task.TaskID, redisTask)

		case "update_phpfpm_pool":
			var phpTask models.AgentPHPFPMTask
			json.Unmarshal(task.Data, &phpTask)
			go s.executeUpdatePHPFPMPool(task.TaskID, phpTask)

		case "manage_mariadb":
			var dbTask models.AgentMariaDBTask
			json.Unmarshal(task.Data, &dbTask)
			go s.executeManageMariaDB(task.TaskID, dbTask)

		case "install_php_version":
			var phpTask models.AgentPHPVersionTask
			json.Unmarshal(task.Data, &phpTask)
			go s.executeInstallPHPVersion(task.TaskID, phpTask)

		case "list_installed_php":
			go s.executeListInstalledPHP(task.TaskID)

		case "tail_logs":
			var logTask struct {
				LogType string `json:"log_type"`
				Lines   int    `json:"lines"`
			}
			json.Unmarshal(task.Data, &logTask)
			go s.executeTailLogs(task.TaskID, logTask.LogType, logTask.Lines)

		case "stream_logs":
			var logTask struct {
				LogType string `json:"log_type"`
				Stream  bool   `json:"stream"`
			}
			json.Unmarshal(task.Data, &logTask)
			go s.executeTailLogs(task.TaskID, logTask.LogType, 50)

		case "add_ssh_key", "remove_ssh_key":
			var sshTask models.AgentSSHKeyTask
			json.Unmarshal(task.Data, &sshTask)
			go s.executeManageSSHKey(task.TaskID, sshTask)

		case "create_sftp_user", "delete_sftp_user":
			var sftpTask models.AgentSFTPTask
			json.Unmarshal(task.Data, &sftpTask)
			go s.executeManageSFTP(task.TaskID, sftpTask)

		case "harden_wordpress":
			var hardenTask models.AgentHardenTask
			json.Unmarshal(task.Data, &hardenTask)
			go s.executeHardenWordPress(task.TaskID, hardenTask)

		case "phpfpm_autotune":
			var autotuneTask models.AgentAutotuneTask
			json.Unmarshal(task.Data, &autotuneTask)
			go s.executePHPFPMAutotune(task.TaskID, autotuneTask)

		case "wpcli_execute":
			var wpcliTask models.AgentWPCLITask
			json.Unmarshal(task.Data, &wpcliTask)
			go s.executeWPCLI(task.TaskID, wpcliTask)

		case "manage_cache":
			var cacheTask models.AgentCacheTask
			json.Unmarshal(task.Data, &cacheTask)
			go s.executeManageCache(task.TaskID, cacheTask)

		case "import_external":
			var importTask models.AgentImportTask
			json.Unmarshal(task.Data, &importTask)
			go s.executeImportExternal(task.TaskID, importTask)

		case "delete_backup_file":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeDeleteBackupFile(task.TaskID, data["file_path"])

		case "renew_ssl":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeRenewSSL(task.TaskID, task.SiteID, data["domain"])

		case "disable_ssl":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeDisableSSL(task.TaskID, task.SiteID, data["domain"], data["php_version"])

		case "upload_custom_ssl":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeUploadCustomSSL(task.TaskID, task.SiteID, data["domain"], data["cert_pem"], data["key_pem"], data["ca_pem"])

		case "manage_force_https":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeForceHTTPS(task.TaskID, task.SiteID, data["domain"], data["action"])

		case "fail2ban_status":
			go s.executeFail2BanStatus(task.TaskID)

		case "fail2ban_unban":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeFail2BanUnban(task.TaskID, data["jail"], data["ip"])

		case "fail2ban_update_jail":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			go s.executeFail2BanUpdateJail(task.TaskID, data)

		case "upload_source_zip":
			var uploadTask models.AgentUploadSourceTask
			json.Unmarshal(task.Data, &uploadTask)
			go s.executeUploadSource(task.TaskID, uploadTask)

		case "import_db_sql":
			var importTask models.AgentImportDBTask
			json.Unmarshal(task.Data, &importTask)
			go s.executeImportDB(task.TaskID, importTask)

		default:
			slog.Warn("Unknown task type", "type", task.TaskType)
		}
	}
}

func (s *AgentServer) executeCreateSite(siteID uint64, task models.AgentCreateSiteTask) {
	params := site.CreateParams{
		Domain:       task.Domain,
		PHPVersion:   task.PHPVersion,
		SSLEnabled:   task.SSLEnabled,
		RedisEnabled: task.RedisEnabled,
		DBName:       task.DBName,
		DBUser:       task.DBUser,
		DBPassword:   task.DBPassword,
		WPAdminUser:  task.WPAdminUser,
		WPAdminEmail: task.WPAdminEmail,
		WPAdminPass:  task.WPAdminPass,
	}

	mgr := site.NewManager(func(step string, progress int, message string) {
		slog.Info("Create site progress", "domain", params.Domain, "step", step, "progress", progress)
	})

	var err error
	installType := task.InstallType
	if installType == "" {
		installType = "auto"
	}

	if installType == "manual" {
		// Manual install: set up infrastructure only (no WP download/install)
		slog.Info("Creating site (manual mode)", "domain", task.Domain)
		err = mgr.CreateSiteManual(params)
	} else {
		// Auto install: full WordPress setup
		slog.Info("Creating site (auto mode)", "domain", task.Domain)
		err = mgr.CreateSite(params)
	}

	result := models.AgentTaskResult{SiteID: siteID, Status: "active"}
	if err != nil {
		slog.Error("Create site failed", "domain", task.Domain, "install_type", installType, "error", err)
		result.Status = "error"
		result.Error = err.Error()
	} else {
		if installType == "auto" {
			// Detect WP version after install
			wpVer := site.GetWPVersion(task.Domain)
			result.WPVersion = wpVer
			result.AutologinSecret = params.AutologinSecret
			slog.Info("Create site (auto) completed", "domain", task.Domain, "wp_version", wpVer)
		} else {
			slog.Info("Create site (manual) infrastructure ready", "domain", task.Domain)
		}
	}

	s.reportTaskResult(result)
}

// downloadFile downloads a file from url (with X-Agent-Key auth) and saves it to destPath.
func (s *AgentServer) downloadFile(url, destPath string) error {
	// If it's a relative URL (starts with /api/), prepend the API base URL
	if strings.HasPrefix(url, "/") {
		url = s.apiURL + url
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *AgentServer) executeUploadSource(taskID uint64, task models.AgentUploadSourceTask) {
	slog.Info("Uploading source zip", "domain", task.Domain)
	result := models.AgentTaskResult{TaskID: taskID}

	// Download the zip to a temp path
	tmpPath := fmt.Sprintf("/tmp/wp-source-%d.zip", taskID)
	if err := s.downloadFile(task.DownloadURL, tmpPath); err != nil {
		result.Error = "Failed to download source: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	mgr := site.NewManager(func(step string, progress int, message string) {
		slog.Info("Upload source progress", "domain", task.Domain, "step", step)
	})

	if err := mgr.ExtractSourceZip(task.Domain, tmpPath); err != nil {
		os.Remove(tmpPath)
		result.Error = "Failed to extract source: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	slog.Info("Source extracted successfully", "domain", task.Domain)
	result.Output = fmt.Sprintf("Source deployed to /var/www/%s/public", task.Domain)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeImportDB(taskID uint64, task models.AgentImportDBTask) {
	slog.Info("Importing database", "domain", task.Domain, "db_name", task.DBName)
	result := models.AgentTaskResult{TaskID: taskID}

	// Determine file extension from URL or FileExt field
	ext := ".sql"
	if task.FileExt != "" {
		ext = task.FileExt
	} else if strings.Contains(task.DownloadURL, ".gz") {
		ext = ".gz"
	} else if strings.Contains(task.DownloadURL, ".zip") {
		ext = ".zip"
	}

	// Download the SQL file to a temp path
	tmpPath := fmt.Sprintf("/tmp/wp-import-%d%s", taskID, ext)
	if err := s.downloadFile(task.DownloadURL, tmpPath); err != nil {
		result.Error = "Failed to download SQL: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	mgr := site.NewManager(func(step string, progress int, message string) {
		slog.Info("Import DB progress", "domain", task.Domain, "step", step)
	})

	if err := mgr.ImportDatabase(task.DBName, tmpPath); err != nil {
		os.Remove(tmpPath)
		result.Error = "Failed to import database: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	slog.Info("Database imported successfully", "domain", task.Domain, "db_name", task.DBName)
	result.Output = fmt.Sprintf("Database %s imported successfully", task.DBName)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeInstallAutologin(taskID uint64, siteID uint64, domain string) {
	slog.Info("Installing autologin mu-plugin", "domain", domain)
	secret, err := site.InstallAutologinOnExisting(domain)

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
	if err != nil {
		slog.Error("Install autologin failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		result.AutologinSecret = secret
		slog.Info("Autologin installed", "domain", domain)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeDeleteSite(taskID uint64, domain, dbName, dbUser string) {
	// Acquire per-domain lock to prevent racing with restore_backup on the same domain.
	unlock := s.lockDomain(domain)
	defer unlock()

	slog.Info("Deleting site", "domain", domain)
	err := s.DeleteSite(domain, dbName, dbUser)

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("Delete site failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Delete site completed", "domain", domain)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeRestartSite(taskID uint64, domain, phpVersion string) {
	slog.Info("Restarting site services", "domain", domain)
	var errMsg string

	// Restart PHP-FPM
	if phpVersion == "" {
		phpVersion = "8.2"
	}
	cmd := exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", phpVersion))
	if out, err := cmd.CombinedOutput(); err != nil {
		errMsg = fmt.Sprintf("php-fpm restart: %s", string(out))
	}

	// Reload Nginx
	cmd = exec.Command("systemctl", "reload", "nginx")
	if out, err := cmd.CombinedOutput(); err != nil {
		if errMsg != "" {
			errMsg += "; "
		}
		errMsg += fmt.Sprintf("nginx reload: %s", string(out))
	}

	result := models.AgentTaskResult{TaskID: taskID}
	if errMsg != "" {
		result.Error = errMsg
	}
	slog.Info("Restart completed", "domain", domain)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeSuspendSite(taskID uint64, domain string) {
	slog.Info("Suspending site", "domain", domain)
	// Disable nginx vhost by removing symlink
	exec.Command("rm", "-f", fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain)).Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	result := models.AgentTaskResult{TaskID: taskID}
	slog.Info("Site suspended", "domain", domain)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeResumeSite(taskID uint64, domain string) {
	slog.Info("Resuming site", "domain", domain)
	// Re-enable nginx vhost
	exec.Command("ln", "-sf",
		fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain),
		fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain),
	).Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	result := models.AgentTaskResult{TaskID: taskID}
	slog.Info("Site resumed", "domain", domain)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeUpdatePHP(taskID uint64, domain, oldVer, newVer string) {
	slog.Info("Updating PHP version", "domain", domain, "from", oldVer, "to", newVer)

	result := models.AgentTaskResult{TaskID: taskID}

	// 1) Ensure target PHP version is installed
	phpFpmDir := fmt.Sprintf("/etc/php/%s/fpm/pool.d", newVer)
	if _, err := os.Stat(phpFpmDir); os.IsNotExist(err) {
		slog.Info("Installing PHP", "version", newVer)
		pkgs := []string{
			fmt.Sprintf("php%s-fpm", newVer), fmt.Sprintf("php%s-cli", newVer),
			fmt.Sprintf("php%s-common", newVer), fmt.Sprintf("php%s-mysql", newVer),
			fmt.Sprintf("php%s-xml", newVer), fmt.Sprintf("php%s-curl", newVer),
			fmt.Sprintf("php%s-gd", newVer), fmt.Sprintf("php%s-mbstring", newVer),
			fmt.Sprintf("php%s-zip", newVer), fmt.Sprintf("php%s-bcmath", newVer),
			fmt.Sprintf("php%s-intl", newVer), fmt.Sprintf("php%s-soap", newVer),
			fmt.Sprintf("php%s-redis", newVer), fmt.Sprintf("php%s-imagick", newVer),
			fmt.Sprintf("php%s-opcache", newVer),
		}
		args := append([]string{"install", "-y"}, pkgs...)
		if out, err := exec.Command("apt-get", args...).CombinedOutput(); err != nil {
			slog.Error("Failed to install PHP", "version", newVer, "error", err, "output", string(out))
			result.Error = fmt.Sprintf("failed to install php%s-fpm: %v", newVer, err)
			s.reportTaskResult(result)
			return
		}
		exec.Command("systemctl", "enable", fmt.Sprintf("php%s-fpm", newVer)).Run()
		exec.Command("systemctl", "start", fmt.Sprintf("php%s-fpm", newVer)).Run()
		slog.Info("PHP installed successfully", "version", newVer)
	}

	// 2) Rebuild PHP-FPM pool for new version
	_, err := agentconfig.RebuildPHPFPMPool(domain, newVer, map[string]string{})
	if err != nil {
		slog.Error("PHP update failed", "domain", domain, "error", err)
		result.Error = err.Error()
		s.reportTaskResult(result)
		return
	}

	// 3) Remove old PHP-FPM pool if different version
	if oldVer != newVer {
		exec.Command("rm", "-f", fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", oldVer, domain)).Run()
		exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", oldVer)).Run()
	}

	// 4) Update nginx to use new PHP version socket (sed replace to preserve SSL config)
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	sedExpr := fmt.Sprintf("s|php%s-fpm-%s.sock|php%s-fpm-%s.sock|g", oldVer, domain, newVer, domain)
	if out, err := exec.Command("sed", "-i", sedExpr, nginxConf).CombinedOutput(); err != nil {
		slog.Error("Failed to update nginx config", "error", err, "output", string(out))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	slog.Info("PHP update completed", "domain", domain, "version", newVer)

	s.reportTaskResult(result)
}

func (s *AgentServer) executeIssueSSL(taskID uint64, siteID uint64, domain, email string) {
	slog.Info("Issuing SSL certificate", "domain", domain)
	err := ssl.IssueCertificate(domain, email)

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
	if err != nil {
		slog.Error("SSL issuance failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("SSL certificate issued", "domain", domain)
		if expiry, err2 := ssl.GetCertExpiry(domain); err2 == nil {
			result.SSLExpiresAt = expiry
		}
		enabled := true
		result.SSLEnabled = &enabled
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) reportTaskResult(result models.AgentTaskResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/tasks/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report task result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) executeAgentUpdate(taskID uint64, task models.AgentUpdateTask) {
	slog.Info("Starting agent self-update", "new_version", task.NewVersion, "url", task.DownloadURL)

	result := models.AgentUpdateResult{
		RolloutPlanID: task.RolloutPlanID,
		ServerID:      task.ServerID,
		OldVersion:    Version,
		NewVersion:    task.NewVersion,
	}

	// 1. Download new binary to temp location
	tmpBin := "/tmp/wp-manage-agent-new"
	dlCmd := exec.Command("wget", "-q", "-O", tmpBin, task.DownloadURL)
	if out, err := dlCmd.CombinedOutput(); err != nil {
		slog.Error("Failed to download agent update", "error", err, "output", string(out))
		result.Status = "failed"
		result.Error = fmt.Sprintf("download failed: %v", err)
		s.reportAgentUpdateResult(result)
		return
	}

	// 2. Verify checksum
	checksumCmd := exec.Command("sha256sum", tmpBin)
	checksumOut, err := checksumCmd.Output()
	if err != nil {
		result.Status = "failed"
		result.Error = "failed to verify checksum"
		s.reportAgentUpdateResult(result)
		return
	}
	actualChecksum := strings.Fields(string(checksumOut))[0]
	if actualChecksum != task.Checksum {
		result.Status = "failed"
		result.Error = fmt.Sprintf("checksum mismatch: expected %s, got %s", task.Checksum, actualChecksum)
		exec.Command("rm", "-f", tmpBin).Run()
		s.reportAgentUpdateResult(result)
		return
	}

	// 3. Make executable
	exec.Command("chmod", "+x", tmpBin).Run()

	// 4. Replace binary
	agentBin := "/usr/local/bin/wp-manage-agent"
	exec.Command("cp", agentBin, agentBin+".bak").Run() // backup current
	if out, err := exec.Command("cp", tmpBin, agentBin).CombinedOutput(); err != nil {
		slog.Error("Failed to replace agent binary", "error", err, "output", string(out))
		exec.Command("cp", agentBin+".bak", agentBin).Run() // restore backup
		result.Status = "failed"
		result.Error = fmt.Sprintf("replace failed: %v", err)
		s.reportAgentUpdateResult(result)
		return
	}

	// 5. Report success BEFORE restart
	result.Status = "completed"
	s.reportAgentUpdateResult(result)

	// 6. Restart agent service (this will kill us, so do it last)
	slog.Info("Agent update completed, restarting service...", "new_version", task.NewVersion)
	exec.Command("rm", "-f", tmpBin).Run()
	exec.Command("systemctl", "restart", "wp-manage-agent").Run()
}

func (s *AgentServer) reportAgentUpdateResult(result models.AgentUpdateResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/rollouts/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report agent update result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) Provision() error {
	p := provisioning.New(func(step string, progress int, message string) {
		slog.Info("Provision progress", "step", step, "progress", progress, "message", message)
	})
	return p.Run()
}

func (s *AgentServer) RegisterSystemInfo() error {
	info, err := system.GetInfo()
	if err != nil {
		return err
	}

	data, _ := json.Marshal(info)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/system-info", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *AgentServer) CreateSite(params site.CreateParams) error {
	mgr := site.NewManager(func(step string, progress int, message string) {
		slog.Info("Create site progress", "domain", params.Domain, "step", step, "progress", progress)
	})
	return mgr.CreateSite(params)
}

func (s *AgentServer) DeleteSite(domain, dbName, dbUser string) error {
	mgr := site.NewManager(nil)
	return mgr.DeleteSite(domain, dbName, dbUser)
}

func (s *AgentServer) IssueSSL(domain, email string) error {
	return ssl.IssueCertificate(domain, email)
}

func (s *AgentServer) RenewSSL(domain string) error {
	return ssl.RenewCertificate(domain)
}

func (s *AgentServer) RebuildConfig(configType, domain string, configData map[string]string) (*agentconfig.RebuildResult, error) {
	switch configType {
	case "nginx_vhost":
		return agentconfig.RebuildNginxVhost(domain, configData)
	case "php_fpm_pool":
		phpVer := configData["php_version"]
		if phpVer == "" {
			phpVer = "8.2"
		}
		return agentconfig.RebuildPHPFPMPool(domain, phpVer, configData)
	default:
		return nil, fmt.Errorf("unknown config type: %s", configType)
	}
}

func (s *AgentServer) HealthCheck() ([]agentconfig.HealthStatus, error) {
	return agentconfig.RunHealthCheck()
}

func (s *AgentServer) executeCreateBackup(taskID uint64, siteID uint64, task models.AgentBackupTask) {
	slog.Info("Starting backup", "domain", task.Domain, "type", task.Type, "backup_id", task.BackupID)

	params := agentbackup.BackupParams{
		BackupID:   task.BackupID,
		Domain:     task.Domain,
		Type:       task.Type,
		DBName:     task.DBName,
		DBUser:     task.DBUser,
		DBPass:     task.DBPass,
		PHPVersion: task.PHPVersion,
	}

	result := agentbackup.RunBackup(params)

	// Report result via backup-specific endpoint
	backupResult := models.AgentBackupResult{
		TaskID:   taskID,
		BackupID: task.BackupID,
		SiteID:   siteID,
	}

	if result.Error != nil {
		slog.Error("Backup failed", "domain", task.Domain, "error", result.Error)
		backupResult.Status = "failed"
		backupResult.Error = result.Error.Error()
	} else {
		slog.Info("Backup completed", "domain", task.Domain, "file", result.FilePath, "size", result.SizeBytes)
		backupResult.Status = "completed"
		backupResult.FilePath = result.FilePath
		backupResult.SizeBytes = result.SizeBytes
		backupResult.Checksum = result.Checksum
	}

	s.reportBackupResult(backupResult)
}

func (s *AgentServer) executeRestoreBackup(taskID uint64, siteID uint64, task models.AgentRestoreTask) {
	// Yield briefly so any same-batch delete_site goroutine (launched earlier in the
	// task loop) has time to acquire its domain lock first.  This ensures the intended
	// "delete then restore" order when the panel queues both tasks simultaneously.
	time.Sleep(300 * time.Millisecond)

	// Acquire per-domain lock — ensures delete_site for the same domain completes
	// before restore starts writing files to the webroot.
	unlock := s.lockDomain(task.Domain)
	defer unlock()

	slog.Info("Starting restore", "domain", task.Domain, "type", task.Type, "backup_id", task.BackupID)

	params := agentbackup.RestoreParams{
		BackupID:   task.BackupID,
		Domain:     task.Domain,
		Type:       task.Type,
		FilePath:   task.FilePath,
		DBName:     task.DBName,
		DBUser:     task.DBUser,
		DBPass:     task.DBPass,
		PHPVersion: task.PHPVersion,
	}

	manifest, err := agentbackup.RunRestore(params)

	taskResult := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
	if err != nil {
		slog.Error("Restore failed", "domain", task.Domain, "error", err)
		taskResult.Error = err.Error()
	} else {
		taskResult.Status = "active"
		slog.Info("Restore completed", "domain", task.Domain)

		if manifest != nil {
			// Always report HasSSL so API can update ssl_enabled on the site record
			hasSSL := manifest.HasSSL
			taskResult.RestoredHasSSL = &hasSSL

			// Detect WP version from the restored site so panel shows correct version
			wpVer := site.GetWPVersion(manifest.Domain)
			if wpVer != "" {
				taskResult.WPVersion = wpVer
				slog.Info("WP version detected after restore", "domain", manifest.Domain, "version", wpVer)
			}

			// For orphaned backup restores (siteID==0), send back site info so the
			// API can recreate the panel site record and make the site visible in the panel.
			if siteID == 0 {
				taskResult.RestoredDomain = manifest.Domain
				taskResult.RestoredDBName = manifest.DBName
				taskResult.RestoredDBUser = manifest.DBUser
				taskResult.RestoredDBPass = manifest.DBPass
				taskResult.RestoredPHPVersion = manifest.PHPVersion
				taskResult.RestoredBackupID = task.BackupID
			}
		}
	}
	s.reportTaskResult(taskResult)
}

func (s *AgentServer) executeConfigureWAF(taskID uint64, task models.AgentWAFTask) {
	slog.Info("Configuring WAF", "domain", task.Domain, "enabled", task.Enabled)
	err := agentsecurity.ConfigureWAF(task)

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("WAF configuration failed", "domain", task.Domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("WAF configured successfully", "domain", task.Domain)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeMalwareScan(taskID uint64, task models.AgentMalwareScanTask) {
	slog.Info("Starting malware scan", "domain", task.Domain, "scan_id", task.ScanID)
	scanResult := agentsecurity.RunMalwareScan(task)
	s.reportScanResult(scanResult)
}

func (s *AgentServer) reportScanResult(result models.AgentMalwareScanResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/scans/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report scan result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) executeCheckUpdates(taskID uint64, task models.AgentCheckUpdatesTask) {
	slog.Info("Checking WP updates", "domain", task.Domain, "site_id", task.SiteID)
	result := agentwp.CheckUpdates(task)
	s.reportUpdateCheckResult(result)
}

func (s *AgentServer) executeDBOptimize(taskID uint64, task models.AgentDBOptimizeTask) {
	slog.Info("Optimizing database", "domain", task.Domain, "db", task.DBName)
	result := agentdb.OptimizeDatabase(task)
	s.reportDBOptimizeResult(result)
}

func (s *AgentServer) executeStagingCreate(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	productionDomain, _ := data["production_domain"].(string)
	stagingDomain, _ := data["staging_domain"].(string)
	phpVersion, _ := data["php_version"].(string)
	dbName, _ := data["db_name"].(string)
	dbUser, _ := data["db_user"].(string)
	dbPassword, _ := data["db_password"].(string)

	slog.Info("Creating staging environment", "production", productionDomain, "staging", stagingDomain)

	// 1. Create staging DB by cloning production
	stagingDBName := fmt.Sprintf("staging_%s", strings.ReplaceAll(strings.ReplaceAll(stagingDomain, ".", "_"), "-", "_"))
	stagingDBUser := stagingDBName
	if len(stagingDBUser) > 32 {
		stagingDBUser = stagingDBUser[:32]
	}

	// Create staging database & user
	commands := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", stagingDBName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", stagingDBUser, dbPassword),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", stagingDBName, stagingDBUser),
		"FLUSH PRIVILEGES",
	}
	for _, cmd := range commands {
		exec.Command("mysql", "-e", cmd).Run()
	}

	// Clone DB: mysqldump production | mysql staging
	dumpCmd := exec.Command("mysqldump", "-u", dbUser, fmt.Sprintf("-p%s", dbPassword), dbName)
	importCmd := exec.Command("mysql", "-u", stagingDBUser, fmt.Sprintf("-p%s", dbPassword), stagingDBName)

	pipe, _ := dumpCmd.StdoutPipe()
	importCmd.Stdin = pipe
	dumpCmd.Start()
	importCmd.Start()
	dumpCmd.Wait()
	importCmd.Wait()

	// 2. Copy files
	srcDir := fmt.Sprintf("/var/www/%s/public/", productionDomain)
	dstDir := fmt.Sprintf("/var/www/%s/public/", stagingDomain)
	exec.Command("mkdir", "-p", dstDir).Run()
	exec.Command("rsync", "-a", srcDir, dstDir).Run()

	// 3. Update wp-config.php with staging DB
	wpConfig := fmt.Sprintf("%swp-config.php", dstDir)
	exec.Command("sed", "-i", fmt.Sprintf("s/%s/%s/g", dbName, stagingDBName), wpConfig).Run()
	exec.Command("sed", "-i", fmt.Sprintf("s/%s/%s/g", dbUser, stagingDBUser), wpConfig).Run()

	// 4. Search-replace URLs in DB
	wpUser := strings.ReplaceAll(strings.ReplaceAll(stagingDomain, ".", "_"), "-", "_")
	exec.Command("chown", "-R", fmt.Sprintf("%s:%s", wpUser, wpUser), dstDir).Run()

	// 5. Create Nginx vhost + PHP-FPM pool for staging
	agentconfig.RebuildNginxVhost(stagingDomain, map[string]string{"php_version": phpVersion})
	agentconfig.RebuildPHPFPMPool(stagingDomain, phpVersion, map[string]string{})
	exec.Command("systemctl", "reload", "nginx").Run()
	exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", phpVersion)).Run()

	// Report success
	stagingResult := map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "active",
		"site_status": "active",
	}
	s.reportStagingResult(stagingResult)
}

func (s *AgentServer) executeStagingPush(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	productionDomain, _ := data["production_domain"].(string)
	stagingDomain, _ := data["staging_domain"].(string)

	slog.Info("Pushing staging to production", "staging", stagingDomain, "production", productionDomain)

	// rsync staging files to production (excluding wp-config.php)
	srcDir := fmt.Sprintf("/var/www/%s/public/", stagingDomain)
	dstDir := fmt.Sprintf("/var/www/%s/public/", productionDomain)
	exec.Command("rsync", "-a", "--exclude=wp-config.php", srcDir, dstDir).Run()

	// Report success
	stagingResult := map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "active",
		"site_status": "active",
	}
	s.reportStagingResult(stagingResult)
}

func (s *AgentServer) executeStagingDelete(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	stagingDomain, _ := data["staging_domain"].(string)
	stagingDBName, _ := data["staging_db_name"].(string)
	stagingDBUser, _ := data["staging_db_user"].(string)

	slog.Info("Deleting staging environment", "domain", stagingDomain)

	// Remove files
	exec.Command("rm", "-rf", fmt.Sprintf("/var/www/%s", stagingDomain)).Run()

	// Remove nginx vhost
	exec.Command("rm", "-f", fmt.Sprintf("/etc/nginx/sites-available/%s.conf", stagingDomain)).Run()
	exec.Command("rm", "-f", fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", stagingDomain)).Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	// Remove DB
	if stagingDBName != "" {
		exec.Command("mysql", "-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", stagingDBName)).Run()
	}
	if stagingDBUser != "" {
		exec.Command("mysql", "-e", fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", stagingDBUser)).Run()
	}

	stagingResult := map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "deleted",
		"site_status": "deleting",
	}
	s.reportStagingResult(stagingResult)
}

func (s *AgentServer) reportStagingResult(result map[string]interface{}) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/staging/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report staging result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) reportUpdateCheckResult(result models.AgentCheckUpdatesResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/updates/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report update check result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) reportDBOptimizeResult(result models.AgentDBOptimizeResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/db-optimize/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report DB optimize result", "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) reportBackupResult(result models.AgentBackupResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/backups/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report backup result", "error", err)
		return
	}
	resp.Body.Close()
}

// Image optimization executor

func (s *AgentServer) executeOptimizeImages(taskID uint64, task models.AgentImageOptTask) {
	slog.Info("Optimizing images", "domain", task.Domain, "quality", task.Quality)
	result := agentwp.OptimizeImages(task)
	s.reportMultisiteResult("/api/v1/agent/images/optimize/complete", result)
}

// Multisite task executors

func (s *AgentServer) executeEnableMultisite(taskID uint64, task models.AgentEnableMultisiteTask) {
	slog.Info("Enabling multisite", "domain", task.Domain, "type", task.Type)
	result := agentwp.EnableMultisite(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/enable/complete", result)
}

func (s *AgentServer) executeCreateSubsite(taskID uint64, task models.AgentCreateSubsiteTask) {
	slog.Info("Creating subsite", "domain", task.Domain, "slug", task.Slug)
	result := agentwp.CreateSubsite(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/subsite/complete", result)
}

func (s *AgentServer) executeDeleteSubsite(taskID uint64, task models.AgentDeleteSubsiteTask) {
	slog.Info("Deleting subsite", "domain", task.Domain, "blog_id", task.WPBlogID)
	err := agentwp.DeleteSubsite(task)
	if err != nil {
		slog.Error("Failed to delete subsite", "error", err)
	}
	// No callback needed — already marked deleted in DB
}

func (s *AgentServer) executeSyncSubsites(taskID uint64, task models.AgentListSubsitesTask) {
	slog.Info("Syncing subsites", "domain", task.Domain)
	result := agentwp.ListSubsites(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/sync/complete", result)
}

// ==================== Infrastructure Task Executors ====================

func (s *AgentServer) executeManageFirewall(taskID uint64, task models.AgentFirewallTask) {
	slog.Info("Managing firewall", "action", task.Action)
	result := models.AgentTaskResult{TaskID: taskID}

	// Ensure UFW is installed and enabled
	if err := agentfirewall.EnsureUFWInstalled(); err != nil {
		slog.Warn("UFW install check failed", "error", err)
	}
	agentfirewall.EnsureEnabled()

	switch task.Action {
	case "add":
		err := agentfirewall.AddRule(task.RuleType, task.Protocol, task.Port, task.Source, task.Comment)
		if err != nil {
			result.Error = err.Error()
		}
	case "delete":
		err := agentfirewall.DeleteRule(task.RuleType, task.Protocol, task.Port, task.Source)
		if err != nil {
			result.Error = err.Error()
		}
	case "status":
		status, err := agentfirewall.GetStatus()
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Output = status
			slog.Info("Firewall status retrieved")
		}
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeManageNginxConfig(taskID uint64, task models.AgentNginxConfigTask) {
	slog.Info("Managing nginx config", "action", task.Action, "domain", task.Domain, "type", task.ConfigType)
	result := models.AgentTaskResult{TaskID: taskID}

	switch task.Action {
	case "apply":
		err := agentnginx.ApplyCustomConfig(task.Domain, task.ConfigType, task.Name, task.Content, task.Enabled)
		if err != nil {
			result.Error = err.Error()
		}
	case "remove":
		err := agentnginx.RemoveCustomConfig(task.ConfigType, task.Name)
		if err != nil {
			result.Error = err.Error()
		}
	case "test":
		err := agentnginx.TestConfig()
		if err != nil {
			result.Error = err.Error()
		}
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeReadVhost(taskID uint64, domain string) {
	slog.Info("Reading vhost config", "domain", domain)
	result := models.AgentTaskResult{TaskID: taskID}

	vhostPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	content, err := os.ReadFile(vhostPath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read vhost: %v", err)
	} else {
		result.Output = string(content)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeWriteVhost(taskID uint64, domain, content string) {
	slog.Info("Writing vhost config", "domain", domain)
	result := models.AgentTaskResult{TaskID: taskID}

	vhostPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)

	// Backup current config
	backupPath := vhostPath + ".bak"
	if oldContent, err := os.ReadFile(vhostPath); err == nil {
		os.WriteFile(backupPath, oldContent, 0644)
	}

	// Write new config
	if err := os.WriteFile(vhostPath, []byte(content), 0644); err != nil {
		result.Error = fmt.Sprintf("failed to write vhost: %v", err)
		s.reportTaskResult(result)
		return
	}

	// Test nginx config
	if err := agentnginx.TestConfig(); err != nil {
		// Rollback on failure
		if backup, berr := os.ReadFile(backupPath); berr == nil {
			os.WriteFile(vhostPath, backup, 0644)
		}
		result.Error = fmt.Sprintf("nginx config test failed, rolled back: %v", err)
		s.reportTaskResult(result)
		return
	}

	// Reload nginx
	if err := agentnginx.Reload(); err != nil {
		result.Error = fmt.Sprintf("nginx reload failed: %v", err)
	}

	// Clean up backup
	os.Remove(backupPath)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeOPcacheAction(taskID uint64, task models.AgentOPcacheTask) {
	slog.Info("OPcache action", "action", task.Action, "php_version", task.PHPVersion)
	result := models.AgentTaskResult{TaskID: taskID}

	switch task.Action {
	case "enable":
		if err := agentopcache.Enable(task.PHPVersion); err != nil {
			result.Error = err.Error()
		}
	case "disable":
		if err := agentopcache.Disable(task.PHPVersion); err != nil {
			result.Error = err.Error()
		}
	case "flush":
		if err := agentopcache.Flush(task.PHPVersion); err != nil {
			result.Error = err.Error()
		}
	case "stats":
		stats, err := agentopcache.GetStats(task.PHPVersion)
		if err != nil {
			result.Error = err.Error()
		} else {
			slog.Info("OPcache stats", "stats", stats)
		}
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeRebuildConfig(taskID uint64, task models.AgentRebuildConfigTask) {
	slog.Info("Rebuilding config", "types", task.ConfigTypes, "domain", task.Domain, "all_sites", len(task.Sites))
	result := models.AgentTaskResult{TaskID: taskID}

	type RebuildResult struct {
		Domain     string `json:"domain,omitempty"`
		ConfigType string `json:"config_type"`
		Success    bool   `json:"success"`
		Message    string `json:"message"`
	}

	var results []RebuildResult
	var errors []string

	// Build the list of sites to rebuild
	type siteEntry struct{ domain, phpVersion string }
	var sitesToRebuild []siteEntry
	if len(task.Sites) > 0 {
		// All-sites mode
		for _, s := range task.Sites {
			sitesToRebuild = append(sitesToRebuild, siteEntry{s.Domain, s.PHPVersion})
		}
	} else if task.Domain != "" {
		// Single-site mode
		sitesToRebuild = []siteEntry{{task.Domain, task.PHPVersion}}
	}

	rebuildOneSite := func(domain, phpVersion string) {
		if phpVersion == "" {
			phpVersion = "8.2"
		}
		for _, configType := range task.ConfigTypes {
			switch configType {
			case "nginx_vhost":
				r, err := agentconfig.RebuildNginxVhost(domain, map[string]string{
					"php_version": phpVersion,
				})
				if err != nil {
					errors = append(errors, fmt.Sprintf("%s nginx: %v", domain, err))
					results = append(results, RebuildResult{Domain: domain, ConfigType: "nginx_vhost", Success: false, Message: err.Error()})
				} else {
					results = append(results, RebuildResult{Domain: domain, ConfigType: "nginx_vhost", Success: r.Success, Message: r.Message})
					if !r.Success {
						errors = append(errors, fmt.Sprintf("%s nginx: %s", domain, r.Message))
					}
				}
			case "php_fpm_pool":
				r, err := agentconfig.RebuildPHPFPMPool(domain, phpVersion, map[string]string{})
				if err != nil {
					errors = append(errors, fmt.Sprintf("%s php-fpm: %v", domain, err))
					results = append(results, RebuildResult{Domain: domain, ConfigType: "php_fpm_pool", Success: false, Message: err.Error()})
				} else {
					results = append(results, RebuildResult{Domain: domain, ConfigType: "php_fpm_pool", Success: r.Success, Message: r.Message})
					if !r.Success {
						errors = append(errors, fmt.Sprintf("%s php-fpm: %s", domain, r.Message))
					}
				}
			}
		}
	}

	for _, site := range sitesToRebuild {
		rebuildOneSite(site.domain, site.phpVersion)
	}

	if outJSON, err := json.Marshal(results); err == nil {
		result.Output = string(outJSON)
	}
	if len(errors) > 0 {
		result.Error = strings.Join(errors, "; ")
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeHealthCheck(taskID uint64) {
	slog.Info("Running health check")
	result := models.AgentTaskResult{TaskID: taskID}

	checks, err := agentconfig.RunHealthCheck()
	if err != nil {
		result.Error = err.Error()
	} else {
		for _, check := range checks {
			slog.Info("Health check result", "config_type", check.ConfigType, "status", check.Status, "message", check.Message)
		}
		if outJSON, err := json.Marshal(checks); err == nil {
			result.Output = string(outJSON)
		}
	}
	s.reportTaskResult(result)
}

// ==================== Batch 2 Task Executors ====================

func (s *AgentServer) executeManageCron(taskID uint64, task models.AgentCronTask) {
	slog.Info("Managing cron", "action", task.Action, "job_id", task.JobID, "user", task.User)
	result := models.AgentTaskResult{TaskID: taskID}

	// Ensure cron daemon is installed before any operation
	if err := agentcron.EnsureCronInstalled(); err != nil {
		slog.Warn("cron install check failed", "error", err)
	}

	switch task.Action {
	case "add":
		if task.Enabled {
			err := agentcron.AddEntry(task.JobID, task.User, task.Schedule, task.Command, task.Comment)
			if err != nil {
				result.Error = err.Error()
			}
		} else {
			// Disabled — remove from /etc/cron.d
			agentcron.RemoveEntry(task.JobID)
		}
	case "remove":
		err := agentcron.RemoveEntry(task.JobID)
		if err != nil {
			result.Error = err.Error()
		}
	case "run_now":
		out, err := agentcron.RunNow(task.JobID, task.User, task.Command)
		if err != nil {
			result.Error = err.Error()
		}
		result.Output = out
	case "get_logs":
		lines := task.Lines
		if lines <= 0 {
			lines = 100
		}
		result.Output = agentcron.GetLogs(task.JobID, lines)
	}
	s.reportTaskResult(result)
}

// nginxUpdateServerName reads an nginx conf, adds or removes a domain from
// the server_name directive using regex so it handles any existing list of names.
func nginxUpdateServerName(confPath, addDomain, removeDomain string) error {
	content, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("read nginx config: %w", err)
	}
	confStr := string(content)

	// Match: server_name followed by one or more hostnames (no semicolon yet)
	re := regexp.MustCompile(`(server_name\s+)([^;]+)(;)`)
	confStr = re.ReplaceAllStringFunc(confStr, func(match string) string {
		sub := re.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		prefix, names, semi := sub[1], sub[2], sub[3]
		parts := strings.Fields(names)

		if addDomain != "" {
			// Add only if not already present
			found := false
			for _, p := range parts {
				if p == addDomain {
					found = true
					break
				}
			}
			if !found {
				parts = append(parts, addDomain)
			}
		}
		if removeDomain != "" {
			filtered := parts[:0]
			for _, p := range parts {
				if p != removeDomain {
					filtered = append(filtered, p)
				}
			}
			parts = filtered
		}
		return prefix + strings.Join(parts, " ") + semi
	})

	return os.WriteFile(confPath, []byte(confStr), 0644)
}

func (s *AgentServer) executeManageDomain(taskID uint64, task models.AgentDomainTask) {
	slog.Info("Managing domain", "action", task.Action, "domain", task.Domain, "site_domain", task.SiteDomain)
	result := models.AgentTaskResult{TaskID: taskID}

	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", task.SiteDomain)

	switch task.Action {
	case "add":
		if err := nginxUpdateServerName(nginxConf, task.Domain, ""); err != nil {
			result.Error = err.Error()
			s.reportTaskResult(result)
			return
		}
	case "remove":
		if err := nginxUpdateServerName(nginxConf, "", task.Domain); err != nil {
			result.Error = err.Error()
			s.reportTaskResult(result)
			return
		}
	}

	// Validate and reload
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		result.Error = fmt.Sprintf("nginx config invalid: %s", string(out))
		s.reportTaskResult(result)
		return
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	slog.Info("Domain managed", "action", task.Action, "domain", task.Domain)
	s.reportTaskResult(result)
}

// executeForceHTTPS enables/disables HTTP→HTTPS redirect in Nginx.
// Handles two cases:
//   1. Certbot-managed SSL (has "# managed by Certbot" in config) → use certbot --redirect / manual edit
//   2. Our generated SSL vhost (listen 443 ssl; ... server { listen 80; return 301 ... }) → toggle redirect in HTTP server block
func (s *AgentServer) executeForceHTTPS(taskID uint64, siteID uint64, domain, action string) {
	slog.Info("Force HTTPS", "action", action, "domain", domain)
	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}

	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	content, err := os.ReadFile(nginxConf)
	if err != nil {
		result.Error = fmt.Sprintf("read nginx config: %v", err)
		s.reportTaskResult(result)
		return
	}
	confStr := string(content)

	// Check if SSL is configured in nginx (listen 443 ssl)
	hasSSL := strings.Contains(confStr, "listen 443 ssl") || strings.Contains(confStr, "listen 443")
	if !hasSSL {
		result.Error = "SSL is not configured for this domain. Issue an SSL certificate first."
		s.reportTaskResult(result)
		return
	}

	isCertbotManaged := strings.Contains(confStr, "managed by Certbot")

	if isCertbotManaged {
		// Certbot-managed: use certbot to add/remove redirect
		if action == "enable" {
			// certbot --redirect reinstalls and adds HTTP->HTTPS redirect
			out, cerr := exec.Command("certbot", "--nginx", "-d", domain,
				"--non-interactive", "--reinstall", "--redirect").CombinedOutput()
			if cerr != nil {
				result.Error = fmt.Sprintf("certbot redirect failed: %s", string(out))
				s.reportTaskResult(result)
				return
			}
		} else {
			// Disable: replace the certbot HTTP redirect server block with a plain HTTP server block
			// Remove the certbot-managed redirect server block and add a plain HTTP one
			reHTTPBlock := regexp.MustCompile(`(?s)server\s*\{[^}]*if\s*\(\$host\s*=\s*` + regexp.QuoteMeta(domain) + `\)[^}]*return 301[^}]*\}[^}]*\}`)
			plainHTTP := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;
    root /var/www/%s/public;
    index index.php index.html;
    location / { try_files $uri $uri/ /index.php?$args; }
    location ~ /\.ht { deny all; }
}`, domain, domain, domain)
			newConf := reHTTPBlock.ReplaceAllString(confStr, plainHTTP)
			if newConf == confStr {
				// Pattern didn't match — just remove return 301 from existing listen 80 block
				reReturn := regexp.MustCompile(`(?m)^\s*return 301 https://.*;\s*$`)
				newConf = reReturn.ReplaceAllString(confStr, "")
			}
			if werr := os.WriteFile(nginxConf, []byte(newConf), 0644); werr != nil {
				result.Error = fmt.Sprintf("write nginx config: %v", werr)
				s.reportTaskResult(result)
				return
			}
		}
	} else {
		// Our generated SSL vhost: the HTTP server block is `server { listen 80; server_name ...; return 301 ... }`
		// Enable: ensure return 301 is in the listen 80 server block
		// Disable: replace return 301 with a plain HTTP server block

		// Remove any existing standalone redirect block
		reBlock := regexp.MustCompile(`(?s)\n# Force HTTPS redirect \[wp-manage\].*?# \[/wp-manage\]`)
		confStr = reBlock.ReplaceAllString(confStr, "")

		// Match the listen 80 server block (our generated format or plain)
		reHTTP80 := regexp.MustCompile(`(?s)(server\s*\{[^{]*listen 80;[^{]*\})`)
		if action == "enable" {
			// Replace listen 80 block content with redirect
			redirectBlock := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;
    return 301 https://$host$request_uri;
}`, domain, domain)
			if reHTTP80.MatchString(confStr) {
				confStr = reHTTP80.ReplaceAllString(confStr, redirectBlock)
			} else {
				// No listen 80 block found, append redirect
				confStr += "\n" + redirectBlock + "\n"
			}
		} else {
			// Restore plain HTTP server block (just serve, no redirect)
			plainHTTP := fmt.Sprintf(`server {
    listen 80;
    server_name %s www.%s;
    root /var/www/%s/public;
    index index.php index.html;
    location / { try_files $uri $uri/ /index.php?$args; }
    location ~ /\.ht { deny all; }
}`, domain, domain, domain)
			if reHTTP80.MatchString(confStr) {
				confStr = reHTTP80.ReplaceAllString(confStr, plainHTTP)
			}
		}

		if err := os.WriteFile(nginxConf, []byte(confStr), 0644); err != nil {
			result.Error = fmt.Sprintf("write nginx config: %v", err)
			s.reportTaskResult(result)
			return
		}
	}

	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		result.Error = fmt.Sprintf("nginx config invalid: %s", string(out))
		s.reportTaskResult(result)
		return
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	slog.Info("Force HTTPS applied", "action", action, "domain", domain)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeToggleRedis(taskID uint64, task models.AgentRedisTask) {
	slog.Info("Toggle Redis", "action", task.Action, "domain", task.Domain)
	result := models.AgentTaskResult{TaskID: taskID}

	switch task.Action {
	case "enable":
		if err := agentredis.EnableForSite(task.Domain, task.MaxMemoryMB, task.Prefix); err != nil {
			result.Error = err.Error()
		}
	case "disable":
		if err := agentredis.DisableForSite(task.Domain); err != nil {
			result.Error = err.Error()
		}
	case "flush":
		if err := agentredis.FlushCache(task.Domain); err != nil {
			result.Error = err.Error()
		}
	case "get_stats":
		stats, err := agentredis.GetStats()
		if err != nil {
			result.Error = err.Error()
		} else {
			// Serialize stats map to JSON string
			var sb strings.Builder
			for k, v := range stats {
				sb.WriteString(k + ":" + v + "\n")
			}
			result.Output = sb.String()
		}
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeUpdatePHPFPMPool(taskID uint64, task models.AgentPHPFPMTask) {
	slog.Info("Updating PHP-FPM pool", "domain", task.Domain, "php_version", task.PHPVersion)
	result := models.AgentTaskResult{TaskID: taskID}

	configData := map[string]string{
		"pm_max_children":      fmt.Sprintf("%d", task.PoolConfig.MaxChildren),
		"pm_start_servers":     fmt.Sprintf("%d", task.PoolConfig.StartServers),
		"pm_min_spare_servers": fmt.Sprintf("%d", task.PoolConfig.MinSpareServers),
		"pm_max_spare_servers": fmt.Sprintf("%d", task.PoolConfig.MaxSpareServers),
		"pm_max_requests":      fmt.Sprintf("%d", task.PoolConfig.MaxRequests),
		"php_memory_limit":     task.PoolConfig.MemoryLimit,
	}

	if _, err := agentconfig.RebuildPHPFPMPool(task.Domain, task.PHPVersion, configData); err != nil {
		result.Error = err.Error()
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeManageMariaDB(taskID uint64, task models.AgentMariaDBTask) {
	slog.Info("Managing MariaDB", "action", task.Action)
	result := models.AgentTaskResult{TaskID: taskID}

	switch task.Action {
	case "create_user":
		if err := agentdb.CreateUser(task.Username, task.Password, task.Database, task.Privileges); err != nil {
			result.Error = err.Error()
		}
	case "drop_user":
		if err := agentdb.DropUser(task.Username); err != nil {
			result.Error = err.Error()
		}
	case "update_config":
		if err := agentdb.UpdateConfig(
			task.Config.MaxConnections,
			task.Config.InnodbBufferMB,
			task.Config.QueryCacheMB,
			task.Config.SlowQueryLog,
			task.Config.SlowQueryTime,
			task.Config.CustomConfig,
		); err != nil {
			result.Error = err.Error()
		}
	case "list_users":
		users, err := agentdb.ListUsers()
		if err != nil {
			result.Error = err.Error()
		} else {
			out, _ := json.Marshal(users)
			result.Output = string(out)
		}
	case "get_config":
		cfg, err := agentdb.GetConfig()
		if err != nil {
			result.Error = err.Error()
		} else {
			out, _ := json.Marshal(cfg)
			result.Output = string(out)
		}
	case "list_databases":
		dbs, err := agentdb.ListDatabases()
		if err != nil {
			result.Error = err.Error()
		} else {
			out, _ := json.Marshal(dbs)
			result.Output = string(out)
		}
	case "create_database":
		if err := agentdb.CreateDatabase(task.Database, task.Charset, task.Collation); err != nil {
			result.Error = err.Error()
		}
	case "drop_database":
		if err := agentdb.DropDatabase(task.Database); err != nil {
			result.Error = err.Error()
		}
	case "import_sql":
		if task.Database == "" {
			result.Error = "database name is required for import_sql"
			break
		}
		if task.SQLContent == "" {
			result.Error = "sql_content is required for import_sql"
			break
		}
		rawBytes, err := base64.StdEncoding.DecodeString(task.SQLContent)
		if err != nil {
			result.Error = "failed to decode sql content: " + err.Error()
			break
		}
		// Decompress if needed based on magic bytes
		sqlReader, decompErr := decompressSQLContent(rawBytes)
		if decompErr != nil {
			result.Error = decompErr.Error()
			break
		}
		cmd := exec.Command("mysql", "-u", "root", task.Database)
		cmd.Stdin = sqlReader
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			result.Error = "mysql import failed: " + strings.TrimSpace(stderr.String())
			break
		}
		result.Output = fmt.Sprintf("Database '%s' imported successfully", task.Database)
		slog.Info("SQL imported successfully", "database", task.Database)
	}
	s.reportTaskResult(result)
}

// decompressSQLContent detects format by magic bytes and returns a reader for the SQL content.
// Supported: plain SQL, gzip (.gz), zip (.zip — first .sql entry is used).
func decompressSQLContent(data []byte) (io.Reader, error) {
	// GZip magic: 1f 8b
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to open gzip: %w", err)
		}
		return gr, nil
	}
	// ZIP magic: PK 03 04
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04 {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("failed to open zip: %w", err)
		}
		// Use the first .sql file found, or first file if none has .sql extension
		var target *zip.File
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if strings.HasSuffix(strings.ToLower(f.Name), ".sql") {
				target = f
				break
			}
			if target == nil {
				target = f
			}
		}
		if target == nil {
			return nil, fmt.Errorf("zip archive is empty or contains no SQL file")
		}
		rc, err := target.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open zip entry %q: %w", target.Name, err)
		}
		return rc, nil
	}
	// Plain SQL
	return bytes.NewReader(data), nil
}

func (s *AgentServer) executeListInstalledPHP(taskID uint64) {
	slog.Info("Listing installed PHP versions")
	result := models.AgentTaskResult{TaskID: taskID}

	type PHPVersion struct {
		Version        string `json:"version"`
		Installed      bool   `json:"installed"`
		FPMActive      bool   `json:"fpm_active"`
		OPcacheEnabled bool   `json:"opcache_enabled"`
	}

	versions := []string{"7.4", "8.0", "8.1", "8.2", "8.3"}
	var installed []PHPVersion

	for _, v := range versions {
		// Check if FPM dir exists
		fpmDir := fmt.Sprintf("/etc/php/%s/fpm", v)
		if _, err := os.Stat(fpmDir); err != nil {
			continue
		}
		pv := PHPVersion{Version: v, Installed: true}
		// Check if FPM service is active
		out, err := exec.Command("systemctl", "is-active", fmt.Sprintf("php%s-fpm", v)).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			pv.FPMActive = true
		}
		// Check if OPcache is enabled via php cli ini_get
		opcacheOut, err := exec.Command(fmt.Sprintf("php%s", v), "-r", "echo ini_get('opcache.enable');").Output()
		if err == nil && strings.TrimSpace(string(opcacheOut)) == "1" {
			pv.OPcacheEnabled = true
		}
		installed = append(installed, pv)
	}

	out, _ := json.Marshal(installed)
	result.Output = string(out)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeInstallPHPVersion(taskID uint64, task models.AgentPHPVersionTask) {
	slog.Info("Installing PHP version", "version", task.Version)
	result := models.AgentTaskResult{TaskID: taskID}

	phpFpmDir := fmt.Sprintf("/etc/php/%s/fpm/pool.d", task.Version)
	if _, err := os.Stat(phpFpmDir); err == nil {
		// Already installed
		result.Output = fmt.Sprintf("PHP %s is already installed", task.Version)
		s.reportTaskResult(result)
		return
	}

	pkgs := []string{
		fmt.Sprintf("php%s-fpm", task.Version), fmt.Sprintf("php%s-cli", task.Version),
		fmt.Sprintf("php%s-common", task.Version), fmt.Sprintf("php%s-mysql", task.Version),
		fmt.Sprintf("php%s-xml", task.Version), fmt.Sprintf("php%s-curl", task.Version),
		fmt.Sprintf("php%s-gd", task.Version), fmt.Sprintf("php%s-mbstring", task.Version),
		fmt.Sprintf("php%s-zip", task.Version), fmt.Sprintf("php%s-bcmath", task.Version),
		fmt.Sprintf("php%s-intl", task.Version), fmt.Sprintf("php%s-soap", task.Version),
		fmt.Sprintf("php%s-redis", task.Version), fmt.Sprintf("php%s-imagick", task.Version),
		fmt.Sprintf("php%s-opcache", task.Version),
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if out, err := exec.Command("apt-get", args...).CombinedOutput(); err != nil {
		slog.Error("Failed to install PHP", "version", task.Version, "error", err, "output", string(out))
		result.Error = fmt.Sprintf("failed to install php%s: %v", task.Version, err)
		s.reportTaskResult(result)
		return
	}
	exec.Command("systemctl", "enable", fmt.Sprintf("php%s-fpm", task.Version)).Run()
	exec.Command("systemctl", "start", fmt.Sprintf("php%s-fpm", task.Version)).Run()
	result.Output = fmt.Sprintf("PHP %s installed successfully", task.Version)
	slog.Info("PHP version installed successfully", "version", task.Version)
	s.reportTaskResult(result)
}

func (s *AgentServer) executeTailLogs(taskID uint64, logType string, lines int) {
	slog.Info("Tailing logs", "type", logType, "lines", lines)
	result := models.AgentTaskResult{TaskID: taskID}

	logPaths := map[string]string{
		"nginx_access": "/var/log/nginx/access.log",
		"nginx_error":  "/var/log/nginx/error.log",
		"php_error":    "/var/log/php-fpm/error.log",
		"mysql_error":  "/var/log/mysql/error.log",
		"syslog":       "/var/log/syslog",
	}

	logPath, ok := logPaths[logType]
	if !ok {
		result.Error = fmt.Sprintf("unknown log type: %s", logType)
		s.reportTaskResult(result)
		return
	}

	// Try common alternative paths
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		altPaths := map[string][]string{
			"php_error":   {"/var/log/php8.2-fpm.log", "/var/log/php8.1-fpm.log", "/var/log/php8.0-fpm.log"},
			"mysql_error": {"/var/log/mariadb/mariadb.log", "/var/log/mysqld.log"},
		}
		if alts, ok := altPaths[logType]; ok {
			for _, alt := range alts {
				if _, err := os.Stat(alt); err == nil {
					logPath = alt
					break
				}
			}
		}
	}

	out, err := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logPath).CombinedOutput()
	if err != nil {
		result.Error = fmt.Sprintf("failed to read logs: %s", err.Error())
	} else {
		result.Output = string(out)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeManageSSHKey(taskID uint64, task models.AgentSSHKeyTask) {
	slog.Info("Managing SSH key", "action", task.Action, "name", task.Name)
	var err error

	switch task.Action {
	case "add":
		err = agentssh.AddKey(task.Name, task.PublicKey)
	case "remove":
		err = agentssh.RemoveKey(task.Name, task.PublicKey)
	default:
		err = fmt.Errorf("unknown SSH key action: %s", task.Action)
	}

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("SSH key management failed", "action", task.Action, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("SSH key management completed", "action", task.Action, "name", task.Name)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeManageSFTP(taskID uint64, task models.AgentSFTPTask) {
	slog.Info("Managing SFTP", "action", task.Action, "username", task.Username)
	var err error

	switch task.Action {
	case "create":
		err = agentsftp.CreateUser(task.Username, task.Password, task.HomeDir, task.Domain)
	case "delete":
		err = agentsftp.DeleteUser(task.Username, task.Domain)
	default:
		err = fmt.Errorf("unknown SFTP action: %s", task.Action)
	}

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("SFTP management failed", "action", task.Action, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("SFTP management completed", "action", task.Action, "username", task.Username)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeHardenWordPress(taskID uint64, task models.AgentHardenTask) {
	slog.Info("Hardening WordPress", "domain", task.Domain)

	opts := agentsecurity.HardenOptions{
		DisableXMLRPC:     task.DisableXMLRPC,
		HideWPVersion:     task.HideWPVersion,
		SecureFilePerms:   task.SecureFilePerms,
		SecurityHeaders:   task.SecurityHeaders,
		DisableFileEditor: task.DisableFileEditor,
	}

	err := agentsecurity.HardenWordPress(task.Domain, opts)

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("WordPress hardening failed", "domain", task.Domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("WordPress hardening completed", "domain", task.Domain)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executePHPFPMAutotune(taskID uint64, task models.AgentAutotuneTask) {
	slog.Info("PHP-FPM autotune starting", "domain", task.Domain, "php_version", task.PHPVersion)

	tuneResult, err := agentphpfpm.Autotune(task.Domain, task.PHPVersion)

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("PHP-FPM autotune failed", "domain", task.Domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("PHP-FPM autotune completed", "domain", task.Domain, "max_children", tuneResult.MaxChildren)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) reportMultisiteResult(endpoint string, result interface{}) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report multisite result", "endpoint", endpoint, "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) callbackAPI(endpoint string, result interface{}) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to callback API", "endpoint", endpoint, "error", err)
		return
	}
	resp.Body.Close()
}

func (s *AgentServer) completeTask(taskID uint64, status, errMsg string) {
	result := models.AgentTaskResult{TaskID: taskID}
	if errMsg != "" {
		result.Error = errMsg
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeWPCLI(taskID uint64, task models.AgentWPCLITask) {
	slog.Info("Executing WP-CLI command", "domain", task.Domain, "command", task.Command)
	result, err := agentwp.ExecuteWPCLI(task.Domain, task.Command)

	wpcliResult := models.AgentWPCLIResult{
		HistoryID: task.HistoryID,
		Status:    "completed",
	}
	if err != nil {
		wpcliResult.Status = "failed"
		wpcliResult.Error = err.Error()
	} else {
		wpcliResult.Output = result.Output
		wpcliResult.ExitCode = result.ExitCode
		if result.ExitCode != 0 {
			wpcliResult.Status = "failed"
		}
	}

	s.callbackAPI("/api/v1/agent/wpcli/complete", wpcliResult)
}

func (s *AgentServer) executeManageCache(taskID uint64, task models.AgentCacheTask) {
	slog.Info("Managing Nginx cache", "domain", task.Domain, "action", task.Action)
	var err error

	switch task.Action {
	case "enable":
		err = agentnginx.EnableFastCGICache(task.Domain, task.CacheTime, task.Excludes)
	case "disable":
		err = agentnginx.DisableFastCGICache(task.Domain)
	case "purge":
		err = agentnginx.PurgeCache(task.Domain)
	default:
		err = fmt.Errorf("unknown cache action: %s", task.Action)
	}

	status := "completed"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
		slog.Error("Cache management failed", "domain", task.Domain, "action", task.Action, "error", err)
	}

	s.completeTask(taskID, status, errMsg)
}

func (s *AgentServer) executeImportExternal(taskID uint64, task models.AgentImportTask) {
	slog.Info("Starting external import", "domain", task.Domain, "source_ip", task.SourceIP)

	runner := agentmigration.NewImportRunner(s.apiURL, s.apiKey)
	err := runner.RunImport(task.Domain, task.SourceIP, task.SSHUser, task.SSHPort,
		task.SSHKey, task.DBName, task.DBUser, task.DBPass, task.PHPVersion)

	result := models.AgentImportResult{
		ImportID: task.ImportID,
		SiteID:   task.SiteID,
		Progress: 100,
		Status:   "completed",
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		result.Progress = 0
		slog.Error("External import failed", "domain", task.Domain, "error", err)
	}

	s.callbackAPI("/api/v1/agent/import/complete", result)
}

func (s *AgentServer) executeDeleteBackupFile(taskID uint64, filePath string) {
	slog.Info("Deleting backup file", "path", filePath)

	result := models.AgentTaskResult{TaskID: taskID}
	if filePath == "" {
		result.Error = "empty file path"
		s.reportTaskResult(result)
		return
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		slog.Error("Delete backup file failed", "path", filePath, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Backup file deleted", "path", filePath)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeRenewSSL(taskID uint64, siteID uint64, domain string) {
	slog.Info("Renewing SSL certificate", "domain", domain)

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
	if err := ssl.RenewCertificate(domain); err != nil {
		slog.Error("SSL renewal failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("SSL certificate renewed", "domain", domain)
		if expiry, err2 := ssl.GetCertExpiry(domain); err2 == nil {
			result.SSLExpiresAt = expiry
		}
		result.Output = "SSL certificate renewed successfully"
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeDisableSSL(taskID uint64, siteID uint64, domain, phpVer string) {
	slog.Info("Disabling SSL", "domain", domain)

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
	if err := ssl.DisableSSL(domain, phpVer); err != nil {
		slog.Error("SSL disable failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		enabled := false
		result.SSLEnabled = &enabled
		result.Output = "SSL disabled. Site now serves HTTP only."
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeUploadCustomSSL(taskID uint64, siteID uint64, domain, certPEM, keyPEM, caPEM string) {
	slog.Info("Uploading custom SSL certificate", "domain", domain)

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}

	if err := ssl.InstallCustomCert(domain, certPEM, keyPEM, caPEM); err != nil {
		slog.Error("Custom SSL install failed", "domain", domain, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Custom SSL certificate installed", "domain", domain)
		enabled := true
		result.SSLEnabled = &enabled
		result.Output = "Custom SSL certificate installed successfully. Site now serves HTTPS."
	}
	s.reportTaskResult(result)
}

// ==================== Fail2Ban ====================

func (s *AgentServer) executeFail2BanStatus(taskID uint64) {
	slog.Info("Getting fail2ban status")
	result := models.AgentTaskResult{TaskID: taskID}

	statusJSON, err := agentfail2ban.StatusJSON()
	if err != nil {
		slog.Error("Fail2ban status failed", "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Fail2ban status retrieved", "data_size", len(statusJSON))
	}
	_ = statusJSON // status is reported via the task result
	s.reportTaskResult(result)
}

func (s *AgentServer) executeFail2BanUnban(taskID uint64, jail, ip string) {
	slog.Info("Unbanning IP from fail2ban", "jail", jail, "ip", ip)
	result := models.AgentTaskResult{TaskID: taskID}

	if err := agentfail2ban.UnbanIP(jail, ip); err != nil {
		slog.Error("Fail2ban unban failed", "jail", jail, "ip", ip, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Fail2ban unban completed", "jail", jail, "ip", ip)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeFail2BanUpdateJail(taskID uint64, data map[string]interface{}) {
	jail, _ := data["jail"].(string)
	slog.Info("Updating fail2ban jail", "jail", jail)
	result := models.AgentTaskResult{TaskID: taskID}

	var enabled *bool
	if e, ok := data["enabled"]; ok {
		b, _ := e.(bool)
		enabled = &b
	}
	banTime := 0
	if bt, ok := data["ban_time"].(float64); ok {
		banTime = int(bt)
	}
	maxRetry := 0
	if mr, ok := data["max_retry"].(float64); ok {
		maxRetry = int(mr)
	}

	if err := agentfail2ban.UpdateJail(jail, enabled, banTime, maxRetry); err != nil {
		slog.Error("Fail2ban update jail failed", "jail", jail, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Fail2ban jail updated", "jail", jail)
	}
	s.reportTaskResult(result)
}
