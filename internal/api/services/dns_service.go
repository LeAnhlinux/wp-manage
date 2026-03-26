package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type DNSService struct {
	db *gorm.DB
}

func NewDNSService(db *gorm.DB) *DNSService {
	return &DNSService{db: db}
}


func (s *DNSService) ListAll() ([]models.DNSProvider, error) {
	var providers []models.DNSProvider
	err := s.db.Preload("Server").Order("server_id, id").Find(&providers).Error
	return providers, err
}

func (s *DNSService) Delete(providerID uint64) error {
	return s.db.Delete(&models.DNSProvider{}, providerID).Error
}

// CreateGlobal creates a DNS provider not bound to any server.
func (s *DNSService) CreateGlobal(req models.CreateGlobalDNSProviderRequest) (*models.DNSProvider, error) {
	provider := &models.DNSProvider{
		ProviderType: req.ProviderType,
		APIKey:       req.APIKey,
		APIEmail:     req.APIEmail,
		ZoneID:       req.ZoneID,
		Domain:       req.Domain,
	}
	if err := s.db.Create(provider).Error; err != nil {
		return nil, err
	}
	return provider, nil
}

// LookupZone finds the Zone ID for a domain from the provider API.
func (s *DNSService) LookupZone(req models.LookupZoneRequest) (*models.LookupZoneResult, error) {
	switch req.ProviderType {
	case "cloudflare":
		return s.cfLookupZone(req.APIKey, req.Domain)
	case "route53":
		return s.r53LookupZone(req.APIKey, req.APIEmail, req.Domain)
	case "godaddy":
		return s.gdLookupZone(req.APIKey, req.APIEmail, req.Domain)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", req.ProviderType)
	}
}

func (s *DNSService) cfLookupZone(apiKey, domain string) (*models.LookupZoneResult, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", domain)
	body, status, err := cfHTTP("GET", url, apiKey, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("cloudflare returned %d: %s", status, string(body))
	}
	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	json.Unmarshal(body, &resp)
	if !resp.Success {
		msg := "lookup failed"
		if len(resp.Errors) > 0 {
			msg = resp.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare: %s", msg)
	}
	if len(resp.Result) == 0 {
		return nil, fmt.Errorf("no zone found for domain: %s", domain)
	}
	return &models.LookupZoneResult{
		ZoneID:   resp.Result[0].ID,
		ZoneName: resp.Result[0].Name,
	}, nil
}

// ListRecordsByProvider lists DNS records using only provider ID (no server scoping).
func (s *DNSService) ListRecordsByProvider(providerID uint64) ([]CFDNSRecord, error) {
	return s.ListDNSRecords(providerID)
}

// CreateRecordByProvider creates a DNS record using only provider ID.
func (s *DNSService) CreateRecordByProvider(providerID uint64, rec CFDNSRecord) (*CFDNSRecord, error) {
	return s.CreateDNSRecord(providerID, rec)
}

// DeleteRecordByProvider deletes a DNS record using only provider ID.
func (s *DNSService) DeleteRecordByProvider(providerID uint64, recordID string) error {
	return s.DeleteDNSRecord(providerID, recordID)
}

// AutoCreateRecord finds the matching DNS provider for a domain and creates/updates an A record.
func (s *DNSService) AutoCreateRecord(domain, serverIP string) (*models.AutoCreateDNSRecordResult, error) {
	var providers []models.DNSProvider
	if err := s.db.Find(&providers).Error; err != nil {
		return nil, err
	}

	var matched *models.DNSProvider
	for i := range providers {
		p := &providers[i]
		if domain == p.Domain || strings.HasSuffix(domain, "."+p.Domain) {
			matched = p
			break
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("no DNS provider found matching domain: %s", domain)
	}

	switch matched.ProviderType {
	case "cloudflare":
		return s.cfAutoCreateRecord(*matched, domain, serverIP)
	case "route53":
		return s.r53AutoCreateRecord(*matched, domain, serverIP)
	case "godaddy":
		return s.gdAutoCreateRecord(*matched, domain, serverIP)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", matched.ProviderType)
	}
}

// AutoUpdateRecord finds the DNS provider for the domain and updates (or creates) the A record.
// Used after migration to point domain to the new server IP.
func (s *DNSService) AutoUpdateRecord(domain, serverIP string) error {
	_, err := s.AutoCreateRecord(domain, serverIP)
	return err
}

func (s *DNSService) cfAutoCreateRecord(provider models.DNSProvider, domain, serverIP string) (*models.AutoCreateDNSRecordResult, error) {
	// Check if an A record already exists
	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=A&name=%s", provider.ZoneID, domain)
	body, _, err := cfHTTP("GET", listURL, provider.APIKey, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	var listResp struct {
		Result []struct {
			ID      string `json:"id"`
			Proxied bool   `json:"proxied"`
		} `json:"result"`
	}
	json.Unmarshal(body, &listResp)

	// Preserve existing proxied status when updating; default false when creating new
	proxied := false
	action := "created"
	var reqURL, method string
	if len(listResp.Result) > 0 {
		proxied = listResp.Result[0].Proxied // keep existing proxy setting
		action = "updated"
		method = "PUT"
		reqURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", provider.ZoneID, listResp.Result[0].ID)
	} else {
		method = "POST"
		reqURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", provider.ZoneID)
	}

	rec := CFDNSRecord{Type: "A", Name: domain, Content: serverIP, TTL: 1, Proxied: proxied}
	payload, _ := json.Marshal(rec)

	respBody, status, err := cfHTTP(method, reqURL, provider.APIKey, payload)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("cloudflare returned %d: %s", status, string(respBody))
	}

	var resp struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	json.Unmarshal(respBody, &resp)

	return &models.AutoCreateDNSRecordResult{
		RecordID:   resp.Result.ID,
		ProviderID: provider.ID,
		Domain:     domain,
		IP:         serverIP,
		Action:     action,
		ZoneName:   provider.Domain,
		Proxied:    proxied,
	}, nil
}

// UpdateRecordByProvider updates a DNS record using only provider ID.
func (s *DNSService) UpdateRecordByProvider(providerID uint64, recordID string, rec CFDNSRecord) (*CFDNSRecord, error) {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return nil, fmt.Errorf("provider not found")
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.cfUpdateRecord(provider, recordID, rec)
	case "route53":
		return s.r53UpdateRecord(provider, recordID, rec)
	case "godaddy":
		return s.gdUpdateRecord(provider, recordID, rec)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

func (s *DNSService) cfUpdateRecord(provider models.DNSProvider, recordID string, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL == 0 {
		rec.TTL = 1
	}
	payload, _ := json.Marshal(rec)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", provider.ZoneID, recordID)
	body, status, err := cfHTTP("PUT", url, provider.APIKey, payload)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("cloudflare returned %d: %s", status, string(body))
	}
	var resp struct {
		Success bool        `json:"success"`
		Result  CFDNSRecord `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// TestProvider verifies that the API key and zone ID can authenticate with the provider.
func (s *DNSService) TestProvider(req models.TestDNSProviderRequest) (*models.TestDNSProviderResult, error) {
	switch req.ProviderType {
	case "cloudflare":
		return s.testCloudflare(req)
	case "route53":
		return s.testRoute53(req)
	case "godaddy":
		return s.testGoDaddy(req)
	default:
		return &models.TestDNSProviderResult{Success: false, Message: "Unsupported provider: " + req.ProviderType}, nil
	}
}

func (s *DNSService) testCloudflare(req models.TestDNSProviderRequest) (*models.TestDNSProviderResult, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s", req.ZoneID)
	body, status, err := cfHTTP("GET", url, req.APIKey, nil)
	if err != nil {
		return &models.TestDNSProviderResult{Success: false, Message: "Connection failed: " + err.Error()}, nil
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	json.Unmarshal(body, &resp)

	if status >= 400 || !resp.Success {
		msg := fmt.Sprintf("Authentication failed (HTTP %d)", status)
		if len(resp.Errors) > 0 {
			msg = resp.Errors[0].Message
		}
		return &models.TestDNSProviderResult{Success: false, Message: msg}, nil
	}

	return &models.TestDNSProviderResult{
		Success:  true,
		Message:  fmt.Sprintf("Connected! Zone: %s", resp.Result.Name),
		ZoneName: resp.Result.Name,
	}, nil
}

// CFDNSRecord is a Cloudflare DNS record DTO.
type CFDNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

// ListDNSRecords returns all DNS records for a provider's zone.
func (s *DNSService) ListDNSRecords(providerID uint64) ([]CFDNSRecord, error) {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return nil, fmt.Errorf("provider not found")
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.cfListRecords(provider)
	case "route53":
		return s.r53ListRecords(provider)
	case "godaddy":
		return s.gdListRecords(provider)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

// CreateDNSRecord creates a new DNS record in a zone.
func (s *DNSService) CreateDNSRecord(providerID uint64, rec CFDNSRecord) (*CFDNSRecord, error) {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return nil, fmt.Errorf("provider not found")
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.cfCreateRecord(provider, rec)
	case "route53":
		return s.r53CreateRecord(provider, rec)
	case "godaddy":
		return s.gdCreateRecord(provider, rec)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

// DeleteDNSRecord deletes a DNS record by its record ID.
func (s *DNSService) DeleteDNSRecord(providerID uint64, recordID string) error {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return fmt.Errorf("provider not found")
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.cfDeleteRecord(provider, recordID)
	case "route53":
		return s.r53DeleteRecord(provider, recordID)
	case "godaddy":
		return s.gdDeleteRecord(provider, recordID)
	default:
		return fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

// ─── Cloudflare helpers ───────────────────────────────────────────────────────

func cfHTTP(method, url, apiKey string, body []byte) ([]byte, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func (s *DNSService) cfListRecords(provider models.DNSProvider) ([]CFDNSRecord, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=100", provider.ZoneID)
	body, status, err := cfHTTP("GET", url, provider.APIKey, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("cloudflare returned %d: %s", status, string(body))
	}
	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
			TTL     int    `json:"ttl"`
			Proxied bool   `json:"proxied"`
			Comment string `json:"comment"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	records := make([]CFDNSRecord, len(resp.Result))
	for i, r := range resp.Result {
		records[i] = CFDNSRecord{
			ID: r.ID, Type: r.Type, Name: r.Name,
			Content: r.Content, TTL: r.TTL, Proxied: r.Proxied, Comment: r.Comment,
		}
	}
	return records, nil
}

func (s *DNSService) cfCreateRecord(provider models.DNSProvider, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL == 0 {
		rec.TTL = 1 // auto
	}
	payload, _ := json.Marshal(rec)
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", provider.ZoneID)
	body, status, err := cfHTTP("POST", url, provider.APIKey, payload)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return nil, fmt.Errorf("cloudflare returned %d: %s", status, string(body))
	}
	var resp struct {
		Success bool        `json:"success"`
		Result  CFDNSRecord `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func (s *DNSService) cfDeleteRecord(provider models.DNSProvider, recordID string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", provider.ZoneID, recordID)
	body, status, err := cfHTTP("DELETE", url, provider.APIKey, nil)
	if err != nil {
		return fmt.Errorf("cloudflare API: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("cloudflare returned %d: %s", status, string(body))
	}
	return nil
}

