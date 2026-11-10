package services

import (
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

const (
	botTaskType          = "configure_bot_rules"
	botSiteTaskTimeout   = 60 * time.Second
	botServerTaskTimeout = 120 * time.Second
)

// BotService stores bot-control rules and pushes the effective rules to agents.
type BotService struct {
	db *gorm.DB
}

func NewBotService(db *gorm.DB) *BotService {
	return &BotService{db: db}
}

// loadScope returns the config and rules of a scope. A scope that was never
// saved gets the defaults that change nothing: server defaults disabled, site
// inheriting.
func (s *BotService) loadScope(db *gorm.DB, scopeType string, scopeID uint64) (models.BotConfig, []models.BotRule, error) {
	cfg := models.BotConfig{ScopeType: scopeType, ScopeID: scopeID, Enabled: false, Inherit: true}
	err := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).First(&cfg).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return cfg, nil, err
	}
	rules := []models.BotRule{}
	if err := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).Order("id ASC").Find(&rules).Error; err != nil {
		return cfg, nil, err
	}
	return cfg, rules, nil
}

// replaceScope overwrites a scope's config and rules. Run inside a transaction.
func (s *BotService) replaceScope(tx *gorm.DB, cfg models.BotConfig, rules []models.BotRule) error {
	var existing models.BotConfig
	err := tx.Where("scope_type = ? AND scope_id = ?", cfg.ScopeType, cfg.ScopeID).First(&existing).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		row := models.BotConfig{ScopeType: cfg.ScopeType, ScopeID: cfg.ScopeID, Enabled: cfg.Enabled, Inherit: cfg.Inherit}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		// Map, not struct: a struct update skips false values.
		if err := tx.Model(&models.BotConfig{}).Where("id = ?", existing.ID).
			Updates(map[string]interface{}{"enabled": cfg.Enabled, "inherit": cfg.Inherit}).Error; err != nil {
			return err
		}
	}

	if err := tx.Where("scope_type = ? AND scope_id = ?", cfg.ScopeType, cfg.ScopeID).Delete(&models.BotRule{}).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	rows := make([]models.BotRule, len(rules))
	for i, r := range rules {
		rows[i] = models.BotRule{
			ScopeType: cfg.ScopeType, ScopeID: cfg.ScopeID,
			BotKey: r.BotKey, UserAgent: r.UserAgent, Action: r.Action, RatePerMin: r.RatePerMin,
		}
	}
	return tx.Create(&rows).Error
}

// effective returns the merged rules the agent should enforce for one site.
func (s *BotService) effective(site models.Site) (models.AgentBotSite, error) {
	serverCfg, serverRules, err := s.loadScope(s.db, botScopeServer, site.ServerID)
	if err != nil {
		return models.AgentBotSite{}, err
	}
	siteCfg, siteRules, err := s.loadScope(s.db, botScopeSite, site.ID)
	if err != nil {
		return models.AgentBotSite{}, err
	}
	return models.AgentBotSite{
		SiteID: site.ID,
		Domain: site.Domain,
		Rules:  MergeBotRules(serverCfg.Enabled, siteCfg.Inherit, serverRules, siteRules),
	}, nil
}

// effectiveAll computes effective rules for several sites, keyed by site id.
func (s *BotService) effectiveAll(sites []models.Site) (map[uint64]models.AgentBotSite, error) {
	out := make(map[uint64]models.AgentBotSite, len(sites))
	for _, site := range sites {
		eff, err := s.effective(site)
		if err != nil {
			return nil, err
		}
		out[site.ID] = eff
	}
	return out, nil
}

// changedSites picks the sites the agent must be told about: those with
// something enforced before or after the change. A site with nothing enforced
// on either side has no files on disk and needs none, so servers that never use
// the feature never receive the task.
func changedSites(sites []models.Site, before, after map[uint64]models.AgentBotSite) []models.AgentBotSite {
	var out []models.AgentBotSite
	for _, site := range sites {
		if HasEnforcedBotRule(before[site.ID].Rules) || HasEnforcedBotRule(after[site.ID].Rules) {
			out = append(out, after[site.ID])
		}
	}
	return out
}

// dispatch queues the task and waits for the agent. serverID routes the task.
func (s *BotService) dispatch(serverID, siteID uint64, sites []models.AgentBotSite, timeout time.Duration) error {
	if len(sites) == 0 {
		return nil
	}
	payload, err := BuildBotTask(sites)
	if err != nil {
		return err
	}
	task := models.TaskQueue{
		ServerID: serverID,
		SiteID:   siteID,
		TaskType: botTaskType,
		Data:     payload,
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue bot rules task: %w", err)
	}

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, timeout)
	if err != nil {
		// The caller rolls the DB back. Cancel the task if the agent has not
		// picked it up, or it would apply the rolled-back rules later.
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
func (s *BotService) SiteServerID(siteID uint64) (uint64, bool) {
	var site models.Site
	if err := s.db.Select("server_id").First(&site, siteID).Error; err != nil {
		return 0, false
	}
	return site.ServerID, true
}

// GetServerRules returns the server-wide defaults.
func (s *BotService) GetServerRules(serverID uint64) (*models.BotRulesResponse, error) {
	cfg, rules, err := s.loadScope(s.db, botScopeServer, serverID)
	if err != nil {
		return nil, err
	}
	return &models.BotRulesResponse{Enabled: cfg.Enabled, Inherit: true, Rules: rules}, nil
}

// GetSiteRules returns a site's overrides plus the server defaults it may inherit.
func (s *BotService) GetSiteRules(siteID uint64) (*models.BotRulesResponse, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	cfg, rules, err := s.loadScope(s.db, botScopeSite, siteID)
	if err != nil {
		return nil, err
	}
	serverCfg, serverRules, err := s.loadScope(s.db, botScopeServer, site.ServerID)
	if err != nil {
		return nil, err
	}
	return &models.BotRulesResponse{
		Enabled:       true,
		Inherit:       cfg.Inherit,
		Rules:         rules,
		ServerEnabled: serverCfg.Enabled,
		ServerRules:   serverRules,
	}, nil
}

// UpdateSiteRules replaces a site's overrides and waits for the agent to apply
// them. If the agent fails or times out, the previous rules are restored so the
// panel keeps showing what nginx actually enforces.
func (s *BotService) UpdateSiteRules(siteID uint64, req models.UpdateBotRulesRequest) (*models.BotRulesResponse, error) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, fmt.Errorf("site not found")
	}
	if site.Status != "active" {
		return nil, fmt.Errorf("site is not active")
	}
	rules, err := NormalizeBotRules(botScopeSite, siteID, req.Rules)
	if err != nil {
		return nil, err
	}

	oldCfg, oldRules, err := s.loadScope(s.db, botScopeSite, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to load bot rules")
	}
	sites := []models.Site{site}
	before, err := s.effectiveAll(sites)
	if err != nil {
		return nil, fmt.Errorf("failed to load bot rules")
	}

	newCfg := oldCfg
	if req.Inherit != nil {
		newCfg.Inherit = *req.Inherit
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error { return s.replaceScope(tx, newCfg, rules) }); err != nil {
		return nil, fmt.Errorf("failed to save bot rules")
	}

	after, err := s.effectiveAll(sites)
	if err == nil {
		err = s.dispatch(site.ServerID, siteID, changedSites(sites, before, after), botSiteTaskTimeout)
	}
	if err != nil {
		s.db.Transaction(func(tx *gorm.DB) error { return s.replaceScope(tx, oldCfg, oldRules) })
		return nil, err
	}
	return s.GetSiteRules(siteID)
}

// UpdateServerRules replaces the server defaults and re-applies every active
// site on the server in a single agent task (one nginx reload).
func (s *BotService) UpdateServerRules(serverID uint64, req models.UpdateBotRulesRequest) (*models.BotRulesResponse, error) {
	var server models.Server
	if err := s.db.First(&server, serverID).Error; err != nil {
		return nil, fmt.Errorf("server not found")
	}
	rules, err := NormalizeBotRules(botScopeServer, serverID, req.Rules)
	if err != nil {
		return nil, err
	}

	oldCfg, oldRules, err := s.loadScope(s.db, botScopeServer, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to load bot rules")
	}
	var sites []models.Site
	if err := s.db.Where("server_id = ? AND status = ?", serverID, "active").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("failed to load sites")
	}
	before, err := s.effectiveAll(sites)
	if err != nil {
		return nil, fmt.Errorf("failed to load bot rules")
	}

	newCfg := oldCfg
	if req.Enabled != nil {
		newCfg.Enabled = *req.Enabled
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error { return s.replaceScope(tx, newCfg, rules) }); err != nil {
		return nil, fmt.Errorf("failed to save bot rules")
	}

	after, err := s.effectiveAll(sites)
	if err == nil {
		err = s.dispatch(serverID, 0, changedSites(sites, before, after), botServerTaskTimeout)
	}
	if err != nil {
		s.db.Transaction(func(tx *gorm.DB) error { return s.replaceScope(tx, oldCfg, oldRules) })
		return nil, err
	}
	return s.GetServerRules(serverID)
}

// QueueForSite pushes a site's effective rules to its agent without waiting.
// Used when a site appears on a server outside of a rule save: after creation,
// import, or migration. No-op when nothing is enforced.
func (s *BotService) QueueForSite(siteID uint64) {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return
	}
	eff, err := s.effective(site)
	if err != nil || !HasEnforcedBotRule(eff.Rules) {
		return
	}
	payload, err := BuildBotTask([]models.AgentBotSite{eff})
	if err != nil {
		return
	}
	s.db.Create(&models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   site.ID,
		TaskType: botTaskType,
		Data:     payload,
		Status:   "pending",
	})
}
