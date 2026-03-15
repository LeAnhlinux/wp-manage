package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// PushService sends Firebase Cloud Messaging (FCM) push notifications to
// registered mobile devices and stores them in the MobileNotification table
// for the in-app notification center.
//
// Configuration:
//   - Set FCM_SERVER_KEY env var (Legacy HTTP API) or use FCM v1 with service account
//   - Set FCM_PROJECT_ID for v1 API
type PushService struct {
	db         *gorm.DB
	fcmKey     string // FCM Legacy Server Key (Bearer token for legacy API)
	fcmProject string // GCP project ID for FCM v1
	httpClient *http.Client
}

func NewPushService(db *gorm.DB, fcmKey, fcmProject string) *PushService {
	return &PushService{
		db:         db,
		fcmKey:     fcmKey,
		fcmProject: fcmProject,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ─── Notification types ───────────────────────────────────────────────────────

const (
	PushTypeServerOffline    = "server_offline"
	PushTypeAlertFired       = "alert_fired"
	PushTypeBackupFailed     = "backup_failed"
	PushTypeBackupCompleted  = "backup_completed"
	PushTypeThreatsFound     = "threats_found"
	PushTypeSiteDown         = "site_down"
	PushTypeSiteUp           = "site_up"
	PushTypeMigrationFailed  = "migration_failed"
	PushTypeSSLExpiring      = "ssl_expiring"
	PushTypeAgentUpdated     = "agent_updated"
)

// PushPayload holds the structured data for a push notification.
type PushPayload struct {
	Type    string                 `json:"type"`
	Title   string                 `json:"title"`
	Body    string                 `json:"body"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ─── Public send methods ──────────────────────────────────────────────────────

// NotifyServerOffline sends a push when a server stops heartbeating.
func (s *PushService) NotifyServerOffline(serverID uint64, serverName string) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeServerOffline,
		Title: "⚠️ Server Offline",
		Body:  fmt.Sprintf("Server '%s' has stopped responding", serverName),
		Data:  map[string]interface{}{"server_id": serverID, "server_name": serverName},
	})
}

// NotifyAlertFired sends a push when a metric threshold alert fires.
func (s *PushService) NotifyAlertFired(serverID uint64, serverName, metric string, value float64, message string) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeAlertFired,
		Title: "🔔 Alert: " + serverName,
		Body:  message,
		Data:  map[string]interface{}{"server_id": serverID, "metric": metric, "value": value},
	})
}

// NotifyBackupFailed sends a push when a backup job fails.
func (s *PushService) NotifyBackupFailed(siteID uint64, domain string, errMsg string) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeBackupFailed,
		Title: "❌ Backup Failed",
		Body:  fmt.Sprintf("Backup failed for %s: %s", domain, truncate(errMsg, 100)),
		Data:  map[string]interface{}{"site_id": siteID, "domain": domain},
	})
}

// NotifyThreatsFound sends a push when malware scan finds threats.
func (s *PushService) NotifyThreatsFound(siteID uint64, domain string, count int) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeThreatsFound,
		Title: "🦠 Threats Detected",
		Body:  fmt.Sprintf("%d threat(s) found on %s", count, domain),
		Data:  map[string]interface{}{"site_id": siteID, "domain": domain, "threats_found": count},
	})
}

// NotifySiteDown sends a push when an uptime check fails.
func (s *PushService) NotifySiteDown(siteID uint64, url string, statusCode int) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeSiteDown,
		Title: "🔴 Site Down",
		Body:  fmt.Sprintf("%s returned HTTP %d", url, statusCode),
		Data:  map[string]interface{}{"site_id": siteID, "url": url, "status_code": statusCode},
	})
}

// NotifySiteUp sends a push when an uptime check recovers.
func (s *PushService) NotifySiteUp(siteID uint64, url string) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeSiteUp,
		Title: "🟢 Site Recovered",
		Body:  fmt.Sprintf("%s is back online", url),
		Data:  map[string]interface{}{"site_id": siteID, "url": url},
	})
}

// NotifyMigrationFailed sends a push when a migration fails.
func (s *PushService) NotifyMigrationFailed(migrationID uint64, domain string) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeMigrationFailed,
		Title: "❌ Migration Failed",
		Body:  fmt.Sprintf("Migration failed for %s", domain),
		Data:  map[string]interface{}{"migration_id": migrationID, "domain": domain},
	})
}

// NotifySSLExpiring sends a push when SSL certificate is about to expire.
func (s *PushService) NotifySSLExpiring(siteID uint64, domain string, daysLeft int) {
	s.sendToAllAdmins(PushPayload{
		Type:  PushTypeSSLExpiring,
		Title: "🔒 SSL Expiring Soon",
		Body:  fmt.Sprintf("%s SSL expires in %d day(s)", domain, daysLeft),
		Data:  map[string]interface{}{"site_id": siteID, "domain": domain, "days_left": daysLeft},
	})
}

// ─── Core delivery ────────────────────────────────────────────────────────────

// SendToUser sends a push to all registered devices of a specific user.
func (s *PushService) SendToUser(userID uint64, payload PushPayload) {
	var tokens []models.MobileRefreshToken
	s.db.Where("user_id = ? AND (fcm_token != '' OR apns_token != '')", userID).Find(&tokens)

	// Store in notification center
	notif := models.MobileNotification{
		UserID:  userID,
		Type:    payload.Type,
		Title:   payload.Title,
		Body:    payload.Body,
		SentAt:  time.Now(),
	}
	if d, err := json.Marshal(payload.Data); err == nil {
		notif.Payload = string(d)
	}
	s.db.Create(&notif)

	for _, token := range tokens {
		if token.FCMToken != "" {
			go s.sendFCM(token.FCMToken, payload, notif.ID)
		}
	}
}

// sendToAllAdmins sends to all admin users (for system-level events).
func (s *PushService) sendToAllAdmins(payload PushPayload) {
	var admins []models.User
	s.db.Where("role = 'admin'").Find(&admins)
	for _, admin := range admins {
		s.SendToUser(admin.ID, payload)
	}
}

// sendFCM delivers a single push via FCM Legacy HTTP API.
// For production use, migrate to FCM v1 API with OAuth2 service account.
func (s *PushService) sendFCM(fcmToken string, payload PushPayload, notifID uint64) {
	if s.fcmKey == "" {
		s.logDelivery(notifID, fcmToken, "", "skipped", "FCM_SERVER_KEY not configured")
		return
	}

	body := map[string]interface{}{
		"to": fcmToken,
		"notification": map[string]string{
			"title": payload.Title,
			"body":  payload.Body,
		},
		"data": payload.Data,
		"android": map[string]interface{}{
			"priority": "high",
		},
		"apns": map[string]interface{}{
			"headers": map[string]string{"apns-priority": "10"},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		s.logDelivery(notifID, fcmToken, "", "failed", err.Error())
		return
	}

	req, err := http.NewRequest("POST", "https://fcm.googleapis.com/fcm/send", bytes.NewReader(bodyBytes))
	if err != nil {
		s.logDelivery(notifID, fcmToken, "", "failed", err.Error())
		return
	}
	req.Header.Set("Authorization", "key="+s.fcmKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logDelivery(notifID, fcmToken, "", "failed", err.Error())
		return
	}
	defer resp.Body.Close()

	var result struct {
		MessageID string `json:"message_id"`
		Error     string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Error != "" {
		s.logDelivery(notifID, fcmToken, result.MessageID, "failed", result.Error)
	} else {
		s.logDelivery(notifID, fcmToken, result.MessageID, "sent", "")
	}
}

func (s *PushService) logDelivery(notifID uint64, deviceToken, messageID, status, errMsg string) {
	log := models.MobilePushLog{
		Type:         fmt.Sprintf("notif:%d", notifID),
		DeviceID:     deviceToken[:min(len(deviceToken), 20)] + "...",
		Status:       status,
		FCMMessageID: messageID,
		Error:        errMsg,
		CreatedAt:    time.Now(),
	}
	s.db.Create(&log)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
