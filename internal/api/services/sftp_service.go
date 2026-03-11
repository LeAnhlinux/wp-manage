package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type SFTPService struct {
	db *gorm.DB
}

func NewSFTPService(db *gorm.DB) *SFTPService {
	return &SFTPService{db: db}
}

func (s *SFTPService) List(siteID uint64) ([]models.SFTPAccount, error) {
	var accounts []models.SFTPAccount
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Find(&accounts).Error
	return accounts, err
}

// SFTPCreateResult wraps the created account with one-time credentials for display.
type SFTPCreateResult struct {
	*models.SFTPAccount
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"` // returned once, not stored in DB
}

func (s *SFTPService) Create(siteID uint64, req models.CreateSFTPRequest) (*SFTPCreateResult, error) {
	// Get site for domain/server info
	var site models.Site
	if err := s.db.Preload("Server").First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	homeDir := fmt.Sprintf("/var/www/%s", site.Domain)

	account := models.SFTPAccount{
		SiteID:   siteID,
		Username: req.Username,
		HomeDir:  homeDir,
	}
	if err := s.db.Create(&account).Error; err != nil {
		return nil, err
	}

	// Queue agent task
	taskData, _ := json.Marshal(models.AgentSFTPTask{
		Action:   "create",
		Username: req.Username,
		Password: req.Password,
		HomeDir:  homeDir,
		Domain:   site.Domain,
	})
	s.db.Create(&models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "create_sftp_user",
		Data:     string(taskData),
		Status:   "pending",
	})

	host := ""
	if site.Server != nil {
		host = site.Server.IPAddress
	}

	return &SFTPCreateResult{
		SFTPAccount: &account,
		Host:        host,
		Port:        22,
		Password:    req.Password,
	}, nil
}

func (s *SFTPService) Delete(siteID, accountID uint64) error {
	var account models.SFTPAccount
	if err := s.db.Where("id = ? AND site_id = ?", accountID, siteID).First(&account).Error; err != nil {
		return fmt.Errorf("SFTP account not found")
	}

	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	// Queue agent task
	taskData, _ := json.Marshal(models.AgentSFTPTask{
		Action:   "delete",
		Username: account.Username,
		Domain:   site.Domain,
	})
	s.db.Create(&models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "delete_sftp_user",
		Data:     string(taskData),
		Status:   "pending",
	})

	return s.db.Delete(&account).Error
}
