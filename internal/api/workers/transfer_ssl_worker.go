package workers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// TransferSSLWorker polls TransferSiteResult rows with status='ssl_pending_reissue'
// and, once DNS points to the target server, queues a renew_ssl task for each site.
// This implements M11 of the Transfer Tool plan.
type TransferSSLWorker struct {
	db *gorm.DB
}

func NewTransferSSLWorker(db *gorm.DB) *TransferSSLWorker {
	return &TransferSSLWorker{db: db}
}

// Start runs the worker: check at startup, then every 6 hours.
func (w *TransferSSLWorker) Start() {
	go func() {
		// Stagger from other workers
		time.Sleep(45 * time.Second)
		w.checkAll()

		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		slog.Info("[TransferSSL] Worker started (every 6h)")
		for range ticker.C {
			w.checkAll()
		}
	}()
}

// checkAll scans for pending SSL reissue results and queues renewal tasks where DNS is ready.
func (w *TransferSSLWorker) checkAll() {
	// Only process results that have a linked site (site was successfully created)
	var results []models.TransferSiteResult
	if err := w.db.
		Where("status = ? AND site_id IS NOT NULL", "ssl_pending_reissue").
		Find(&results).Error; err != nil {
		slog.Error("[TransferSSL] Failed to query pending results", "error", err)
		return
	}

	if len(results) == 0 {
		return
	}

	slog.Info("[TransferSSL] Checking DNS for pending SSL reissue results", "count", len(results))

	for _, r := range results {
		w.processResult(r)
	}
}

func (w *TransferSSLWorker) processResult(r models.TransferSiteResult) {
	// Load the site to get the server's IP
	var site models.Site
	if err := w.db.Preload("Server").First(&site, *r.SiteID).Error; err != nil {
		slog.Warn("[TransferSSL] Site not found for result", "result_id", r.ID, "site_id", r.SiteID)
		return
	}

	serverIP := ""
	if site.Server.IPAddress != "" {
		serverIP = strings.TrimSpace(site.Server.IPAddress)
	}

	// Check DNS: resolve target domain, verify it points to the target server
	if err := w.checkDNS(r.TargetDomain, serverIP); err != nil {
		slog.Info("[TransferSSL] DNS not ready, will retry next cycle",
			"result_id", r.ID, "domain", r.TargetDomain, "reason", err)
		// Append note to log excerpt
		note := fmt.Sprintf("[%s] SSL pending: DNS not yet ready — %s",
			time.Now().Format("2006-01-02 15:04:05"), err.Error())
		w.appendLog(r.ID, note)
		return
	}

	// Dedup: skip if a renew_ssl task is already pending/running for this site
	var count int64
	w.db.Model(&models.TaskQueue{}).
		Where("site_id = ? AND task_type = ? AND status IN ('pending','running')", *r.SiteID, "renew_ssl").
		Count(&count)
	if count > 0 {
		slog.Info("[TransferSSL] renew_ssl task already queued, skipping",
			"result_id", r.ID, "domain", r.TargetDomain)
		return
	}

	// DNS is ready — queue renew_ssl task
	payload, _ := json.Marshal(map[string]interface{}{"domain": r.TargetDomain})
	siteID := *r.SiteID
	task := models.TaskQueue{
		ServerID: r.TargetServerID,
		SiteID:   siteID,
		TaskType: "renew_ssl",
		Data:     string(payload),
		Status:   "pending",
	}
	if err := w.db.Create(&task).Error; err != nil {
		slog.Error("[TransferSSL] Failed to queue renew_ssl task",
			"result_id", r.ID, "domain", r.TargetDomain, "error", err)
		return
	}

	slog.Info("[TransferSSL] Queued renew_ssl task",
		"result_id", r.ID, "domain", r.TargetDomain, "task_id", task.ID)

	// Update result status → done
	note := fmt.Sprintf("[%s] DNS verified → queued SSL reissue (task #%d)",
		time.Now().Format("2006-01-02 15:04:05"), task.ID)
	w.db.Model(&models.TransferSiteResult{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
		"status": "done",
	})
	w.appendLog(r.ID, note)
}

// checkDNS resolves domain and checks that at least one A record matches serverIP.
// If serverIP is empty (server record has no IP), we skip the DNS check and proceed.
func (w *TransferSSLWorker) checkDNS(domain, serverIP string) error {
	if serverIP == "" {
		return nil // can't check — proceed optimistically
	}

	addrs, err := net.LookupHost(domain)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %v", err)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			addr = v4.String()
		}
		if addr == serverIP {
			return nil
		}
	}

	return fmt.Errorf("domain resolves to %v but server IP is %s — DNS not yet pointed", addrs, serverIP)
}

// appendLog appends a line to the result's log_excerpt, keeping last 2000 chars.
func (w *TransferSSLWorker) appendLog(resultID uint64, line string) {
	var r models.TransferSiteResult
	if err := w.db.First(&r, resultID).Error; err != nil {
		return
	}
	updated := r.LogExcerpt
	if updated != "" {
		updated += "\n"
	}
	updated += line
	if len(updated) > 2000 {
		updated = updated[len(updated)-2000:]
	}
	w.db.Model(&models.TransferSiteResult{}).Where("id = ?", resultID).Update("log_excerpt", updated)
}
