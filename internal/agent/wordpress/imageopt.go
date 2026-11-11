package wordpress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"

	"github.com/gofiber-team/wp-manage/pkg/utils"
)

const (
	imageOptTimeout       = 60 * time.Minute // hard stop after 60 min
	imageOptProgressEvery = 50               // send progress update every N processed images
	imageOptMaxWorkers    = 8                // cap parallelism to avoid disk thrashing / OOM on small VPS
)

// OptimizeImages runs image optimization on a site's uploads directory.
// progressFn is called every imageOptProgressEvery images; returns true to cancel.
//
// Concurrency model:
//   - Worker pool sized to min(NumCPU, 8)
//   - Atomic counters (no mutex needed for hot path)
//   - Single batch `chown -R` at end (defer) replaces 3 per-file chown calls
//   - cwebp tuned: -m 2 -mt -low_memory (~4x faster than defaults, ~1.5% larger files)
//   - copyFile uses io.Copy streaming (no full-file load into RAM)
//
// Expected speedup on 4-core VPS: ~10x vs sequential implementation.
func OptimizeImages(task models.AgentImageOptTask, progressFn func(models.AgentImageOptResult) bool) models.AgentImageOptResult {
	result := models.AgentImageOptResult{
		ResultID: task.ResultID,
		SiteID:   task.SiteID,
		Status:   "completed",
	}

	uploadsDir := fmt.Sprintf("/var/www/%s/public/wp-content/uploads", task.Domain)
	siteUser := utils.SiteLinuxUser(task.Domain)

	// Check if uploads dir exists
	if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
		result.Status = "failed"
		result.Error = "uploads directory not found"
		return result
	}

	// Ensure optimization tools are installed
	ensureOptTools()

	// Batch chown -R at end (replaces per-file chown calls inside the hot loop).
	// Idempotent — only metadata change for already-correct files.
	defer exec.Command("chown", "-R", siteUser+":"+siteUser, uploadsDir).Run()

	// Find all JPEG and PNG images (single sequential walk — I/O bound, not worth parallelizing)
	var images []string
	filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			images = append(images, path)
		}
		return nil
	})

	result.TotalImages = len(images)
	workers := optimizerWorkers()
	slog.Info("Found images to optimize",
		"count", len(images), "domain", task.Domain, "workers", workers)

	deadline := time.Now().Add(imageOptTimeout)

	// Thread-safe counters
	var (
		optimizedCount int64
		skippedCount   int64
		errorCount     int64
		bytesSaved     int64
		processedCount int64
		cancelledFlag  int32
		timedOutFlag   int32
	)

	jobs := make(chan string, workers*2)
	var wg sync.WaitGroup

	// Spawn worker pool
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for imgPath := range jobs {
				// Fast-exit on cancel/timeout — drain channel without processing
				if atomic.LoadInt32(&cancelledFlag) != 0 {
					continue
				}
				if time.Now().After(deadline) {
					atomic.StoreInt32(&timedOutFlag, 1)
					continue
				}

				outcome, saved := processOneImage(imgPath, task)

				switch outcome {
				case "skipped":
					atomic.AddInt64(&skippedCount, 1)
				case "optimized":
					atomic.AddInt64(&optimizedCount, 1)
					if saved > 0 {
						atomic.AddInt64(&bytesSaved, saved)
					}
				case "error":
					atomic.AddInt64(&errorCount, 1)
				}

				// Progress reporting — naturally serialized via atomic increment
				processed := atomic.AddInt64(&processedCount, 1)
				if progressFn != nil && processed%imageOptProgressEvery == 0 {
					snapshot := models.AgentImageOptResult{
						ResultID:       task.ResultID,
						SiteID:         task.SiteID,
						Status:         "completed",
						TotalImages:    len(images),
						OptimizedCount: int(atomic.LoadInt64(&optimizedCount)),
						SkippedCount:   int(atomic.LoadInt64(&skippedCount)),
						ErrorCount:     int(atomic.LoadInt64(&errorCount)),
						BytesSaved:     atomic.LoadInt64(&bytesSaved),
					}
					if progressFn(snapshot) {
						slog.Info("Image optimization cancelled by user", "domain", task.Domain)
						atomic.StoreInt32(&cancelledFlag, 1)
					}
				}
			}
		}()
	}

	// Feed jobs into the channel
	for _, img := range images {
		if atomic.LoadInt32(&cancelledFlag) != 0 {
			break
		}
		jobs <- img
	}
	close(jobs)
	wg.Wait()

	// Finalize counters
	result.OptimizedCount = int(optimizedCount)
	result.SkippedCount = int(skippedCount)
	result.ErrorCount = int(errorCount)
	result.BytesSaved = bytesSaved

	if atomic.LoadInt32(&cancelledFlag) != 0 {
		result.Status = "cancelled"
	}
	if atomic.LoadInt32(&timedOutFlag) != 0 {
		slog.Info("Image optimization timeout reached", "domain", task.Domain)
	}

	slog.Info("Image optimization complete",
		"domain", task.Domain,
		"total", result.TotalImages,
		"optimized", result.OptimizedCount,
		"skipped", result.SkippedCount,
		"errors", result.ErrorCount,
		"saved_bytes", result.BytesSaved,
		"workers", workers)

	return result
}

// processOneImage handles a single image: marker check → backup → optimize → WebP → marker.
// Returns (outcome, bytesSaved). Outcome is "skipped" | "optimized" | "error".
// No per-file chown — batch chown -R is done at end of OptimizeImages.
func processOneImage(imgPath string, task models.AgentImageOptTask) (string, int64) {
	markerFile := imgPath + ".wpmanage-optimized"
	if _, err := os.Stat(markerFile); err == nil {
		return "skipped", 0
	}

	info, err := os.Stat(imgPath)
	if err != nil {
		return "error", 0
	}
	originalSize := info.Size()

	ext := strings.ToLower(filepath.Ext(imgPath))

	// Backup original before optimizing (skip if backup already exists).
	// Streams via io.Copy — no full-file load into RAM.
	origBackup := imgPath + ".wpmanage-orig"
	if _, err := os.Stat(origBackup); os.IsNotExist(err) {
		copyFile(imgPath, origBackup)
	}

	var optimized bool
	switch ext {
	case ".jpg", ".jpeg":
		cmd := exec.Command("jpegoptim",
			fmt.Sprintf("--max=%d", task.Quality),
			"--strip-all",
			"--quiet",
			imgPath)
		if err := cmd.Run(); err != nil {
			slog.Debug("jpegoptim failed", "file", imgPath, "error", err)
			os.Remove(origBackup)
			return "error", 0
		}
		optimized = true

	case ".png":
		cmd := exec.Command("optipng", "-o2", "-quiet", imgPath)
		if err := cmd.Run(); err != nil {
			slog.Debug("optipng failed", "file", imgPath, "error", err)
			os.Remove(origBackup)
			return "error", 0
		}
		optimized = true
	}

	if !optimized {
		return "error", 0
	}

	// Calculate bytes saved
	var saved int64
	if newInfo, err := os.Stat(imgPath); err == nil {
		if d := originalSize - newInfo.Size(); d > 0 {
			saved = d
		}
	}

	// Generate WebP version with tuned flags:
	//   -m 2          method 2 instead of default 4 — ~4x faster encode, ~1.5% larger output
	//   -mt           multi-threaded encoding inside cwebp itself
	//   -quiet        suppress progress bar (less log spam)
	//   -low_memory   smaller working set — safer when running many workers in parallel
	if task.WebPEnabled {
		webpPath := imgPath + ".webp"
		if _, err := os.Stat(webpPath); os.IsNotExist(err) {
			exec.Command("cwebp",
				"-q", fmt.Sprintf("%d", task.Quality),
				"-m", "2",
				"-mt",
				"-quiet",
				"-low_memory",
				imgPath,
				"-o", webpPath,
			).Run()
		}
	}

	// Create marker file (ownership fixed by batch chown -R at end of OptimizeImages)
	os.WriteFile(markerFile, []byte(""), 0644)
	return "optimized", saved
}

// optimizerWorkers returns the worker pool size.
//
// Priority:
//  1. Env var WPM_IMAGEOPT_WORKERS (manual override — admin tuning)
//  2. runtime.NumCPU() capped at imageOptMaxWorkers (auto-scan default)
//
// Auto cap exists because too many parallel workers cause disk I/O thrashing
// and OOM on small VPS (each cwebp process uses ~30-50MB RAM).
//
// Manual override is allowed up to a hard sanity limit of 64 — bypasses the
// auto cap so admins on big NVMe servers can push higher if they know what
// they're doing.
//
// Examples (set in /etc/systemd/system/wp-manage-agent.service.d/override.conf):
//
//	[Service]
//	Environment="WPM_IMAGEOPT_WORKERS=2"   # force low — 1GB VPS sharing CPU with WP
//	Environment="WPM_IMAGEOPT_WORKERS=16"  # push high — 32-core NVMe dedicated box
func optimizerWorkers() int {
	if v := os.Getenv("WPM_IMAGEOPT_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 64 {
				n = 64 // hard sanity ceiling — prevents typo disasters (e.g. 1000)
			}
			slog.Info("Image optimizer using env override",
				"workers", n, "var", "WPM_IMAGEOPT_WORKERS")
			return n
		}
		slog.Warn("Invalid WPM_IMAGEOPT_WORKERS — falling back to auto-scan",
			"value", v)
	}

	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > imageOptMaxWorkers {
		n = imageOptMaxWorkers
	}
	return n
}

// SendProgressUpdate posts a progress update to the API.
// Returns true if the API signals to cancel the run.
func SendProgressUpdate(apiURL, apiKey string, result models.AgentImageOptResult) bool {
	data, err := json.Marshal(result)
	if err != nil {
		return false
	}
	req, err := http.NewRequest("POST", apiURL+"/api/v1/agent/images/optimize/progress", bytes.NewReader(data))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", apiKey)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("Progress update failed", "error", err)
		return false
	}
	defer resp.Body.Close()
	// API returns {cancel: true} when user requested cancellation
	var reply struct {
		Cancel bool `json:"cancel"`
	}
	json.NewDecoder(resp.Body).Decode(&reply)
	return reply.Cancel
}

// copyFile streams src to dst using io.Copy (no full-file load into RAM).
// Important when multiple workers run in parallel — avoids GC pressure spikes
// on large images (5-20MB).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// GetOriginalsStats returns count and total size of .wpmanage-orig backup files
func GetOriginalsStats(domain string) (count int, sizeBytes int64) {
	uploadsDir := fmt.Sprintf("/var/www/%s/public/wp-content/uploads", domain)
	filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".wpmanage-orig") {
			count++
			sizeBytes += info.Size()
		}
		return nil
	})
	return
}

// RevertImages restores all .wpmanage-orig backups to the original filenames,
// removes the .wpmanage-optimized marker and the .webp sidecar so the image
// can be re-optimized in the future.  Returns the number of files reverted.
func RevertImages(domain string) (reverted int, errMsg string) {
	uploadsDir := fmt.Sprintf("/var/www/%s/public/wp-content/uploads", domain)
	siteUser := utils.SiteLinuxUser(domain)

	filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".wpmanage-orig") {
			return nil
		}
		origPath := strings.TrimSuffix(path, ".wpmanage-orig")
		if copyErr := copyFile(path, origPath); copyErr != nil {
			slog.Warn("Failed to revert image", "file", origPath, "error", copyErr)
			return nil
		}
		os.Remove(path)                             // remove .wpmanage-orig
		os.Remove(origPath + ".wpmanage-optimized") // remove marker so it can be re-optimized
		os.Remove(origPath + ".webp")               // remove WebP sidecar
		reverted++
		return nil
	})

	// Batch chown -R at end — same pattern as OptimizeImages
	exec.Command("chown", "-R", siteUser+":"+siteUser, uploadsDir).Run()
	return
}

// DeleteOriginals removes all .wpmanage-orig backup files to free disk space.
// Returns the number of files deleted and total bytes freed.
func DeleteOriginals(domain string) (deleted int, sizeFreed int64) {
	uploadsDir := fmt.Sprintf("/var/www/%s/public/wp-content/uploads", domain)
	filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".wpmanage-orig") {
			sizeFreed += info.Size()
			os.Remove(path)
			deleted++
		}
		return nil
	})
	return
}

// ensureOptTools ensures optimization tools are installed
func ensureOptTools() {
	tools := map[string]string{
		"jpegoptim": "jpegoptim",
		"optipng":   "optipng",
		"cwebp":     "webp",
	}

	for binary, pkg := range tools {
		if _, err := exec.LookPath(binary); err != nil {
			slog.Info("Installing missing tool", "tool", binary, "package", pkg)
			exec.Command("apt-get", "install", "-y", pkg).Run()
		}
	}
}

// ToggleImageOpt installs or removes the mu-plugin that auto-compresses on WP upload.
// The mu-plugin uses the same tuned cwebp flags as the batch optimizer.
func ToggleImageOpt(domain string, enabled bool, quality int, webpEnabled bool) error {
	muDir := fmt.Sprintf("/var/www/%s/public/wp-content/mu-plugins", domain)
	muFile := muDir + "/wp-manage-imageopt.php"

	if !enabled {
		os.Remove(muFile)
		slog.Info("Image opt mu-plugin removed", "domain", domain)
		return nil
	}

	// Ensure tools are installed before installing plugin
	ensureOptTools()

	if err := os.MkdirAll(muDir, 0755); err != nil {
		return fmt.Errorf("cannot create mu-plugins dir: %w", err)
	}

	webpStr := "false"
	if webpEnabled {
		webpStr = "true"
	}

	phpContent := fmt.Sprintf(`<?php
/**
 * WP Manage — Image Optimization (auto-generated, do not edit)
 * Quality: %d | WebP: %s
 */

add_filter( 'wp_handle_upload', 'wpmanage_imageopt_on_upload' );

function wpmanage_imageopt_on_upload( $upload ) {
    if ( empty( $upload['file'] ) ) return $upload;
    $file = $upload['file'];
    $ext  = strtolower( pathinfo( $file, PATHINFO_EXTENSION ) );

    if ( in_array( $ext, [ 'jpg', 'jpeg' ], true ) ) {
        @exec( 'jpegoptim --max=%d --strip-all --quiet ' . escapeshellarg( $file ) . ' >/dev/null 2>&1 &' );
    } elseif ( $ext === 'png' ) {
        @exec( 'optipng -o2 -quiet ' . escapeshellarg( $file ) . ' >/dev/null 2>&1 &' );
    }

    if ( %s && in_array( $ext, [ 'jpg', 'jpeg', 'png' ], true ) ) {
        $webp = $file . '.webp';
        @exec( 'cwebp -q %d -m 2 -mt -quiet -low_memory ' . escapeshellarg( $file ) . ' -o ' . escapeshellarg( $webp ) . ' >/dev/null 2>&1 &' );
    }

    return $upload;
}
`, quality, webpStr, quality, webpStr, quality)

	if err := os.WriteFile(muFile, []byte(phpContent), 0644); err != nil {
		return fmt.Errorf("cannot write mu-plugin: %w", err)
	}

	// Fix ownership to site user
	siteUser := utils.SiteLinuxUser(domain)
	exec.Command("chown", "-R", siteUser+":"+siteUser, muDir).Run()

	slog.Info("Image opt mu-plugin installed", "domain", domain, "quality", quality, "webp", webpEnabled)
	return nil
}
