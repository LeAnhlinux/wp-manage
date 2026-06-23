package services

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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
		WebURL:       req.WebURL,
		WebChecksum:  req.WebChecksum,
		ReleaseNotes: req.ReleaseNotes,
		FileSize:     req.FileSize,
	}
	// Upsert: if same version already exists, update download_url/checksum and return it.
	if err := s.db.Where(models.PanelRelease{Version: req.Version}).
		Assign(models.PanelRelease{
			DownloadURL:  req.DownloadURL,
			Checksum:     req.Checksum,
			WebURL:       req.WebURL,
			WebChecksum:  req.WebChecksum,
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

// TriggerUpdate stages the new API binary, applies the web archive, then hands
// off to wp-manage-updater.
//
// Flow:
//  1. Download API binary from release.DownloadURL → /var/lib/wp-manage/api-staged
//  2. Verify SHA-256 checksum of API binary
//  3. Download web archive from release.WebURL, verify checksum, extract to /opt/wp-manage/web/
//  4. (step 3 only runs when WebURL is set)
//  5. Write PendingUpdate metadata (target=wp-manage-api, write_result_file=true)
//  6. systemctl start wp-manage-updater  → returns immediately (202)
//  7. Updater stops this API, swaps binary, starts API, writes ResultFilePath
//  8. Frontend polls /settings/panel-update until API responds, then shows result
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

	// ── 3. Download and apply web archive (if provided) ──────────────────────
	if release.WebURL != "" {
		webDir := "/opt/wp-manage/web"
		webStaged := "/var/lib/wp-manage/web-staged.tar.gz"

		slog.Info("Downloading web archive", "url", release.WebURL)
		if err := downloadFile(release.WebURL, webStaged); err != nil {
			os.Remove(stagedPath)
			return fmt.Errorf("web archive download failed: %w", err)
		}

		// Verify web archive checksum (if provided)
		if release.WebChecksum != "" {
			out, err := exec.Command("sha256sum", webStaged).Output()
			if err != nil {
				os.Remove(stagedPath)
				os.Remove(webStaged)
				return fmt.Errorf("sha256sum web failed: %w", err)
			}
			actual := ""
			if len(out) >= 64 {
				actual = string(out[:64])
			}
			if actual != release.WebChecksum {
				os.Remove(stagedPath)
				os.Remove(webStaged)
				return fmt.Errorf("web checksum mismatch: expected %s, got %s", release.WebChecksum, actual)
			}
			slog.Info("Web archive checksum verified", "sha256", actual)
		}

		// Apply atomically: unpack into a staging dir, then swap it in. This removes
		// stale files (e.g. old hashed index-*.js) instead of letting them accumulate —
		// clients can't SSH in to clean up by hand.
		if err := applyWebArchive(webStaged, webDir); err != nil {
			os.Remove(stagedPath)
			os.Remove(webStaged)
			return fmt.Errorf("web archive extraction failed: %w", err)
		}
		os.Remove(webStaged)
		slog.Info("Web archive applied", "dst", webDir)

		// Make sure nginx never caches index.html so browsers always fetch the new
		// asset hashes. Idempotent + self-healing: patches panels installed before
		// this guard, validates with `nginx -t`, rolls back on failure, never fatal.
		if err := EnsurePanelNginxNoCacheIndex(); err != nil {
			slog.Warn("Could not apply nginx no-cache for index.html (non-fatal)", "error", err)
		}
	}

	// ── 4. Write updater metadata ─────────────────────────────────────────────
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

	// ── 5. Start wp-manage-updater (one-shot service) ─────────────────────────
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

// applyWebArchive unpacks the web tarball into a staging dir next to webDir,
// then atomically swaps it in. This guarantees files removed in the new release
// (notably old content-hashed assets) are gone, instead of piling up forever.
func applyWebArchive(src, webDir string) error {
	newDir := webDir + ".new"
	oldDir := webDir + ".old"

	os.RemoveAll(newDir)
	if err := extractTarGz(src, newDir); err != nil {
		os.RemoveAll(newDir)
		return err
	}
	// Refuse to swap in an archive that has no index.html — that would brick the panel.
	if _, err := os.Stat(filepath.Join(newDir, "index.html")); err != nil {
		os.RemoveAll(newDir)
		return fmt.Errorf("archive missing index.html — refusing to swap")
	}

	os.RemoveAll(oldDir)
	if _, err := os.Stat(webDir); err == nil {
		if err := os.Rename(webDir, oldDir); err != nil {
			os.RemoveAll(newDir)
			return fmt.Errorf("cannot move current web dir aside: %w", err)
		}
	}
	if err := os.Rename(newDir, webDir); err != nil {
		if _, statErr := os.Stat(oldDir); statErr == nil {
			os.Rename(oldDir, webDir) // rollback
		}
		return fmt.Errorf("cannot swap in new web dir: %w", err)
	}
	os.RemoveAll(oldDir)
	return nil
}

const panelNginxConf = "/etc/nginx/sites-available/wp-manage-panel.conf"

// EnsurePanelNginxNoCacheIndex makes the panel vhost send Cache-Control: no-cache
// for index.html, so browsers always pick up new asset hashes after an update.
// Idempotent (skips if already present), validates with `nginx -t`, and rolls back
// the file if validation fails. Returns nil when the config isn't found (custom setups).
func EnsurePanelNginxNoCacheIndex() error {
	data, err := os.ReadFile(panelNginxConf)
	if err != nil {
		return nil // non-standard install — nothing to patch
	}
	content := string(data)
	if strings.Contains(content, "location = /index.html") {
		return nil // already patched
	}

	// Anchor the insert inside the web-serving server block (the dotfile deny rule
	// only exists there, not in the cert-bot HTTP→HTTPS redirect block).
	marker := "location ~ /\\."
	idx := strings.Index(content, marker)
	if idx < 0 {
		return fmt.Errorf("insertion point not found in nginx config")
	}

	webRoot := "/opt/wp-manage/web"
	if m := regexp.MustCompile(`root\s+(\S+?)/?;`).FindStringSubmatch(content); len(m) == 2 {
		webRoot = strings.TrimSuffix(m[1], "/")
	}

	block := fmt.Sprintf("location = /index.html {\n"+
		"        root %s;\n"+
		"        add_header Cache-Control \"no-cache, no-store, must-revalidate\";\n"+
		"        expires off;\n"+
		"    }\n\n    ", webRoot)
	patched := content[:idx] + block + content[idx:]

	if err := os.WriteFile(panelNginxConf, []byte(patched), 0644); err != nil {
		return err
	}
	if out, terr := exec.Command("nginx", "-t").CombinedOutput(); terr != nil {
		os.WriteFile(panelNginxConf, data, 0644) // rollback
		return fmt.Errorf("nginx -t failed after patch, rolled back: %s", strings.TrimSpace(string(out)))
	}
	exec.Command("systemctl", "reload", "nginx").Run()
	slog.Info("Patched nginx: index.html now served with no-cache")
	return nil
}

// extractTarGz extracts a .tar.gz archive into dst directory.
// Existing files in dst are overwritten; dst is created if it does not exist.
func extractTarGz(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// Security: reject paths that escape dst
		target := filepath.Join(dst, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && target != filepath.Clean(dst) {
			return fmt.Errorf("tar: illegal file path %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
