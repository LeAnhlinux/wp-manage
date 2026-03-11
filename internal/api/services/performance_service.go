package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type PerformanceService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewPerformanceService(db *gorm.DB, cfg *config.Config) *PerformanceService {
	return &PerformanceService{db: db, cfg: cfg}
}

// RunPageSpeed calls Google PageSpeed Insights API and stores the result
func (s *PerformanceService) RunPageSpeed(siteID uint64, deviceType string) (*models.PerformanceMetric, error) {
	if deviceType == "" {
		deviceType = "mobile"
	}

	// Get site domain
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}

	if site.Status != "active" {
		return nil, fmt.Errorf("site is not active")
	}

	// Build URL
	scheme := "http"
	if site.SSLEnabled {
		scheme = "https"
	}
	siteURL := fmt.Sprintf("%s://%s", scheme, site.Domain)

	apiURL := fmt.Sprintf("https://www.googleapis.com/pagespeedonline/v5/runPagespeed?url=%s&strategy=%s&category=performance&category=accessibility",
		url.QueryEscape(siteURL), deviceType)

	// Add API key if configured
	if s.cfg.Google.PageSpeedAPIKey != "" {
		apiURL += "&key=" + s.cfg.Google.PageSpeedAPIKey
	}

	// Call Google PageSpeed API with 90s timeout
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("PageSpeed API request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API response: %v", err)
	}

	if resp.StatusCode != 200 {
		// Try to extract error message from Google API response
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				if msg, ok := errObj["message"].(string); ok {
					return nil, fmt.Errorf("PageSpeed API error: %s", msg)
				}
			}
		}
		return nil, fmt.Errorf("PageSpeed API returned status %d", resp.StatusCode)
	}

	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %v", err)
	}

	metric := &models.PerformanceMetric{
		SiteID:     siteID,
		DeviceType: deviceType,
		MeasuredAt: time.Now(),
	}

	// Extract scores from lighthouseResult.categories
	if lr, ok := result["lighthouseResult"].(map[string]interface{}); ok {
		if cats, ok := lr["categories"].(map[string]interface{}); ok {
			if perf, ok := cats["performance"].(map[string]interface{}); ok {
				if score, ok := perf["score"].(float64); ok {
					metric.PerformanceScore = score * 100 // 0-1 → 0-100
				}
			}
			if acc, ok := cats["accessibility"].(map[string]interface{}); ok {
				if score, ok := acc["score"].(float64); ok {
					metric.AccessibilityScore = score * 100
				}
			}
		}

		// Extract audit metrics
		if audits, ok := lr["audits"].(map[string]interface{}); ok {
			metric.LCPMS = extractAuditNumericValue(audits, "largest-contentful-paint")
			metric.CLS = extractAuditNumericValue(audits, "cumulative-layout-shift")
			metric.FIDMS = extractAuditNumericValue(audits, "max-potential-fid")
			metric.TTFBMS = extractAuditNumericValue(audits, "server-response-time")
		}
	}

	if err := s.db.Create(metric).Error; err != nil {
		return nil, fmt.Errorf("failed to save metric: %v", err)
	}

	return metric, nil
}

// GetMetrics returns performance metrics for a site with optional filters
func (s *PerformanceService) GetMetrics(siteID uint64, deviceType string, days int, page, perPage int) ([]models.PerformanceMetric, int64, error) {
	var metrics []models.PerformanceMetric
	var total int64

	query := s.db.Where("site_id = ?", siteID)
	if deviceType != "" {
		query = query.Where("device_type = ?", deviceType)
	}
	if days > 0 {
		since := time.Now().AddDate(0, 0, -days)
		query = query.Where("measured_at >= ?", since)
	}

	query.Model(&models.PerformanceMetric{}).Count(&total)

	offset := (page - 1) * perPage
	if err := query.Order("measured_at DESC").Offset(offset).Limit(perPage).Find(&metrics).Error; err != nil {
		return nil, 0, err
	}

	return metrics, total, nil
}

// GetLatest returns the latest metrics for both mobile and desktop
func (s *PerformanceService) GetLatest(siteID uint64) (map[string]*models.PerformanceMetric, error) {
	result := map[string]*models.PerformanceMetric{
		"mobile":  nil,
		"desktop": nil,
	}

	for _, device := range []string{"mobile", "desktop"} {
		var metric models.PerformanceMetric
		err := s.db.Where("site_id = ? AND device_type = ?", siteID, device).
			Order("measured_at DESC").First(&metric).Error
		if err == nil {
			result[device] = &metric
		}
	}

	return result, nil
}

// extractAuditNumericValue extracts numericValue from a Lighthouse audit
func extractAuditNumericValue(audits map[string]interface{}, key string) float64 {
	if audit, ok := audits[key].(map[string]interface{}); ok {
		if val, ok := audit["numericValue"].(float64); ok {
			return val
		}
	}
	return 0
}
