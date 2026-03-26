package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	r53Endpoint   = "https://route53.amazonaws.com"
	r53APIVersion = "2013-04-01"
	r53Service    = "route53"
	r53Region     = "us-east-1" // Route53 is global but signs with us-east-1
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// r53StripZonePrefix removes the /hostedzone/ prefix if present.
func r53StripZonePrefix(id string) string {
	return strings.TrimPrefix(id, "/hostedzone/")
}

// r53HMAC computes HMAC-SHA256.
func r53HMAC(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// r53SHA256Hex returns the hex-encoded SHA-256 hash of a string.
func r53SHA256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// xmlEscape escapes XML special characters in record values.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// r53RecordID creates a synthetic record ID: "name||type||value".
// Route53 records have no UUID; we encode identity in the composite key.
func r53RecordID(name, rtype, value string) string {
	return name + "||" + rtype + "||" + value
}

// r53ParseRecordID parses a synthetic record ID back into components.
func r53ParseRecordID(id string) (name, rtype, value string, err error) {
	parts := strings.SplitN(id, "||", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid route53 record ID: %s", id)
	}
	return parts[0], parts[1], parts[2], nil
}

// r53FQDN appends a trailing dot if not present.
func r53FQDN(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// ── AWS SigV4 HTTP client ─────────────────────────────────────────────────────

// r53HTTP performs an AWS SigV4-signed HTTP request to Route53.
func r53HTTP(method, fullPath, accessKey, secretKey, body string) ([]byte, int, error) {
	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")
	host := "route53.amazonaws.com"

	// Split path from query string; canonical query must be sorted+encoded
	urlPath := fullPath
	canonQuery := ""
	if idx := strings.Index(fullPath, "?"); idx >= 0 {
		urlPath = fullPath[:idx]
		rawQuery := fullPath[idx+1:]
		if vals, err := url.ParseQuery(rawQuery); err == nil {
			canonQuery = vals.Encode() // url.Values.Encode() sorts by key
		} else {
			canonQuery = rawQuery
		}
	}

	payloadHash := r53SHA256Hex(body)

	// Canonical headers sorted alphabetically: content-type < host < x-amz-date
	var canonHeaders, signedHeaders string
	if body != "" {
		canonHeaders = "content-type:application/xml\nhost:" + host + "\nx-amz-date:" + amzDate + "\n"
		signedHeaders = "content-type;host;x-amz-date"
	} else {
		canonHeaders = "host:" + host + "\nx-amz-date:" + amzDate + "\n"
		signedHeaders = "host;x-amz-date"
	}

	canonicalRequest := strings.Join([]string{
		method, urlPath, canonQuery, canonHeaders, signedHeaders, payloadHash,
	}, "\n")

	credentialScope := dateStamp + "/" + r53Region + "/" + r53Service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + r53SHA256Hex(canonicalRequest)

	signingKey := r53HMAC(
		r53HMAC(r53HMAC(r53HMAC([]byte("AWS4"+secretKey), dateStamp), r53Region), r53Service),
		"aws4_request",
	)
	signature := hex.EncodeToString(r53HMAC(signingKey, stringToSign))
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	// Build final URL with canonical query string
	fullURL := r53Endpoint + urlPath
	if canonQuery != "" {
		fullURL += "?" + canonQuery
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("x-amz-date", amzDate)
	if body != "" {
		req.Header.Set("Content-Type", "application/xml")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// r53ParseError extracts the error message from a Route53 XML error response.
func r53ParseError(body []byte, status int) error {
	var errResp struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}
	if err := xml.Unmarshal(body, &errResp); err == nil && errResp.Error.Code != "" {
		return fmt.Errorf("route53 %s: %s", errResp.Error.Code, errResp.Error.Message)
	}
	// Also handle InvalidChangeBatch format
	var changeBatchErr struct {
		XMLName  xml.Name `xml:"InvalidChangeBatch"`
		Messages []string `xml:"Messages>Message"`
	}
	if err := xml.Unmarshal(body, &changeBatchErr); err == nil && len(changeBatchErr.Messages) > 0 {
		return fmt.Errorf("route53 InvalidChangeBatch: %s", strings.Join(changeBatchErr.Messages, "; "))
	}
	return fmt.Errorf("route53 HTTP %d: %s", status, string(body))
}

// ── LookupZone ────────────────────────────────────────────────────────────────

func (s *DNSService) r53LookupZone(accessKey, secretKey, domain string) (*models.LookupZoneResult, error) {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	path := fmt.Sprintf("/%s/hostedzone?dnsname=%s&maxitems=5", r53APIVersion, domain)

	body, status, err := r53HTTP("GET", path, accessKey, secretKey, "")
	if err != nil {
		return nil, fmt.Errorf("route53 API: %w", err)
	}
	if status >= 400 {
		return nil, r53ParseError(body, status)
	}

	var resp struct {
		XMLName     xml.Name `xml:"ListHostedZonesResponse"`
		HostedZones []struct {
			ID   string `xml:"Id"`
			Name string `xml:"Name"`
		} `xml:"HostedZones>HostedZone"`
	}
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(resp.HostedZones) == 0 {
		return nil, fmt.Errorf("no hosted zone found for domain: %s", domain)
	}

	// Find the most specific matching zone
	for _, z := range resp.HostedZones {
		zoneName := strings.TrimSuffix(strings.ToLower(z.Name), ".")
		if zoneName == domain || strings.HasSuffix(domain, "."+zoneName) {
			return &models.LookupZoneResult{
				ZoneID:   r53StripZonePrefix(z.ID),
				ZoneName: zoneName,
			}, nil
		}
	}
	return nil, fmt.Errorf("no exact zone found for domain: %s", domain)
}

// ── TestProvider ──────────────────────────────────────────────────────────────

func (s *DNSService) testRoute53(req models.TestDNSProviderRequest) (*models.TestDNSProviderResult, error) {
	zoneID := r53StripZonePrefix(req.ZoneID)
	path := fmt.Sprintf("/%s/hostedzone/%s", r53APIVersion, zoneID)

	body, status, err := r53HTTP("GET", path, req.APIKey, req.APIEmail, "")
	if err != nil {
		return &models.TestDNSProviderResult{Success: false, Message: "Connection failed: " + err.Error()}, nil
	}
	if status >= 400 {
		return &models.TestDNSProviderResult{Success: false, Message: r53ParseError(body, status).Error()}, nil
	}

	var resp struct {
		XMLName    xml.Name `xml:"GetHostedZoneResponse"`
		HostedZone struct {
			Name string `xml:"Name"`
		} `xml:"HostedZone"`
	}
	xml.Unmarshal(body, &resp)
	zoneName := strings.TrimSuffix(resp.HostedZone.Name, ".")

	return &models.TestDNSProviderResult{
		Success:  true,
		Message:  fmt.Sprintf("Connected! Zone: %s", zoneName),
		ZoneName: zoneName,
	}, nil
}

// ── List Records ──────────────────────────────────────────────────────────────

func (s *DNSService) r53ListRecords(provider models.DNSProvider) ([]CFDNSRecord, error) {
	zoneID := r53StripZonePrefix(provider.ZoneID)
	path := fmt.Sprintf("/%s/hostedzone/%s/rrset?maxitems=300", r53APIVersion, zoneID)

	body, status, err := r53HTTP("GET", path, provider.APIKey, provider.APIEmail, "")
	if err != nil {
		return nil, fmt.Errorf("route53 API: %w", err)
	}
	if status >= 400 {
		return nil, r53ParseError(body, status)
	}

	var resp struct {
		XMLName            xml.Name `xml:"ListResourceRecordSetsResponse"`
		ResourceRecordSets []struct {
			Name            string `xml:"Name"`
			Type            string `xml:"Type"`
			TTL             int    `xml:"TTL"`
			ResourceRecords []struct {
				Value string `xml:"Value"`
			} `xml:"ResourceRecords>ResourceRecord"`
			// AliasTarget records (e.g., CloudFront) have no ResourceRecords
			AliasTarget *struct {
				DNSName string `xml:"DNSName"`
			} `xml:"AliasTarget"`
		} `xml:"ResourceRecordSets>ResourceRecordSet"`
	}
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var records []CFDNSRecord
	for _, rrSet := range resp.ResourceRecordSets {
		name := strings.TrimSuffix(rrSet.Name, ".")
		ttl := rrSet.TTL
		if ttl == 0 {
			ttl = 300
		}

		if rrSet.AliasTarget != nil && len(rrSet.ResourceRecords) == 0 {
			// Alias record — show as ALIAS type
			aliasTarget := strings.TrimSuffix(rrSet.AliasTarget.DNSName, ".")
			records = append(records, CFDNSRecord{
				ID:      r53RecordID(name, "ALIAS", aliasTarget),
				Type:    rrSet.Type + " (alias)",
				Name:    name,
				Content: aliasTarget,
				TTL:     ttl,
				Proxied: false,
			})
			continue
		}

		for _, rr := range rrSet.ResourceRecords {
			records = append(records, CFDNSRecord{
				ID:      r53RecordID(name, rrSet.Type, rr.Value),
				Type:    rrSet.Type,
				Name:    name,
				Content: rr.Value,
				TTL:     ttl,
				Proxied: false,
			})
		}
	}
	return records, nil
}

// ── Create Record ─────────────────────────────────────────────────────────────

func (s *DNSService) r53CreateRecord(provider models.DNSProvider, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL <= 0 {
		rec.TTL = 300
	}
	// If an RRSet already exists for this name+type, append value instead of replacing
	currentValues, ttl, _ := s.r53GetRRSet(provider, rec.Name, rec.Type)
	if len(currentValues) > 0 {
		for _, v := range currentValues {
			if v == rec.Content {
				// Already exists
				existing := rec
				existing.ID = r53RecordID(rec.Name, rec.Type, rec.Content)
				return &existing, nil
			}
		}
		if ttl > 0 {
			rec.TTL = ttl // keep existing TTL consistent
		}
		return s.r53ChangeMultiValue(provider, "UPSERT", rec.Name, rec.Type,
			append(currentValues, rec.Content), rec.TTL)
	}
	return s.r53ChangeRecord(provider, "CREATE", rec)
}

// ── Update Record ─────────────────────────────────────────────────────────────

func (s *DNSService) r53UpdateRecord(provider models.DNSProvider, recordID string, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL <= 0 {
		rec.TTL = 300
	}
	oldName, oldType, oldValue, err := r53ParseRecordID(recordID)
	if err != nil {
		// Fallback: simple UPSERT
		return s.r53ChangeRecord(provider, "UPSERT", rec)
	}

	// If name or type changed: delete old value, create new
	if oldName != rec.Name || oldType != rec.Type {
		_ = s.r53DeleteByNameType(provider, oldName, oldType, oldValue)
		return s.r53ChangeRecord(provider, "UPSERT", rec)
	}

	// Same name+type: fetch current RRSet and replace old value with new
	currentValues, ttl, _ := s.r53GetRRSet(provider, rec.Name, rec.Type)
	if len(currentValues) <= 1 {
		// Single value or lookup failed: simple UPSERT (replaces entire RRSet)
		return s.r53ChangeRecord(provider, "UPSERT", rec)
	}

	if ttl > 0 {
		rec.TTL = ttl
	}
	newValues := make([]string, 0, len(currentValues))
	replaced := false
	for _, v := range currentValues {
		if v == oldValue && !replaced {
			newValues = append(newValues, rec.Content)
			replaced = true
		} else {
			newValues = append(newValues, v)
		}
	}
	if !replaced {
		newValues = append(newValues, rec.Content)
	}
	return s.r53ChangeMultiValue(provider, "UPSERT", rec.Name, rec.Type, newValues, rec.TTL)
}

// ── Delete Record ─────────────────────────────────────────────────────────────

func (s *DNSService) r53DeleteRecord(provider models.DNSProvider, recordID string) error {
	name, rtype, value, err := r53ParseRecordID(recordID)
	if err != nil {
		return fmt.Errorf("invalid record ID: %w", err)
	}
	return s.r53DeleteByNameType(provider, name, rtype, value)
}

func (s *DNSService) r53DeleteByNameType(provider models.DNSProvider, name, rtype, value string) error {
	currentValues, ttl, err := s.r53GetRRSet(provider, name, rtype)
	if err != nil {
		return fmt.Errorf("get RRSet before delete: %w", err)
	}
	if ttl == 0 {
		ttl = 300
	}

	if len(currentValues) <= 1 {
		// Only one value (or none): delete entire RRSet
		useValue := value
		if len(currentValues) == 1 {
			useValue = currentValues[0] // use actual stored value to avoid mismatch
		}
		_, err := s.r53ChangeRecord(provider, "DELETE", CFDNSRecord{
			Type: rtype, Name: name, Content: useValue, TTL: ttl,
		})
		return err
	}

	// Multi-value RRSet: remove this specific value, keep the rest
	newValues := make([]string, 0, len(currentValues)-1)
	for _, v := range currentValues {
		if v != value {
			newValues = append(newValues, v)
		}
	}
	if len(newValues) == 0 {
		newValues = currentValues[:len(currentValues)-1] // safety: keep at least nothing removed
	}
	_, err = s.r53ChangeMultiValue(provider, "UPSERT", name, rtype, newValues, ttl)
	return err
}

// ── AutoCreateRecord ──────────────────────────────────────────────────────────

func (s *DNSService) r53AutoCreateRecord(provider models.DNSProvider, domain, serverIP string) (*models.AutoCreateDNSRecordResult, error) {
	existingValues, _, _ := s.r53GetRRSet(provider, domain, "A")
	action := "created"
	if len(existingValues) > 0 {
		action = "updated"
	}

	rec := CFDNSRecord{Type: "A", Name: domain, Content: serverIP, TTL: 300}
	created, err := s.r53ChangeRecord(provider, "UPSERT", rec)
	if err != nil {
		return nil, err
	}

	return &models.AutoCreateDNSRecordResult{
		RecordID:   created.ID,
		ProviderID: provider.ID,
		Domain:     domain,
		IP:         serverIP,
		Action:     action,
		ZoneName:   provider.Domain,
		Proxied:    false,
	}, nil
}

// ── Low-level helpers ─────────────────────────────────────────────────────────

// r53GetRRSet fetches all values and TTL for a specific name+type RRSet.
// Returns nil, 0, nil if the RRSet does not exist.
func (s *DNSService) r53GetRRSet(provider models.DNSProvider, name, rtype string) (values []string, ttl int, err error) {
	zoneID := r53StripZonePrefix(provider.ZoneID)
	path := fmt.Sprintf("/%s/hostedzone/%s/rrset?name=%s&type=%s&maxitems=1",
		r53APIVersion, zoneID, r53FQDN(name), rtype)

	body, status, err := r53HTTP("GET", path, provider.APIKey, provider.APIEmail, "")
	if err != nil {
		return nil, 0, err
	}
	if status >= 400 {
		return nil, 0, r53ParseError(body, status)
	}

	var resp struct {
		XMLName            xml.Name `xml:"ListResourceRecordSetsResponse"`
		ResourceRecordSets []struct {
			Name            string `xml:"Name"`
			Type            string `xml:"Type"`
			TTL             int    `xml:"TTL"`
			ResourceRecords []struct {
				Value string `xml:"Value"`
			} `xml:"ResourceRecords>ResourceRecord"`
		} `xml:"ResourceRecordSets>ResourceRecordSet"`
	}
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}

	for _, rrSet := range resp.ResourceRecordSets {
		rrName := strings.TrimSuffix(strings.ToLower(rrSet.Name), ".")
		if rrName == strings.ToLower(name) && rrSet.Type == rtype {
			for _, rr := range rrSet.ResourceRecords {
				values = append(values, rr.Value)
			}
			return values, rrSet.TTL, nil
		}
	}
	return nil, 0, nil // not found
}

// r53ChangeRecord sends a ChangeResourceRecordSets request with a single value.
func (s *DNSService) r53ChangeRecord(provider models.DNSProvider, action string, rec CFDNSRecord) (*CFDNSRecord, error) {
	if rec.TTL <= 0 {
		rec.TTL = 300
	}

	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>%s</Action>
        <ResourceRecordSet>
          <Name>%s</Name>
          <Type>%s</Type>
          <TTL>%d</TTL>
          <ResourceRecords>
            <ResourceRecord>
              <Value>%s</Value>
            </ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`,
		action, r53FQDN(rec.Name), rec.Type, rec.TTL, xmlEscape(rec.Content))

	zoneID := r53StripZonePrefix(provider.ZoneID)
	path := fmt.Sprintf("/%s/hostedzone/%s/rrset", r53APIVersion, zoneID)
	respBody, status, err := r53HTTP("POST", path, provider.APIKey, provider.APIEmail, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("route53 API: %w", err)
	}
	if status >= 400 {
		return nil, r53ParseError(respBody, status)
	}

	result := rec
	result.ID = r53RecordID(rec.Name, rec.Type, rec.Content)
	result.Proxied = false
	return &result, nil
}

// r53ChangeMultiValue sends a ChangeResourceRecordSets request with multiple values (for one RRSet).
func (s *DNSService) r53ChangeMultiValue(provider models.DNSProvider, action, name, rtype string, values []string, ttl int) (*CFDNSRecord, error) {
	if ttl <= 0 {
		ttl = 300
	}
	var rrBuf strings.Builder
	for _, v := range values {
		rrBuf.WriteString(fmt.Sprintf("            <ResourceRecord><Value>%s</Value></ResourceRecord>\n", xmlEscape(v)))
	}

	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>%s</Action>
        <ResourceRecordSet>
          <Name>%s</Name>
          <Type>%s</Type>
          <TTL>%d</TTL>
          <ResourceRecords>
%s          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`,
		action, r53FQDN(name), rtype, ttl, rrBuf.String())

	zoneID := r53StripZonePrefix(provider.ZoneID)
	path := fmt.Sprintf("/%s/hostedzone/%s/rrset", r53APIVersion, zoneID)
	respBody, status, err := r53HTTP("POST", path, provider.APIKey, provider.APIEmail, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("route53 API: %w", err)
	}
	if status >= 400 {
		return nil, r53ParseError(respBody, status)
	}

	return &CFDNSRecord{
		ID:      r53RecordID(name, rtype, values[0]),
		Type:    rtype,
		Name:    name,
		Content: strings.Join(values, "\n"),
		TTL:     ttl,
		Proxied: false,
	}, nil
}
