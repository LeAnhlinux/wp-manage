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
	"strconv"
	"strings"
	"time"

	agentsecurity "github.com/gofiber-team/wp-manage/internal/agent/security"
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

	// Always clean up SSH key when done (deferred)
	defer func() {
		os.Remove(keyFile)
		os.Remove(keyFile + ".pub")
		r.cleanupSSHAccess(task.MigrationID)
	}()

	// ── Pre-flight on target: disk space + domain conflict ────────────────────
	// FIX #7: Check disk space before starting any transfer
	r.reportProgress(task.MigrationID, "preparation", 13, "running", "Kiểm tra tài nguyên server đích...")

	srcSizeOut, _ := exec.Command("du", "-sb", siteRoot).Output()
	if srcFields := strings.Fields(string(srcSizeOut)); len(srcFields) > 0 {
		diskCheckCmd := fmt.Sprintf(
			"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s "+
				"'df --output=avail -B1 /var/www 2>/dev/null || df -B1 / 2>/dev/null | tail -1 | awk \"{print \\$4}\"'",
			keyFile, task.TargetServerIP)
		diskOut, _ := exec.Command("bash", "-c", diskCheckCmd).Output()
		availStr := strings.TrimSpace(strings.Split(string(diskOut), "\n")[1] /* skip header */)
		if availStr == "" {
			// fallback: pick last line
			lines := strings.Fields(string(diskOut))
			if len(lines) > 0 {
				availStr = lines[len(lines)-1]
			}
		}
		srcSize, srcErr := strconv.ParseInt(srcFields[0], 10, 64)
		avail, availErr := strconv.ParseInt(availStr, 10, 64)
		if srcErr == nil && availErr == nil {
			required := srcSize * 2 // 2× source: files + DB dump
			if avail < required {
				r.reportFailed(task.MigrationID, "preparation",
					fmt.Sprintf("Server đích không đủ dung lượng: cần %s, còn trống %s",
						formatBytes(required), formatBytes(avail)))
				return
			}
			r.reportProgress(task.MigrationID, "preparation", 14, "running",
				fmt.Sprintf("✓ Dung lượng server đích đủ: %s trống (cần %s)",
					formatBytes(avail), formatBytes(required)))
		}
	}

	// FIX #7b: Check if target domain already exists → warn (not fail — migration may be a retry)
	conflictCheckCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s "+
			"'[ -f /etc/nginx/sites-enabled/%s.conf ] && echo CONFLICT || echo OK'",
		keyFile, task.TargetServerIP, targetDomain)
	if conflictOut, _ := exec.Command("bash", "-c", conflictCheckCmd).Output(); strings.Contains(string(conflictOut), "CONFLICT") {
		r.reportProgress(task.MigrationID, "preparation", 14, "running",
			fmt.Sprintf("⚠️  Domain %s đã tồn tại trên server đích — dữ liệu cũ sẽ bị ghi đè", targetDomain))
	}

	// Check PHP version compatibility on target — auto-install if missing
	phpCheckCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s 'php%s -v >/dev/null 2>&1 && echo OK || echo NOT_FOUND'",
		keyFile, task.TargetServerIP, task.PHPVersion)
	phpCheckOut, _ := exec.Command("bash", "-c", phpCheckCmd).Output()
	if strings.Contains(string(phpCheckOut), "NOT_FOUND") {
		r.reportProgress(task.MigrationID, "preparation", 15, "running",
			fmt.Sprintf("PHP %s chưa có trên server đích — đang cài tự động...", task.PHPVersion))

		// Auto-install PHP version on target via SSH
		phpInstallScript := fmt.Sprintf(`
			export DEBIAN_FRONTEND=noninteractive
			add-apt-repository -y ppa:ondrej/php 2>/dev/null || true
			apt-get update -qq
			apt-get install -y php%s-fpm php%s-mysql php%s-xml php%s-curl php%s-mbstring \
				php%s-zip php%s-gd php%s-intl php%s-bcmath php%s-soap php%s-imagick 2>&1 | tail -3
			systemctl enable php%s-fpm
			systemctl start php%s-fpm
			echo PHP_INSTALL_OK
		`, task.PHPVersion, task.PHPVersion, task.PHPVersion, task.PHPVersion, task.PHPVersion,
			task.PHPVersion, task.PHPVersion, task.PHPVersion, task.PHPVersion, task.PHPVersion,
			task.PHPVersion, task.PHPVersion, task.PHPVersion)

		// FIX: Use ServerAliveInterval to prevent SSH dropping on slow apt-get
		installCmd := fmt.Sprintf(
			"ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o ServerAliveInterval=30 -o ServerAliveCountMax=20 -i %s root@%s '%s'",
			keyFile, task.TargetServerIP, phpInstallScript)
		installOut, installErr := exec.Command("bash", "-c", installCmd).CombinedOutput()
		if installErr != nil || !strings.Contains(string(installOut), "PHP_INSTALL_OK") {
			r.reportFailed(task.MigrationID, "preparation",
				fmt.Sprintf("Không thể cài PHP %s trên server đích: %s", task.PHPVersion, string(installOut)))
			return
		}

		// Also copy ionCube loader if available on source
		ioncubeSrc := fmt.Sprintf("/opt/wp-manage/ioncube/ioncube/ioncube_loader_lin_%s.so", task.PHPVersion)
		if _, err := os.Stat(ioncubeSrc); err == nil {
			extDirOut, _ := exec.Command("bash", "-c",
				fmt.Sprintf("ssh -o StrictHostKeyChecking=no -i %s root@%s 'php%s -r \"echo PHP_EXTENSION_DIR;\"'",
					keyFile, task.TargetServerIP, task.PHPVersion)).Output()
			extDir := strings.TrimSpace(string(extDirOut))
			if extDir != "" {
				scpBase := []string{"-o", "StrictHostKeyChecking=no", "-i", keyFile}
				scpArgs := append(scpBase, ioncubeSrc,
					fmt.Sprintf("root@%s:%s/ioncube_loader_lin_%s.so", task.TargetServerIP, extDir, task.PHPVersion))
				exec.Command("scp", scpArgs...).Run()
				iniContent := fmt.Sprintf("; Managed by wp-manage\\nzend_extension=ioncube_loader_lin_%s.so", task.PHPVersion)
				for _, sapi := range []string{"cli", "fpm"} {
					iniCmd := fmt.Sprintf(
						"ssh -o StrictHostKeyChecking=no -i %s root@%s 'printf \"%s\" > /etc/php/%s/%s/conf.d/00-ioncube.ini'",
						keyFile, task.TargetServerIP, iniContent, task.PHPVersion, sapi)
					exec.Command("bash", "-c", iniCmd).Run()
				}
				exec.Command("bash", "-c", fmt.Sprintf(
					"ssh -o StrictHostKeyChecking=no -i %s root@%s 'systemctl restart php%s-fpm'",
					keyFile, task.TargetServerIP, task.PHPVersion)).Run()
			}
		}

		r.reportProgress(task.MigrationID, "preparation", 15, "running",
			fmt.Sprintf("✓ PHP %s đã được cài thành công trên server đích", task.PHPVersion))
	} else {
		r.reportProgress(task.MigrationID, "preparation", 15, "running",
			fmt.Sprintf("✓ PHP %s có trên server đích", task.PHPVersion))
	}

	// SSH/rsync options using temp key
	sshOpts := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o BatchMode=yes -i %s", keyFile)

	// FIX #9 + #14: Backup DB
	//   --no-tablespaces: prevents "Access denied" errors from DEFINER views with tablespace info
	//   --result-file: writes directly to disk, avoids loading full dump into RAM (OOM on large DBs)
	r.reportProgress(task.MigrationID, "preparation", 17, "running", "Tạo backup database nguồn...")
	dumpCmd := exec.Command("mysqldump",
		"--user="+task.DBUser,
		"--password="+task.DBPass,
		"--single-transaction",
		"--quick",
		"--lock-tables=false",
		"--no-tablespaces",
		"--result-file="+dumpFile,
		task.DBName,
	)
	if out, err := dumpCmd.CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "preparation",
			fmt.Sprintf("mysqldump thất bại: %s %v", string(out), err))
		return
	}
	if _, statErr := os.Stat(dumpFile); statErr != nil {
		r.reportFailed(task.MigrationID, "preparation", "Dump file không tồn tại sau mysqldump")
		return
	}
	defer os.Remove(dumpFile)
	r.reportProgress(task.MigrationID, "preparation", 20, "running", "Backup database hoàn tất")

	// ── PHASE 2: INITIAL SYNC ─────────────────────────────────────────────────
	r.reportProgress(task.MigrationID, "initial_sync", 25, "running", "Đồng bộ files lên server đích (lần 1)...")

	rsyncArgs := []string{
		"-rltvz", "--delete",
		"--no-owner", "--no-group", // don't copy UID/GID from source; chown explicitly after
		"--exclude", "logs/*.log",  // skip large log files
		"--exclude", "logs/*.gz",
		"-e", sshOpts,
		siteRoot + "/",
		fmt.Sprintf("root@%s:%s/", task.TargetServerIP, targetSiteRoot),
	}
	if out, err := exec.Command("rsync", rsyncArgs...).CombinedOutput(); err != nil {
		// FIX #13: rsync exit code 23 = partial transfer (some files skipped due to permission).
		// Treat as warning, not failure — WordPress files are all there, just some system files skipped.
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if exitCode == 23 || exitCode == 24 {
			r.reportProgress(task.MigrationID, "initial_sync", 40, "running",
				fmt.Sprintf("⚠️  rsync partial transfer (exit %d) — một số file bị skip, tiếp tục...", exitCode))
		} else {
			r.reportFailed(task.MigrationID, "initial_sync", fmt.Sprintf("rsync thất bại (exit %d): %s", exitCode, string(out)))
			return
		}
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
	// FIX #6: Check delta sync errors — non-fatal but warn
	if deltaOut, deltaErr := exec.Command("rsync", rsyncArgs...).CombinedOutput(); deltaErr != nil {
		exitCode := 0
		if exitErr, ok := deltaErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if exitCode != 23 && exitCode != 24 {
			r.reportProgress(task.MigrationID, "delta_sync", 60, "running",
				fmt.Sprintf("⚠️  Delta sync có lỗi (exit %d, non-fatal): %s", exitCode, truncateStr(string(deltaOut), 200)))
		}
	}
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
	// FIX #8: DROP first to clear stale data from previously failed migration.
	// Write SQL to a temp file and SCP it — avoids all SSH quoting issues with passwords.
	r.reportProgress(task.MigrationID, "switchover", 72, "running", "Tạo database và user trên server đích...")
	setupSQLFile := fmt.Sprintf("/tmp/migration_%d_setup.sql", task.MigrationID)
	setupSQL := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`;\n"+
			"CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
			"DROP USER IF EXISTS '%s'@'localhost';\n"+
			"CREATE USER '%s'@'localhost' IDENTIFIED BY '%s';\n"+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';\n"+
			"FLUSH PRIVILEGES;\n",
		task.DBName, task.DBName,
		task.DBUser, task.DBUser, task.DBPass,
		task.DBName, task.DBUser)
	if err := os.WriteFile(setupSQLFile, []byte(setupSQL), 0600); err == nil {
		scpSetup := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile,
			setupSQLFile, fmt.Sprintf("root@%s:%s", task.TargetServerIP, setupSQLFile)}
		exec.Command("scp", scpSetup...).Run()
		execSetup := fmt.Sprintf(
			"ssh -o StrictHostKeyChecking=no -i %s root@%s 'mysql -u root < %s; rm -f %s'",
			keyFile, task.TargetServerIP, setupSQLFile, setupSQLFile)
		exec.Command("bash", "-c", execSetup).Run()
		os.Remove(setupSQLFile)
	}

	// 4d: Import DB using mysql root (site user may not have sufficient privileges yet)
	// FIX: Removed --binary-mode=1 (causes issues with standard WP dumps; not needed for utf8mb4)
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
	// FIX #2: Use --path=.../public — wp-config.php is in /public, not /var/www/domain root
	// FIX #15: Add --skip-themes --skip-plugins to avoid fatal errors from incompatible themes (e.g. create_function() PHP 8.0 removed)
	// FIX #16: Fallback to direct SQL if WP-CLI fails (serialization-safe for options/posts/postmeta)
	if task.Mode == "change_domain" && task.NewDomain != "" {
		r.reportProgress(task.MigrationID, "switchover", 85, "running",
			fmt.Sprintf("Thay thế domain %s → %s...", task.Domain, task.NewDomain))

		// Try WP-CLI first (handles serialized data correctly)
		srCmd := fmt.Sprintf(
			`ssh -o StrictHostKeyChecking=no -i %s root@%s 'wp --path=%s/public search-replace "%s" "%s" --all-tables --skip-themes --skip-plugins --allow-root 2>&1'`,
			keyFile, task.TargetServerIP, targetSiteRoot, task.Domain, task.NewDomain)
		if out, err := exec.Command("bash", "-c", srCmd).CombinedOutput(); err != nil {
			// WP-CLI failed (e.g. incompatible theme/plugin) → fallback to direct SQL
			slog.Warn("wp search-replace failed, falling back to direct SQL", "migration", task.MigrationID, "error", string(out))
			r.reportProgress(task.MigrationID, "switchover", 86, "running",
				fmt.Sprintf("⚠️ WP-CLI search-replace thất bại, dùng SQL trực tiếp: %s", truncateStr(string(out), 200)))

			dbName := dbNameForDomain(task.NewDomain)
			sqlFallback := fmt.Sprintf(
				`ssh -o StrictHostKeyChecking=no -i %s root@%s 'mysql %s -e "
UPDATE wp_options SET option_value = REPLACE(option_value, \"%s\", \"%s\") WHERE option_name IN (\"siteurl\",\"home\");
UPDATE wp_posts SET post_content = REPLACE(post_content, \"%s\", \"%s\");
UPDATE wp_posts SET guid = REPLACE(guid, \"%s\", \"%s\");
UPDATE wp_postmeta SET meta_value = REPLACE(CAST(meta_value AS CHAR), \"%s\", \"%s\");
" 2>&1'`,
				keyFile, task.TargetServerIP, dbName,
				task.Domain, task.NewDomain,
				task.Domain, task.NewDomain,
				task.Domain, task.NewDomain,
				task.Domain, task.NewDomain)
			if sqlOut, sqlErr := exec.Command("bash", "-c", sqlFallback).CombinedOutput(); sqlErr != nil {
				r.reportFailed(task.MigrationID, "switchover",
					fmt.Sprintf("search-replace thất bại (WP-CLI + SQL): %s", string(sqlOut)))
				return
			}
			r.reportProgress(task.MigrationID, "switchover", 88, "running",
				fmt.Sprintf("✓ Thay thế domain bằng SQL (wp_options/posts/postmeta): %s → %s", task.Domain, task.NewDomain))
		} else {
			r.reportProgress(task.MigrationID, "switchover", 88, "running",
				fmt.Sprintf("✓ Thay thế domain %s → %s hoàn tất", task.Domain, task.NewDomain))
		}
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
	r.setupTargetConfigs(task, keyFile, targetDomain, linuxUser, sslType)
	r.reportProgress(task.MigrationID, "switchover", 97, "running", "Nginx + PHP-FPM trên server đích đã sẵn sàng")

	// ── PHASE 5: CLEANUP + HEALTH CHECK ─────────────────────────────────────
	r.reportProgress(task.MigrationID, "cleanup", 98, "running", "Dọn dẹp files tạm trên server đích...")

	cleanDumpCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s 'rm -f %s'",
		keyFile, task.TargetServerIP, dumpFile)
	exec.Command("bash", "-c", cleanDumpCmd).Run()

	// For keep_domain: disable source nginx (site has moved to target)
	// Phase 4a already removed the symlink; this is a belt-and-suspenders reload
	if task.Mode == "keep_domain" {
		exec.Command("bash", "-c", fmt.Sprintf(
			"rm -f /etc/nginx/sites-enabled/%s.conf; nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
			task.Domain)).Run()
		r.reportProgress(task.MigrationID, "cleanup", 99, "running",
			"Đã vô hiệu hóa site trên server nguồn. Đang xóa SSH key tạm...")
	}

	// Optional: clean up all source artifacts (files, DB, configs)
	if task.CleanupSource {
		r.cleanupSourceSite(task)
	}

	// FIX #11: HTTP health check — verify target nginx is actually serving the site
	// Use direct IP request with Host header (DNS may not have propagated yet for keep_domain)
	r.reportProgress(task.MigrationID, "cleanup", 99, "running", "Kiểm tra site trên server đích...")
	healthCheckCmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -i %s root@%s "+
			"'curl -s -o /dev/null -w %%{http_code} -H \"Host: %s\" --connect-timeout 8 http://127.0.0.1/ 2>/dev/null'",
		keyFile, task.TargetServerIP, targetDomain)
	if hOut, _ := exec.Command("bash", "-c", healthCheckCmd).Output(); len(hOut) > 0 {
		httpCode := strings.TrimSpace(string(hOut))
		if httpCode == "200" || httpCode == "301" || httpCode == "302" {
			r.reportProgress(task.MigrationID, "cleanup", 99, "running",
				fmt.Sprintf("✓ Health check OK — site trả về HTTP %s", httpCode))
		} else {
			r.reportProgress(task.MigrationID, "cleanup", 99, "running",
				fmt.Sprintf("⚠️  Health check trả về HTTP %s — kiểm tra nginx/PHP-FPM trên server đích", httpCode))
		}
	} else {
		r.reportProgress(task.MigrationID, "cleanup", 99, "running",
			"⚠️  Health check không phản hồi — site có thể chưa sẵn sàng")
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

	// Bot-control and country-access files are keyed by site id, and the vhost
	// is the only place this server records which id belongs to the domain —
	// read it before the vhost is removed, or the files are orphaned forever.
	includes := agentsecurity.ManagedIncludesForDomain(domain)

	// 1. Remove nginx sites-available config (symlink already removed above)
	os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain))
	os.Remove(fmt.Sprintf("/etc/nginx/sites-enabled/%s.conf", domain))
	agentsecurity.RemoveManagedFiles(includes)

	// 2. Remove ModSecurity config
	os.Remove(fmt.Sprintf("/etc/nginx/modsec/%s.conf", domain))

	// Reload nginx after removing configs
	exec.Command("bash", "-c", "nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true").Run()

	// 3. Remove PHP-FPM pool and reload
	for _, ver := range []string{"8.6", "8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"} {
		poolPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, domain)
		if _, err := os.Stat(poolPath); err == nil {
			os.Remove(poolPath)
			exec.Command("bash", "-c",
				fmt.Sprintf("systemctl reload php%s-fpm 2>/dev/null || systemctl restart php%s-fpm 2>/dev/null || true", ver, ver),
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
// sslType is pre-detected by the caller to avoid reading source config twice.
func (r *MigrationRunner) setupTargetConfigs(task models.AgentMigrateTask, keyFile, targetDomain, linuxUser, sslType string) {
	sshBase := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s", keyFile, task.TargetServerIP)
	scpBase := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile}

	isChangeDomain := task.Mode == "change_domain" && task.NewDomain != ""

	// ── Nginx config ──────────────────────────────────────────────────────────
	nginxSrcConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", task.Domain)
	nginxDstConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", targetDomain)

	if _, err := os.Stat(nginxSrcConf); err == nil {
		exec.Command("bash", "-c", fmt.Sprintf("%s 'mkdir -p /etc/nginx/sites-available'", sshBase)).Run()

		raw, err := os.ReadFile(nginxSrcConf)
		if err == nil {
			// Always sanitize server_name: keep only targetDomain and www.targetDomain.
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
	phpVersions := []string{"8.6", "8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"}
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

		// FIX #1: Reload PHP-FPM on target — reload first, fall back to restart.
		// Wait up to 15s for socket to appear before proceeding.
		sockPath := fmt.Sprintf("/run/php/php%s-fpm-%s.sock", ver, targetDomain)
		reloadScript := fmt.Sprintf(
			"systemctl reload php%s-fpm 2>/dev/null || systemctl restart php%s-fpm 2>/dev/null; "+
				"for i in $(seq 1 15); do [ -S %s ] && echo SOCK_OK && break; sleep 1; done",
			ver, ver, sockPath)
		reloadOut, _ := exec.Command("bash", "-c", fmt.Sprintf("%s '%s'", sshBase, reloadScript)).CombinedOutput()
		if !strings.Contains(string(reloadOut), "SOCK_OK") {
			r.reportProgress(task.MigrationID, "switchover", 95, "running",
				fmt.Sprintf("⚠️  PHP-FPM socket chưa xuất hiện sau reload — restart php%s-fpm...", ver))
			exec.Command("bash", "-c", fmt.Sprintf("%s 'systemctl restart php%s-fpm'", sshBase, ver)).Run()
			// Extra 10s grace period after hard restart
			exec.Command("bash", "-c", fmt.Sprintf(
				"%s 'for i in $(seq 1 10); do [ -S %s ] && echo SOCK_OK2 && break; sleep 1; done'",
				sshBase, sockPath)).Run()
		} else {
			r.reportProgress(task.MigrationID, "switchover", 95, "running",
				fmt.Sprintf("✓ PHP-FPM socket sẵn sàng: %s", sockPath))
		}
		break
	}

	// ── SSL handling + Enable Nginx site on target ───────────────────────────
	//
	// IMPORTANT ordering:
	//  - For letsencrypt: MUST activate HTTP-only config FIRST, then run certbot.
	//    certbot --nginx needs a working HTTP vhost for the HTTP-01 ACME challenge.
	//  - For custom: copy certs first, then enable site.
	//  - For none: just enable site.

	// FIX #3: Ensure logs dir exists on target — nginx conf references it; missing dir → nginx -t fails
	exec.Command("bash", "-c", fmt.Sprintf(
		"%s 'mkdir -p /var/www/%s/logs'", sshBase, targetDomain)).Run()

	// FIX #5/#10: enableSite checks EXIT code via embedded marker — more reliable than string matching.
	// nginx -t exit code 0 = success, non-zero = failure regardless of output wording.
	enableSite := func() (string, bool) {
		out, _ := exec.Command("bash", "-c", fmt.Sprintf(
			"%s 'mkdir -p /etc/nginx/sites-enabled && ln -sf %s /etc/nginx/sites-enabled/%s.conf; "+
				"nginx -t 2>&1; NGINX_RC=$?; [ $NGINX_RC -eq 0 ] && systemctl reload nginx 2>/dev/null; "+
				"echo __EXIT__$NGINX_RC'",
			sshBase, nginxDstConf, targetDomain)).CombinedOutput()
		output := string(out)
		ok := strings.Contains(output, "__EXIT__0")
		if !ok {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("⚠️  nginx -t thất bại trên server đích: %s", truncateStr(output, 400)))
		}
		return output, ok
	}

	switch sslType {
	case "custom":
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
					"⚠️  Không tìm thấy thư mục SSL custom — kiểm tra lại sau migrate")
			}
		} else {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				"⚠️  SSL custom không thể chuyển sang domain mới — site sẽ chạy HTTP. Cài SSL mới cho "+targetDomain)
			r.downgradeToHTTP(task, keyFile, targetDomain, task.PHPVersion)
		}
		enableSite()

	case "letsencrypt":
		// Step 1: Deploy HTTP-only config first so nginx can serve the ACME challenge
		r.reportProgress(task.MigrationID, "switchover", 93, "running",
			"Kích hoạt site HTTP tạm thời để cấp SSL...")
		r.downgradeToHTTP(task, keyFile, targetDomain, task.PHPVersion)
		enableSite()

		// FIX #4: Warn explicitly — certbot HTTP-01 requires DNS to already point to target IP.
		// In keep_domain mode, DNS still points to source during migration.
		certEmail := task.WPAdminEmail
		if certEmail == "" {
			certEmail = "admin@" + targetDomain
		}
		if task.Mode == "keep_domain" {
			r.reportProgress(task.MigrationID, "switchover", 94, "running",
				fmt.Sprintf("⚠️  Let's Encrypt cần DNS của %s trỏ về IP server đích (%s) trước khi cấp cert. "+
					"Nếu DNS chưa cập nhật, certbot sẽ fail và site tạm chạy HTTP — bạn có thể cấp SSL sau.",
					targetDomain, task.TargetServerIP))
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
				fmt.Sprintf("⚠️  Không thể cấp SSL Let's Encrypt (DNS chưa trỏ?). Site chạy HTTP. Lỗi: %s",
					truncateStr(string(out), 300)))
		} else {
			r.reportProgress(task.MigrationID, "switchover", 96, "running",
				fmt.Sprintf("✓ SSL Let's Encrypt đã cấp thành công cho %s", targetDomain))
			exec.Command("bash", "-c", fmt.Sprintf(
				"%s 'ln -sf %s /etc/nginx/sites-enabled/%s.conf 2>/dev/null; systemctl reload nginx 2>/dev/null || true'",
				sshBase, nginxDstConf, targetDomain)).Run()

			// FIX #2 (https variant): wp search-replace http→https with correct --path
			// FIX #15: Add --skip-themes --skip-plugins to avoid fatal errors from incompatible themes
			// FIX #16: SQL fallback if WP-CLI fails
			r.reportProgress(task.MigrationID, "switchover", 97, "running",
				"Cập nhật URL https trong WordPress database...")
			srHTTPS := fmt.Sprintf(
				`%s 'wp --path=/var/www/%s/public search-replace "http://%s" "https://%s" --all-tables --skip-themes --skip-plugins --allow-root 2>&1 | tail -5'`,
				sshBase, targetDomain, targetDomain, targetDomain)
			if srOut, srErr := exec.Command("bash", "-c", srHTTPS).CombinedOutput(); srErr != nil {
				slog.Warn("wp search-replace https failed, falling back to SQL", "domain", targetDomain, "error", string(srOut))
				dbName := dbNameForDomain(targetDomain)
				sqlHTTPS := fmt.Sprintf(
					`%s 'mysql %s -e "UPDATE wp_options SET option_value = REPLACE(option_value, \"http://%s\", \"https://%s\") WHERE option_name IN (\"siteurl\",\"home\"); UPDATE wp_posts SET post_content = REPLACE(post_content, \"http://%s\", \"https://%s\"); UPDATE wp_posts SET guid = REPLACE(guid, \"http://%s\", \"https://%s\");" 2>&1'`,
					sshBase, dbName,
					targetDomain, targetDomain,
					targetDomain, targetDomain,
					targetDomain, targetDomain)
				if sqlOut, sqlErr := exec.Command("bash", "-c", sqlHTTPS).CombinedOutput(); sqlErr != nil {
					r.reportProgress(task.MigrationID, "switchover", 97, "running",
						fmt.Sprintf("⚠️  Cập nhật https thất bại (WP-CLI + SQL): %s", truncateStr(string(sqlOut), 200)))
				} else {
					r.reportProgress(task.MigrationID, "switchover", 97, "running",
						fmt.Sprintf("✓ Đã cập nhật URL https bằng SQL: %s", truncateStr(string(sqlOut), 100)))
				}
			} else {
				r.reportProgress(task.MigrationID, "switchover", 97, "running",
					fmt.Sprintf("✓ Đã cập nhật URL https: %s", truncateStr(string(srOut), 200)))
			}
		}
		return // certbot already reloaded nginx; no further reload needed

	default: // "none" — no SSL
		r.reportProgress(task.MigrationID, "switchover", 96, "running", "Site không dùng SSL, bỏ qua bước cấp cert")
		enableSite()
	}

	// Final symlink + reload (for custom/none paths; letsencrypt already returned above)
	exec.Command("bash", "-c", fmt.Sprintf(
		"%s 'ln -sf %s /etc/nginx/sites-enabled/%s.conf 2>/dev/null; nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true'",
		sshBase, nginxDstConf, targetDomain)).Run()
}

// dbNameForDomain computes the WordPress DB name for a given domain.
// Must match the formula used during site creation: wp_{sanitized_domain}
func dbNameForDomain(domain string) string {
	sanitized := strings.NewReplacer(".", "_", "-", "_").Replace(domain)
	return "wp_" + sanitized
}

// downgradeToHTTP rewrites the nginx config on target to HTTP-only (no SSL).
// Used when Let's Encrypt fails or custom SSL can't be transferred.
func (r *MigrationRunner) downgradeToHTTP(task models.AgentMigrateTask, keyFile, targetDomain, phpVer string) {
	sshBase := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o ConnectTimeout=30 -i %s root@%s", keyFile, task.TargetServerIP)
	nginxDstConf := fmt.Sprintf("/etc/nginx/sites-available/%s.conf", targetDomain)

	tmpHTTP := fmt.Sprintf("/tmp/migration_%d_http.conf", task.MigrationID)
	httpConf := buildHTTPOnlyNginxConf(targetDomain, phpVer)
	if err := os.WriteFile(tmpHTTP, []byte(httpConf), 0644); err != nil {
		return
	}
	defer os.Remove(tmpHTTP)

	scpArgs := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=30", "-i", keyFile,
		tmpHTTP, fmt.Sprintf("root@%s:%s", task.TargetServerIP, nginxDstConf)}
	exec.Command("scp", scpArgs...).Run()

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

// sanitizeNginxServerName rewrites every `server_name ...;` line in the nginx
// config so it contains ONLY `domain www.domain`.
func sanitizeNginxServerName(raw []byte, domain string) []byte {
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server_name ") {
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

// formatBytes formats a byte count into a human-readable string (KB/MB/GB).
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// suspendSite removes the nginx sites-enabled symlink, effectively suspending the site.
func (r *MigrationRunner) suspendSite(domain string) {
	exec.Command("bash", "-c", fmt.Sprintf(
		"rm -f /etc/nginx/sites-enabled/%s.conf && nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
		domain)).Run()
}

// unsuspendSite re-creates the nginx sites-enabled symlink, restoring the site.
// Also reloads PHP-FPM so the socket is recreated if missing.
func (r *MigrationRunner) unsuspendSite(domain string) {
	exec.Command("bash", "-c", fmt.Sprintf(
		"ln -sf /etc/nginx/sites-available/%s.conf /etc/nginx/sites-enabled/%s.conf && nginx -t 2>/dev/null && systemctl reload nginx 2>/dev/null || true",
		domain, domain)).Run()

	for _, ver := range []string{"8.6", "8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"} {
		poolPath := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", ver, domain)
		if _, err := os.Stat(poolPath); err == nil {
			exec.Command("bash", "-c", fmt.Sprintf(
				"systemctl reload php%s-fpm 2>/dev/null || systemctl restart php%s-fpm 2>/dev/null || true", ver, ver)).Run()
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
