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
)

// ── Constants ─────────────────────────────────────────────────────────────────

const gdEndpoint = "https://api.godaddy.com"

// ── Auth helper ───────────────────────────────────────────────────────────────

// gdHTTP performs an authenticated request to the GoDaddy API.
// apiKey = GoDaddy API Key, apiSecret = GoDaddy API Secret (stored in api_email field).
func gdHTTP(method, path, apiKey, apiSecret string, body []byte) ([]byte, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, gdEndpoint+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "sso-key "+apiKey+":"+apiSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// gdParseError extracts the error message from a GoDaddy API error response.
func gdParseError(body []byte, status int) error {
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Fields  []struct {
			Message string `json:"message"`
			Path    string `json:"path"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		msg := errResp.Message
		if len(errResp.Fields) > 0 {
			msg += ": " + errResp.Fields[0].Message
		}
		return fmt.Errorf("godaddy %s: %s", errResp.Code, msg)
	}
	return fmt.Errorf("godaddy HTTP %d: %s", status, string(body))
}

// gdRecord is the GoDaddy DNS record DTO.
type gdRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// gdRelativeName converts a full name (e.g. "www.example.com") to GoDaddy
// relative format (e.g. "www", or "@" for the root domain).
func gdRelativeName(fullName, domain string) string {
	if fullName == domain || fullName == "@" || fullName == "" {
		return "@"
	}
	rel := strings.TrimSuffix(fullName, "."+domain)
	if rel == fullName {
		// Didn't change — might already be relative (user typed "www")
		return fullName
	}
	return rel
}

// gdFullName converts a GoDaddy relative name to a full name.
func gdFullName(relativeName, domain string) string {
	if relativeName == "@" || relativeName == "" {
		return domain
	}
	// Already full name?
	if strings.HasSuffix(relativeName, "."+domain) || relativeName == domain {
		return relativeName
	}
	return relativeName + "." + domain
}

// gdRecordID creates a synthetic record ID: "name||type||data".
// GoDaddy has no UUID per record; identity = type + name.
func gdRecordID(name, rtype, data string) string {
	return name + "||" + rtype + "||" + data
}

// gdParseRecordID parses a synthetic ID back to components.
func gdParseRecordID(id string) (name, rtype, data string, err error) {
	parts := strings.SplitN(id, "||", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid godaddy record ID: %s", id)
	}
	return parts[0], parts[1], parts[2], nil
}

// ── LookupZone ────────────────────────────────────────────────────────────────

// gdLookupZone verifies that the domain exists in the account and returns
// zone_id = domain (GoDaddy has no separate zone ID).
func (s *DNSService) gdLookupZone(apiKey, apiSecret, domain string) (*models.LookupZoneResult, error) {
	body, status, err := gdHTTP("GET", "/v1/domains/"+domain, apiKey, apiSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("godaddy API: %w", err)
	}
	if status == 404 {
		return nil, fmt.Errorf("domain %s not found in your GoDaddy account", domain)
	}
	if status >= 400 {
		return nil, gdParseError(body, status)
	}
	// zone_id = domain for GoDaddy
	return &models.LookupZoneResult{ZoneID: domain, ZoneName: domain}, nil
}

// ── TestProvider ──────────────────────────────────────────────────────────────

func (s *DNSService) testGoDaddy(req models.TestDNSProviderRequest) (*models.TestDNSProviderResult, error) {
	// Use zone_id (= domain) for the test; fall back to req.Domain
	target := req.ZoneID
	if target == "" {
		target = req.Domain
	}
	body, status, err := gdHTTP("GET", "/v1/domains/"+target, req.APIKey, req.APIEmail, nil)
	if err != nil {
		return &models.TestDNSProviderResult{Success: false, Message: "Connection failed: " + err.Error()}, nil
	}
	if status >= 400 {
		return &models.TestDNSProviderResult{Success: false, Message: gdParseError(body, status).Error()}, nil
	}
	var resp struct {
		Domain string `json:"domain"`
		Status string `json:"status"`
	}
	json.Unmarshal(body, &resp)
	zoneName := resp.Domain
	if zoneName == "" {
		zoneName = target
	}
	return &models.TestDNSProviderResult{
		Success:  true,
		Message:  fmt.Sprintf("Connected! Domain: %s (%s)", zoneName, resp.Status),
		ZoneName: zoneName,
	}, nil
}

// ── List Records ──────────────────────────────────────────────────────────────

func (s *DNSService) gdListRecords(provider models.DNSProvider) ([]CFDNSRecord, error) {
	domain := provider.Domain
	body, status, err := gdHTTP("GET", "/v1/domains/"+domain+"/records", provider.APIKey, provider.APIEmail, nil)
	if err != nil {
		return nil, fmt.Errorf("godaddy API: %w", err)
	}
	if status >= 400 {
		return nil, gdParseError(body, status)
	}

	var raw []gdRecord
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	records := make([]CFDNSRecord, 0, len(raw))
	for _, r := range raw {
		fullName := gdFullName(r.Name, domain)
		ttl := r.TTL
		if ttl == 0 {
			ttl = 3600
		}
		content := r.Data
		// For MX/SRV, prepend priority to content for display
		if (r.Type == "MX" || r.Type == "SRV") && r.Priority > 0 {
			content = fmt.Sprintf("%d %s", r.Priority, r.Data)
		}
		records = append(records, CFDNSRecord{
			ID:      gdRecordID(fullName, r.Type, r.Data),
			Type:    r.Type,
			Name:    fullName,
			Content: content,
			TTL:     ttl,
			Proxied: false,
		})
	}
	return records, nil
}

// ── Create Record ─────────────────────────────────────────────────────────────

func (s *DNSService) gdCreateRecord(provider models.DNSProvider, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL <= 0 {
		rec.TTL = 3600
	}
	domain := provider.Domain
	relName := gdRelativeName(rec.Name, domain)

	gd := gdRecord{
		Name: relName,
		Type: rec.Type,
		Data: rec.Content,
		TTL:  rec.TTL,
	}
	// Extract priority from content if MX/SRV format is "priority data"
	gdExtractPriority(&gd)

	payload, _ := json.Marshal([]gdRecord{gd})
	// PATCH adds records without removing existing ones
	body, status, err := gdHTTP("PATCH", "/v1/domains/"+domain+"/records", provider.APIKey, provider.APIEmail, payload)
	if err != nil {
		return nil, fmt.Errorf("godaddy API: %w", err)
	}
	if status >= 400 {
		return nil, gdParseError(body, status)
	}

	result := rec
	result.ID = gdRecordID(rec.Name, rec.Type, rec.Content)
	result.Proxied = false
	return &result, nil
}

// ── Update Record ─────────────────────────────────────────────────────────────

// gdUpdateRecord replaces all records of the given type+name with the new value.
// GoDaddy PUT /records/{type}/{name} replaces the entire set for that type+name.
func (s *DNSService) gdUpdateRecord(provider models.DNSProvider, recordID string, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL <= 0 {
		rec.TTL = 3600
	}
	domain := provider.Domain
	_, oldType, _, err := gdParseRecordID(recordID)
	if err != nil {
		oldType = rec.Type
	}

	relName := gdRelativeName(rec.Name, domain)
	gd := gdRecord{
		Name: relName,
		Type: rec.Type,
		Data: rec.Content,
		TTL:  rec.TTL,
	}
	gdExtractPriority(&gd)

	payload, _ := json.Marshal([]gdRecord{gd})
	// Use the old type/name to target the right record set
	path := fmt.Sprintf("/v1/domains/%s/records/%s/%s", domain, oldType, relName)
	body, status, err := gdHTTP("PUT", path, provider.APIKey, provider.APIEmail, payload)
	if err != nil {
		return nil, fmt.Errorf("godaddy API: %w", err)
	}
	if status >= 400 {
		return nil, gdParseError(body, status)
	}

	result := rec
	result.ID = gdRecordID(rec.Name, rec.Type, rec.Content)
	result.Proxied = false
	return &result, nil
}

// ── Delete Record ─────────────────────────────────────────────────────────────

// gdDeleteRecord deletes all records of the given type+name.
// GoDaddy DELETE /records/{type}/{name} removes all records of that type+name.
func (s *DNSService) gdDeleteRecord(provider models.DNSProvider, recordID string) error {
	fullName, rtype, _, err := gdParseRecordID(recordID)
	if err != nil {
		return fmt.Errorf("invalid record ID: %w", err)
	}
	domain := provider.Domain
	relName := gdRelativeName(fullName, domain)
	path := fmt.Sprintf("/v1/domains/%s/records/%s/%s", domain, rtype, relName)
	body, status, err := gdHTTP("DELETE", path, provider.APIKey, provider.APIEmail, nil)
	if err != nil {
		return fmt.Errorf("godaddy API: %w", err)
	}
	if status >= 400 {
		return gdParseError(body, status)
	}
	return nil
}

// ── AutoCreateRecord ──────────────────────────────────────────────────────────

func (s *DNSService) gdAutoCreateRecord(provider models.DNSProvider, domain, serverIP string) (*models.AutoCreateDNSRecordResult, error) {
	// Check if A record already exists
	existingBody, status, err := gdHTTP("GET",
		fmt.Sprintf("/v1/domains/%s/records/A/%s", provider.Domain, gdRelativeName(domain, provider.Domain)),
		provider.APIKey, provider.APIEmail, nil)
	action := "created"
	if err == nil && status == 200 {
		var existing []gdRecord
		if json.Unmarshal(existingBody, &existing) == nil && len(existing) > 0 {
			action = "updated"
		}
	}

	// PUT replaces the A record for this name
	relName := gdRelativeName(domain, provider.Domain)
	gd := gdRecord{Name: relName, Type: "A", Data: serverIP, TTL: 3600}
	payload, _ := json.Marshal([]gdRecord{gd})
	path := fmt.Sprintf("/v1/domains/%s/records/A/%s", provider.Domain, relName)
	body, status, err := gdHTTP("PUT", path, provider.APIKey, provider.APIEmail, payload)
	if err != nil {
		return nil, fmt.Errorf("godaddy API: %w", err)
	}
	if status >= 400 {
		return nil, gdParseError(body, status)
	}

	return &models.AutoCreateDNSRecordResult{
		RecordID:   gdRecordID(domain, "A", serverIP),
		ProviderID: provider.ID,
		Domain:     domain,
		IP:         serverIP,
		Action:     action,
		ZoneName:   provider.Domain,
		Proxied:    false,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// gdExtractPriority parses "10 mail.example.com" format into Priority + Data
// for MX/SRV records where the user may have entered priority inline.
func gdExtractPriority(r *gdRecord) {
	if r.Type != "MX" && r.Type != "SRV" {
		return
	}
	var priority int
	var data string
	if _, err := fmt.Sscanf(r.Data, "%d %s", &priority, &data); err == nil {
		r.Priority = priority
		r.Data = data
	}
}
