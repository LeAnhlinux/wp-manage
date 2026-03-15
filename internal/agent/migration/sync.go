package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

type MigrationRunner struct {
	apiURL string
	apiKey string
}

func NewMigrationRunner(apiURL, apiKey string) *MigrationRunner {
	return &MigrationRunner{apiURL: apiURL, apiKey: apiKey}
}

// RunMigration executes a full VPS-to-VPS WordPress migration.
//
// Two modes:
//   - keep_domain: suspend source → migrate → auto DNS update → disable source nginx
//   - change_domain: source stays running → migrate → wp search-replace old→new domain
//
// SSH access between servers is established automatically via a temporary key pair:
//  1. Source generates a temp ed25519 key
//  2. Calls API to add pubkey to target's authorized_keys
//  3. Uses the key for all rsync/scp/ssh operations
//  4. Calls API to remove the key after cleanup
func (r *MigrationRunner) RunMigration(task models.AgentMigrateTask) {
	slog.Info("Starting migration", "domain", task.Domain, "target", task.TargetServerIP, "mode", task.Mode)

	siteRoot := fmt.Sprintf("/var/www/%s", task.Domain)
	dumpFile := fmt.Sprintf("/tmp/migration_%d_db.sql", task.MigrationID)
	keyFile := fmt.Sprintf("/tmp/migration_%d_key", task.MigrationID)

	// Resolve linux user for chown (per-site isolation).
	// NEVER fall back to www-data — derive from domain if not set.
	linuxUser := task.LinuxUser
	if linuxUser == "" {
		sanitized := strings.NewReplacer(".", "_", "-", "_").Replace(task.Domain)
		linuxUser = "wp_" + sanitized
	}

	// Effective domain/root on target
	targetDomain := task.Domain
	if task.Mode == "change_domain" && task.NewDomain != "" {
		targetDomain = task.NewDomain
	}
	targetSiteRoot := fmt.Sprintf("/var/www/%s", targetDomain)

	// ── PHASE 0: PRE-FLIGHT CHECKS ────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "preparation", 2, "running", "Kiểm tra tính tương thích...")
	sslType := detectSSLType(task.Domain)
	r.reportProgress(task.MigrationID, "preparation", 3, "running",
		fmt.Sprintf("Phát hiện SSL: %s | PHP: %s", sslTypeLabel(sslType), task.PHPVersion))

	// ── PHASE 1: PREPARATION ──────────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "preparation", 5, "running", "Tạo SSH key tạm thời để kết nối server đích...")

	// Generate temp SSH key pair for server-to-server communication
	os.Remove(keyFile)
	os.Remove(keyFile + ".pub")
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyFile, "-N", "", "-q").CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "preparation", fmt.Sprintf("ssh-keygen thất bại: %s %v", string(out), err))
		return
	}
	pubKey, err := os.ReadFile(keyFile + ".pub")
	if err != nil {
		r.reportFailed(task.MigrationID, "preparation", "Đọc public key thất bại: "+err.Error())
		return
	}

	// Register the public key on the target server via API
	r.reportProgress(task.MigrationID, "preparation", 8, "running", "Cấp quyền SSH trên server đích...")
	if err := r.setupSSHAccess(task.MigrationID, string(pubKey)); err != nil {
		os.Remove(keyFile)
		os.Remove(keyFile + ".pub")
		r.reportFailed(task.MigrationID, "preparation", "Không thể cấp quyền SSH trên server đích: "+err.Error())
		return
	}
	r.reportProgress(task.MigrationID, "preparation", 12, "running", "Kết nối SSH tới server đích thành công")

	// Check PHP version compatibility on target
	phpCheckCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s 'php%s -v >/dev/null 2>&1 && echo OK || echo NOT_FOUND'",
		keyFile, task.TargetServerIP, task.PHPVersion)
	phpCheckOut, _ := exec.Command("bash", "-c", phpCheckCmd).Output()
	if strings.Contains(string(phpCheckOut), "NOT_FOUND") {
		r.reportProgress(task.MigrationID, "preparation", 13, "running",
			fmt.Sprintf("⚠️  CẢNH BÁO: PHP %s chưa được cài trên server đích. Site có thể không hoạt động sau migrate. Hãy cài PHP %s trước!", task.PHPVersion, task.PHPVersion))
	} else {
		r.reportProgress(task.MigrationID, "preparation", 13, "running",
			fmt.Sprintf("✓ PHP %s có trên server đích", task.PHPVersion))
	}

	// Always clean up SSH key when done (deferred)
	defer func() {
		os.Remove(keyFile)
		os.Remove(keyFile + ".pub")
		r.cleanupSSHAccess(task.MigrationID)
	}()

	// SSH/rsync options using temp key
	sshOpts := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o BatchMode=yes -i %s", keyFile)

	// Backup DB
	r.reportProgress(task.MigrationID, "preparation", 15, "running", "Tạo backup database nguồn...")
	dumpCmd := exec.Command("mysqldump",
		"--user="+task.DBUser,
		"--password="+task.DBPass,
		"--single-transaction",
		"--quick",
		"--lock-tables=false",
		task.DBName,
	)
	dumpOut, err := dumpCmd.Output()
	if err != nil {
		r.reportFailed(task.MigrationID, "preparation", "mysqldump thất bại: "+err.Error())
		return
	}
	if err := os.WriteFile(dumpFile, dumpOut, 0600); err != nil {
		r.reportFailed(task.MigrationID, "preparation", "Ghi dump file thất bại: "+err.Error())
		return
	}
	defer os.Remove(dumpFile)
	r.reportProgress(task.MigrationID, "preparation", 20, "running", "Backup database hoàn tất")

	// ── PHASE 2: INITIAL SYNC ─────────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "initial_sync", 25, "running", "Đồng bộ files lên server đích (lần 1)...")

	rsyncArgs := []string{
		"-rltvz", "--delete",
		"--no-owner", "--no-group", // don't copy UID/GID from source; chown explicitly after
		"-e", sshOpts,
		siteRoot + "/",
		fmt.Sprintf("root@%s:%s/", task.TargetServerIP, targetSiteRoot),
	}
	if out, err := exec.Command("rsync", rsyncArgs...).CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "initial_sync", fmt.Sprintf("rsync thất bại: %s", string(out)))
		return
	}
	r.reportProgress(task.MigrationID, "initial_sync", 42, "running", "Đồng bộ files hoàn tất")

	// Transfer DB dump to target using temp key
	scpArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=30",
		"-i", keyFile,
		dumpFile,
		fmt.Sprintf("root@%s:%s", task.TargetServerIP, dumpFile),
	}
	if out, err := exec.Command("scp", scpArgs...).CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "initial_sync", fmt.Sprintf("SCP database thất bại: %s", string(out)))
		return
	}
	r.reportProgress(task.MigrationID, "initial_sync", 50, "running", "Transfer database hoàn tất")

	// ── PHASE 3: DELTA SYNC ───────────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "delta_sync", 55, "running", "Đồng bộ lần 2 (delta sync)...")
	exec.Command("rsync", rsyncArgs...).Run() // catch files changed during initial sync
	r.reportProgress(task.MigrationID, "delta_sync", 65, "running", "Delta sync hoàn tất")

	// ── PHASE 4: SWITCHOVER ───────────────────────────────────────────────────
	// 4a: Suspend source site (keep_domain only)
	if task.Mode == "keep_domain" {
		r.reportProgress(task.MigrationID, "switchover", 67, "running", "Tạm dừng site trên server nguồn...")
		r.suspendSite(task.Domain)
	}

	// 4b: Create Linux user on target (per-site isolation)
	r.reportProgress(task.MigrationID, "switchover", 70, "running", "Chuẩn bị môi trường server đích...")
	createUserCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s 'id %s 2>/dev/null || useradd -r -M -s /usr/sbin/nologin %s'",
		keyFile, task.TargetServerIP, linuxUser, linuxUser)
	exec.Command("bash", "-c", createUserCmd).Run()

	// 4c: Create DB + user on target.
	// Write SQL to a temp file and SCP it — avoids all SSH quoting issues with passwords.
	r.reportProgress(task.MigrationID, "switchover", 72, "running", "Tạo database và user trên server đích...")
	setupSQLFile := fmt.Sprintf("/tmp/migration_%d_setup.sql", task.MigrationID)
	setupSQL := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
			"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';\n"+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';\n"+
			"FLUSH PRIVILEGES;\n",
		task.DBName, task.DBUser, task.DBPass, task.DBName, task.DBUser)
	if err := os.WriteFile(setupSQLFile, []byte(setupSQL), 0600); err == nil {
		// SCP setup SQL to target
		scpSetup := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile,
			setupSQLFile, fmt.Sprintf("root@%s:%s", task.TargetServerIP, setupSQLFile)}
		exec.Command("scp", scpSetup...).Run()
		// Execute on target using mysql root (linux root → MariaDB root via unix socket, no password)
		execSetup := fmt.Sprintf(
			"ssh -o StrictHostKeyChecking=no -i %s root@%s 'mysql -u root < %s; rm -f %s'",
			keyFile, task.TargetServerIP, setupSQLFile, setupSQLFile)
		exec.Command("bash", "-c", execSetup).Run()
		os.Remove(setupSQLFile)
	}

	// 4d: Import DB using mysql root (site user may not have sufficient privileges yet)
	r.reportProgress(task.MigrationID, "switchover", 75, "running", "Nhập database trên server đích...")
	importCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s 'mysql -u root %s < %s 2>&1'",
		keyFile, task.TargetServerIP, task.DBName, dumpFile)
	if out, err := exec.Command("bash", "-c", importCmd).CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "switchover", fmt.Sprintf("Import database thất bại: %s", string(out)))
		if task.Mode == "keep_domain" {
			r.unsuspendSite(task.Domain)
		}
		return
	}
	r.reportProgress(task.MigrationID, "switchover", 82, "running", "Import database hoàn tất")

	// 4e: wp search-replace (change_domain mode)
	if task.Mode == "change_domain" && task.NewDomain != "" {
		r.reportProgress(task.MigrationID, "switchover", 85, "running",
			fmt.Sprintf("Thay thế domain %s → %s...", task.Domain, task.NewDomain))
		srCmd := fmt.Sprintf(
			`ssh -o StrictHostKeyChecking=no -i %s root@%s 'cd %s && wp search-replace "%s" "%s" --all-tables --allow-root 2>&1'`,
			keyFile, task.TargetServerIP, targetSiteRoot, task.Domain, task.NewDomain)
		if out, err := exec.Command("bash", "-c", srCmd).CombinedOutput(); err != nil {
			r.reportFailed(task.MigrationID, "switchover",
				fmt.Sprintf("wp search-replace thất bại: %s", string(out)))
			return
		}
		r.reportProgress(task.MigrationID, "switchover", 88, "running",
			fmt.Sprintf("Thay thế domain %s → %s hoàn tất", task.Domain, task.NewDomain))
	}

	// 4f: Fix ownership on target (per-site linux user)
	r.reportProgress(task.MigrationID, "switchover", 90, "running", "Thiết lập quyền file trên server đích...")
	chownCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s 'chown -R %s:%s %s'",
		keyFile, task.TargetServerIP, linuxUser, linuxUser, targetSiteRoot)
	exec.Command("bash", "-c", chownCmd).Run()
	r.reportProgress(task.MigrationID, "switchover", 92, "running", "Thiết lập quyền file hoàn tất")

	// 4g: Copy Nginx + ModSec + PHP-FPM configs to target and enable site
	r.reportProgress(task.MigrationID, "switchover", 94, "running", "Cấu hình Nginx, PHP-FPM trên server đích...")
	r.setupTargetConfigs(task, keyFile, targetDomain, linuxUser)
	r.reportProgress(task.MigrationID, "switchover", 97, "running", "Nginx + PHP-FPM trên server đích đã sẵn sàng")

	// ── PHASE 5: CLEANUP ─────────────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "cleanup", 98, "running", "Dọn dẹp files tạm trên server đích...")

	cleanDumpCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s 'rm -f %s'",
		keyFile, task.TargetServerIP, dumpFile)
	exec.Command("bash", "-c", cleanDumpCmd).Run()

	// For keep_domain: disable source nginx (site has moved to target)
	if task.Mode == "keep_domain" {
		disableCmd := fmt.Sprintf(
			"rm -f /etc/nginx/sites-enabled/%s.conf && nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
			task.Domain)
		exec.Command("bash", "-c", disableCmd).Run()
		r.reportProgress(task.MigrationID, "cleanup", 99, "running",
			"Đã vô hiệu hóa site trên server nguồn. Đang xóa SSH key tạm...")
	}

	// Optional: clean up all source artifacts (files, DB, configs)
	if task.CleanupSource {
		r.cleanupSourceSite(task)
	}

	// defer will handle: os.Remove(keyFile), os.Remove(keyFile+".pub"), r.cleanupSSHAccess()
	r.reportProgress(task.MigrationID, "cleanup", 100, "completed",
		"Migration hoàn tất thành công!")
	slog.Info("Migration completed", "domain", task.Domain, "target", task.TargetServerIP, "mode", task.Mode)
}

// RollbackMigration reverses a migration.
//   - keep_domain: re-enables source nginx (source DB/files were never modified).
//   - change_domain: source site was never suspended — just mark as rolled_back.
func (r *MigrationRunner) RollbackMigration(migrationID uint64, domain, mode string) {
	slog.Info("Rolling back migration", "id", migrationID, "domain", domain, "mode", mode)
	if mode == "keep_domain" {
		r.reportProgress(migrationID, "cleanup", 30, "running", "Khôi phục site trên server nguồn...")
		r.unsuspendSite(domain)
		r.reportProgress(migrationID, "cleanup", 100, "rolled_back",
			"Rollback hoàn tất. Site đã hoạt động trở lại trên server nguồn.")
	} else {
		r.reportProgress(migrationID, "cleanup", 100, "rolled_back",
			"Rollback ghi nhận. Site nguồn vẫn đang chạy bình thường với domain cũ.")
	}
}

// CleanupTargetOnRollback runs on the TARGET server when a migration is rolled back.
// Removes all partially migrated data: files, DB, nginx/php-fpm configs, SSL cert.
func (r *MigrationRunner) CleanupTargetOnRollback(task models.AgentMigrateTask) {
	r.reportProgress(task.MigrationID, "cleanup", 20, "running",
		fmt.Sprintf("Đang dọn dẹp dữ liệu đã migrate trên server đích (%s)...", task.Domain))
	r.cleanupSourceSite(task) // same cleanup logic, runs locally
	r.reportProgress(task.MigrationID, "cleanup", 100, "rolled_back",
		fmt.Sprintf("✓ Đã xóa toàn bộ dữ liệu đã migrate của %s khỏi server đích", task.Domain))
}

// CleanupSourceOnly is called when the user triggers "dọn dẹp server nguồn" from the UI
// after a completed migration. Reports progress back to API.
func (r *MigrationRunner) CleanupSourceOnly(task models.AgentMigrateTask) {
	r.reportProgress(task.MigrationID, "cleanup", 50, "running",
		fmt.Sprintf("Bắt đầu dọn dẹp server nguồn cho site %s...", task.Domain))
	r.cleanupSourceSite(task)
	r.reportProgress(task.MigrationID, "cleanup", 100, "completed",
		fmt.Sprintf("✓ Đã dọn dẹp hoàn tất server nguồn — site %s đã được xóa khỏi server cũ", task.Domain))
}

// cleanupSourceSite removes all site artifacts from the SOURCE server after a
// successful migration when CleanupSource=true. This is irreversible.
// Order: nginx config → PHP-FPM pool → site files → DB + user
func (r *MigrationRunner) cleanupSourceSite(task models.AgentMigrateTask) {
	domain := task.Domain
	r.reportProgress(task.MigrationID, "cleanup", 99, "running",
		fmt.Sprintf("Đang xóa toàn bộ dữ liệu site %s trên server nguồn...", domain))

	// 1. Remove nginx sites-available config (symlink already removed above)
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain))

	// 2. Remove ModSecurity config
	os.Remove(fmt.Sprintf("/etc/nginx/modsec/%s.conf", domain))

	// Reload nginx after removing configs
	exec.Command("bash", "-c", "nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true").Run()

	// 3. Remove PHP-FPM pool and reload
	for _, ver := range []string{"8.3", "8.2", "8.1", "8.0", "7.4"} {
		poolPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, domain)
		if _, err := os.Stat(poolPath); err == nil {
			os.Remove(poolPath)
			exec.Command("bash", "-c",
				fmt.Sprintf("php%s-fpm -t 2>/dev/null && systemctl reload php%s-fpm 2>/dev/null || true", ver, ver),
			).Run()
			break
		}
	}

	// 4. Remove site files
	os.RemoveAll(fmt.Sprintf("/var/www/%s", domain))

	// 5. Drop DB and user (use mysql root — no password via unix socket)
	dropSQL := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'localhost'; FLUSH PRIVILEGES;",
		task.DBName, task.DBUser)
	exec.Command("mysql", "-u", "root", "-e", dropSQL).Run()

	// 6. Remove Let's Encrypt cert if present
	exec.Command("certbot", "delete", "--cert-name", domain, "--non-interactive").Run()

	r.reportProgress(task.MigrationID, "cleanup", 99, "running",
		fmt.Sprintf("✓ Đã xóa toàn bộ dữ liệu site %s khỏi server nguồn", domain))
}

// setupSSHAccess calls the central API to add the temp public key to the target server's authorized_keys.
// This is synchronous — the API queues a task on the target agent and waits for completion.
func (r *MigrationRunner) setupSSHAccess(migrationID uint64, publicKey string) error {
	payload, _ := json.Marshal(map[string]string{"public_key": publicKey})
	url := fmt.Sprintf("%s/api/v1/agent/migrations/%d/setup-ssh", r.apiURL, migrationID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", r.apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// cleanupSSHAccess calls the central API to remove the temp SSH key from the target server.
// Fire-and-forget — errors are logged but not returned.
func (r *MigrationRunner) cleanupSSHAccess(migrationID uint64) {
	payload, _ := json.Marshal(map[string]string{})
	url := fmt.Sprintf("%s/api/v1/agent/migrations/%d/cleanup-ssh", r.apiURL, migrationID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", r.apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("cleanup-ssh API call failed", "migration_id", migrationID, "error", err)
		return
	}
	resp.Body.Close()
}

// setupTargetConfigs copies Nginx, ModSecurity, PHP-FPM configs and SSL certs from source
// to target, then enables the nginx site. Handles SSL: custom (copy certs), letsencrypt
// (try certbot on target; fall back to HTTP-only if it fails), none (skip SSL).
func (r *MigrationRunner) setupTargetConfigs(task models.AgentMigrateTask, keyFile, targetDomain, linuxUser string) {
	sshBase := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s", keyFile, task.TargetServerIP)
	scpBase := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile}

	sslType := detectSSLType(task.Domain)
	isChangeDomain := task.Mode == "change_domain" && task.NewDomain != ""

	// ── Nginx config ──────────────────────────────────────────────────────────
	nginxSrcConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", task.Domain)
	nginxDstConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", targetDomain)

	if _, err := os.Stat(nginxSrcConf); err == nil {
		exec.Command("bash", "-c", fmt.Sprintf("%s 'mkdir -p /etc/nginx/sites-available'", sshBase)).Run()

		raw, err := os.ReadFile(nginxSrcConf)
		if err == nil {
			// Always sanitize server_name: keep only targetDomain and www.targetDomain.
			// This prevents domain aliases from source polluting target and conflicting
			// with other sites already hosted there.
			sanitized := sanitizeNginxServerName(raw, targetDomain)
			if isChangeDomain {
				sanitized = bytes.ReplaceAll(sanitized, []byte(task.Domain), []byte(task.NewDomain))
			}
			tmpConf := fmt.Sprintf("/tmp/migration_%d_nginx.conf", task.MigrationID)
			if os.WriteFile(tmpConf, sanitized, 0644) == nil {
				scpArgs := append(scpBase, tmpConf, fmt.Sprintf("root@%s:%s", task.TargetServerIP, nginxDstConf))
				exec.Command("scp", scpArgs...).Run()
				os.Remove(tmpConf)
			}
		}
	}

	// ── ModSecurity config ────────────────────────────────────────────────────
	modsecSrcConf := fmt.Sprintf("/etc/nginx/modsec/%s.conf", task.Domain)
	modsecDstConf := fmt.Sprintf("/etc/nginx/modsec/%s.conf", targetDomain)
	if _, err := os.Stat(modsecSrcConf); err == nil {
		exec.Command("bash", "-c", fmt.Sprintf("%s 'mkdir -p /etc/nginx/modsec'", sshBase)).Run()
		if isChangeDomain {
			tmpModsec := fmt.Sprintf("/tmp/migration_%d_modsec.conf", task.MigrationID)
			if raw, err := os.ReadFile(modsecSrcConf); err == nil {
				replaced := bytes.ReplaceAll(raw, []byte(task.Domain), []byte(task.NewDomain))
				if os.WriteFile(tmpModsec, replaced, 0644) == nil {
					scpArgs := append(scpBase, tmpModsec, fmt.Sprintf("root@%s:%s", task.TargetServerIP, modsecDstConf))
					exec.Command("scp", scpArgs...).Run()
					os.Remove(tmpModsec)
				}
			}
		} else {
			scpArgs := append(scpBase, modsecSrcConf, fmt.Sprintf("root@%s:%s", task.TargetServerIP, modsecDstConf))
			exec.Command("scp", scpArgs...).Run()
		}
	}

	// ── PHP-FPM pool config ───────────────────────────────────────────────────
	phpVersions := []string{"8.3", "8.2", "8.1", "8.0", "7.4"}
	for _, ver := range phpVersions {
		phpSrcPool := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, task.Domain)
		if _, err := os.Stat(phpSrcPool); err != nil {
			continue
		}
		phpDstPool := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, targetDomain)
		exec.Command("bash", "-c", fmt.Sprintf("%s 'mkdir -p /etc/php/%s/fpm/pool.d'", sshBase, ver)).Run()
		if isChangeDomain {
			tmpPool := fmt.Sprintf("/tmp/migration_%d_phpfpm.conf", task.MigrationID)
			if raw, err := os.ReadFile(phpSrcPool); err == nil {
				replaced := bytes.ReplaceAll(raw, []byte(task.Domain), []byte(task.NewDomain))
				if os.WriteFile(tmpPool, replaced, 0644) == nil {
					scpArgs := append(scpBase, tmpPool, fmt.Sprintf("root@%s:%s", task.TargetServerIP, phpDstPool))
					exec.Command("scp", scpArgs...).Run()
					os.Remove(tmpPool)
				}
			}
		} else {
			scpArgs := append(scpBase, phpSrcPool, fmt.Sprintf("root@%s:%s", task.TargetServerIP, phpDstPool))
			exec.Command("scp", scpArgs...).Run()
		}
		exec.Command("bash", "-c", fmt.Sprintf(
			"%s 'php%s-fpm -t 2>/dev/null && systemctl reload php%s-fpm 2>/dev/null || true'",
			sshBase, ver, ver)).Run()
		break
	}

	// ── SSL handling + Enable Nginx site on target ───────────────────────────
	//
	// IMPORTANT ordering:
	//  - For letsencrypt: we MUST activate the site with HTTP-only config first,
	//    then run certbot. certbot --nginx needs a working HTTP vhost to complete
	//    the HTTP-01 ACME challenge. If we copy the source's SSL config (which
	//    references /etc/letsencrypt/live/... paths that don't exist on target yet),
	//    nginx -t fails → nginx never reloads → certbot can't serve the challenge →
	//    certbot fails → 404.
	//
	//  - For custom SSL: copy certs first, then enable site (certs exist before nginx loads).
	//  - For none: just enable site.

	// Helper: enable site in nginx (create symlink, test config, reload).
	// Returns nginx -t output for diagnostics.
	enableSite := func() string {
		out, _ := exec.Command("bash", "-c", fmt.Sprintf(
			"%s 'mkdir -p /etc/nginx/sites-enabled && ln -sf %s /etc/nginx/sites-enabled/%s.conf && nginx -t 2>&1 && systemctl reload nginx 2>/dev/null; echo EXIT:$?'",
			sshBase, nginxDstConf, targetDomain)).CombinedOutput()
		return string(out)
	}

	switch sslType {
	case "custom":
		// Copy custom cert files to target (keep_domain only; change_domain has no valid cert for new domain)
		if !isChangeDomain {
			customSSLSrc := fmt.Sprintf("/etc/nginx/ssl/%s", task.Domain)
			customSSLDst := fmt.Sprintf("/etc/nginx/ssl/%s", targetDomain)
			if _, err := os.Stat(customSSLSrc); err == nil {
				exec.Command("bash", "-c", fmt.Sprintf("%s 'mkdir -p %s'", sshBase, customSSLDst)).Run()
				scpSSL := append(scpBase, "-r", customSSLSrc+"/", fmt.Sprintf("root@%s:%s/", task.TargetServerIP, customSSLDst))
				if out, err := exec.Command("scp", scpSSL...).CombinedOutput(); err != nil {
					r.reportProgress(task.MigrationID, "switchover", 96, "running",
						fmt.Sprintf("⚠️  Không thể copy SSL certificate: %s", string(out)))
				} else {
					r.reportProgress(task.MigrationID, "switchover", 96, "running",
						"✓ SSL certificate (custom) đã copy sang server đích")
				}
			} else {
				r.reportProgress(task.MigrationID, "switchover", 96, "running",
					"⚠️  CẢNH BÁO: Không tìm thấy thư mục SSL custom. Kiểm tra lại sau migrate.")
			}
		} else {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				"⚠️  CẢNH BÁO: SSL custom không thể chuyển sang domain mới. Site sẽ chạy HTTP sau migrate. Hãy cài SSL mới cho "+targetDomain)
			r.downgradeToHTTP(task, keyFile, targetDomain, task.PHPVersion)
		}
		nginxOut := enableSite()
		if strings.Contains(nginxOut, "failed") || strings.Contains(nginxOut, "error") {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("⚠️  nginx -t có lỗi: %s", truncateStr(nginxOut, 300)))
		}

	case "letsencrypt":
		// Step 1: Deploy HTTP-only config first so nginx can serve the ACME challenge.
		r.reportProgress(task.MigrationID, "switchover", 93, "running",
			"Kích hoạt site HTTP tạm thời để cấp SSL...")
		r.downgradeToHTTP(task, keyFile, targetDomain, task.PHPVersion)
		nginxOut := enableSite()
		if strings.Contains(nginxOut, "failed") || strings.Contains(nginxOut, "error") {
			r.reportProgress(task.MigrationID, "switchover", 94, "running",
				fmt.Sprintf("⚠️  nginx -t lỗi khi kích hoạt HTTP: %s", truncateStr(nginxOut, 300)))
		}

		// Step 2: Run certbot — it will modify the nginx config to add SSL.
		certEmail := task.WPAdminEmail
		if certEmail == "" {
			certEmail = "admin@" + targetDomain
		}
		r.reportProgress(task.MigrationID, "switchover", 95, "running",
			fmt.Sprintf("Đang cấp SSL Let's Encrypt cho %s trên server đích...", targetDomain))

		// Auto-install certbot on target if missing
		ensureCmd := fmt.Sprintf(
			"%s 'which certbot >/dev/null 2>&1 || (apt-get update -y && apt-get install -y certbot python3-certbot-nginx) 2>&1'",
			sshBase)
		exec.Command("bash", "-c", ensureCmd).Run()

		certbotCmd := fmt.Sprintf(
			"%s 'certbot --nginx -d %s --email %s --non-interactive --agree-tos 2>&1'",
			sshBase, targetDomain, certEmail)
		if out, err := exec.Command("bash", "-c", certbotCmd).CombinedOutput(); err != nil {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("⚠️  Không thể cấp SSL Let's Encrypt (DNS chưa trỏ?). Site sẽ chạy HTTP. Lỗi: %s",
					truncateStr(string(out), 300)))
		} else {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("✓ SSL Let's Encrypt đã cấp thành công cho %s", targetDomain))
			// certbot --nginx may have removed and recreated the symlink; ensure it exists
			exec.Command("bash", "-c", fmt.Sprintf(
				"%s 'ln -sf %s /etc/nginx/sites-enabled/%s.conf 2>/dev/null; systemctl reload nginx 2>/dev/null || true'",
				sshBase, nginxDstConf, targetDomain)).Run()

			// Update WordPress URLs from http:// → https:// in the DB so there is no
			// mixed-content warning after SSL is issued.
			r.reportProgress(task.MigrationID, "switchover", 97, "running",
				fmt.Sprintf("Cập nhật URL https trong WordPress database..."))
			srHTTPS := fmt.Sprintf(
				`%s 'cd /var/www/%s && wp search-replace "http://%s" "https://%s" --all-tables --allow-root 2>&1 | tail -5'`,
				sshBase, targetDomain, targetDomain, targetDomain)
			if srOut, srErr := exec.Command("bash", "-c", srHTTPS).CombinedOutput(); srErr != nil {
				r.reportProgress(task.MigrationID, "switchover", 97, "running",
					fmt.Sprintf("⚠️  wp search-replace https thất bại: %s", truncateStr(string(srOut), 200)))
			} else {
				r.reportProgress(task.MigrationID, "switchover", 97, "running",
					fmt.Sprintf("✓ Đã cập nhật URL https: %s", truncateStr(string(srOut), 200)))
			}
		}
		// No second enableSite() call needed: either certbot reloaded nginx, or HTTP-only is already active.
		return

	default: // "none" — no SSL, just enable site
		r.reportProgress(task.MigrationID, "switchover", 96, "running", "Site không dùng SSL, bỏ qua bước cấp cert")
		nginxOut := enableSite()
		if strings.Contains(nginxOut, "failed") || strings.Contains(nginxOut, "error") {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("⚠️  nginx -t có lỗi: %s", truncateStr(nginxOut, 300)))
		}
	}

	// Final reload to ensure symlink is active (for custom/none paths)
	exec.Command("bash", "-c", fmt.Sprintf(
		"%s 'ln -sf %s /etc/nginx/sites-enabled/%s.conf 2>/dev/null; nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true'",
		sshBase, nginxDstConf, targetDomain)).Run()
}

// downgradeToHTTP rewrites the nginx config on target to HTTP-only (no SSL).
// Used when Let's Encrypt fails or custom SSL can't be transferred.
func (r *MigrationRunner) downgradeToHTTP(task models.AgentMigrateTask, keyFile, targetDomain, phpVer string) {
	sshBase := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s", keyFile, task.TargetServerIP)
	nginxDstConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", targetDomain)

	// Write HTTP-only nginx config to a local temp file, SCP to target
	tmpHTTP := fmt.Sprintf("/tmp/migration_%d_http.conf", task.MigrationID)
	httpConf := buildHTTPOnlyNginxConf(targetDomain, phpVer)
	if err := os.WriteFile(tmpHTTP, []byte(httpConf), 0644); err != nil {
		return
	}
	defer os.Remove(tmpHTTP)

	scpArgs := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile,
		tmpHTTP, fmt.Sprintf("root@%s:%s", task.TargetServerIP, nginxDstConf)}
	exec.Command("scp", scpArgs...).Run()

	// Reload nginx on target with new HTTP-only config
	exec.Command("bash", "-c", fmt.Sprintf(
		"%s 'nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true'", sshBase)).Run()
}

// buildHTTPOnlyNginxConf generates a minimal HTTP-only nginx vhost config.
func buildHTTPOnlyNginxConf(domain, phpVer string) string {
	if phpVer == "" {
		phpVer = "8.2"
	}
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

// detectSSLType reads the nginx config for a domain and returns:
//   - "letsencrypt" if the cert is managed by Certbot (/etc/letsencrypt/live/)
//   - "custom"      if the cert is stored in /etc/nginx/ssl/
//   - "none"        if no SSL is configured
func detectSSLType(domain string) string {
	confPath := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
	raw, err := os.ReadFile(confPath)
	if err != nil {
		return "none"
	}
	content := string(raw)
	if strings.Contains(content, "/etc/letsencrypt/live/") {
		return "letsencrypt"
	}
	if strings.Contains(content, "/etc/nginx/ssl/") {
		return "custom"
	}
	return "none"
}

func sslTypeLabel(t string) string {
	switch t {
	case "letsencrypt":
		return "Let's Encrypt"
	case "custom":
		return "Custom Certificate"
	default:
		return "Không có SSL"
	}
}

// truncateStr truncates s to max n characters, appending "..." if cut.
// sanitizeNginxServerName rewrites every `server_name ...;` line in the nginx
// config so it contains ONLY `domain www.domain`. This prevents domain aliases
// from the source server polluting the target and conflicting with other sites.
func sanitizeNginxServerName(raw []byte, domain string) []byte {
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server_name ") {
			// Preserve leading whitespace, rewrite value
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%sserver_name %s www.%s;", indent, domain, domain)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// suspendSite removes the nginx sites-enabled symlink, effectively suspending the site.
func (r *MigrationRunner) suspendSite(domain string) {
	exec.Command("bash", "-c", fmt.Sprintf(
		"rm -f /etc/nginx/sites-enabled/%s.conf && nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
		domain)).Run()
}

// unsuspendSite re-creates the nginx sites-enabled symlink, restoring the site.
// Also ensures PHP-FPM is running the pool for this domain (socket may be missing
// if FPM was reloaded while the migration had the pool config in an intermediate state).
func (r *MigrationRunner) unsuspendSite(domain string) {
	// Re-enable nginx vhost
	exec.Command("bash", "-c", fmt.Sprintf(
		"ln -sf /etc/nginx/sites-available/%s.conf /etc/nginx/sites-enabled/%s.conf && nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
		domain, domain)).Run()

	// Reload PHP-FPM so the pool socket is recreated if missing.
	for _, ver := range []string{"8.3", "8.2", "8.1", "8.0", "7.4"} {
		poolPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, domain)
		if _, err := os.Stat(poolPath); err == nil {
			exec.Command("bash", "-c", fmt.Sprintf(
				"systemctl reload php%s-fpm 2>/dev/null || true", ver)).Run()
			break
		}
	}
}

func (r *MigrationRunner) reportProgress(migrationID uint64, phase string, progress int, status, message string) {
	r.sendUpdate(models.AgentMigrationUpdate{
		MigrationID: migrationID,
		Phase:       phase,
		Progress:    progress,
		Status:      status,
		LogMessage:  message,
		LogLevel:    "info",
	})
}

func (r *MigrationRunner) reportFailed(migrationID uint64, phase, errMsg string) {
	slog.Error("Migration failed", "migration_id", migrationID, "phase", phase, "error", errMsg)
	r.sendUpdate(models.AgentMigrationUpdate{
		MigrationID: migrationID,
		Phase:       phase,
		Progress:    0,
		Status:      "failed",
		Error:       errMsg,
		LogMessage:  "Lỗi: " + errMsg,
		LogLevel:    "error",
	})
}

func (r *MigrationRunner) sendUpdate(update models.AgentMigrationUpdate) {
	data, _ := json.Marshal(update)
	req, _ := http.NewRequest("POST", r.apiURL+"/api/v1/agent/migrations/update", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", r.apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to report migration update", "error", err)
		return
	}
	resp.Body.Close()
}
