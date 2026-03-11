package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type ImportService struct {
	db *gorm.DB
}

func NewImportService(db *gorm.DB) *ImportService {
	return &ImportService{db: db}
}

func (s *ImportService) StartImport(siteID uint64, req models.ImportExternalRequest) (*models.ExternalImport, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found: %w", err)
	}

	imp := &models.ExternalImport{
		SiteID:   siteID,
		Domain:   req.Domain,
		SourceIP: req.SourceIP,
		SSHUser:  req.SSHUser,
		SSHPort:  req.SSHPort,
		Status:   "pending",
	}
	if imp.SSHUser == "" {
		imp.SSHUser = "root"
	}
	if imp.SSHPort == 0 {
		imp.SSHPort = 22
	}
	if err := s.db.Create(imp).Error; err != nil {
		return nil, err
	}

	phpVersion := req.PHPVersion
	if phpVersion == "" {
		phpVersion = site.PHPVersion
	}

	taskData := models.AgentImportTask{
		ImportID:   imp.ID,
		SiteID:     siteID,
		Domain:     req.Domain,
		SourceIP:   req.SourceIP,
		SSHUser:    imp.SSHUser,
		SSHPort:    imp.SSHPort,
		SSHKey:     req.SSHKey,
		DBName:     req.DBName,
		DBUser:     req.DBUser,
		DBPass:     req.DBPass,
		PHPVersion: phpVersion,
	}
	data, _ := json.Marshal(taskData)
	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "import_external",
		Data:     string(data),
		Status:   "pending",
	}
	s.db.Create(&task)

	return imp, nil
}

func (s *ImportService) GetStatus(importID uint64) (*models.ExternalImport, error) {
	var imp models.ExternalImport
	err := s.db.First(&imp, importID).Error
	return &imp, err
}

func (s *ImportService) ListBySite(siteID uint64) ([]models.ExternalImport, error) {
	var imports []models.ExternalImport
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Find(&imports).Error
	return imports, err
}

func (s *ImportService) UpdateProgress(importID uint64, progress int, status, errMsg string) error {
	updates := map[string]interface{}{
		"progress": progress,
		"status":   status,
	}
	if errMsg != "" {
		updates["error"] = errMsg
	}
	return s.db.Model(&models.ExternalImport{}).Where("id = ?", importID).Updates(updates).Error
}
