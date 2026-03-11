package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type DomainService struct {
	db *gorm.DB
}

func NewDomainService(db *gorm.DB) *DomainService {
	return &DomainService{db: db}
}

func (s *DomainService) ListBySite(siteID uint64) ([]models.Domain, error) {
	var domains []models.Domain
	err := s.db.Where("site_id = ?", siteID).Order("type ASC, domain ASC").Find(&domains).Error
	return domains, err
}

func (s *DomainService) Create(siteID uint64, req models.CreateDomainRequest) (*models.Domain, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	domainType := req.Type
	if domainType == "" {
		domainType = "alias"
	}

	// Check uniqueness
	var count int64
	s.db.Model(&models.Domain{}).Where("domain = ?", req.Domain).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("domain %s already exists", req.Domain)
	}

	domain := models.Domain{
		SiteID: siteID,
		Domain: req.Domain,
		Type:   domainType,
	}

	if err := s.db.Create(&domain).Error; err != nil {
		return nil, err
	}

	// Get site info for agent task
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err == nil {
		s.queueDomainTask(site.ServerID, "add", req.Domain, domainType, site.Domain, site.PHPVersion)
	}

	return &domain, nil
}

func (s *DomainService) Delete(siteID, domainID uint64) error {
	var domain models.Domain
	if err := s.db.Where("id = ? AND site_id = ?", domainID, siteID).First(&domain).Error; err != nil {
		return fmt.Errorf("domain not found")
	}

	if domain.Type == "primary" {
		return fmt.Errorf("cannot delete primary domain")
	}

	// Get site info for agent task
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err == nil {
		s.queueDomainTask(site.ServerID, "remove", domain.Domain, domain.Type, site.Domain, site.PHPVersion)
	}

	return s.db.Delete(&domain).Error
}

func (s *DomainService) queueDomainTask(serverID uint64, action, domain, domainType, siteDomain, phpVersion string) {
	task := models.AgentDomainTask{
		Action:     action,
		Domain:     domain,
		DomainType: domainType,
		SiteDomain: siteDomain,
		PHPVersion: phpVersion,
	}
	data, _ := json.Marshal(task)
	s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_domain",
		Data:     string(data),
		Status:   "pending",
	})
}
