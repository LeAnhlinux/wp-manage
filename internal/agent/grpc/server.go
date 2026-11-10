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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentconfig "github.com/gofiber-team/wp-manage/internal/agent/config"
	agentbackup "github.com/gofiber-team/wp-manage/internal/agent/backup"
	agentdb "github.com/gofiber-team/wp-manage/internal/agent/database"
	agentcron "github.com/gofiber-team/wp-manage/internal/agent/cron"
	"github.com/gofiber-team/wp-manage/internal/agent/updater"
	agentfirewall "github.com/gofiber-team/wp-manage/internal/agent/firewall"
	"github.com/gofiber-team/wp-manage/internal/agent/filemanager"
	agentmigration "github.com/gofiber-team/wp-manage/internal/agent/migration"
	agentnginx "github.com/gofiber-team/wp-manage/internal/agent/nginx"
	agentioncube "github.com/gofiber-team/wp-manage/internal/agent/ioncube"
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
	agenttransfer "github.com/gofiber-team/wp-manage/internal/agent/transfer"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// Version is set at build time via -ldflags="-X github.com/gofiber-team/wp-manage/internal/agent/grpc.Version=..."
var Version = "dev"

type AgentServer struct {
	apiURL          string
	apiKey          string
	siteMu          sync.Map // per-domain mutexes to prevent concurrent site operations
	activeCreations sync.Map // tracks siteIDs currently being created to prevent double-dispatch
	activeLongRunning int32  // atomic counter: transfer + backup goroutines in-flight
}

func NewAgentServer(apiURL, apiKey string) *AgentServer {
	return &AgentServer{apiURL: apiURL, apiKey: apiKey}
}

// validateTaskDomain rejects tasks whose domain field fails RFC 1123 validation.
// This prevents a compromised or malicious API response from triggering operations
// on paths like "/../../etc/passwd" or other injection attempts. (H14/H17)
func validateTaskDomain(domain string) error {
	return utils.ValidateDomain(domain)
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

// ensureUFWPorts checks that UFW allows inbound TCP on every port in the list.
// Safe to call when UFW is inactive or not installed — exits silently.
// Idempotent: skips ports already present in `ufw status` output.
func ensureUFWPorts(ports []int) {
	out, err := exec.Command("ufw", "status").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Status: active") {
		return // UFW not installed or inactive — nothing to do
	}
	status := string(out)
	for _, port := range ports {
		portStr := fmt.Sprintf("%d/tcp", port)
		if strings.Contains(status, portStr) {
			continue // rule already present
		}
		if result, err := exec.Command("ufw", "allow", portStr, "comment", "wp-manage-agent").CombinedOutput(); err != nil {
			slog.Warn("UFW: failed to allow port", "port", port, "error", err, "output", string(result))
		} else {
			slog.Info("UFW: allowed port", "port", port)
		}
	}
}

// StartFirewallWatcher periodically ensures required agent ports remain open in UFW.
// Runs once immediately, then repeats on the given interval.
// This self-heals the firewall in case rules are reset (e.g. after ufw reset or OS reboot
// with a policy that doesn't persist rules).
func StartFirewallWatcher(ports []int, interval time.Duration) {
	ensureUFWPorts(ports) // run immediately on startup
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ensureUFWPorts(ports)
		}
	}()
}

// StartHTTPServer starts the agent's HTTPS server for file manager and other direct operations.
// A self-signed TLS certificate is generated in memory on every startup — no key material
// is written to disk.  The API server connects with InsecureSkipVerify (traffic is still
// encrypted; this prevents passive eavesdropping over the internal network).
func (s *AgentServer) StartHTTPServer(port int) {
	// Firewall watcher is started separately via StartFirewallWatcher in main.go.

	mux := http.NewServeMux()

	// Register file manager routes
	fm := filemanager.NewHandler(s.apiKey)
	fm.RegisterRoutes(mux)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"` + Version + `"}`))
	})

	tlsCfg, err := newAgentTLSConfig()
	if err != nil {
		slog.Error("Failed to generate TLS certificate for agent HTTP server", "error", err)
		slog.Warn("Falling back to plain HTTP — strongly recommended to fix TLS generation")
		addr := fmt.Sprintf(":%d", port)
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("Agent HTTP server failed", "error", err)
		}
		return
	}

	srv := &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   mux,
		TLSConfig: tlsCfg,
	}
	slog.Info("Starting agent HTTPS server (self-signed TLS)", "port", port)
	// ListenAndServeTLS with empty cert/key paths uses srv.TLSConfig.Certificates
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		slog.Error("Agent HTTPS server failed", "error", err)
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
			// H14: Validate domain before dispatching any task that touches the filesystem
			if err := validateTaskDomain(siteTask.Domain); err != nil {
				slog.Error("Rejecting create_site task with invalid domain", "domain", siteTask.Domain, "error", err)
				continue
			}
			// Prevent double-dispatch: skip if a goroutine is already creating this site
			if _, alreadyRunning := s.activeCreations.LoadOrStore(task.SiteID, true); alreadyRunning {
				slog.Warn("Skipping duplicate create_site task", "site_id", task.SiteID, "domain", siteTask.Domain)
				continue
			}
			go func(taskID, sID uint64, t models.AgentCreateSiteTask) {
				defer s.activeCreations.Delete(sID)
				s.executeCreateSite(taskID, sID, t)
			}(task.TaskID, task.SiteID, siteTask)

		case "delete_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			if err := validateTaskDomain(data["domain"]); err != nil {
				slog.Error("Rejecting delete_site task with invalid domain", "domain", data["domain"], "error", err)
				continue
			}
			go s.executeDeleteSite(task.TaskID, data["domain"], data["db_name"], data["db_user"])

		case "restart_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			if err := validateTaskDomain(data["domain"]); err != nil {
				slog.Error("Rejecting restart_site task with invalid domain", "domain", data["domain"], "error", err)
				continue
			}
			go s.executeRestartSite(task.TaskID, data["domain"], data["php_version"])

		case "suspend_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			if err := validateTaskDomain(data["domain"]); err != nil {
				slog.Error("Rejecting suspend_site task with invalid domain", "domain", data["domain"], "error", err)
				continue
			}
			go s.executeSuspendSite(task.TaskID, data["domain"])

		case "resume_site":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			if err := validateTaskDomain(data["domain"]); err != nil {
				slog.Error("Rejecting resume_site task with invalid domain", "domain", data["domain"], "error", err)
				continue
			}
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
			atomic.AddInt32(&s.activeLongRunning, 1)
			go func(tID, sID uint64, t models.AgentBackupTask) {
				defer atomic.AddInt32(&s.activeLongRunning, -1)
				s.executeCreateBackup(tID, sID, t)
			}(task.TaskID, task.SiteID, backupTask)

		case "restore_backup":
			var restoreTask models.AgentRestoreTask
			json.Unmarshal(task.Data, &restoreTask)
			atomic.AddInt32(&s.activeLongRunning, 1)
			go func(tID, sID uint64, t models.AgentRestoreTask) {
				defer atomic.AddInt32(&s.activeLongRunning, -1)
				s.executeRestoreBackup(tID, sID, t)
			}(task.TaskID, task.SiteID, restoreTask)

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
			mode, _ := data["mode"].(string)
			runner := agentmigration.NewMigrationRunner(s.apiURL, s.apiKey)
			go runner.RollbackMigration(migrationID, domain, mode)

		case "cleanup_source_migration":
			var migrateTask models.AgentMigrateTask
			json.Unmarshal(task.Data, &migrateTask)
			runner := agentmigration.NewMigrationRunner(s.apiURL, s.apiKey)
			go runner.CleanupSourceOnly(migrateTask)

		case "cleanup_target_migration":
			// Runs on TARGET server — removes partially migrated data on rollback
			var migrateTask models.AgentMigrateTask
			json.Unmarshal(task.Data, &migrateTask)
			runner := agentmigration.NewMigrationRunner(s.apiURL, s.apiKey)
			go runner.CleanupTargetOnRollback(migrateTask)

		case "configure_waf":
			var wafTask models.AgentWAFTask
			json.Unmarshal(task.Data, &wafTask)
			go s.executeConfigureWAF(task.TaskID, wafTask)

		case "configure_bot_rules":
			var botTask models.AgentBotRulesTask
			json.Unmarshal(task.Data, &botTask)
			go s.executeConfigureBotRules(task.TaskID, botTask)

		case "configure_geo_rules":
			var geoTask models.AgentGeoRulesTask
			json.Unmarshal(task.Data, &geoTask)
			go s.executeConfigureGeoRules(task.TaskID, geoTask)

		case "refresh_geo_data":
			go s.executeRefreshGeoData(task.TaskID)

		case "malware_scan":
			var scanTask models.AgentMalwareScanTask
			json.Unmarshal(task.Data, &scanTask)
			go s.executeMalwareScan(task.TaskID, scanTask)

		case "install_clamav":
			go s.executeInstallClamAV(task.TaskID)

		case "clamav_status":
			go s.executeClamAVStatus(task.TaskID)

		case "check_updates":
			var wpTask models.AgentCheckUpdatesTask
			json.Unmarshal(task.Data, &wpTask)
			go s.executeCheckUpdates(task.TaskID, wpTask)

		case "apply_updates":
			var applyTask models.AgentApplyUpdatesTask
			json.Unmarshal(task.Data, &applyTask)
			go s.executeApplyUpdates(task.TaskID, applyTask)

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
			// Defer self-update while a long-running task (transfer/backup/restore)
			// is in-flight — restarting mid-transfer would orphan the goroutine and
			// leave the task stuck in "processing" forever.
			if atomic.LoadInt32(&s.activeLongRunning) > 0 {
				slog.Info("Deferring agent_update: long-running task in progress", "task_id", task.TaskID)
				continue
			}
			var updateTask models.AgentUpdateTask
			json.Unmarshal(task.Data, &updateTask)
			go s.executeAgentUpdate(task.TaskID, updateTask)

		case "optimize_images":
			var imgTask models.AgentImageOptTask
			json.Unmarshal(task.Data, &imgTask)
			go s.executeOptimizeImages(task.TaskID, imgTask)

		case "toggle_image_opt":
			var toggleTask models.AgentToggleImageOptTask
			json.Unmarshal(task.Data, &toggleTask)
			go s.executeToggleImageOpt(task.TaskID, toggleTask)

		case "toggle_nginx_webp":
			var webpTask models.AgentNginxWebPTask
			json.Unmarshal(task.Data, &webpTask)
			go s.executeToggleNginxWebP(task.TaskID, webpTask)

		case "image_originals_stats":
			var t models.AgentImageOriginalsTask
			json.Unmarshal(task.Data, &t)
			go s.executeImageOriginalsStats(task.TaskID, t)

		case "revert_images":
			var t models.AgentImageOriginalsTask
			json.Unmarshal(task.Data, &t)
			go s.executeRevertImages(task.TaskID, t)

		case "delete_image_originals":
			var t models.AgentImageOriginalsTask
			json.Unmarshal(task.Data, &t)
			go s.executeDeleteImageOriginals(task.TaskID, t)

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

		case "cloudflare_firewall":
			var cfTask models.AgentCloudflareFirewallTask
			json.Unmarshal(task.Data, &cfTask)
			go s.executeCloudflareFirewall(task.TaskID, cfTask)

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

		case "manage_ioncube":
			var icTask models.AgentIonCubeTask
			json.Unmarshal(task.Data, &icTask)
			go s.executeManageIonCube(task.TaskID, icTask)

		case "rebuild_config":
			var rebuildTask models.AgentRebuildConfigTask
			json.Unmarshal(task.Data, &rebuildTask)
			go s.executeRebuildConfig(task.TaskID, rebuildTask)

		case "health_check":
			go s.executeHealthCheck(task.TaskID)

		case "check_services":
			go s.executeCheckServices(task.TaskID)

		case "get_wp_version":
			var data struct {
				SiteID     uint64 `json:"site_id"`
				Domain     string `json:"domain"`
				PHPVersion string `json:"php_version"`
			}
			json.Unmarshal(task.Data, &data)
			go func(taskID uint64, siteID uint64, domain string) {
				result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID}
				result.WPVersion = site.GetWPVersion(domain)
				s.reportTaskResult(result)
				s.completeTask(taskID, "completed", "")
			}(task.TaskID, data.SiteID, data.Domain)

		case "install_service":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			// Chạy đồng bộ (không goroutine) để tránh dpkg lock conflict
			// khi nhiều install_service tasks được queue cùng lúc
			s.executeInstallService(task.TaskID, data["service"])

		case "add_authorized_key":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeAddAuthorizedKey(task.TaskID, data["public_key"], data["comment"])

		case "remove_authorized_key":
			var data map[string]string
			json.Unmarshal(task.Data, &data)
			go s.executeRemoveAuthorizedKey(task.TaskID, data["comment"])

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

		case "scan_disk_usage":
			var scanTask models.AgentDiskScanTask
			json.Unmarshal(task.Data, &scanTask)
			go s.executeScanDiskUsage(task.TaskID, scanTask)

		case "set_resource_limit":
			var rlTask models.AgentResourceLimitTask
			json.Unmarshal(task.Data, &rlTask)
			if err := validateTaskDomain(rlTask.Domain); err != nil {
				slog.Error("Rejecting set_resource_limit with invalid domain", "domain", rlTask.Domain)
				s.completeTask(task.TaskID, "failed", "invalid domain")
				continue
			}
			go s.executeSetResourceLimit(task.TaskID, rlTask)

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

		case "php_update_status":
			var phpTask models.AgentPHPVersionTask
			json.Unmarshal(task.Data, &phpTask)
			go s.executePHPUpdateStatus(task.TaskID, phpTask.Refresh)

		case "update_php_version":
			var phpTask models.AgentPHPVersionTask
			json.Unmarshal(task.Data, &phpTask)
			go s.executeUpdatePHPVersion(task.TaskID, phpTask)

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

		case "fail2ban_toggle_service":
			var data map[string]interface{}
			json.Unmarshal(task.Data, &data)
			enable, _ := data["enable"].(bool)
			go s.executeFail2BanToggleService(task.TaskID, enable)

		case "fail2ban_setup_jails":
			go s.executeFail2BanSetupJails(task.TaskID)

		case "upload_source_zip":
			var uploadTask models.AgentUploadSourceTask
			json.Unmarshal(task.Data, &uploadTask)
			go s.executeUploadSource(task.TaskID, uploadTask)

		case "import_db_sql":
			var importTask models.AgentImportDBTask
			json.Unmarshal(task.Data, &importTask)
			go s.executeImportDB(task.TaskID, importTask)

		// ── Transfer Tool tasks ────────────────────────────────────────────────
		case "transfer_discover":
			var discoverTask models.AgentTransferDiscoverTask
			json.Unmarshal(task.Data, &discoverTask)
			go s.executeTransferDiscover(task.TaskID, discoverTask)

		case "transfer_import_site":
			var importTask models.AgentTransferImportTask
			json.Unmarshal(task.Data, &importTask)
			atomic.AddInt32(&s.activeLongRunning, 1)
			go func(tID uint64, t models.AgentTransferImportTask) {
				defer atomic.AddInt32(&s.activeLongRunning, -1)
				s.executeTransferImport(tID, t)
			}(task.TaskID, importTask)

		default:
			// Fail the task instead of just warning. An unrecognised type is never
			// going to succeed, but leaving the row at 'processing' meant the API's
			// stale-task reset flipped it back to 'pending' and handed it over again
			// every 5 minutes — forever. A production panel had two such types
			// (queued by the mobile API under names the agent never had handlers for)
			// looping for months: 36k "Unknown task type" warnings in a single log
			// rotation, and the feature silently doing nothing the whole time.
			// Completing it as failed stops the loop and surfaces the mismatch in the
			// panel instead of only in the agent log.
			slog.Warn("Unknown task type", "type", task.TaskType, "task_id", task.TaskID)
			s.completeTask(task.TaskID, "failed", "agent does not handle task type: "+task.TaskType)
		}
	}
}

func (s *AgentServer) executeCreateSite(taskID uint64, siteID uint64, task models.AgentCreateSiteTask) {
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

	result := models.AgentTaskResult{TaskID: taskID, SiteID: siteID, Status: "active"}
	if err != nil {
		slog.Error("Create site failed", "domain", task.Domain, "install_type", installType, "error", err)
		result.Status = "error"
		result.Error = err.Error()
	} else {
		// Always report the linux user so API can store it in the site record
		result.LinuxUser = site.SiteUser(task.Domain)
		if installType == "auto" {
			// Detect WP version after install
			wpVer := site.GetWPVersion(task.Domain)
			result.WPVersion = wpVer
			result.AutologinSecret = params.AutologinSecret
			slog.Info("Create site (auto) completed", "domain", task.Domain, "wp_version", wpVer, "linux_user", result.LinuxUser)
		} else {
			slog.Info("Create site (manual) infrastructure ready", "domain", task.Domain, "linux_user", result.LinuxUser)
		}

		// If SSL was requested, extract the cert expiry so the API can store it in the DB.
		// Without this, ssl_expires_at stays NULL and the panel shows SSL as broken even
		// though the certificate is valid — requiring the user to disable+re-enable SSL.
		if task.SSLEnabled {
			if expiry, err2 := ssl.GetCertExpiry(task.Domain); err2 == nil {
				result.SSLExpiresAt = expiry
				enabled := true
				result.SSLEnabled = &enabled
				slog.Info("SSL certificate ready after site creation", "domain", task.Domain, "expires_at", expiry)
			} else {
				// Non-fatal: certbot may have failed during creation (DNS not pointed yet).
				// User can issue SSL from the panel once DNS is ready.
				slog.Warn("SSL requested but cert not found after creation (DNS may not be ready)", "domain", task.Domain, "error", err2)
			}
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

	if err := mgr.ExtractSourceZip(task.Domain, tmpPath, task.DBName, task.DBUser, task.DBPass, task.DBHost); err != nil {
		os.Remove(tmpPath)
		result.Error = "Failed to extract source: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	slog.Info("Source extracted successfully", "domain", task.Domain)
	result.Output = fmt.Sprintf("Source deployed to /var/www/%s/public — wp-config.php patched with new DB credentials", task.Domain)
	// Read WP version from version.php so UI shows correct version immediately
	result.WPVersion = site.GetWPVersion(task.Domain)
	if result.WPVersion != "" {
		slog.Info("WP version detected from source", "domain", task.Domain, "version", result.WPVersion)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeImportDB(taskID uint64, task models.AgentImportDBTask) {
	slog.Info("Importing database", "domain", task.Domain, "db_name", task.DBName)
	result := models.AgentTaskResult{TaskID: taskID, SiteID: task.SiteID}

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

	if err := mgr.ImportDatabase(task.DBName, tmpPath, task.Domain, task.OldDomain, task.PHPVersion); err != nil {
		os.Remove(tmpPath)
		result.Error = "Failed to import database: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	slog.Info("Database imported successfully", "domain", task.Domain, "db_name", task.DBName, "old_domain", task.OldDomain)
	result.Output = fmt.Sprintf("Database %s imported — site URL updated to match domain %s", task.DBName, task.Domain)
	// Read WP version from version.php — no PHP/WP-CLI needed, always works
	result.WPVersion = site.GetWPVersion(task.Domain)
	if result.WPVersion != "" {
		slog.Info("WP version detected after import", "domain", task.Domain, "version", result.WPVersion)
	}
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
	// Read before the vhost is deleted: it is the only place the agent can
	// learn which bot-control / country-access files belong to this domain.
	includes := agentsecurity.ManagedIncludesForDomain(domain)
	err := s.DeleteSite(domain, dbName, dbUser)
	if err == nil {
		agentsecurity.RemoveManagedFiles(includes)
	}

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
		phpVersion = "8.3"
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

// executeAgentUpdate downloads and stages a new agent binary, then hands off
// to the wp-manage-updater service for the actual swap + restart.
//
// The agent NEVER replaces its own binary while running — that causes race
// conditions between "report success" and "systemctl restart kills us".
// Instead:
//   1. Agent downloads + verifies the new binary (staging)
//   2. Agent writes metadata to /var/lib/wp-manage/pending-update.json
//   3. Agent starts wp-manage-updater (one-shot systemd service) and returns
//   4. Updater stops agent → swaps binary → starts agent → verifies → reports API
//   5. On failure, updater auto-rolls back to .bak and reports failure
func (s *AgentServer) executeAgentUpdate(taskID uint64, task models.AgentUpdateTask) {
	slog.Info("Staging agent update", "new_version", task.NewVersion, "url", task.DownloadURL)

	fail := func(msg string) {
		slog.Error("Agent update staging failed", "reason", msg)
		s.reportAgentUpdateResult(models.AgentUpdateResult{
			RolloutPlanID: task.RolloutPlanID,
			ServerID:      task.ServerID,
			OldVersion:    Version,
			NewVersion:    task.NewVersion,
			Status:        "failed",
			Error:         msg,
		})
	}

	// Ensure staging directory exists
	os.MkdirAll("/var/lib/wp-manage", 0750)
	stagedPath := updater.StagedBinaryPath

	// ── Step 1: Download new binary ──────────────────────────────────────────
	slog.Info("Downloading new agent binary...")
	dlCmd := exec.Command("wget", "-q", "--timeout=120", "-O", stagedPath, task.DownloadURL)
	if out, err := dlCmd.CombinedOutput(); err != nil {
		os.Remove(stagedPath)
		fail(fmt.Sprintf("download failed: %v — %s", err, string(out)))
		return
	}
	exec.Command("chmod", "+x", stagedPath).Run()

	// ── Step 2: Verify checksum ───────────────────────────────────────────────
	if task.Checksum != "" {
		checksumOut, err := exec.Command("sha256sum", stagedPath).Output()
		if err != nil {
			os.Remove(stagedPath)
			fail("sha256sum failed: " + err.Error())
			return
		}
		actual := strings.Fields(string(checksumOut))[0]
		if actual != task.Checksum {
			os.Remove(stagedPath)
			fail(fmt.Sprintf("checksum mismatch: expected %s, got %s", task.Checksum, actual))
			return
		}
		slog.Info("Checksum verified", "sha256", actual)
	} else {
		slog.Warn("No checksum provided — skipping verification")
	}

	// ── Step 3: Write metadata for the updater service ───────────────────────
	meta := updater.PendingUpdate{
		StagedPath:    stagedPath,
		TargetBinary:  updater.AgentBinaryPath,
		TargetService: "wp-manage-agent",
		Checksum:      task.Checksum,
		NewVersion:    task.NewVersion,
		OldVersion:    Version,
		RolloutPlanID: task.RolloutPlanID,
		ServerID:      task.ServerID,
		CallbackURL:   s.apiURL + "/api/v1/agent/rollouts/complete",
		CallbackKey:   s.apiKey,
	}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(updater.MetadataPath, metaData, 0600); err != nil {
		os.Remove(stagedPath)
		fail("failed to write update metadata: " + err.Error())
		return
	}

	// ── Step 4: Start wp-manage-updater (one-shot, takes over from here) ─────
	// The updater will: stop agent → swap binary → start agent → verify → report.
	// We do NOT wait for it (it will stop us anyway).
	// Mark task completed BEFORE starting the updater.
	// The updater will immediately stop this agent process (via systemctl stop),
	// which kills all goroutines — so completeTask() would never be reached after
	// the systemctl call. Marking it now ensures the task_queue row is updated
	// and the API doesn't leave it stuck in "processing" forever.
	s.completeTask(taskID, "completed", "")

	slog.Info("Handing off to wp-manage-updater service — agent will be restarted by updater")
	if out, err := exec.Command("systemctl", "start", "wp-manage-updater").CombinedOutput(); err != nil {
		// Updater service not installed — fall back to old self-update (legacy servers)
		slog.Warn("wp-manage-updater service not available, falling back to self-update",
			"error", err, "output", string(out))
		os.Remove(updater.MetadataPath)
		s.executeAgentUpdateLegacy(task, stagedPath)
		return
	}

	slog.Info("wp-manage-updater started — this agent process will be stopped by the updater")
	// The updater will report the final result to the API.
	// This goroutine will be killed when systemctl stops the agent service.
}

// executeAgentUpdateLegacy is the old self-update path, used only when
// wp-manage-updater is not yet installed on the server (legacy).
// Remove once all servers have the updater service deployed.
func (s *AgentServer) executeAgentUpdateLegacy(task models.AgentUpdateTask, stagedPath string) {
	slog.Warn("Using legacy self-update (wp-manage-updater not installed)")
	agentBin := updater.AgentBinaryPath
	backupBin := agentBin + updater.BackupSuffix
	exec.Command("cp", agentBin, backupBin).Run()
	if out, err := exec.Command("cp", stagedPath, agentBin).CombinedOutput(); err != nil {
		exec.Command("cp", backupBin, agentBin).Run()
		s.reportAgentUpdateResult(models.AgentUpdateResult{
			RolloutPlanID: task.RolloutPlanID, ServerID: task.ServerID,
			OldVersion: Version, NewVersion: task.NewVersion,
			Status: "failed", Error: fmt.Sprintf("replace failed: %v — %s", err, string(out)),
		})
		return
	}
	s.reportAgentUpdateResult(models.AgentUpdateResult{
		RolloutPlanID: task.RolloutPlanID, ServerID: task.ServerID,
		OldVersion: Version, NewVersion: task.NewVersion, Status: "completed",
	})
	exec.Command("rm", "-f", stagedPath).Run()
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
		return s.rebuildNginxVhost(domain, configData)
	case "php_fpm_pool":
		phpVer := configData["php_version"]
		if phpVer == "" {
			phpVer = "8.3"
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
		BackupID:      task.BackupID,
		Domain:        task.Domain,
		Type:          task.Type,
		DBName:        task.DBName,
		DBUser:        task.DBUser,
		DBPass:        task.DBPass,
		PHPVersion:    task.PHPVersion,
		EncryptionKey: s.apiKey, // used to AES-encrypt DBPass in manifest
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
		BackupID:      task.BackupID,
		Domain:        task.Domain,
		Type:          task.Type,
		FilePath:      task.FilePath,
		DBName:        task.DBName,
		DBUser:        task.DBUser,
		DBPass:        task.DBPass,
		PHPVersion:    task.PHPVersion,
		EncryptionKey: s.apiKey, // used to AES-decrypt DBPass from manifest
	}

	// A restore can overwrite the vhost with the copy stored in the backup,
	// which predates (or simply lacks) the bot-control / country-access includes.
	includes := agentsecurity.ManagedIncludesForDomain(task.Domain)

	manifest, err := agentbackup.RunRestore(params)
	if err == nil && includes.Any() {
		if rerr := agentsecurity.RestoreManagedIncludes(task.Domain, includes); rerr != nil {
			slog.Error("Failed to restore access rules after backup restore", "domain", task.Domain, "error", rerr)
		}
	}

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

func (s *AgentServer) executeConfigureBotRules(taskID uint64, task models.AgentBotRulesTask) {
	result := models.AgentTaskResult{TaskID: taskID}

	// Lock every domain in the task. Sorted + de-duplicated so two concurrent
	// multi-site tasks always take the locks in the same order (no deadlock).
	seen := map[string]bool{}
	domains := make([]string, 0, len(task.Sites))
	for _, site := range task.Sites {
		if err := validateTaskDomain(site.Domain); err != nil {
			result.Error = err.Error()
			s.reportTaskResult(result)
			return
		}
		if !seen[site.Domain] {
			seen[site.Domain] = true
			domains = append(domains, site.Domain)
		}
	}
	sort.Strings(domains)
	for _, d := range domains {
		unlock := s.lockDomain(d)
		defer unlock()
	}

	slog.Info("Configuring bot rules", "sites", len(task.Sites))
	if err := agentsecurity.ApplyBotRules(task); err != nil {
		slog.Error("Bot rules configuration failed", "error", err)
		result.Error = err.Error()
	}
	s.reportTaskResult(result)
}

// geoDeps gathers what country access needs besides the rules themselves:
// provider IP ranges (pulled from the panel, which refreshes them weekly) and
// this server's own addresses. Providers stays nil when the panel cannot be
// reached; the agent then keeps whatever ranges it already has on disk.
func (s *AgentServer) geoDeps() agentsecurity.GeoDeps {
	deps := agentsecurity.GeoDeps{SelfIPs: agentsecurity.LocalIPs()}

	req, _ := http.NewRequest("GET", s.apiURL+"/api/v1/agent/geo/providers", nil)
	req.Header.Set("X-Agent-Key", s.apiKey)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		slog.Warn("Could not fetch provider IP ranges", "error", err)
		return deps
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		slog.Warn("Could not fetch provider IP ranges", "status", resp.StatusCode)
		return deps
	}
	var body struct {
		Data models.AgentGeoProviders `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&body); err != nil || len(body.Data.Providers) == 0 {
		slog.Warn("Provider IP ranges response was empty or invalid", "error", err)
		return deps
	}
	deps.Providers = body.Data.Providers
	return deps
}

func (s *AgentServer) executeConfigureGeoRules(taskID uint64, task models.AgentGeoRulesTask) {
	result := models.AgentTaskResult{TaskID: taskID}

	// Same locking discipline as bot rules: every domain, sorted, de-duplicated.
	seen := map[string]bool{}
	domains := make([]string, 0, len(task.Sites))
	for _, site := range task.Sites {
		if err := validateTaskDomain(site.Domain); err != nil {
			result.Error = err.Error()
			s.reportTaskResult(result)
			return
		}
		if !seen[site.Domain] {
			seen[site.Domain] = true
			domains = append(domains, site.Domain)
		}
	}
	sort.Strings(domains)
	for _, d := range domains {
		unlock := s.lockDomain(d)
		defer unlock()
	}

	slog.Info("Configuring country access", "sites", len(task.Sites))
	if err := agentsecurity.ApplyGeoRules(task, s.geoDeps()); err != nil {
		slog.Error("Country access configuration failed", "error", err)
		result.Error = err.Error()
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeRefreshGeoData(taskID uint64) {
	result := models.AgentTaskResult{TaskID: taskID}
	if err := agentsecurity.RefreshGeoData(s.geoDeps()); err != nil {
		slog.Error("Country access data refresh failed", "error", err)
		result.Error = err.Error()
	}
	s.reportTaskResult(result)
}

// rebuildNginxVhost regenerates a vhost from the template and then puts back
// the bot-control and country-access includes, which the template does not know about.
func (s *AgentServer) rebuildNginxVhost(domain string, configData map[string]string) (*agentconfig.RebuildResult, error) {
	includes := agentsecurity.ManagedIncludesForDomain(domain)
	r, err := agentconfig.RebuildNginxVhost(domain, configData)
	if err == nil && r != nil && r.Success && includes.Any() {
		if rerr := agentsecurity.RestoreManagedIncludes(domain, includes); rerr != nil {
			slog.Error("Failed to restore access rules after vhost rebuild", "domain", domain, "error", rerr)
			r.Message += " (warning: bot control / country access rules were not restored — save them again in the panel)"
		}
	}
	return r, err
}

func (s *AgentServer) executeMalwareScan(taskID uint64, task models.AgentMalwareScanTask) {
	slog.Info("Starting malware scan", "domain", task.Domain, "scan_id", task.ScanID)
	scanResult := agentsecurity.RunMalwareScan(task)
	s.reportScanResult(scanResult)

	// Close out the queue row as well — reportScanResult only updates the
	// MalwareScan record. Without this the task sits at 'processing' forever, the
	// API's 5-minute stale-task reset flips it back to 'pending', and the agent is
	// handed the same scan again every 5 minutes indefinitely; each re-dispatch
	// spawns another RunMalwareScan goroutine until malwareScanSem is exhausted.
	errMsg := ""
	if scanResult.Status == "failed" {
		errMsg = scanResult.Error
	}
	s.completeTask(taskID, scanResult.Status, errMsg)
}

func (s *AgentServer) executeInstallClamAV(taskID uint64) {
	result := models.AgentTaskResult{TaskID: taskID, Status: "completed"}
	slog.Info("Installing ClamAV", "task_id", taskID)
	if err := agentsecurity.InstallClamAV(); err != nil {
		slog.Error("ClamAV installation failed", "error", err)
		result.Status = "failed"
		result.Error = err.Error()
	} else {
		slog.Info("ClamAV installed successfully")
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeClamAVStatus(taskID uint64) {
	result := models.AgentTaskResult{TaskID: taskID, Status: "completed"}
	slog.Info("Checking ClamAV status", "task_id", taskID)
	status := agentsecurity.GetClamAVStatus()
	data, _ := json.Marshal(status)
	result.Output = string(data)
	s.reportTaskResult(result)
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
	// Mark task complete so API side can wake up via WaitForTaskComplete
	s.completeTask(taskID, "completed", "")
}

func (s *AgentServer) executeApplyUpdates(taskID uint64, task models.AgentApplyUpdatesTask) {
	slog.Info("Applying WP updates", "domain", task.Domain, "core", task.CoreUpdates, "plugins", task.PluginUpdates, "themes", task.ThemeUpdates)
	result := agentwp.ApplyUpdates(task)
	s.reportApplyUpdatesResult(result)
	errMsg := ""
	if result.Status == "failed" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
}

func (s *AgentServer) executeDBOptimize(taskID uint64, task models.AgentDBOptimizeTask) {
	slog.Info("Optimizing database", "domain", task.Domain, "db", task.DBName)
	result := agentdb.OptimizeDatabase(task)
	s.reportDBOptimizeResult(result)
	s.completeTask(taskID, result.Status, result.Error)
}

func (s *AgentServer) executeStagingCreate(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	productionDomain, _ := data["production_domain"].(string)
	stagingDomain, _ := data["staging_domain"].(string)
	phpVersion, _ := data["php_version"].(string)
	dbName, _ := data["db_name"].(string)
	stagingDBName, _ := data["staging_db_name"].(string)
	stagingDBUser, _ := data["staging_db_user"].(string)
	stagingDBPassword, _ := data["staging_db_password"].(string)

	slog.Info("Creating staging environment", "production", productionDomain, "staging", stagingDomain)

	// fail helper — report error back to API and complete task
	fail := func(msg string) {
		slog.Error("Staging create failed", "staging", stagingDomain, "error", msg)
		s.reportStagingResult(map[string]interface{}{
			"staging_id":  stagingID,
			"status":      "failed",
			"site_status": "error",
			"error":       msg,
		})
		s.completeTask(taskID, "failed", msg)
	}

	// Fix #2 — create Linux user for staging site
	stagingLinuxUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(stagingDomain, ".", "_"), "-", "_")
	exec.Command("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin",
		"--home-dir", fmt.Sprintf("/var/www/%s", stagingDomain), stagingLinuxUser).Run()

	// 1. Create staging DB & user
	sqlCmds := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", stagingDBName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", stagingDBUser, stagingDBPassword),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'", stagingDBName, stagingDBUser),
		"FLUSH PRIVILEGES",
	}
	for _, cmd := range sqlCmds {
		exec.Command("mysql", "-e", cmd).Run()
	}

	// Fix #6 — safe dump via file (no pipe, with error handling)
	// --single-transaction + --skip-lock-tables: tránh LOCK TABLES (lỗi 1449 DEFINER)
	// --no-tablespaces: tránh lỗi ACCESS DENIED với tablespace info
	// Sau dump: sed strip DEFINER clause khỏi file (--skip-definer không có trong MariaDB 10.11)
	dumpFile := fmt.Sprintf("/tmp/staging-dump-%s.sql", stagingDomain)
	if out, err := exec.Command("mysqldump",
		"--single-transaction",
		"--skip-lock-tables",
		"--no-tablespaces",
		"--result-file="+dumpFile,
		dbName).CombinedOutput(); err != nil {
		slog.Error("Staging DB dump failed", "error", err, "output", string(out))
		fail("DB dump failed: " + string(out))
		return
	}
	// Strip DEFINER clauses — tránh lỗi 1449 khi import vào DB mới
	exec.Command("sed", "-i",
		"s/DEFINER=[^ ]* //g",
		dumpFile).Run()
	defer os.Remove(dumpFile)

	sqlBytes, _ := os.ReadFile(dumpFile)
	importCmd := exec.Command("mysql", stagingDBName)
	importCmd.Stdin = bytes.NewReader(sqlBytes)
	if out, err := importCmd.CombinedOutput(); err != nil {
		slog.Error("Staging DB import failed", "error", err, "output", string(out))
		fail("DB import failed: " + string(out))
		return
	}

	// 2. Copy files
	srcDir := fmt.Sprintf("/var/www/%s/public/", productionDomain)
	dstDir := fmt.Sprintf("/var/www/%s/public/", stagingDomain)
	exec.Command("mkdir", "-p", dstDir).Run()
	exec.Command("rsync", "-a", srcDir, dstDir).Run()

	// Fix #3 — create logs and tmp directories
	exec.Command("mkdir", "-p", fmt.Sprintf("/var/www/%s/logs", stagingDomain)).Run()
	exec.Command("mkdir", "-p", fmt.Sprintf("/var/www/%s/tmp", stagingDomain)).Run()

	// 3. Update wp-config.php with staging DB creds using WP-CLI config set
	// (safer than sed — handles special chars in passwords correctly)
	wpPath := fmt.Sprintf("/var/www/%s/public", stagingDomain)
	exec.Command("wp", "config", "set", "DB_NAME", stagingDBName, "--path="+wpPath, "--allow-root").Run()
	exec.Command("wp", "config", "set", "DB_USER", stagingDBUser, "--path="+wpPath, "--allow-root").Run()
	exec.Command("wp", "config", "set", "DB_PASSWORD", stagingDBPassword, "--path="+wpPath, "--allow-root").Run()

	// Fix #5 — search-replace production URL → staging URL in DB
	stagingURL := "http://" + stagingDomain
	for _, prodURL := range []string{"https://" + productionDomain, "http://" + productionDomain} {
		exec.Command("wp", "search-replace", prodURL, stagingURL,
			"--path="+wpPath,
			"--skip-themes", "--skip-plugins",
			"--allow-root").Run()
	}

	// Chown files to staging linux user (non-critical)
	exec.Command("chown", "-R", fmt.Sprintf("%s:%s", stagingLinuxUser, stagingLinuxUser), dstDir).Run()

	// Fix #4 — ensure webp.conf exists before rebuilding nginx vhost
	site.EnsureWebPConfd()

	// 5. Create Nginx vhost + PHP-FPM pool for staging
	agentconfig.RebuildNginxVhost(stagingDomain, map[string]string{"php_version": phpVersion})

	// Symlink into sites-enabled (RebuildNginxVhost only writes sites-available)
	nginxConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", stagingDomain)
	nginxEnabled := fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", stagingDomain)
	os.Remove(nginxEnabled) // remove stale symlink if any
	os.Symlink(nginxConf, nginxEnabled)

	agentconfig.RebuildPHPFPMPool(stagingDomain, phpVersion, map[string]string{})
	exec.Command("systemctl", "reload", "nginx").Run()
	exec.Command("systemctl", "restart", fmt.Sprintf("php%s-fpm", phpVersion)).Run()

	// Fix #8 — report success and complete task
	s.reportStagingResult(map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "active",
		"site_status": "active",
	})
	s.completeTask(taskID, "completed", "")
}

func (s *AgentServer) executeStagingPush(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	productionDomain, _ := data["production_domain"].(string)
	stagingDomain, _ := data["staging_domain"].(string)
	dbName, _ := data["db_name"].(string)
	stagingDBName, _ := data["staging_db_name"].(string)
	sslEnabled, _ := data["ssl_enabled"].(bool)

	slog.Info("Pushing staging to production", "staging", stagingDomain, "production", productionDomain, "ssl", sslEnabled)

	// fail helper
	fail := func(msg string) {
		slog.Error("Staging push failed", "staging", stagingDomain, "error", msg)
		s.reportStagingResult(map[string]interface{}{
			"staging_id":  stagingID,
			"status":      "failed",
			"site_status": "error",
			"error":       msg,
		})
		s.completeTask(taskID, "failed", msg)
	}

	// Fix #9 — sync DB: backup production, dump staging, import into production

	// 1. Backup production DB → keep in site's backup dir for 7 days (rollback safety)
	backupDir := fmt.Sprintf("/var/www/%s/backups", productionDomain)
	os.MkdirAll(backupDir, 0700)
	backupFile := fmt.Sprintf("%s/pre-push-%d-%s.sql",
		backupDir, stagingID, time.Now().Format("20060102-150405"))
	if out, err := exec.Command("mysqldump",
		"--single-transaction", "--skip-lock-tables", "--no-tablespaces",
		"--result-file="+backupFile, dbName).CombinedOutput(); err != nil {
		slog.Warn("Production DB backup before push failed (non-fatal)", "error", string(out))
	} else {
		slog.Info("Production DB backed up before push", "file", backupFile)
		// Auto-delete backups older than 7 days
		go func() {
			exec.Command("find", backupDir, "-name", "pre-push-*.sql",
				"-mtime", "+7", "-delete").Run()
		}()
	}

	// 2. Dump staging DB (temp file, deleted after import)
	stagingDumpFile := fmt.Sprintf("/tmp/staging-push-dump-%d.sql", stagingID)
	if out, err := exec.Command("mysqldump",
		"--single-transaction", "--skip-lock-tables", "--no-tablespaces",
		"--result-file="+stagingDumpFile, stagingDBName).CombinedOutput(); err != nil {
		fail("Failed to dump staging DB: " + string(out))
		return
	}
	defer os.Remove(stagingDumpFile)

	// 3. Drop + recreate production DB, then import staging dump
	exec.Command("mysql", "-e",
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
			dbName, dbName)).Run()

	sqlBytes, _ := os.ReadFile(stagingDumpFile)
	importCmd := exec.Command("mysql", dbName)
	importCmd.Stdin = bytes.NewReader(sqlBytes)
	if out, err := importCmd.CombinedOutput(); err != nil {
		fail("Failed to import staging DB to production: " + string(out))
		return
	}

	// 4. Search-replace staging URL → production URL in production DB
	wpPath := fmt.Sprintf("/var/www/%s/public", productionDomain)
	prodScheme := "http"
	if sslEnabled {
		prodScheme = "https"
	}
	prodURL := prodScheme + "://" + productionDomain
	stagingURL := "http://" + stagingDomain
	exec.Command("wp", "search-replace", stagingURL, prodURL,
		"--path="+wpPath, "--skip-themes", "--skip-plugins", "--allow-root").Run()
	// Also normalize any stale http:// refs to production domain
	if sslEnabled {
		exec.Command("wp", "search-replace", "http://"+productionDomain, prodURL,
			"--path="+wpPath, "--skip-themes", "--skip-plugins", "--allow-root").Run()
	}

	// rsync staging files to production (excluding wp-config.php)
	srcDir := fmt.Sprintf("/var/www/%s/public/", stagingDomain)
	dstDir := fmt.Sprintf("/var/www/%s/public/", productionDomain)
	exec.Command("rsync", "-a", "--exclude=wp-config.php", srcDir, dstDir).Run()

	// Fix #10 — report success and complete task
	s.reportStagingResult(map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "active",
		"site_status": "active",
	})
	s.completeTask(taskID, "completed", "")
}

func (s *AgentServer) executeStagingDelete(taskID uint64, data map[string]interface{}) {
	stagingID := uint64(data["staging_id"].(float64))
	stagingDomain, _ := data["staging_domain"].(string)
	stagingDBName, _ := data["staging_db_name"].(string)
	stagingDBUser, _ := data["staging_db_user"].(string)

	slog.Info("Deleting staging environment", "domain", stagingDomain)

	// Remove nginx vhost
	exec.Command("rm", "-f", fmt.Sprintf("/etc/nginx/sites-available/%s.conf", stagingDomain)).Run()
	exec.Command("rm", "-f", fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", stagingDomain)).Run()
	exec.Command("systemctl", "reload", "nginx").Run()

	// Fix #11 — remove PHP-FPM pools for all versions and reload
	for _, v := range []string{"7.4", "8.0", "8.1", "8.2", "8.3"} {
		poolConf := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", v, stagingDomain)
		if err := os.Remove(poolConf); err == nil {
			exec.Command("systemctl", "reload", fmt.Sprintf("php%s-fpm", v)).Run()
		}
	}

	// Remove files
	exec.Command("rm", "-rf", fmt.Sprintf("/var/www/%s", stagingDomain)).Run()

	// Fix #12 — drop DB using credentials from task data
	if stagingDBName != "" {
		exec.Command("mysql", "-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", stagingDBName)).Run()
	}
	if stagingDBUser != "" {
		exec.Command("mysql", "-e", fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", stagingDBUser)).Run()
		exec.Command("mysql", "-e", "FLUSH PRIVILEGES").Run()
	}

	// Fix #11 — remove the staging Linux user
	stagingLinuxUser := "wp_" + strings.ReplaceAll(strings.ReplaceAll(stagingDomain, ".", "_"), "-", "_")
	exec.Command("userdel", stagingLinuxUser).Run()

	// Fix #13 — report success and complete task
	s.reportStagingResult(map[string]interface{}{
		"staging_id":  stagingID,
		"status":      "deleted",
		"site_status": "deleted",
	})
	s.completeTask(taskID, "completed", "")
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

func (s *AgentServer) reportApplyUpdatesResult(result models.AgentApplyUpdatesResult) {
	data, _ := json.Marshal(result)
	req, _ := http.NewRequest("POST", s.apiURL+"/api/v1/agent/updates/apply-complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", s.apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report apply updates result", "error", err)
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
	// Progress callback: called every ~50 images; returns true to cancel
	progressFn := func(partial models.AgentImageOptResult) bool {
		return agentwp.SendProgressUpdate(s.apiURL, s.apiKey, partial)
	}
	result := agentwp.OptimizeImages(task, progressFn)

	// If WebP was enabled, ensure nginx is patched to serve .webp automatically
	if task.WebPEnabled && result.Status != "failed" {
		if err := site.EnsureWebPNginx(task.Domain); err != nil {
			slog.Warn("Failed to patch nginx for WebP", "domain", task.Domain, "error", err)
		} else {
			slog.Info("Nginx WebP serving enabled", "domain", task.Domain)
		}
	}

	s.reportMultisiteResult("/api/v1/agent/images/optimize/complete", result)
	// Mark task complete so it's not re-queued indefinitely
	errMsg := ""
	if result.Status == "failed" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
}

func (s *AgentServer) executeToggleImageOpt(taskID uint64, task models.AgentToggleImageOptTask) {
	slog.Info("Toggle image opt", "domain", task.Domain, "enabled", task.Enabled)
	err := agentwp.ToggleImageOpt(task.Domain, task.Enabled, task.Quality, task.WebPEnabled)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
		slog.Error("Toggle image opt failed", "domain", task.Domain, "error", err)
	}
	s.completeTask(taskID, status, errMsg)
}

func (s *AgentServer) executeToggleNginxWebP(taskID uint64, task models.AgentNginxWebPTask) {
	slog.Info("Toggle nginx WebP", "domain", task.Domain, "enabled", task.Enabled)
	var err error
	if task.Enabled {
		err = site.EnsureWebPNginx(task.Domain)
	} else {
		err = site.RevertWebPNginx(task.Domain)
	}
	status, errMsg := "completed", ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
		slog.Error("Toggle nginx WebP failed", "domain", task.Domain, "error", err)
	}
	s.completeTask(taskID, status, errMsg)
}

func (s *AgentServer) executeImageOriginalsStats(taskID uint64, task models.AgentImageOriginalsTask) {
	slog.Info("Getting image originals stats", "domain", task.Domain)
	count, sizeBytes := agentwp.GetOriginalsStats(task.Domain)
	out, _ := json.Marshal(models.AgentImageOriginalsResult{Count: count, SizeBytes: sizeBytes})
	s.completeTaskWithOutput(taskID, "completed", "", string(out))
}

func (s *AgentServer) executeRevertImages(taskID uint64, task models.AgentImageOriginalsTask) {
	slog.Info("Reverting images to originals", "domain", task.Domain)
	reverted, errMsg := agentwp.RevertImages(task.Domain)
	out, _ := json.Marshal(models.AgentImageOriginalsResult{Count: reverted})
	status := "completed"
	if errMsg != "" {
		status = "failed"
	}
	s.completeTaskWithOutput(taskID, status, errMsg, string(out))
}

func (s *AgentServer) executeDeleteImageOriginals(taskID uint64, task models.AgentImageOriginalsTask) {
	slog.Info("Deleting image originals", "domain", task.Domain)
	deleted, sizeFreed := agentwp.DeleteOriginals(task.Domain)
	out, _ := json.Marshal(models.AgentImageOriginalsResult{Count: deleted, SizeBytes: sizeFreed})
	s.completeTaskWithOutput(taskID, "completed", "", string(out))
}

// Multisite task executors

func (s *AgentServer) executeEnableMultisite(taskID uint64, task models.AgentEnableMultisiteTask) {
	slog.Info("Enabling multisite", "domain", task.Domain, "type", task.Type)
	result := agentwp.EnableMultisite(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/enable/complete", result)
	// Fix #1: call completeTask so task is marked done and not stuck "processing"
	errMsg := ""
	if result.Error != "" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
}

func (s *AgentServer) executeCreateSubsite(taskID uint64, task models.AgentCreateSubsiteTask) {
	slog.Info("Creating subsite", "domain", task.Domain, "slug", task.Slug)
	result := agentwp.CreateSubsite(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/subsite/complete", result)
	// Fix #3: call completeTask so task is marked done and not stuck "processing"
	errMsg := ""
	if result.Error != "" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
}

func (s *AgentServer) executeDeleteSubsite(taskID uint64, task models.AgentDeleteSubsiteTask) {
	slog.Info("Deleting subsite", "domain", task.Domain, "blog_id", task.WPBlogID)
	result := agentwp.DeleteSubsite(task)
	// Bug 1 fix: report result back so API can confirm deletion or rollback on failure
	s.reportMultisiteResult("/api/v1/agent/multisite/delete/complete", result)
	// Fix #4: call completeTask so task is marked done and not stuck "processing"
	errMsg := ""
	if result.Error != "" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
}

func (s *AgentServer) executeSyncSubsites(taskID uint64, task models.AgentListSubsitesTask) {
	slog.Info("Syncing subsites", "domain", task.Domain)
	result := agentwp.ListSubsites(task)
	s.reportMultisiteResult("/api/v1/agent/multisite/sync/complete", result)
	// Fix #5: call completeTask so task is marked done and not stuck "processing"
	errMsg := ""
	if result.Error != "" {
		errMsg = result.Error
	}
	s.completeTask(taskID, result.Status, errMsg)
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

func (s *AgentServer) executeCloudflareFirewall(taskID uint64, task models.AgentCloudflareFirewallTask) {
	slog.Info("CloudflareFirewall: executing", "action", task.Action)
	var errMsg string

	switch task.Action {
	case "enable":
		if err := agentfirewall.EnableCloudflareOnly(task.IPv4s, task.IPv6s); err != nil {
			errMsg = err.Error()
		}
	case "disable":
		if err := agentfirewall.DisableCloudflareOnly(); err != nil {
			errMsg = err.Error()
		}
	case "sync":
		if err := agentfirewall.SyncCloudflareIPs(task.IPv4s, task.IPv6s); err != nil {
			errMsg = err.Error()
		}
	default:
		errMsg = fmt.Sprintf("unknown cloudflare_firewall action: %s", task.Action)
	}

	status := "completed"
	if errMsg != "" {
		status = "failed"
		slog.Error("CloudflareFirewall: action failed", "action", task.Action, "error", errMsg)
	}
	s.completeTask(taskID, status, errMsg)
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

func (s *AgentServer) executeManageIonCube(taskID uint64, task models.AgentIonCubeTask) {
	slog.Info("ionCube action", "action", task.Action, "php_version", task.PHPVersion)
	result := models.AgentTaskResult{TaskID: taskID}

	switch task.Action {
	case "get_status":
		statuses, err := agentioncube.GetStatus()
		if err != nil {
			result.Error = err.Error()
		} else {
			out, _ := json.Marshal(statuses)
			result.Output = string(out)
		}
	case "enable":
		if err := agentioncube.Enable(task.PHPVersion); err != nil {
			result.Error = err.Error()
		}
	case "disable":
		if err := agentioncube.Disable(task.PHPVersion); err != nil {
			result.Error = err.Error()
		}
	default:
		result.Error = fmt.Sprintf("unknown ionCube action: %s", task.Action)
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
	type siteEntry struct {
		domain             string
		phpVersion         string
		openBasedirEnabled bool
	}
	var sitesToRebuild []siteEntry
	if len(task.Sites) > 0 {
		// All-sites mode
		for _, s := range task.Sites {
			sitesToRebuild = append(sitesToRebuild, siteEntry{s.Domain, s.PHPVersion, s.OpenBasedirEnabled})
		}
	} else if task.Domain != "" {
		// Single-site mode
		sitesToRebuild = []siteEntry{{task.Domain, task.PHPVersion, task.OpenBasedirEnabled}}
	}

	rebuildOneSite := func(e siteEntry) {
		domain := e.domain
		phpVersion := e.phpVersion
		if phpVersion == "" {
			phpVersion = "8.3"
		}
		openBasedirVal := "true"
		if !e.openBasedirEnabled {
			openBasedirVal = "false"
		}
		for _, configType := range task.ConfigTypes {
			switch configType {
			case "nginx_vhost":
				r, err := s.rebuildNginxVhost(domain, map[string]string{
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
				r, err := agentconfig.RebuildPHPFPMPool(domain, phpVersion, map[string]string{
					"open_basedir_enabled": openBasedirVal,
				})
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
		rebuildOneSite(site)
	}

	if outJSON, err := json.Marshal(results); err == nil {
		result.Output = string(outJSON)
	}
	if len(errors) > 0 {
		result.Error = strings.Join(errors, "; ")
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeCheckServices(taskID uint64) {
	type ServiceStatus struct {
		Service string `json:"service"`
		Active  bool   `json:"active"`
	}

	checkActive := func(svc string) bool {
		switch svc {
		case "ufw":
			// UFW doesn't run as a typical systemd service; check its config file
			out, err := os.ReadFile("/etc/ufw/ufw.conf")
			if err != nil {
				return false
			}
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.EqualFold(line, "ENABLED=yes") {
					return true
				}
			}
			return false
		case "wpcli":
			// WP-CLI is a standalone binary, not a service
			_, err := os.Stat("/usr/local/bin/wp")
			return err == nil
		case "certbot":
			// certbot is a standalone binary (no daemon), check binary + nginx plugin
			_, errBin := os.Stat("/usr/bin/certbot")
			_, errBin2 := os.Stat("/usr/local/bin/certbot")
			if errBin != nil && errBin2 != nil {
				return false
			}
			// Also check nginx plugin installed
			out, _ := exec.Command("python3", "-c", "import certbot_nginx").CombinedOutput()
			return len(out) == 0 // no output = import success
		case "php8.3-fpm":
			// Check bất kỳ PHP-FPM version nào đang active
			for _, ver := range []string{"8.3", "8.2", "8.1", "8.0", "7.4"} {
				out, _ := exec.Command("systemctl", "is-active", fmt.Sprintf("php%s-fpm", ver)).Output()
				if strings.TrimSpace(string(out)) == "active" {
					return true
				}
			}
			return false
		default:
			out, _ := exec.Command("systemctl", "is-active", svc).Output()
			return strings.TrimSpace(string(out)) == "active"
		}
	}

	services := []string{"nginx", "php8.3-fpm", "mariadb", "redis-server", "fail2ban", "ufw", "cron", "wpcli", "certbot"}
	var statuses []ServiceStatus
	for _, svc := range services {
		statuses = append(statuses, ServiceStatus{Service: svc, Active: checkActive(svc)})
	}
	result := models.AgentTaskResult{TaskID: taskID}
	if outJSON, err := json.Marshal(statuses); err == nil {
		result.Output = string(outJSON)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeInstallService(taskID uint64, service string) {
	slog.Info("Installing service", "service", service)
	result := models.AgentTaskResult{TaskID: taskID}

	// PHP install cần tải nhiều package — timeout 20 phút
	// Các service khác timeout 10 phút
	timeout := 10 * time.Minute
	if service == "php" || service == "php8.3-fpm" {
		timeout = 20 * time.Minute
	}

	done := make(chan error, 1)
	p := provisioning.New(func(step string, progress int, message string) {
		slog.Info("Install service progress", "service", service, "step", step, "message", message)
	})
	go func() {
		done <- p.InstallService(service)
	}()

	select {
	case err := <-done:
		if err != nil {
			slog.Error("Install service failed", "service", service, "error", err)
			result.Error = err.Error()
		} else {
			result.Output = "installed"
			slog.Info("Service installed successfully", "service", service)
		}
	case <-time.After(timeout):
		slog.Error("Install service timeout", "service", service, "timeout", timeout)
		result.Error = fmt.Sprintf("timeout after %s — installation may still be running in background", timeout)
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
	// Dedicated nginx config for a redirect domain
	redirectConf := fmt.Sprintf("/etc/nginx/sites-available/%s-redirect-%s.conf", task.SiteDomain, task.Domain)

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

	case "issue_ssl":
		// Run certbot --expand to add the alias domain to the primary site's certificate.
		email := task.Email
		if email == "" {
			email = "admin@" + task.SiteDomain
		}
		out, err := exec.Command("certbot", "--nginx",
			"-d", task.SiteDomain, "-d", task.Domain,
			"--expand", "--non-interactive", "--agree-tos",
			"--email", email,
		).CombinedOutput()
		if err != nil {
			result.Error = fmt.Sprintf("certbot failed: %s — %s", err.Error(), strings.TrimSpace(string(out)))
			s.reportTaskResult(result)
			return
		}
		slog.Info("SSL issued for alias domain", "domain", task.Domain, "site", task.SiteDomain)
		s.reportTaskResult(result)
		return

	case "set_redirect":
		// Remove domain from primary server_name and create a dedicated redirect server block.
		_ = nginxUpdateServerName(nginxConf, "", task.Domain) // ignore error if domain wasn't in server_name

		redirectType := task.RedirectType
		if redirectType != 302 {
			redirectType = 301
		}
		redirectConf := fmt.Sprintf("/etc/nginx/sites-available/%s-redirect-%s.conf", task.SiteDomain, task.Domain)
		content := fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %s;
    return %d %s$request_uri;
}
`, task.Domain, redirectType, task.RedirectTo)
		if err := os.WriteFile(redirectConf, []byte(content), 0644); err != nil {
			result.Error = fmt.Sprintf("failed to write redirect config: %s", err.Error())
			s.reportTaskResult(result)
			return
		}
		// Symlink into sites-enabled if not already present
		enabledLink := fmt.Sprintf("/etc/nginx/sites-enabled/%s-redirect-%s.conf", task.SiteDomain, task.Domain)
		os.Remove(enabledLink) // remove stale symlink if any
		if err := os.Symlink(redirectConf, enabledLink); err != nil && !os.IsExist(err) {
			result.Error = fmt.Sprintf("failed to enable redirect config: %s", err.Error())
			s.reportTaskResult(result)
			return
		}

	case "remove_redirect":
		// Delete the redirect config and re-add domain to primary server_name.
		os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s-redirect-%s.conf", task.SiteDomain, task.Domain))
		os.Remove(redirectConf)
		// Re-add to primary server_name
		_ = nginxUpdateServerName(nginxConf, task.Domain, "")

	default:
		result.Error = fmt.Sprintf("unknown domain action: %s", task.Action)
		s.reportTaskResult(result)
		return
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

	// NOTE: keys MUST match what RebuildPHPFPMPool reads (max_children,
	// memory_limit, request_terminate_timeout). Previously these were
	// mis-keyed (pm_max_children/php_memory_limit) so the rebuild silently
	// fell back to defaults — fixed here.
	configData := map[string]string{
		"max_children":  fmt.Sprintf("%d", task.PoolConfig.MaxChildren),
		"memory_limit":  task.PoolConfig.MemoryLimit,
	}
	if task.PoolConfig.RequestTerminateTimeout > 0 {
		configData["request_terminate_timeout"] = fmt.Sprintf("%d", task.PoolConfig.RequestTerminateTimeout)
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

	versions := []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6"}
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

// executePHPUpdateStatus reports installed vs. available version per PHP branch.
func (s *AgentServer) executePHPUpdateStatus(taskID uint64, refresh bool) {
	result := models.AgentTaskResult{TaskID: taskID}
	status, err := agentphpfpm.UpdateStatus(refresh)
	if status == nil && err != nil {
		result.Error = err.Error()
		s.reportTaskResult(result)
		return
	}
	if err != nil {
		// Package lists could not be refreshed; the cached view is still useful.
		slog.Warn("PHP update check used cached package lists", "error", err)
	}
	out, _ := json.Marshal(status)
	result.Output = string(out)
	s.reportTaskResult(result)
}

// phpUpdateMu serialises PHP package updates: apt holds a global lock anyway,
// and two branches restarting at once doubles the blast radius.
var phpUpdateMu sync.Mutex

// executeUpdatePHPVersion applies the patch update of one installed branch.
func (s *AgentServer) executeUpdatePHPVersion(taskID uint64, task models.AgentPHPVersionTask) {
	result := models.AgentTaskResult{TaskID: taskID}
	phpUpdateMu.Lock()
	defer phpUpdateMu.Unlock()

	slog.Info("Updating PHP branch", "version", task.Version)
	msg, err := agentphpfpm.UpdateBranch(task.Version)
	if err != nil {
		slog.Error("PHP update failed", "version", task.Version, "error", err)
		result.Error = err.Error()
	} else {
		result.Output = msg
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeInstallPHPVersion(taskID uint64, task models.AgentPHPVersionTask) {
	slog.Info("Installing PHP version", "version", task.Version)
	result := models.AgentTaskResult{TaskID: taskID}

	// The version ends up in package names, paths and unit names.
	if !models.ValidPHPVersion(task.Version) {
		result.Error = fmt.Sprintf("unsupported PHP version %q", task.Version)
		s.reportTaskResult(result)
		return
	}

	phpFpmDir := fmt.Sprintf("/etc/php/%s/fpm/pool.d", task.Version)
	if _, err := os.Stat(phpFpmDir); err == nil {
		// Already installed
		result.Output = fmt.Sprintf("PHP %s is already installed", task.Version)
		s.reportTaskResult(result)
		return
	}

	v := task.Version

	// Required packages — must all install successfully.
	requiredPkgs := []string{
		"php" + v + "-fpm", "php" + v + "-cli", "php" + v + "-common",
		"php" + v + "-mysql", "php" + v + "-xml", "php" + v + "-curl",
		"php" + v + "-gd", "php" + v + "-mbstring", "php" + v + "-zip",
		"php" + v + "-bcmath", "php" + v + "-intl", "php" + v + "-soap",
	}
	// Optional packages — silently skip if not available for this version.
	// php{X}-opcache was merged into php{X}-common starting in PHP 8.5.
	// php{X}-redis and php{X}-imagick depend on PECL build availability.
	optionalPkgs := []string{
		"php" + v + "-opcache",
		"php" + v + "-redis",
		"php" + v + "-imagick",
	}

	args := append([]string{"install", "-y"}, requiredPkgs...)
	if out, err := exec.Command("apt-get", args...).CombinedOutput(); err != nil {
		slog.Error("Failed to install PHP", "version", v, "error", err, "output", string(out))
		result.Error = fmt.Sprintf("failed to install php%s: %v\n%s", v, err, string(out))
		s.reportTaskResult(result)
		return
	}

	// Install optional packages one by one — skip those not in repo
	var skipped []string
	for _, pkg := range optionalPkgs {
		if chk, _ := exec.Command("apt-cache", "show", pkg).CombinedOutput(); !strings.Contains(string(chk), "Package:") {
			skipped = append(skipped, pkg)
			continue
		}
		if out, err := exec.Command("apt-get", "install", "-y", pkg).CombinedOutput(); err != nil {
			slog.Warn("Optional PHP package skipped", "pkg", pkg, "error", string(out))
			skipped = append(skipped, pkg)
		}
	}

	exec.Command("systemctl", "enable", "php"+v+"-fpm").Run()
	exec.Command("systemctl", "start", "php"+v+"-fpm").Run()

	msg := fmt.Sprintf("PHP %s installed successfully", v)
	if len(skipped) > 0 {
		msg += fmt.Sprintf(" (skipped optional: %s)", strings.Join(skipped, ", "))
	}
	result.Output = msg
	slog.Info("PHP version installed successfully", "version", v, "skipped_optional", skipped)
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
			"php_error":   {"/var/log/php8.3-fpm.log", "/var/log/php8.2-fpm.log", "/var/log/php8.1-fpm.log"},
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
	opts := agentsecurity.HardenOptions{
		DisableXMLRPC:      task.DisableXMLRPC,
		HideWPVersion:      task.HideWPVersion,
		SecureFilePerms:    task.SecureFilePerms,
		SecurityHeaders:    task.SecurityHeaders,
		DisableFileEditor:  task.DisableFileEditor,
		LimitLoginAttempts: task.LimitLoginAttempts,
	}

	var err error
	if task.Revert {
		slog.Info("Reverting WordPress hardening", "domain", task.Domain)
		err = agentsecurity.RevertHardenWordPress(task.Domain, opts)
	} else {
		slog.Info("Hardening WordPress", "domain", task.Domain)
		err = agentsecurity.HardenWordPress(task.Domain, opts)
	}

	result := models.AgentTaskResult{TaskID: taskID}
	if err != nil {
		slog.Error("WordPress harden task failed", "domain", task.Domain, "revert", task.Revert, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("WordPress harden task completed", "domain", task.Domain, "revert", task.Revert)
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

func (s *AgentServer) completeTaskWithOutput(taskID uint64, status, errMsg, output string) {
	result := models.AgentTaskResult{TaskID: taskID, Output: output}
	if errMsg != "" {
		result.Error = errMsg
	}
	s.reportTaskResult(result)
}

// executeScanDiskUsage scans file + DB size for all listed sites in one pass
// and returns the result as a JSON array in task.Output.
func (s *AgentServer) executeScanDiskUsage(taskID uint64, task models.AgentDiskScanTask) {
	slog.Info("Scanning disk usage", "sites", len(task.Sites))
	results := agentwp.ScanDiskUsage(task.Sites)
	data, err := json.Marshal(results)
	if err != nil {
		s.completeTask(taskID, "failed", err.Error())
		return
	}
	s.completeTaskWithOutput(taskID, "completed", "", string(data))
}

// executeSetResourceLimit applies or removes a per-site cgroup CPU/RAM limit
// (dedicated PHP-FPM service). Serialized per-domain to avoid racing with
// create/delete/php-version changes on the same site.
func (s *AgentServer) executeSetResourceLimit(taskID uint64, task models.AgentResourceLimitTask) {
	unlock := s.lockDomain(task.Domain)
	defer unlock()

	slog.Info("Setting resource limit", "domain", task.Domain, "action", task.Action,
		"cpu_pct", task.CPUQuotaPercent, "mem_mb", task.MemoryMaxMB)

	var err error
	switch task.Action {
	case "apply":
		err = agentphpfpm.ApplyResourceLimit(task.Domain, task.PHPVersion, task.CPUQuotaPercent, task.MemoryMaxMB)
	case "remove":
		err = agentphpfpm.RemoveResourceLimit(task.Domain, task.PHPVersion)
	default:
		err = fmt.Errorf("unknown action: %s", task.Action)
	}

	if err != nil {
		s.completeTask(taskID, "failed", err.Error())
		return
	}
	s.completeTask(taskID, "completed", "")
}

func (s *AgentServer) executeWPCLI(taskID uint64, task models.AgentWPCLITask) {
	slog.Info("Executing WP-CLI command", "domain", task.Domain, "command", task.Command, "php", task.PHPVersion)
	result, err := agentwp.ExecuteWPCLI(task.Domain, task.PHPVersion, task.Command)

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
		result.Output = string(statusJSON)
		slog.Info("Fail2ban status retrieved", "data_size", len(statusJSON))
	}
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

func (s *AgentServer) executeFail2BanToggleService(taskID uint64, enable bool) {
	action := "stop"
	if enable {
		action = "start"
	}
	slog.Info("Toggling fail2ban service", "action", action)
	result := models.AgentTaskResult{TaskID: taskID}

	if err := agentfail2ban.ToggleService(enable); err != nil {
		slog.Error("Fail2ban toggle service failed", "enable", enable, "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Fail2ban service toggled", "action", action)
		result.Output = fmt.Sprintf(`{"service_running":%v}`, enable)
	}
	s.reportTaskResult(result)
}

func (s *AgentServer) executeFail2BanSetupJails(taskID uint64) {
	slog.Info("Setting up default fail2ban jails (nginx + wordpress)")
	result := models.AgentTaskResult{TaskID: taskID}

	if err := agentfail2ban.SetupDefaultJails(); err != nil {
		slog.Error("Fail2ban setup jails failed", "error", err)
		result.Error = err.Error()
	} else {
		slog.Info("Fail2ban default jails configured and reloaded")
		result.Output = `{"ok":true}`
	}
	s.reportTaskResult(result)
}

// executeAddAuthorizedKey adds a temporary SSH public key to /root/.ssh/authorized_keys.
// Used during migration to grant source server SSH access to this (target) server.
func (s *AgentServer) executeAddAuthorizedKey(taskID uint64, publicKey, comment string) {
	result := models.AgentTaskResult{TaskID: taskID}

	if publicKey == "" {
		result.Error = "public_key is empty"
		s.reportTaskResult(result)
		return
	}

	sshDir := "/root/.ssh"
	authFile := sshDir + "/authorized_keys"

	// Ensure ~/.ssh directory exists with correct permissions
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		result.Error = "failed to create .ssh dir: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	// Append key with comment (for later removal)
	line := strings.TrimSpace(publicKey)
	if comment != "" {
		// Check if comment is already appended in the key itself
		if !strings.Contains(line, comment) {
			line = line + " " + comment
		}
	}
	line += "\n"

	f, err := os.OpenFile(authFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		result.Error = "failed to open authorized_keys: " + err.Error()
		s.reportTaskResult(result)
		return
	}
	f.WriteString(line)
	f.Close()

	slog.Info("Added temporary SSH key", "comment", comment)
	result.Output = "ok"
	s.reportTaskResult(result)
}

// executeRemoveAuthorizedKey removes lines containing the given comment from authorized_keys.
// Used after migration to revoke the temporary SSH access.
func (s *AgentServer) executeRemoveAuthorizedKey(taskID uint64, comment string) {
	result := models.AgentTaskResult{TaskID: taskID}

	if comment == "" {
		result.Output = "ok (no comment provided, skipped)"
		s.reportTaskResult(result)
		return
	}

	authFile := "/root/.ssh/authorized_keys"
	content, err := os.ReadFile(authFile)
	if err != nil {
		// File doesn't exist — nothing to remove
		result.Output = "ok (file not found)"
		s.reportTaskResult(result)
		return
	}

	var kept []string
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, comment) {
			kept = append(kept, line)
		}
	}

	if err := os.WriteFile(authFile, []byte(strings.Join(kept, "\n")), 0600); err != nil {
		result.Error = "failed to write authorized_keys: " + err.Error()
		s.reportTaskResult(result)
		return
	}

	slog.Info("Removed temporary SSH key", "comment", comment)
	result.Output = "ok"
	s.reportTaskResult(result)
}

// ── Transfer Tool handlers ────────────────────────────────────────────────────

// executeTransferDiscover connects to the source server via SSH, detects the panel,
// discovers all WordPress sites, and POSTs the result back to the API.
func (s *AgentServer) executeTransferDiscover(taskID uint64, task models.AgentTransferDiscoverTask) {
	slog.Info("Transfer discover started", "job_id", task.JobID, "source", task.SourceHost, "panel", task.PanelType)

	// Get list of PHP versions installed on THIS (target) server
	targetPHP := listInstalledPHPVersionsLocal()

	// Run discovery in the runner package
	agenttransfer.RunDiscovery(task, s.apiURL, s.apiKey, targetPHP)

	// Mark task complete
	s.reportTaskResult(models.AgentTaskResult{
		TaskID: taskID,
		Status: "completed",
		Output: "discovery done",
	})
}

// executeTransferImport pulls one WordPress site from the source server and imports it.
func (s *AgentServer) executeTransferImport(taskID uint64, task models.AgentTransferImportTask) {
	slog.Info("Transfer import started",
		"job_id", task.JobID, "result_id", task.ResultID,
		"source_domain", task.Site.PrimaryDomain, "target_domain", task.TargetDomain)

	// Use per-domain mutex to prevent concurrent operations on the same site
	unlock := s.lockDomain(task.TargetDomain)
	defer unlock()

	runner, err := agenttransfer.NewRunner(task, s.apiURL, s.apiKey)
	if err != nil {
		slog.Error("Transfer runner init failed", "error", err)
		s.reportTaskResult(models.AgentTaskResult{TaskID: taskID, Status: "failed", Error: err.Error()})
		return
	}

	runner.Run()

	s.reportTaskResult(models.AgentTaskResult{
		TaskID: taskID,
		Status: "completed",
		Output: "import done",
	})
}

// listInstalledPHPVersionsLocal returns PHP version strings installed on the local agent VPS.
func listInstalledPHPVersionsLocal() []string {
	versions := []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6"}
	var installed []string
	for _, v := range versions {
		fpmDir := fmt.Sprintf("/etc/php/%s/fpm", v)
		if _, err := os.Stat(fpmDir); err == nil {
			installed = append(installed, v)
		}
	}
	return installed
}
