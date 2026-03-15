// Package updater implements safe binary hot-swap for any wp-manage service.
//
// # Why a separate service?
//
// Any long-running process that replaces its own binary while running risks:
//   - Race: the OS / systemd may kill the process before the "completed" HTTP
//     response is sent back → API marks it failed while binary is already swapped.
//   - No recovery: if the new binary is broken the service is dead and cannot
//     report the failure or roll back.
//
// # Architecture
//
// The service being updated (agent or API) only STAGES the binary:
//  1. Downloads new binary to StagedBinaryPath
//  2. Verifies SHA-256 checksum
//  3. Writes PendingUpdate to MetadataPath
//  4. Calls `systemctl start wp-manage-updater` and returns
//
// The wp-manage-updater one-shot service then:
//  1. Reads MetadataPath
//  2. Stops TargetService
//  3. Atomically replaces TargetBinary
//  4. Starts TargetService
//  5. Waits and verifies the service is active
//  6. Reports success/failure to CallbackURL (if set) OR writes ResultFile
//  7. Auto-rolls back to TargetBinary+".bak" on failure
//
// # Supported targets
//
//	TargetService = "wp-manage-agent"  →  binary at /usr/local/bin/wp-manage-agent
//	TargetService = "wp-manage-api"    →  binary at /opt/wp-manage/bin/wp-manage-api
//
// The updater binary itself is tiny (~4 MB), almost never changes, and is
// deployed directly via update.sh (SSH+SCP) — it never self-updates.
package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Well-known paths used by both the staged (agent/API) and the runner.
const (
	MetadataPath     = "/var/lib/wp-manage/pending-update.json"
	ResultFilePath   = "/var/lib/wp-manage/last-update-result.json"
	StagedBinaryPath = "/var/lib/wp-manage/agent-staged" // default; overridden by PendingUpdate.StagedPath

	// Default binary locations (used when PendingUpdate.TargetBinary is empty)
	AgentBinaryPath  = "/usr/local/bin/wp-manage-agent"
	APIBinaryPath    = "/opt/wp-manage/bin/wp-manage-api"
	BackupSuffix     = ".bak"
)

// PendingUpdate is written by the service being updated before it starts the
// updater. The updater reads this on every startup.
//
// Fields are intentionally generic so the same updater binary handles any
// wp-manage service without recompilation.
type PendingUpdate struct {
	// What to replace
	StagedPath    string `json:"staged_path"`   // absolute path of the staged binary
	TargetBinary  string `json:"target_binary"` // absolute path of the installed binary to swap
	TargetService string `json:"target_service"` // systemd service name to stop/start

	// Integrity
	Checksum   string `json:"checksum"`    // SHA-256 hex of staged binary (empty = skip)
	NewVersion string `json:"new_version"`
	OldVersion string `json:"old_version"`

	// Reporting — for agent updates (rollout tracking)
	RolloutPlanID uint64 `json:"rollout_plan_id,omitempty"`
	ServerID      uint64 `json:"server_id,omitempty"`
	CallbackURL   string `json:"callback_url,omitempty"` // POST result here (agent rollouts)
	CallbackKey   string `json:"callback_key,omitempty"` // X-Agent-Key value

	// For panel/API updates: callback is not possible (API is being restarted).
	// Updater writes the result to ResultFilePath instead.
	// The API reads ResultFilePath on startup and stores the outcome.
	WriteResultFile bool `json:"write_result_file,omitempty"`
}

// UpdateResult is written to ResultFilePath and/or POSTed to CallbackURL.
type UpdateResult struct {
	TargetService string `json:"target_service"`
	OldVersion    string `json:"old_version"`
	NewVersion    string `json:"new_version"`
	Status        string `json:"status"` // "completed" | "failed" | "rolled_back"
	Error         string `json:"error,omitempty"`
	Timestamp     string `json:"timestamp"`

	// For rollout tracking
	RolloutPlanID uint64 `json:"rollout_plan_id,omitempty"`
	ServerID      uint64 `json:"server_id,omitempty"`
}

// Run is the entry point for the wp-manage-updater binary.
// It reads MetadataPath, performs the binary swap, and reports the result.
func Run(apiURL, apiKey string) {
	slog.Info("wp-manage-updater started")

	meta, err := readMetadata()
	if err != nil {
		slog.Error("Cannot read pending-update metadata — nothing to do", "error", err)
		// Not fatal: may have been started accidentally; just exit cleanly.
		os.Exit(0)
	}

	// Fill in defaults
	if meta.TargetBinary == "" {
		switch meta.TargetService {
		case "wp-manage-api":
			meta.TargetBinary = APIBinaryPath
		default:
			meta.TargetBinary = AgentBinaryPath
		}
	}
	if meta.StagedPath == "" {
		meta.StagedPath = StagedBinaryPath
	}
	// Use caller-supplied API credentials if metadata doesn't have its own.
	if meta.CallbackURL == "" && apiURL != "" && meta.RolloutPlanID != 0 {
		meta.CallbackURL = apiURL + "/api/v1/agent/rollouts/complete"
		meta.CallbackKey = apiKey
	}

	slog.Info("Applying update",
		"service", meta.TargetService,
		"binary", meta.TargetBinary,
		"old", meta.OldVersion,
		"new", meta.NewVersion)

	applyUpdate(meta)
}

func applyUpdate(meta *PendingUpdate) {
	backupPath := meta.TargetBinary + BackupSuffix

	report := func(status, errMsg string) {
		result := UpdateResult{
			TargetService: meta.TargetService,
			OldVersion:    meta.OldVersion,
			NewVersion:    meta.NewVersion,
			Status:        status,
			Error:         errMsg,
			Timestamp:     time.Now().Format(time.RFC3339),
			RolloutPlanID: meta.RolloutPlanID,
			ServerID:      meta.ServerID,
		}
		// Write to file (always — API reads on startup for panel updates)
		if data, err := json.MarshalIndent(result, "", "  "); err == nil {
			os.WriteFile(ResultFilePath, data, 0644)
		}
		// POST callback (for agent rollout tracking)
		if meta.CallbackURL != "" {
			postCallback(meta.CallbackURL, meta.CallbackKey, result)
		}
	}

	// ── 1. Verify staged binary ───────────────────────────────────────────────
	if err := verifyChecksum(meta.StagedPath, meta.Checksum); err != nil {
		slog.Error("Checksum mismatch — aborting", "error", err)
		report("failed", "checksum mismatch: "+err.Error())
		cleanup(meta)
		return
	}

	// ── 2. Stop the target service ────────────────────────────────────────────
	slog.Info("Stopping service", "service", meta.TargetService)
	exec.Command("systemctl", "stop", meta.TargetService).Run()
	time.Sleep(500 * time.Millisecond)

	// ── 3. Backup current binary ──────────────────────────────────────────────
	if err := copyFile(meta.TargetBinary, backupPath); err != nil {
		slog.Warn("Could not backup current binary (continuing anyway)", "error", err)
	}

	// ── 4. Atomic replace ─────────────────────────────────────────────────────
	if err := replaceAtomically(meta.StagedPath, meta.TargetBinary); err != nil {
		slog.Error("Failed to replace binary", "error", err)
		rollback(meta, backupPath, report, "replace failed: "+err.Error())
		return
	}
	slog.Info("Binary replaced", "path", meta.TargetBinary)

	// ── 5. Start target service ───────────────────────────────────────────────
	slog.Info("Starting service", "service", meta.TargetService)
	if out, err := exec.Command("systemctl", "start", meta.TargetService).CombinedOutput(); err != nil {
		rollback(meta, backupPath, report,
			fmt.Sprintf("start failed: %v — %s", err, string(out)))
		return
	}

	// ── 6. Verify service is alive (wait up to 10s) ───────────────────────────
	slog.Info("Waiting for service to stabilise...")
	alive := false
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if active, _ := isServiceActive(meta.TargetService); active {
			alive = true
			break
		}
	}
	if !alive {
		statusOut, _ := exec.Command("systemctl", "status", meta.TargetService, "--no-pager", "-l").Output()
		rollback(meta, backupPath, report,
			fmt.Sprintf("service not active after 10s. Status:\n%s", string(statusOut)))
		return
	}

	// ── 7. Success ────────────────────────────────────────────────────────────
	slog.Info("Update applied successfully",
		"service", meta.TargetService,
		"version", meta.NewVersion)
	report("completed", "")
	cleanup(meta)
	slog.Info("wp-manage-updater done")
}

// rollback restores the backup binary, restarts the service, and reports failure.
func rollback(meta *PendingUpdate, backupPath string,
	report func(string, string), reason string) {

	slog.Warn("Rolling back", "service", meta.TargetService, "reason", reason)

	if _, err := os.Stat(backupPath); err == nil {
		copyFile(backupPath, meta.TargetBinary)
		os.Chmod(meta.TargetBinary, 0755)
		exec.Command("systemctl", "start", meta.TargetService).Run()
		time.Sleep(3 * time.Second)

		if active, _ := isServiceActive(meta.TargetService); active {
			slog.Info("Rollback succeeded — service restored to previous version")
			report("rolled_back", reason)
		} else {
			slog.Error("Rollback FAILED — service is not running!", "service", meta.TargetService)
			report("failed", reason+" | rollback also failed — manual intervention required")
		}
	} else {
		slog.Error("No backup found — cannot rollback!", "backup", backupPath)
		report("failed", reason+" | no backup available")
	}

	cleanup(meta)
}

// replaceAtomically writes src → dst via a temp file + rename (atomic on same fs).
func replaceAtomically(src, dst string) error {
	tmp := dst + ".tmp"
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close(); os.Remove(tmp)
		return fmt.Errorf("copy: %w", err)
	}
	out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func verifyChecksum(path, expected string) error {
	if expected == "" {
		slog.Warn("No checksum — skipping verification", "path", path)
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("expected %s, got %s", expected, actual)
	}
	return nil
}

func isServiceActive(name string) (bool, error) {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	return string(bytes.TrimSpace(out)) == "active", err
}

func postCallback(url, key string, result UpdateResult) {
	data, _ := json.Marshal(map[string]interface{}{
		"rollout_plan_id": result.RolloutPlanID,
		"server_id":       result.ServerID,
		"status":          result.Status,
		"old_version":     result.OldVersion,
		"new_version":     result.NewVersion,
		"error":           result.Error,
	})
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		slog.Error("Failed to build callback request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", key)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Callback failed", "url", url, "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("Callback sent", "url", url, "http", resp.StatusCode, "status", result.Status)
}

func readMetadata() (*PendingUpdate, error) {
	raw, err := os.ReadFile(MetadataPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", MetadataPath, err)
	}
	var meta PendingUpdate
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if meta.TargetService == "" {
		return nil, fmt.Errorf("target_service is required")
	}
	return &meta, nil
}

func cleanup(meta *PendingUpdate) {
	if meta != nil && meta.StagedPath != "" {
		os.Remove(meta.StagedPath)
	}
	os.Remove(MetadataPath)
}
