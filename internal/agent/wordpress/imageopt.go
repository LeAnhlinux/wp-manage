package wordpress

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// OptimizeImages runs image optimization on a site's uploads directory
func OptimizeImages(task models.AgentImageOptTask) models.AgentImageOptResult {
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

	for _, imgPath := range images {
		// Check if already optimized (marker file)
		markerFile := imgPath + ".wpmanage-optimized"
		if _, err := os.Stat(markerFile); err == nil {
			result.SkippedCount++
			continue
		}

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

			// Generate WebP version
			if task.WebPEnabled {
				webpPath := imgPath + ".webp"
				if _, err := os.Stat(webpPath); os.IsNotExist(err) {
					cmd := exec.Command("cwebp", "-q", fmt.Sprintf("%d", task.Quality), imgPath, "-o", webpPath)
					cmd.Run() // Best-effort, don't fail on WebP conversion
				}
			}

			// Create marker file
			os.WriteFile(markerFile, []byte(""), 0644)
			result.OptimizedCount++
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
