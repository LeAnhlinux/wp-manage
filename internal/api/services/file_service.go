package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

const agentPort = 8443

// agentHTTPClient is a shared HTTP client that uses HTTPS with InsecureSkipVerify.
// The agent uses a self-signed certificate; InsecureSkipVerify still encrypts traffic
// and prevents passive eavesdropping — it only skips CA chain validation.
// Full certificate pinning is planned for a future security hardening sprint.
var agentHTTPClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed cert on internal network
	},
	Timeout: 30 * time.Second,
}

type FileService struct {
	db *gorm.DB
}

func NewFileService(db *gorm.DB) *FileService {
	return &FileService{db: db}
}

// GetSiteWithServer fetches a site with its server info for proxying
func (s *FileService) GetSiteWithServer(siteID uint64) (*models.Site, error) {
	var site models.Site
	err := s.db.Preload("Server").First(&site, siteID).Error
	return &site, err
}

// ProxyToAgent forwards a file manager request to the agent's HTTP server
func (s *FileService) ProxyToAgent(site *models.Site, method, endpoint string, queryParams map[string]string, body []byte) ([]byte, int, error) {
	if site.Server == nil {
		return nil, 0, fmt.Errorf("server not found for site")
	}

	// Build URL
	u := fmt.Sprintf("https://%s:%d%s", site.Server.IPAddress, agentPort, endpoint)
	if len(queryParams) > 0 {
		params := url.Values{}
		for k, v := range queryParams {
			params.Set(k, v)
		}
		u += "?" + params.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, u, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Key", site.Server.AgentAPIKey)

	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reach agent: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// ProxyDownload streams a download response from the agent
func (s *FileService) ProxyDownload(site *models.Site, path string) (*http.Response, error) {
	if site.Server == nil {
		return nil, fmt.Errorf("server not found for site")
	}

	params := url.Values{}
	params.Set("domain", site.Domain)
	params.Set("path", path)
	u := fmt.Sprintf("https://%s:%d/files/download?%s", site.Server.IPAddress, agentPort, params.Encode())

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Key", site.Server.AgentAPIKey)

	downloadClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
		Timeout: 5 * time.Minute,
	}
	return downloadClient.Do(req)
}

// ProxyUpload forwards a multipart upload to the agent
func (s *FileService) ProxyUpload(site *models.Site, destPath string, body io.Reader, contentType string) ([]byte, int, error) {
	if site.Server == nil {
		return nil, 0, fmt.Errorf("server not found for site")
	}

	params := url.Values{}
	params.Set("domain", site.Domain)
	params.Set("path", destPath)
	u := fmt.Sprintf("https://%s:%d/files/upload?%s", site.Server.IPAddress, agentPort, params.Encode())

	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Agent-Key", site.Server.AgentAPIKey)

	uploadClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
		Timeout: 10 * time.Minute,
	}
	resp, err := uploadClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reach agent: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// ProxyUploadChunk forwards a raw binary chunk to the agent
func (s *FileService) ProxyUploadChunk(site *models.Site, uploadID, chunkIndex, filename string, body io.Reader) ([]byte, int, error) {
	if site.Server == nil {
		return nil, 0, fmt.Errorf("server not found for site")
	}

	params := url.Values{}
	params.Set("upload_id", uploadID)
	params.Set("chunk_index", chunkIndex)
	params.Set("filename", filename)
	u := fmt.Sprintf("https://%s:%d/files/upload-chunk?%s", site.Server.IPAddress, agentPort, params.Encode())

	req, err := http.NewRequest("POST", u, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Agent-Key", site.Server.AgentAPIKey)

	chunkClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
		Timeout: 5 * time.Minute,
	}
	resp, err := chunkClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reach agent: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// ProxyUploadMerge asks the agent to assemble all chunks into the final file
func (s *FileService) ProxyUploadMerge(site *models.Site, body []byte) ([]byte, int, error) {
	params := map[string]string{"domain": site.Domain}
	return s.ProxyToAgent(site, "POST", "/files/upload-merge", params, body)
}

// Helper to build common query params
func BuildFileQueryParams(domain, path string) map[string]string {
	return map[string]string{
		"domain": domain,
		"path":   path,
	}
}

// Helper to marshal request body
func MarshalBody(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
