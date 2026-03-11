package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type SSLService struct {
	db *gorm.DB
}

func NewSSLService(db *gorm.DB) *SSLService {
	return &SSLService{db: db}
}

// EnableSSL issues a Let's Encrypt certificate for the site domain (synchronous).
func (s *SSLService) EnableSSL(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found: %w", err)
	}

	email := site.WPAdminEmail
	if email == "" {
		email = "admin@" + site.Domain
	}

	payload := map[string]interface{}{
		"domain": site.Domain,
		"email":  email,
	}
	dataJSON, _ := json.Marshal(payload)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "issue_ssl",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	completed, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("SSL issuance timed out: %w", err)
	}
	if completed.Error != "" {
		return fmt.Errorf("%s", completed.Error)
	}
	return nil
}

// DisableSSL removes the Let's Encrypt certificate and restores HTTP-only nginx config (synchronous).
func (s *SSLService) DisableSSL(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found: %w", err)
	}

	payload := map[string]interface{}{
		"domain":      site.Domain,
		"php_version": site.PHPVersion,
	}
	dataJSON, _ := json.Marshal(payload)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "disable_ssl",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	completed, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("SSL disable timed out: %w", err)
	}
	if completed.Error != "" {
		return fmt.Errorf("%s", completed.Error)
	}

	// Clear expiry and force_https since SSL is gone
	return s.db.Model(&models.Site{}).Where("id = ?", siteID).Updates(map[string]interface{}{
		"ssl_expires_at": nil,
		"force_https":    false,
	}).Error
}

// RenewSSL force-renews the Let's Encrypt certificate (synchronous).
func (s *SSLService) RenewSSL(siteID uint64) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found: %w", err)
	}

	payload := map[string]interface{}{"domain": site.Domain}
	dataJSON, _ := json.Marshal(payload)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "renew_ssl",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	completed, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("SSL renewal timed out: %w", err)
	}
	if completed.Error != "" {
		return fmt.Errorf("%s", completed.Error)
	}
	return nil
}

// UploadCustomSSL installs a custom SSL certificate (synchronous).
func (s *SSLService) UploadCustomSSL(siteID uint64, certPEM, keyPEM, caPEM string) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found: %w", err)
	}

	payload := map[string]interface{}{
		"domain":   site.Domain,
		"cert_pem": certPEM,
		"key_pem":  keyPEM,
		"ca_pem":   caPEM,
	}
	dataJSON, _ := json.Marshal(payload)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "upload_custom_ssl",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	completed, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("SSL install timed out: %w", err)
	}
	if completed.Error != "" {
		return fmt.Errorf("%s", completed.Error)
	}
	return nil
}

// SSLStatusResult is returned by GetSSLStatus, including force_https state.
type SSLStatusResult struct {
	SSLEnabled   bool       `json:"ssl_enabled"`
	SSLExpiresAt *string    `json:"ssl_expires_at"`
	Domain       string     `json:"domain"`
	ForceHTTPS   bool       `json:"force_https"`
}

func (s *SSLService) GetSSLStatus(siteID uint64) (*SSLStatusResult, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, err
	}
	var expiresAt *string
	if site.SSLExpiresAt != nil {
		t := site.SSLExpiresAt.Format("2006-01-02T15:04:05Z")
		expiresAt = &t
	}
	return &SSLStatusResult{
		SSLEnabled:   site.SSLEnabled,
		SSLExpiresAt: expiresAt,
		Domain:       site.Domain,
		ForceHTTPS:   site.ForceHTTPS,
	}, nil
}

// ForceHTTPS queues a synchronous task and updates the force_https column on success.
func (s *SSLService) ForceHTTPS(siteID uint64, enable bool) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found: %w", err)
	}
	action := "enable"
	if !enable {
		action = "disable"
	}
	payload := map[string]interface{}{"domain": site.Domain, "action": action}
	dataJSON, _ := json.Marshal(payload)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "manage_force_https",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return err
	}

	// Wait for the agent to complete the task (synchronous)
	completed, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return fmt.Errorf("force https task timed out: %w", err)
	}
	if completed.Error != "" {
		return fmt.Errorf("force https failed: %s", completed.Error)
	}

	// Persist the new state in DB
	return s.db.Model(&models.Site{}).Where("id = ?", siteID).Update("force_https", enable).Error
}
