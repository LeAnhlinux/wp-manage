package filemanager

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

const maxFileReadSize = 10 * 1024 * 1024 // 10MB max for file content read
const maxUploadSize = 2 * 1024 * 1024 * 1024 // 2GB max upload

// authFailEntry tracks failed authentication attempts per IP address.
type authFailEntry struct {
	count    int
	blockedUntil time.Time
}

// authRateLimiter protects the agent HTTP server against brute-force of the API key.
// After 10 consecutive failures from the same IP within 1 minute, that IP is blocked
// for 5 minutes. Legitimate callers (the API server) should never fail auth.
type authRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*authFailEntry
}

func newAuthRateLimiter() *authRateLimiter {
	rl := &authRateLimiter{entries: make(map[string]*authFailEntry)}
	// Background cleanup every 10 minutes
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, e := range rl.entries {
				if now.After(e.blockedUntil) && e.count == 0 {
					delete(rl.entries, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

const (
	authMaxFailures = 10              // max consecutive failures before block
	authBlockWindow = 1 * time.Minute // sliding window for failures
	authBlockDuration = 5 * time.Minute // how long to block after exceeding limit
)

// isBlocked returns true if the IP is currently blocked. On failure it increments
// the counter; on success it resets it.
func (rl *authRateLimiter) recordFailure(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.entries[ip]
	if !ok {
		e = &authFailEntry{}
		rl.entries[ip] = e
	}
	// If currently blocked, reject immediately
	if time.Now().Before(e.blockedUntil) {
		return true
	}
	e.count++
	if e.count >= authMaxFailures {
		e.blockedUntil = time.Now().Add(authBlockDuration)
		e.count = 0
		slog.Warn("Blocking IP after repeated auth failures on agent HTTP server",
			"ip", ip, "block_until", e.blockedUntil)
		return true
	}
	return false
}

func (rl *authRateLimiter) recordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, ip)
}

func (rl *authRateLimiter) checkBlocked(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.entries[ip]
	if !ok {
		return false
	}
	return time.Now().Before(e.blockedUntil)
}

// globalAuthRateLimiter is shared across all Handler instances on this agent process.
var globalAuthRateLimiter = newAuthRateLimiter()

// Handler handles file manager HTTP requests on the agent
type Handler struct {
	apiKey string
}

func NewHandler(apiKey string) *Handler {
	return &Handler{apiKey: apiKey}
}

// siteUser derives the Linux username for a WordPress site.
// Matches the convention in internal/agent/site/manager.go SiteUser().
func siteUser(domain string) string {
	r := strings.NewReplacer(".", "_", "-", "_")
	return "wp_" + r.Replace(domain)
}

// RegisterRoutes registers file manager routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/files/list", h.authMiddleware(h.handleList))
	mux.HandleFunc("/files/content", h.authMiddleware(h.handleContent))
	mux.HandleFunc("/files/save", h.authMiddleware(h.handleSave))
	mux.HandleFunc("/files/upload", h.authMiddleware(h.handleUpload))
	mux.HandleFunc("/files/upload-chunk", h.authMiddleware(h.handleUploadChunk))
	mux.HandleFunc("/files/upload-merge", h.authMiddleware(h.handleUploadMerge))
	mux.HandleFunc("/files/extract", h.authMiddleware(h.handleExtract))
	mux.HandleFunc("/files/download", h.authMiddleware(h.handleDownload))
	mux.HandleFunc("/files/mkdir", h.authMiddleware(h.handleMkdir))
	mux.HandleFunc("/files/rename", h.authMiddleware(h.handleRename))
	mux.HandleFunc("/files/delete", h.authMiddleware(h.handleDelete))
	mux.HandleFunc("/files/permissions", h.authMiddleware(h.handlePermissions))
	mux.HandleFunc("/files/search", h.authMiddleware(h.handleSearch))
	mux.HandleFunc("/files/copy", h.authMiddleware(h.handleCopy))
	mux.HandleFunc("/files/move", h.authMiddleware(h.handleMove))
	mux.HandleFunc("/files/delete-bulk", h.authMiddleware(h.handleDeleteBulk))
	mux.HandleFunc("/backup/download", h.authMiddleware(h.handleBackupDownload))
}

const chunkTempDir = "/tmp/wp-manage-chunks"

// backupBaseDir is where the agent stores backup archives.
// Must match internal/agent/backup.BackupDir. Kept as a local const to avoid an import cycle.
const backupBaseDir = "/opt/wp-manage/backups"

// handleBackupDownload streams a backup archive from /opt/wp-manage/backups to the caller.
// Backups live outside any site web root (and are 0700/root-only), so they can't go through
// the file-manager download path — this handler validates the request stays within backupBaseDir.
func (h *Handler) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Query().Get("path")
	if reqPath == "" {
		writeJSON(w, 400, models.APIResponse{Error: "path is required"})
		return
	}

	// Confine to backupBaseDir: clean, then verify prefix on the resolved real path.
	cleaned := filepath.Clean(reqPath)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		writeJSON(w, 404, models.APIResponse{Error: "Backup file not found"})
		return
	}
	if resolved != backupBaseDir && !strings.HasPrefix(resolved, backupBaseDir+string(os.PathSeparator)) {
		writeJSON(w, 403, models.APIResponse{Error: "path outside backup directory"})
		return
	}

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		writeJSON(w, 404, models.APIResponse{Error: "Backup file not found"})
		return
	}

	f, err := os.Open(resolved)
	if err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to open backup file"})
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name()))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	io.Copy(w, f)
}

// handleUploadChunk receives a single raw binary chunk and stores it in a temp dir
func (h *Handler) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	uploadID := filepath.Base(r.URL.Query().Get("upload_id")) // sanitize
	chunkIndexStr := r.URL.Query().Get("chunk_index")
	filename := r.URL.Query().Get("filename")

	if uploadID == "" || uploadID == "." || chunkIndexStr == "" || filename == "" {
		writeJSON(w, 400, models.APIResponse{Error: "upload_id, chunk_index, and filename are required"})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 || chunkIndex > 9999 {
		writeJSON(w, 400, models.APIResponse{Error: "invalid chunk_index"})
		return
	}

	chunkDir := filepath.Join(chunkTempDir, uploadID)
	if err := os.MkdirAll(chunkDir, 0700); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "failed to create chunk directory"})
		return
	}

	chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%05d", chunkIndex))
	f, err := os.Create(chunkPath)
	if err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "failed to create chunk file"})
		return
	}
	defer f.Close()

	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024) // 20MB per chunk max
	if _, err := io.Copy(f, r.Body); err != nil {
		os.Remove(chunkPath)
		writeJSON(w, 500, models.APIResponse{Error: "failed to write chunk"})
		return
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Message: fmt.Sprintf("chunk %d received", chunkIndex)})
}

// handleUploadMerge assembles all chunks into the final file
func (h *Handler) handleUploadMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req struct {
		UploadID    string `json:"upload_id"`
		Filename    string `json:"filename"`
		TotalChunks int    `json:"total_chunks"`
		DestPath    string `json:"dest_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "invalid request body"})
		return
	}
	if domain == "" || req.UploadID == "" || req.Filename == "" || req.TotalChunks <= 0 {
		writeJSON(w, 400, models.APIResponse{Error: "domain, upload_id, filename, total_chunks are required"})
		return
	}

	if req.DestPath == "" {
		req.DestPath = "/"
	}
	targetDir, err := safePath(domain, req.DestPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	uploadID := filepath.Base(req.UploadID)
	chunkDir := filepath.Join(chunkTempDir, uploadID)
	defer os.RemoveAll(chunkDir)

	destFile := filepath.Join(targetDir, filepath.Base(req.Filename))
	dst, err := os.Create(destFile)
	if err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "failed to create destination file"})
		return
	}
	defer dst.Close()

	for i := 0; i < req.TotalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%05d", i))
		chunk, err := os.Open(chunkPath)
		if err != nil {
			dst.Close()
			os.Remove(destFile)
			writeJSON(w, 500, models.APIResponse{Error: fmt.Sprintf("chunk %d missing: %s", i, err.Error())})
			return
		}
		_, copyErr := io.Copy(dst, chunk)
		chunk.Close()
		if copyErr != nil {
			dst.Close()
			os.Remove(destFile)
			writeJSON(w, 500, models.APIResponse{Error: fmt.Sprintf("failed to merge chunk %d", i)})
			return
		}
	}

	su := siteUser(domain)
	exec.Command("chown", su+":"+su, destFile).Run()
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "File uploaded successfully", Data: req.Filename})
}

// handleExtract extracts a zip/tar archive into the destination directory
func (h *Handler) handleExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req struct {
		Path     string `json:"path"`
		DestPath string `json:"dest_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}
	if domain == "" || req.Path == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	archivePath, err := safePath(domain, req.Path)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	// Destination defaults to the same directory as the archive
	destPath := req.DestPath
	if destPath == "" {
		destPath = filepath.Dir(req.Path)
		if destPath == "." || destPath == "" {
			destPath = "/"
		}
	}
	destDir, err := safePath(domain, destPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to create destination directory"})
		return
	}

	name := strings.ToLower(filepath.Base(archivePath))
	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(name, ".zip"):
		if _, lookErr := exec.LookPath("unzip"); lookErr != nil {
			exec.Command("apt-get", "install", "-y", "unzip").Run()
		}
		cmd = exec.Command("unzip", "-o", archivePath, "-d", destDir)
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		cmd = exec.Command("tar", "-xzf", archivePath, "-C", destDir)
	case strings.HasSuffix(name, ".tar.bz2"), strings.HasSuffix(name, ".tbz2"):
		cmd = exec.Command("tar", "-xjf", archivePath, "-C", destDir)
	case strings.HasSuffix(name, ".tar"):
		cmd = exec.Command("tar", "-xf", archivePath, "-C", destDir)
	default:
		writeJSON(w, 400, models.APIResponse{Error: "Unsupported archive format (.zip .tar.gz .tgz .tar.bz2 .tar)"})
		return
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Extraction failed: " + strings.TrimSpace(string(out))})
		return
	}

	su := siteUser(domain)
	exec.Command("chown", "-R", su+":"+su, destDir).Run()
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Extracted successfully"})
}

func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract remote IP (strip port)
		remoteIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(remoteIP); err == nil {
			remoteIP = host
		}

		// Pre-check: reject IPs that are already blocked
		if globalAuthRateLimiter.checkBlocked(remoteIP) {
			writeJSON(w, 429, models.APIResponse{Error: "Too many failed attempts. Try again later."})
			return
		}

		key := r.Header.Get("X-Agent-Key")
		if key != h.apiKey {
			// Record failure — block IP after authMaxFailures consecutive bad keys
			globalAuthRateLimiter.recordFailure(remoteIP)
			writeJSON(w, 401, models.APIResponse{Error: "Unauthorized"})
			return
		}

		// Successful auth — reset failure counter
		globalAuthRateLimiter.recordSuccess(remoteIP)
		next(w, r)
	}
}

// safePath resolves the requested path within the site's web root,
// preventing path traversal attacks
func safePath(domain, reqPath string) (string, error) {
	webRoot := filepath.Join("/var/www", domain)
	if reqPath == "" || reqPath == "/" {
		return webRoot, nil
	}

	// Clean and join
	joined := filepath.Join(webRoot, filepath.Clean("/"+reqPath))

	// Resolve symlinks to get the real path
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		// If file doesn't exist yet (for create ops), check the parent
		parent := filepath.Dir(joined)
		resolvedParent, err2 := filepath.EvalSymlinks(parent)
		if err2 != nil {
			return "", fmt.Errorf("invalid path")
		}
		if !strings.HasPrefix(resolvedParent, webRoot) {
			return "", fmt.Errorf("path traversal detected")
		}
		return joined, nil
	}

	if !strings.HasPrefix(resolved, webRoot) {
		return "", fmt.Errorf("path traversal detected")
	}
	return resolved, nil
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	reqPath := r.URL.Query().Get("path")

	if domain == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain is required"})
		return
	}

	fullPath, err := safePath(domain, reqPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		writeJSON(w, 404, models.APIResponse{Error: "Directory not found"})
		return
	}

	files := make([]models.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fi := fileInfoFromOS(info, filepath.Join(reqPath, e.Name()))
		files = append(files, fi)
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Data: files})
}

func (h *Handler) handleContent(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	reqPath := r.URL.Query().Get("path")

	if domain == "" || reqPath == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	fullPath, err := safePath(domain, reqPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		writeJSON(w, 404, models.APIResponse{Error: "File not found"})
		return
	}
	if info.IsDir() {
		writeJSON(w, 400, models.APIResponse{Error: "Cannot read directory as file"})
		return
	}
	if info.Size() > maxFileReadSize {
		writeJSON(w, 400, models.APIResponse{Error: fmt.Sprintf("File too large (max %dMB)", maxFileReadSize/1024/1024)})
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to read file"})
		return
	}

	writeJSON(w, 200, models.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"content":  string(content),
			"path":     reqPath,
			"size":     info.Size(),
			"writable": isWritable(fullPath),
		},
	})
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FileContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.Path == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	fullPath, err := safePath(domain, req.Path)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to create parent directory"})
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to save file"})
		return
	}

	// Fix ownership to site user
	su := siteUser(domain)
	exec.Command("chown", su+":"+su, fullPath).Run()

	writeJSON(w, 200, models.APIResponse{Success: true, Message: "File saved"})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	destPath := r.URL.Query().Get("path")
	if domain == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain is required"})
		return
	}
	if destPath == "" {
		destPath = "/"
	}

	targetDir, err := safePath(domain, destPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB buffer
		writeJSON(w, 400, models.APIResponse{Error: "File too large or invalid form"})
		return
	}

	su := siteUser(domain)
	uploaded := []string{}
	for _, fileHeaders := range r.MultipartForm.File {
		for _, fh := range fileHeaders {
			src, err := fh.Open()
			if err != nil {
				continue
			}

			destFile := filepath.Join(targetDir, filepath.Base(fh.Filename))
			dst, err := os.Create(destFile)
			if err != nil {
				src.Close()
				continue
			}

			_, err = io.Copy(dst, src)
			src.Close()
			dst.Close()

			if err != nil {
				os.Remove(destFile)
				continue
			}

			exec.Command("chown", su+":"+su, destFile).Run()
			uploaded = append(uploaded, fh.Filename)
		}
	}

	writeJSON(w, 200, models.APIResponse{
		Success: true,
		Message: fmt.Sprintf("Uploaded %d file(s)", len(uploaded)),
		Data:    uploaded,
	})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	reqPath := r.URL.Query().Get("path")

	if domain == "" || reqPath == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	fullPath, err := safePath(domain, reqPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		writeJSON(w, 404, models.APIResponse{Error: "File not found"})
		return
	}

	if info.IsDir() {
		// Tar+gzip the directory
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, info.Name()))
		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		tw := tar.NewWriter(gzw)
		defer tw.Close()

		filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(fullPath, path)
			header.Name = filepath.Join(info.Name(), relPath)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if !d.IsDir() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				io.Copy(tw, f)
			}
			return nil
		})
	} else {
		// Serve single file
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name()))
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		f, err := os.Open(fullPath)
		if err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "Failed to open file"})
			return
		}
		defer f.Close()
		io.Copy(w, f)
	}
}

func (h *Handler) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FileMkdirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.Path == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	fullPath, err := safePath(domain, req.Path)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to create directory"})
		return
	}

	su := siteUser(domain)
	exec.Command("chown", su+":"+su, fullPath).Run()
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Directory created"})
}

func (h *Handler) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FileRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.OldPath == "" || req.NewPath == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain, old_path and new_path are required"})
		return
	}

	oldFull, err := safePath(domain, req.OldPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	newFull, err := safePath(domain, req.NewPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	if err := os.Rename(oldFull, newFull); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to rename: " + err.Error()})
		return
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Renamed successfully"})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FileDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.Path == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and path are required"})
		return
	}

	fullPath, err := safePath(domain, req.Path)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	// Prevent deletion of the web root itself
	webRoot := filepath.Join("/var/www", domain)
	if fullPath == webRoot {
		writeJSON(w, 403, models.APIResponse{Error: "Cannot delete site root directory"})
		return
	}

	if err := os.RemoveAll(fullPath); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Failed to delete: " + err.Error()})
		return
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Deleted successfully"})
}

func (h *Handler) handlePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FilePermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.Path == "" || req.Permissions == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain, path and permissions are required"})
		return
	}

	fullPath, err := safePath(domain, req.Path)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	// Parse octal permissions
	mode, err := strconv.ParseUint(req.Permissions, 8, 32)
	if err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid permissions format (use octal like 755)"})
		return
	}

	if req.Recursive {
		args := []string{"-R", req.Permissions, fullPath}
		if out, err := exec.Command("chmod", args...).CombinedOutput(); err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "chmod failed: " + string(out)})
			return
		}
	} else {
		if err := os.Chmod(fullPath, os.FileMode(mode)); err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "Failed to change permissions"})
			return
		}
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Permissions updated"})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}

	domain := r.URL.Query().Get("domain")
	var req models.FileSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, models.APIResponse{Error: "Invalid request body"})
		return
	}

	if domain == "" || req.Pattern == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain and pattern are required"})
		return
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "/"
	}
	fullPath, err := safePath(domain, searchPath)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}

	// Use grep for searching: -r recursive, -n line numbers, --max-count limits per file
	cmd := exec.Command("grep", "-rn", "--include=*.php", "--include=*.html", "--include=*.css",
		"--include=*.js", "--include=*.txt", "--include=*.conf", "--include=*.htaccess",
		"--include=*.env", "--include=*.json", "--include=*.xml", "--include=*.yml",
		"--include=*.yaml", "--include=*.md", "--include=*.sh", "--include=*.sql",
		"--max-count=3", req.Pattern, fullPath)
	out, _ := cmd.Output()

	webRoot := filepath.Join("/var/www", domain)
	results := []models.FileSearchResult{}
	if len(out) > 0 {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			// Format: /var/www/domain/path/file.php:123:content
			// Split into at most 3 parts: file, line number, content
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				continue
			}
			relPath := strings.TrimPrefix(parts[0], webRoot)
			lineNum, _ := strconv.Atoi(parts[1])
			content := parts[2]
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			results = append(results, models.FileSearchResult{
				File:    relPath,
				Line:    lineNum,
				Content: strings.TrimSpace(content),
			})
			if len(results) >= 100 {
				break
			}
		}
	}

	writeJSON(w, 200, models.APIResponse{Success: true, Data: results})
}

// handleCopy copies one or more files/dirs into dest_dir
func (h *Handler) handleCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}
	domain := r.URL.Query().Get("domain")
	var req struct {
		SrcPaths []string `json:"src_paths"`
		DestDir  string   `json:"dest_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || domain == "" || len(req.SrcPaths) == 0 || req.DestDir == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain, src_paths and dest_dir are required"})
		return
	}
	webRoot := filepath.Join("/var/www", domain)
	destDirFull, err := safePath(domain, req.DestDir)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}
	if err := os.MkdirAll(destDirFull, 0755); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Cannot access dest dir: " + err.Error()})
		return
	}
	for _, src := range req.SrcPaths {
		srcFull, err := safePath(domain, src)
		if err != nil {
			writeJSON(w, 403, models.APIResponse{Error: "Invalid src path: " + err.Error()})
			return
		}
		if srcFull == webRoot {
			writeJSON(w, 403, models.APIResponse{Error: "Cannot copy site root"})
			return
		}
		dst := filepath.Join(destDirFull, filepath.Base(srcFull))
		if err := copyRecursive(srcFull, dst); err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "Copy failed: " + err.Error()})
			return
		}
	}
	su := siteUser(domain)
	exec.Command("chown", "-R", su+":"+su, destDirFull).Run() //nolint
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Copied successfully"})
}

// handleMove moves one or more files/dirs into dest_dir
func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}
	domain := r.URL.Query().Get("domain")
	var req struct {
		SrcPaths []string `json:"src_paths"`
		DestDir  string   `json:"dest_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || domain == "" || len(req.SrcPaths) == 0 || req.DestDir == "" {
		writeJSON(w, 400, models.APIResponse{Error: "domain, src_paths and dest_dir are required"})
		return
	}
	webRoot := filepath.Join("/var/www", domain)
	destDirFull, err := safePath(domain, req.DestDir)
	if err != nil {
		writeJSON(w, 403, models.APIResponse{Error: err.Error()})
		return
	}
	if err := os.MkdirAll(destDirFull, 0755); err != nil {
		writeJSON(w, 500, models.APIResponse{Error: "Cannot access dest dir: " + err.Error()})
		return
	}
	for _, src := range req.SrcPaths {
		srcFull, err := safePath(domain, src)
		if err != nil {
			writeJSON(w, 403, models.APIResponse{Error: "Invalid src path: " + err.Error()})
			return
		}
		if srcFull == webRoot {
			writeJSON(w, 403, models.APIResponse{Error: "Cannot move site root"})
			return
		}
		dst := filepath.Join(destDirFull, filepath.Base(srcFull))
		if err := os.Rename(srcFull, dst); err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "Move failed: " + err.Error()})
			return
		}
	}
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Moved successfully"})
}

// handleDeleteBulk deletes multiple files/dirs at once
func (h *Handler) handleDeleteBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, models.APIResponse{Error: "Method not allowed"})
		return
	}
	domain := r.URL.Query().Get("domain")
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || domain == "" || len(req.Paths) == 0 {
		writeJSON(w, 400, models.APIResponse{Error: "domain and paths are required"})
		return
	}
	webRoot := filepath.Join("/var/www", domain)
	for _, p := range req.Paths {
		fullPath, err := safePath(domain, p)
		if err != nil {
			writeJSON(w, 403, models.APIResponse{Error: err.Error()})
			return
		}
		if fullPath == webRoot {
			writeJSON(w, 403, models.APIResponse{Error: "Cannot delete site root"})
			return
		}
		if err := os.RemoveAll(fullPath); err != nil {
			writeJSON(w, 500, models.APIResponse{Error: "Delete failed: " + err.Error()})
			return
		}
	}
	writeJSON(w, 200, models.APIResponse{Success: true, Message: "Deleted successfully"})
}

// copyRecursive copies src to dst (file or directory)
func copyRecursive(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyRecursive(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	info, err := srcFile.Stat()
	if err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Helper functions

func fileInfoFromOS(info os.FileInfo, relPath string) models.FileInfo {
	fi := models.FileInfo{
		Name:        info.Name(),
		Path:        relPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		Permissions: fmt.Sprintf("%o", info.Mode().Perm()),
		ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
			fi.Owner = u.Username
		} else {
			fi.Owner = strconv.Itoa(int(stat.Uid))
		}
		if g, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid))); err == nil {
			fi.Group = g.Name
		} else {
			fi.Group = strconv.Itoa(int(stat.Gid))
		}
	}

	return fi
}

func isWritable(path string) bool {
	return syscall.Access(path, syscall.O_RDWR) == nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
