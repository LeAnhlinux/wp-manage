package wordpress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

const (
	imageOptTimeout       = 60 * time.Minute // hard stop after 60 min
	imageOptProgressEvery = 50               // send progress update every N processed images
)

// OptimizeImages runs image optimization on a site's uploads directory.
// progressFn is called every imageOptProgressEvery images; returns true to cancel.
func OptimizeImages(task models.AgentImageOptTask, progressFn func(models.AgentImageOptResult) bool) models.AgentImageOptResult {
	result := models.AgentImageOptResult{
		ResultID: task.ResultID,
		SiteID:   task.SiteID,
		Status:   "completed",
	}

	uploadsDir := fmt.Sprintf("/var/www/%s/public/wp-content/uploads", task.Domain)

	// Check if uploads dir exists
	if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
		result.Status = "failed"
		result.Error = "uploads directory not found"
		return result
	}

	// Ensure optimization tools are installed
	ensureOptTools()

	// Find all JPEG and PNG images
	var images []string
	filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			images = append(images, path)
		}
		return nil
	})

	result.TotalImages = len(images)
	slog.Info("Found images to optimize", "count", len(images), "domain", task.Domain)

	deadline := time.Now().Add(imageOptTimeout)
	processedCount := 0

	for _, imgPath := range images {
		// Enforce timeout (60 min hard stop)
		if time.Now().After(deadline) {
			slog.Info("Image optimization timeout reached", "domain", task.Domain)
			break
		}
		// Check if already optimized (marker file)
		markerFile := imgPath + ".wpmanage-optimized"
		if _, err := os.Stat(markerFile); err == nil {
			result.SkippedCount++
			continue
		}
		processedCount++

		// Get original size
		info, err := os.Stat(imgPath)
		if err != nil {
			result.ErrorCount++
			continue
		}
		originalSize := info.Size()

		ext := strings.ToLower(filepath.Ext(imgPath))
		var optimized bool

		switch ext {
		case ".jpg", ".jpeg":
			// Optimize JPEG with jpegoptim
			cmd := exec.Command("jpegoptim", fmt.Sprintf("--max=%d", task.Quality), "--strip-all", imgPath)
			if err := cmd.Run(); err != nil {
				slog.Debug("jpegoptim failed", "file", imgPath, "error", err)
				result.ErrorCount++
				continue
			}
			optimized = true

		case ".png":
			// Optimize PNG with optipng
			cmd := exec.Command("optipng", "-o2", "-quiet", imgPath)
			if err := cmd.Run(); err != nil {
				slog.Debug("optipng failed", "file", imgPath, "error", err)
				result.ErrorCount++
				continue
			}
			optimized = true
		}

		if optimized {
			// Calculate saved bytes
			newInfo, err := os.Stat(imgPath)
			if err == nil {
				saved := originalSize - newInfo.Size()
				if saved > 0 {
					result.BytesSaved += saved
				}
			}

			siteUser := "wp_" + strings.ReplaceAll(task.Domain, ".", "_")

			// Generate WebP version
			if task.WebPEnabled {
				webpPath := imgPath + ".webp"
				if _, err := os.Stat(webpPath); os.IsNotExist(err) {
					cmd := exec.Command("cwebp", "-q", fmt.Sprintf("%d", task.Quality), imgPath, "-o", webpPath)
					if cmd.Run() == nil {
						// Fix ownership — agent runs as root, webp must belong to site user
						exec.Command("chown", siteUser+":"+siteUser, webpPath).Run()
					}
				}
			}

			// Create marker file and fix ownership
			os.WriteFile(markerFile, []byte(""), 0644)
			exec.Command("chown", siteUser+":"+siteUser, markerFile).Run()
			result.OptimizedCount++
		}

		// Send progress update every imageOptProgressEvery images; cancel if requested
		if progressFn != nil && processedCount%imageOptProgressEvery == 0 {
			if cancelled := progressFn(result); cancelled {
				slog.Info("Image optimization cancelled by user", "domain", task.Domain)
				result.Status = "cancelled"
				return result
			}
		}
	}

	slog.Info("Image optimization complete",
		"domain", task.Domain,
		"total", result.TotalImages,
		"optimized", result.OptimizedCount,
		"skipped", result.SkippedCount,
		"errors", result.ErrorCount,
		"saved_bytes", result.BytesSaved)

	return result
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
        @exec( 'jpegoptim --max=%d --strip-all ' . escapeshellarg( $file ) . ' >/dev/null 2>&1 &' );
    } elseif ( $ext === 'png' ) {
        @exec( 'optipng -o2 -quiet ' . escapeshellarg( $file ) . ' >/dev/null 2>&1 &' );
    }

    if ( %s && in_array( $ext, [ 'jpg', 'jpeg', 'png' ], true ) ) {
        $webp = $file . '.webp';
        @exec( 'cwebp -q %d ' . escapeshellarg( $file ) . ' -o ' . escapeshellarg( $webp ) . ' >/dev/null 2>&1 &' );
    }

    return $upload;
}
`, quality, webpStr, quality, webpStr, quality)

	if err := os.WriteFile(muFile, []byte(phpContent), 0644); err != nil {
		return fmt.Errorf("cannot write mu-plugin: %w", err)
	}

	// Fix ownership to site user
	siteUser := "wp_" + strings.ReplaceAll(domain, ".", "_")
	exec.Command("chown", "-R", siteUser+":"+siteUser, muDir).Run()

	slog.Info("Image opt mu-plugin installed", "domain", domain, "quality", quality, "webp", webpEnabled)
	return nil
}
