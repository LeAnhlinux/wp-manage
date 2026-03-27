package services

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type DomainService struct {
	db         *gorm.DB
	dnsService *DNSService // optional — used to look up which provider manages a domain
}

func NewDomainService(db *gorm.DB, dnsService *DNSService) *DomainService {
	return &DomainService{db: db, dnsService: dnsService}
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
		s.queueDomainTask(site.ServerID, "add", req.Domain, domainType, site.Domain, site.PHPVersion, "", "", 0)
	}

	return &domain, nil
}

func (s *DomainService) Update(siteID, domainID uint64, req models.UpdateDomainRequest) (*models.Domain, error) {
	var domain models.Domain
	if err := s.db.Where("id = ? AND site_id = ?", domainID, siteID).First(&domain).Error; err != nil {
		return nil, fmt.Errorf("domain not found")
	}
	if domain.Type == "primary" {
		return nil, fmt.Errorf("cannot set redirect on primary domain")
	}

	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	previousRedirectTo := domain.RedirectTo

	// Update DB
	updates := map[string]interface{}{
		"redirect_to":   req.RedirectTo,
		"redirect_type": req.RedirectType,
	}
	if req.RedirectTo == "" {
		updates["redirect_type"] = 0
	}
	s.db.Model(&domain).Updates(updates)

	// Queue agent task
	if req.RedirectTo != "" {
		// Setting a new redirect: create separate nginx server block
		s.queueDomainTask(site.ServerID, "set_redirect", domain.Domain, domain.Type, site.Domain, site.PHPVersion, "", req.RedirectTo, req.RedirectType)
	} else if previousRedirectTo != "" {
		// Clearing redirect: remove nginx server block, re-add to primary server_name
		s.queueDomainTask(site.ServerID, "remove_redirect", domain.Domain, domain.Type, site.Domain, site.PHPVersion, "", "", 0)
	}

	s.db.First(&domain, domainID)
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
		action := "remove"
		if domain.RedirectTo != "" {
			action = "remove_redirect"
		}
		s.queueDomainTask(site.ServerID, action, domain.Domain, domain.Type, site.Domain, site.PHPVersion, "", "", 0)
	}

	return s.db.Delete(&domain).Error
}

// IssueSSL runs certbot --expand to add the alias domain to the primary certificate.
// This is synchronous — waits up to 120s for agent completion.
func (s *DomainService) IssueSSL(siteID, domainID uint64) error {
	var domain models.Domain
	if err := s.db.Where("id = ? AND site_id = ?", domainID, siteID).First(&domain).Error; err != nil {
		return fmt.Errorf("domain not found")
	}
	if domain.Type == "primary" {
		return fmt.Errorf("use site SSL management for the primary domain")
	}

	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	email := site.WPAdminEmail
	if email == "" {
		email = "admin@" + site.Domain
	}

	task := models.AgentDomainTask{
		Action:     "issue_ssl",
		Domain:     domain.Domain,
		SiteDomain: site.Domain,
		Email:      email,
	}
	data, _ := json.Marshal(task)
	tq := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "manage_domain",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&tq).Error; err != nil {
		return fmt.Errorf("failed to queue SSL task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, tq.ID, 120*time.Second)
	if err != nil {
		return fmt.Errorf("SSL issuance timed out: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("certbot error: %s", result.Error)
	}

	// Mark ssl_enabled in DB
	s.db.Model(&models.Domain{}).Where("id = ?", domainID).Update("ssl_enabled", true)
	return nil
}

// CheckDNS resolves the domain and checks whether it points to the site's server IP.
func (s *DomainService) CheckDNS(siteID uint64, domain string) (*models.DNSCheckResult, error) {
	// Load site → server IP
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	var server models.Server
	if err := s.db.First(&server, site.ServerID).Error; err != nil {
		return nil, fmt.Errorf("server not found")
	}
	return s.checkDNSByIP(server.IPAddress, domain), nil
}

// CheckDNSByServer resolves the domain and checks whether it points to a specific server's IP.
// Used for pre-creation checks when no site exists yet.
func (s *DomainService) CheckDNSByServer(serverID uint64, domain string) (*models.DNSCheckResult, error) {
	var server models.Server
	if err := s.db.First(&server, serverID).Error; err != nil {
		return nil, fmt.Errorf("server not found")
	}
	return s.checkDNSByIP(server.IPAddress, domain), nil
}

// checkDNSByIP is the core DNS lookup logic shared by CheckDNS and CheckDNSByServer.
func (s *DomainService) checkDNSByIP(serverIP, domain string) *models.DNSCheckResult {
	res := &models.DNSCheckResult{
		Domain:   domain,
		ServerIP: serverIP,
	}
	addrs, err := net.LookupHost(domain)
	if err != nil {
		res.Message = fmt.Sprintf("DNS lookup failed: %s", err.Error())
	} else {
		res.ResolvedIPs = addrs
		for _, a := range addrs {
			if a == serverIP {
				res.Pointing = true
				break
			}
		}
		if res.Pointing {
			res.Message = fmt.Sprintf("Domain đang trỏ đúng về %s", serverIP)
		} else {
			res.Message = fmt.Sprintf("Domain đang trỏ về %v, cần trỏ về %s", addrs, serverIP)
		}
	}

	// Look up which DNS provider in our DB manages this domain
	if s.dnsService != nil {
		if pType, pName := s.dnsService.FindProviderForDomain(domain); pType != "" {
			res.ProviderType = pType
			res.ProviderName = pName
			res.CanAutoPoint = true
		}
	}
	return res
}

func (s *DomainService) queueDomainTask(serverID uint64, action, domain, domainType, siteDomain, phpVersion, email, redirectTo string, redirectType int) {
	task := models.AgentDomainTask{
		Action:       action,
		Domain:       domain,
		DomainType:   domainType,
		SiteDomain:   siteDomain,
		PHPVersion:   phpVersion,
		Email:        email,
		RedirectTo:   redirectTo,
		RedirectType: redirectType,
	}
	data, _ := json.Marshal(task)
	s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_domain",
		Data:     string(data),
		Status:   "pending",
	})
}
