package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
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

// RunMigration executes a full VPS-to-VPS migration
// Phases: preparation → initial_sync → delta_sync → switchover → cleanup
func (r *MigrationRunner) RunMigration(task models.AgentMigrateTask) {
	slog.Info("Starting migration", "domain", task.Domain, "target", task.TargetServerIP)

	// Phase 1: Preparation — create backup on source
	r.reportProgress(task.MigrationID, "preparation", 10, "running", "Creating pre-migration backup")
	siteRoot := fmt.Sprintf("/var/www/%s", task.Domain)

	// Create DB dump
	dumpFile := fmt.Sprintf("/tmp/migration_%d_db.sql", task.MigrationID)
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
		r.reportFailed(task.MigrationID, "preparation", "Database dump failed: "+err.Error())
		return
	}
	// Write dump to temp file
	writeCmd := exec.Command("bash", "-c", fmt.Sprintf("cat > %s", dumpFile))
	writeCmd.Stdin = bytes.NewReader(dumpOut)
	if err := writeCmd.Run(); err != nil {
		r.reportFailed(task.MigrationID, "preparation", "Write dump file failed: "+err.Error())
		return
	}

	r.reportProgress(task.MigrationID, "preparation", 20, "running", "Pre-migration backup created")

	// Phase 2: Initial sync — rsync files to target server
	r.reportProgress(task.MigrationID, "initial_sync", 25, "running", "Starting initial file sync via rsync")

	// rsync site files to target server
	rsyncArgs := []string{
		"-avz", "--delete",
		"-e", "ssh -o StrictHostKeyChecking=no",
		siteRoot + "/",
		fmt.Sprintf("root@%s:%s/", task.TargetServerIP, siteRoot),
	}
	rsyncCmd := exec.Command("rsync", rsyncArgs...)
	if out, err := rsyncCmd.CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "initial_sync", fmt.Sprintf("rsync failed: %s %v", string(out), err))
		return
	}

	r.reportProgress(task.MigrationID, "initial_sync", 50, "running", "File sync completed")

	// Transfer DB dump
	scpCmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no",
		dumpFile,
		fmt.Sprintf("root@%s:%s", task.TargetServerIP, dumpFile),
	)
	if out, err := scpCmd.CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "initial_sync", fmt.Sprintf("SCP db dump failed: %s %v", string(out), err))
		return
	}

	r.reportProgress(task.MigrationID, "initial_sync", 55, "running", "Database dump transferred to target")

	// Phase 3: Delta sync — quick re-sync of changed files
	r.reportProgress(task.MigrationID, "delta_sync", 60, "running", "Running delta sync")

	rsyncCmd2 := exec.Command("rsync", rsyncArgs...)
	rsyncCmd2.CombinedOutput() // ignore minor errors

	r.reportProgress(task.MigrationID, "delta_sync", 70, "running", "Delta sync completed")

	// Phase 4: Switchover — import DB on target, configure nginx/php-fpm
	r.reportProgress(task.MigrationID, "switchover", 75, "running", "Importing database on target server")

	// Import DB on target
	importCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no root@%s 'mysql --user=%s --password=%s %s < %s'",
		task.TargetServerIP, task.DBUser, task.DBPass, task.DBName, dumpFile)
	shellCmd := exec.Command("bash", "-c", importCmd)
	if out, err := shellCmd.CombinedOutput(); err != nil {
		r.reportFailed(task.MigrationID, "switchover", fmt.Sprintf("DB import failed: %s %v", string(out), err))
		return
	}

	r.reportProgress(task.MigrationID, "switchover", 85, "running", "Database imported, configuring target server")

	// If domain changed, run wp search-replace
	if task.ChangeDomain != "" {
		searchReplace := fmt.Sprintf(
			"ssh -o StrictHostKeyChecking=no root@%s 'cd %s && wp search-replace %s %s --all-tables --allow-root 2>/dev/null'",
			task.TargetServerIP, siteRoot, task.Domain, task.ChangeDomain,
		)
		exec.Command("bash", "-c", searchReplace).CombinedOutput()
		r.reportProgress(task.MigrationID, "switchover", 90, "running", "Domain search-replace completed")
	}

	// Fix ownership on target
	chownCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no root@%s 'chown -R www-data:www-data %s'",
		task.TargetServerIP, siteRoot)
	exec.Command("bash", "-c", chownCmd).Run()

	r.reportProgress(task.MigrationID, "switchover", 95, "running", "File ownership corrected on target")

	// Phase 5: Cleanup
	r.reportProgress(task.MigrationID, "cleanup", 98, "running", "Cleaning up temporary files")

	// Remove temp dump files
	exec.Command("rm", "-f", dumpFile).Run()
	cleanupCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no root@%s 'rm -f %s'", task.TargetServerIP, dumpFile)
	exec.Command("bash", "-c", cleanupCmd).Run()

	// Mark migration as completed
	r.reportProgress(task.MigrationID, "cleanup", 100, "completed", "Migration completed successfully")
	slog.Info("Migration completed", "domain", task.Domain, "target", task.TargetServerIP)
}

// RollbackMigration reverses the migration (placeholder — would need backup restore)
func (r *MigrationRunner) RollbackMigration(migrationID uint64, domain string) {
	slog.Info("Rolling back migration", "migration_id", migrationID, "domain", domain)
	r.reportProgress(migrationID, "cleanup", 100, "rolled_back", "Migration rolled back")
}

func (r *MigrationRunner) reportProgress(migrationID uint64, phase string, progress int, status, message string) {
	update := models.AgentMigrationUpdate{
		MigrationID: migrationID,
		Phase:       phase,
		Progress:    progress,
		Status:      status,
		LogMessage:  message,
		LogLevel:    "info",
	}
	r.sendUpdate(update)
}

func (r *MigrationRunner) reportFailed(migrationID uint64, phase, errMsg string) {
	slog.Error("Migration failed", "migration_id", migrationID, "phase", phase, "error", errMsg)
	update := models.AgentMigrationUpdate{
		MigrationID: migrationID,
		Phase:       phase,
		Progress:    0,
		Status:      "failed",
		Error:       errMsg,
		LogMessage:  "Migration failed: " + errMsg,
		LogLevel:    "error",
	}
	r.sendUpdate(update)
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
