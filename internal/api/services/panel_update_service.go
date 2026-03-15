package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	agentUpdater "github.com/gofiber-team/wp-manage/internal/agent/updater"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type PanelUpdateService struct {
	db             *gorm.DB
	currentVersion string
}

func NewPanelUpdateService(db *gorm.DB, currentVersion string) *PanelUpdateService {
	return &PanelUpdateService{db: db, currentVersion: currentVersion}
}

func (s *PanelUpdateService) GetCurrentVersion() string {
	return s.currentVersion
}

func (s *PanelUpdateService) GetLatestRelease() (*models.PanelRelease, error) {
	var release models.PanelRelease
	err := s.db.Order("created_at DESC").First(&release).Error
	if err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *PanelUpdateService) CreateRelease(req models.CreatePanelReleaseRequest) (*models.PanelRelease, error) {
	release := models.PanelRelease{
		Version:      req.Version,
		DownloadURL:  req.DownloadURL,
		Checksum:     req.Checksum,
		ReleaseNotes: req.ReleaseNotes,
		FileSize:     req.FileSize,
	}
	// Upsert: if same version already exists, update download_url/checksum and return it.
	if err := s.db.Where(models.PanelRelease{Version: req.Version}).
		Assign(models.PanelRelease{
			DownloadURL:  req.DownloadURL,
			Checksum:     req.Checksum,
			ReleaseNotes: req.ReleaseNotes,
		}).
		FirstOrCreate(&release).Error; err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *PanelUpdateService) ListReleases() ([]models.PanelRelease, error) {
	var releases []models.PanelRelease
	err := s.db.Order("created_at DESC").Limit(20).Find(&releases).Error
	return releases, err
}

// TriggerUpdate stages the new API binary and hands off to wp-manage-updater.
//
// Flow:
//  1. Download binary from release.DownloadURL → /var/lib/wp-manage/api-staged
//  2. Verify SHA-256 checksum
//  3. Write PendingUpdate metadata (target=wp-manage-api, write_result_file=true)
//  4. systemctl start wp-manage-updater  → returns immediately (202)
//  5. Updater stops this API, swaps binary, starts API, writes ResultFilePath
//  6. Frontend polls /settings/panel-update until API responds, then shows result
func (s *PanelUpdateService) TriggerUpdate(releaseID uint64) error {
	var release models.PanelRelease
	if err := s.db.First(&release, releaseID).Error; err != nil {
		return fmt.Errorf("release not found")
	}
	if release.Version == s.currentVersion {
		return fmt.Errorf("already running version %s", release.Version)
	}
	if release.DownloadURL == "" {
		return fmt.Errorf("release has no download URL")
	}

	os.MkdirAll("/var/lib/wp-manage", 0750)
	stagedPath := "/var/lib/wp-manage/api-staged"

	// ── 1. Download ───────────────────────────────────────────────────────────
	slog.Info("Downloading panel update", "version", release.Version, "url", release.DownloadURL)
	if err := downloadFile(release.DownloadURL, stagedPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	os.Chmod(stagedPath, 0755)

	// ── 2. Verify checksum ────────────────────────────────────────────────────
	if release.Checksum != "" {
		out, err := exec.Command("sha256sum", stagedPath).Output()
		if err != nil {
			os.Remove(stagedPath)
			return fmt.Errorf("sha256sum failed: %w", err)
		}
		actual := ""
		if len(out) >= 64 {
			actual = string(out[:64])
		}
		if actual != release.Checksum {
			os.Remove(stagedPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", release.Checksum, actual)
		}
		slog.Info("Checksum verified", "sha256", actual)
	}

	// ── 3. Write updater metadata ─────────────────────────────────────────────
	meta := agentUpdater.PendingUpdate{
		StagedPath:      stagedPath,
		TargetBinary:    agentUpdater.APIBinaryPath,
		TargetService:   "wp-manage-api",
		Checksum:        release.Checksum,
		NewVersion:      release.Version,
		OldVersion:      s.currentVersion,
		WriteResultFile: true, // API can't receive callback — write to file instead
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(agentUpdater.MetadataPath, data, 0600); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("write metadata: %w", err)
	}

	// ── 4. Start wp-manage-updater (one-shot service) ─────────────────────────
	// This returns immediately. The updater will stop THIS process shortly.
	slog.Info("Starting wp-manage-updater — API will restart shortly", "version", release.Version)
	if out, err := exec.Command("systemctl", "start", "wp-manage-updater").CombinedOutput(); err != nil {
		os.Remove(stagedPath)
		os.Remove(agentUpdater.MetadataPath)
		return fmt.Errorf("could not start updater service: %v — %s", err, string(out))
	}

	return nil
}

// GetLastUpdateResult reads the result written by the updater after a panel update.
// Called on API startup and via GET /settings/panel-update/last-result.
func (s *PanelUpdateService) GetLastUpdateResult() (*agentUpdater.UpdateResult, error) {
	raw, err := os.ReadFile(agentUpdater.ResultFilePath)
	if err != nil {
		return nil, err // file not present = no previous update
	}
	var result agentUpdater.UpdateResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// downloadFile downloads url to dst using net/http (avoids wget dependency on API server).
func downloadFile(url, dst string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
