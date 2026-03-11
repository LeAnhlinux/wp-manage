package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (s *DNSService) Create(serverID uint64, req models.CreateDNSProviderRequest) (*models.DNSProvider, error) {
	provider := &models.DNSProvider{
		ServerID:     serverID,
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

func (s *DNSService) List(serverID uint64) ([]models.DNSProvider, error) {
	var providers []models.DNSProvider
	err := s.db.Where("server_id = ?", serverID).Find(&providers).Error
	return providers, err
}

func (s *DNSService) Delete(providerID uint64) error {
	return s.db.Delete(&models.DNSProvider{}, providerID).Error
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

// ListDNSRecords returns all DNS records for a provider's zone from Cloudflare.
func (s *DNSService) ListDNSRecords(providerID uint64) ([]CFDNSRecord, error) {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return nil, fmt.Errorf("provider not found")
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.cfListRecords(provider)
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
	default:
		return fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

// UpdateDNSRecord is a quick-update shortcut that upserts an A record for a domain.
func (s *DNSService) UpdateDNSRecord(providerID uint64, domain, ip string) error {
	var provider models.DNSProvider
	if err := s.db.First(&provider, providerID).Error; err != nil {
		return err
	}
	switch provider.ProviderType {
	case "cloudflare":
		return s.updateCloudflare(provider, domain, ip)
	default:
		return fmt.Errorf("unsupported DNS provider: %s", provider.ProviderType)
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

func (s *DNSService) updateCloudflare(provider models.DNSProvider, domain, ip string) error {
	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=A&name=%s", provider.ZoneID, domain)
	body, _, err := cfHTTP("GET", listURL, provider.APIKey, nil)
	if err != nil {
		return fmt.Errorf("cloudflare API error: %w", err)
	}
	var listResp struct {
		Result []struct{ ID string `json:"id"` } `json:"result"`
	}
	json.Unmarshal(body, &listResp)

	recordData, _ := json.Marshal(map[string]interface{}{
		"type": "A", "name": domain, "content": ip, "ttl": 300, "proxied": false,
	})

	var method, reqURL string
	if len(listResp.Result) > 0 {
		method = "PUT"
		reqURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", provider.ZoneID, listResp.Result[0].ID)
	} else {
		method = "POST"
		reqURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", provider.ZoneID)
	}
	respBody, status, err := cfHTTP(method, reqURL, provider.APIKey, recordData)
	if err != nil {
		return fmt.Errorf("cloudflare update: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("cloudflare API returned %d: %s", status, string(respBody))
	}
	return nil
}
