package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

const (
	geoTaskType        = "configure_geo_rules"
	geoRefreshTaskType = "refresh_geo_data"
	// The first apply on a server installs an nginx module and downloads the
	// country database, so these are longer than the bot-rule timeouts.
	geoSiteTaskTimeout   = 90 * time.Second
	geoServerTaskTimeout = 180 * time.Second
)

// GeoService stores country-access settings, keeps the provider IP lists
// fresh, and pushes effective rules to agents.
type GeoService struct {
	db *gorm.DB
}

func NewGeoService(db *gorm.DB) *GeoService {
	return &GeoService{db: db}
}

func geoRuleFromConfig(cfg models.GeoConfig) models.GeoRuleSet {
	rule := models.GeoRuleSet{Mode: cfg.Mode, Countries: []string{}, Providers: map[string]string{}}
	if rule.Mode == "" {
		rule.Mode = models.GeoModeOff
	}
	if cfg.Countries != "" {
		rule.Countries = strings.Split(cfg.Countries, ",")
	}
	if cfg.ProviderRules != "" {
		json.Unmarshal([]byte(cfg.ProviderRules), &rule.Providers)
	}
	return rule
}

// load returns a scope's config. A scope that was never saved gets defaults
// that change nothing: server defaults disabled, site inheriting, mode off.
func (s *GeoService) load(scopeType string, scopeID uint64) (models.GeoConfig, error) {
	cfg := models.GeoConfig{ScopeType: scopeType, ScopeID: scopeID, Enabled: false, Inherit: true, Mode: models.GeoModeOff}
	err := s.db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).First(&cfg).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return cfg, err
	}
	return cfg, nil
}

// save upserts a scope's config. Every column is written explicitly through a
// map so false / empty values are stored (see the GORM bool pattern).
func (s *GeoService) save(cfg models.GeoConfig) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.GeoConfig
		err := tx.Where("scope_type = ? AND scope_id = ?", cfg.ScopeType, cfg.ScopeID).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			row := cfg
			row.ID = 0
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&models.GeoConfig{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"enabled":        cfg.Enabled,
			"inherit":        cfg.Inherit,
			"mode":           cfg.Mode,
			"countries":      cfg.Countries,
			"provider_rules": cfg.ProviderRules,
		}).Error
	})
}

func applyGeoRule(cfg *models.GeoConfig, rule models.GeoRuleSet) {
	cfg.Mode = rule.Mode
	cfg.Countries = strings.Join(rule.Countries, ",")
	providers, _ := json.Marshal(rule.Providers)
	cfg.ProviderRules = string(providers)
}

func (s *GeoService) effective(site models.Site) (GeoSiteRule, error) {
	serverCfg, err := s.load(botScopeServer, site.ServerID)
	if err != nil {
		return GeoSiteRule{}, err
	}
	siteCfg, err := s.load(botScopeSite, site.ID)
	if err != nil {
		return GeoSiteRule{}, err
	}
	return GeoSiteRule{
		SiteID: site.ID,
		Domain: site.Domain,
		Rule:   EffectiveGeoRule(serverCfg.Enabled, geoRuleFromConfig(serverCfg), siteCfg.Inherit, geoRuleFromConfig(siteCfg)),
	}, nil
}

func (s *GeoService) effectiveAll(sites []models.Site) (map[uint64]GeoSiteRule, error) {
	out := make(map[uint64]GeoSiteRule, len(sites))
	for _, site := range sites {
		eff, err := s.effective(site)
		if err != nil {
			return nil, err
		}
		out[site.ID] = eff
	}
	return out, nil
}

// changedGeoSites picks the sites with something enforced before or after a
// change. Servers that never use the feature never receive the task (and so
// never get the nginx module installed).
func changedGeoSites(sites []models.Site, before, after map[uint64]GeoSiteRule) []GeoSiteRule {
	var out []GeoSiteRule
	for _, site := range sites {
		if GeoRuleEnforced(before[site.ID].Rule) || GeoRuleEnforced(after[site.ID].Rule) {
			out = append(out, after[site.ID])
		}
	}
	return out
}

func (s *GeoService) dispatch(serverID, siteID uint64, sites []GeoSiteRule, timeout time.Duration) error {
	if len(sites) == 0 {
		return nil
	}
	// The agent pulls provider ranges from the panel while applying.
	s.EnsureProviders()

	payload, err := BuildGeoTask(sites)
	if err != nil {
		return err
	}
	task := models.TaskQueue{ServerID: serverID, SiteID: siteID, TaskType: geoTaskType, Data: payload, Status: "pending"}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue country access task: %w", err)
	}
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, timeout)
	if err != nil {
		s.db.Model(&models.TaskQueue{}).
			Where("id = ? AND status = ?", task.ID, "pending").
			Updates(map[string]interface{}{"status": "failed", "error": "cancelled: agent did not respond in time"})
		return fmt.Errorf("agent timeout: the server agent did not respond — is it online?")
	}
	if result.Status == "failed" || result.Error != "" {
		return fmt.Errorf("agent error: %s", result.Error)
	}
	return nil
}

// SiteServerID returns the server a site lives on (for access checks).
func (s *GeoService) SiteServerID(siteID uint64) (uint64, bool) {
	var site models.Site
	if err := s.db.Select("server_id").First(&site, siteID).Error; err != nil {
		return 0, false
	}
	return site.ServerID, true
}

func (s *GeoService) GetServerRules(serverID uint64) (*models.GeoRulesResponse, error) {
	cfg, err := s.load(botScopeServer, serverID)
	if err != nil {
		return nil, err
	}
	return &models.GeoRulesResponse{Enabled: cfg.Enabled, Inherit: true, GeoRuleSet: geoRuleFromConfig(cfg)}, nil
}

func (s *GeoService) GetSiteRules(siteID uint64) (*models.GeoRulesResponse, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	cfg, err := s.load(botScopeSite, siteID)
	if err != nil {
		return nil, err
	}
	serverCfg, err := s.load(botScopeServer, site.ServerID)
	if err != nil {
		return nil, err
	}
	serverRule := geoRuleFromConfig(serverCfg)
	return &models.GeoRulesResponse{
		Enabled:       true,
		Inherit:       cfg.Inherit,
		GeoRuleSet:    geoRuleFromConfig(cfg),
		ServerEnabled: serverCfg.Enabled,
		Server:        &serverRule,
	}, nil
}

// UpdateSiteRules saves a site's settings and waits for the agent. On agent
// failure the previous settings are restored so the panel keeps showing what
// nginx actually enforces.
func (s *GeoService) UpdateSiteRules(siteID uint64, req models.UpdateGeoRulesRequest) (*models.GeoRulesResponse, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site is not active")
	}
	rule, err := NormalizeGeoInput(req.Mode, req.Countries, req.Providers)
	if err != nil {
		return nil, err
	}
	oldCfg, err := s.load(botScopeSite, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to load country access settings")
	}
	sites := []models.Site{site}
	before, err := s.effectiveAll(sites)
	if err != nil {
		return nil, fmt.Errorf("failed to load country access settings")
	}

	newCfg := oldCfg
	if req.Inherit != nil {
		newCfg.Inherit = *req.Inherit
	}
	applyGeoRule(&newCfg, rule)
	if err := s.save(newCfg); err != nil {
		return nil, fmt.Errorf("failed to save country access settings")
	}

	after, err := s.effectiveAll(sites)
	if err == nil {
		err = s.dispatch(site.ServerID, siteID, changedGeoSites(sites, before, after), geoSiteTaskTimeout)
	}
	if err != nil {
		s.save(oldCfg)
		return nil, err
	}
	return s.GetSiteRules(siteID)
}

// UpdateServerRules saves the server defaults and re-applies every active site
// on the server in one agent task.
func (s *GeoService) UpdateServerRules(serverID uint64, req models.UpdateGeoRulesRequest) (*models.GeoRulesResponse, error) {
	var server models.Server
	if err := s.db.First(&server, serverID).Error; err != nil {
		return nil, fmt.Errorf("server not found")
	}
	rule, err := NormalizeGeoInput(req.Mode, req.Countries, req.Providers)
	if err != nil {
		return nil, err
	}
	oldCfg, err := s.load(botScopeServer, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to load country access settings")
	}
	var sites []models.Site
	if err := s.db.Where("server_id = ? AND status = ?", serverID, "active").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("failed to load sites")
	}
	before, err := s.effectiveAll(sites)
	if err != nil {
		return nil, fmt.Errorf("failed to load country access settings")
	}

	newCfg := oldCfg
	if req.Enabled != nil {
		newCfg.Enabled = *req.Enabled
	}
	applyGeoRule(&newCfg, rule)
	if err := s.save(newCfg); err != nil {
		return nil, fmt.Errorf("failed to save country access settings")
	}

	after, err := s.effectiveAll(sites)
	if err == nil {
		err = s.dispatch(serverID, 0, changedGeoSites(sites, before, after), geoServerTaskTimeout)
	}
	if err != nil {
		s.save(oldCfg)
		return nil, err
	}
	return s.GetServerRules(serverID)
}

// QueueForSite pushes a site's effective rules to its agent without waiting:
// used after site creation, import and migration. No-op when nothing is enforced.
func (s *GeoService) QueueForSite(siteID uint64) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return
	}
	eff, err := s.effective(site)
	if err != nil || !GeoRuleEnforced(eff.Rule) {
		return
	}
	payload, err := BuildGeoTask([]GeoSiteRule{eff})
	if err != nil {
		return
	}
	s.db.Create(&models.TaskQueue{ServerID: site.ServerID, SiteID: site.ID, TaskType: geoTaskType, Data: payload, Status: "pending"})
}

// --- Provider IP ranges ---

// GeoProviderStatus is a catalog entry plus the state of its cached ranges.
type GeoProviderStatus struct {
	models.GeoProvider
	RangeCount int        `json:"range_count"`
	FetchedAt  *time.Time `json:"fetched_at"`
	LastError  string     `json:"last_error,omitempty"`
}

// Catalog returns the selectable countries and providers.
func (s *GeoService) Catalog() map[string]interface{} {
	var rows []models.IPProvider
	s.db.Select("provider_key", "range_count", "fetched_at", "last_error").Find(&rows)
	byKey := map[string]models.IPProvider{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	providers := make([]GeoProviderStatus, 0, len(models.GeoProviders))
	for _, p := range models.GeoProviders {
		row := byKey[p.Key]
		providers = append(providers, GeoProviderStatus{GeoProvider: p, RangeCount: row.RangeCount, FetchedAt: row.FetchedAt, LastError: row.LastError})
	}
	return map[string]interface{}{"countries": models.GeoCountryCodes, "providers": providers}
}

// ProvidersForAgent returns every cached provider range list.
func (s *GeoService) ProvidersForAgent() models.AgentGeoProviders {
	out := models.AgentGeoProviders{Providers: map[string][]string{}}
	var rows []models.IPProvider
	s.db.Find(&rows)
	for _, r := range rows {
		if _, ok := models.FindGeoProvider(r.Key); !ok || r.CIDRs == "" {
			continue
		}
		out.Providers[r.Key] = strings.Split(r.CIDRs, "\n")
	}
	return out
}

// RefreshProviders downloads every provider's current ranges. A provider whose
// fetch fails keeps its previous ranges; only last_error changes.
func (s *GeoService) RefreshProviders() {
	for _, p := range models.GeoProviders {
		cidrs, err := fetchProviderCIDRs(p.Key)

		var row models.IPProvider
		found := s.db.Where("provider_key = ?", p.Key).First(&row).Error == nil
		if !found {
			row = models.IPProvider{Key: p.Key}
			if cerr := s.db.Create(&row).Error; cerr != nil {
				slog.Error("GeoProviders: could not create row", "provider", p.Key, "error", cerr)
				continue
			}
		}
		if err != nil {
			slog.Warn("GeoProviders: fetch failed, keeping previous ranges", "provider", p.Key, "error", err)
			s.db.Model(&models.IPProvider{}).Where("id = ?", row.ID).Update("last_error", err.Error())
			continue
		}
		now := time.Now()
		s.db.Model(&models.IPProvider{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"cidrs":       strings.Join(cidrs, "\n"),
			"range_count": len(cidrs),
			"last_error":  "",
			"fetched_at":  &now,
		})
		slog.Info("GeoProviders: ranges updated", "provider", p.Key, "ranges", len(cidrs))
	}
}

// EnsureProviders fetches the provider ranges if none are cached yet.
func (s *GeoService) EnsureProviders() {
	var n int64
	s.db.Model(&models.IPProvider{}).Where("range_count > 0").Count(&n)
	if n == 0 {
		s.RefreshProviders()
	}
}

// QueueRefreshForActiveServers asks every server that enforces country access
// to refresh its country database and provider ranges.
func (s *GeoService) QueueRefreshForActiveServers() {
	var servers []models.Server
	s.db.Where("status = ?", "active").Find(&servers)
	for _, server := range servers {
		var sites []models.Site
		s.db.Where("server_id = ? AND status = ?", server.ID, "active").Find(&sites)
		rules, err := s.effectiveAll(sites)
		if err != nil {
			continue
		}
		inUse := false
		for _, r := range rules {
			if GeoRuleEnforced(r.Rule) {
				inUse = true
				break
			}
		}
		if inUse {
			s.db.Create(&models.TaskQueue{ServerID: server.ID, TaskType: geoRefreshTaskType, Data: "{}", Status: "pending"})
		}
	}
}
