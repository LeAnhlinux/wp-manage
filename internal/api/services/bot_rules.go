package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

const (
	botScopeServer  = "server"
	botScopeSite    = "site"
	botCustomPrefix = "custom:"
	maxBotRules     = 100
)

// NormalizeBotRules validates user input and returns rows ready to insert for
// the given scope. Custom rules (no bot_key, or a "custom:" key) are keyed by
// their lowercased User-Agent pattern so the same pattern cannot appear twice.
func NormalizeBotRules(scopeType string, scopeID uint64, in []models.BotRuleInput) ([]models.BotRule, error) {
	if len(in) > maxBotRules {
		return nil, fmt.Errorf("too many bot rules (maximum %d)", maxBotRules)
	}
	seen := make(map[string]bool, len(in))
	out := make([]models.BotRule, 0, len(in))
	for _, r := range in {
		rule := models.BotRule{ScopeType: scopeType, ScopeID: scopeID, Action: r.Action}

		if r.BotKey == "" || strings.HasPrefix(r.BotKey, botCustomPrefix) {
			ua := strings.TrimSpace(r.UserAgent)
			if !models.ValidBotPattern(ua) {
				return nil, fmt.Errorf("invalid user agent %q: use 2-100 letters, digits, spaces or . _ / -", r.UserAgent)
			}
			rule.BotKey = botCustomPrefix + strings.ToLower(ua)
			rule.UserAgent = ua
		} else {
			if _, ok := models.FindBot(r.BotKey); !ok {
				return nil, fmt.Errorf("unknown bot %q", r.BotKey)
			}
			rule.BotKey = r.BotKey
		}

		switch r.Action {
		case models.BotActionAllow, models.BotActionDeny:
		case models.BotActionRateLimit:
			if !models.ValidBotRate(r.RatePerMin) {
				return nil, fmt.Errorf("invalid rate limit %d for %s: must be 10, 30, 60 or 120 requests per minute", r.RatePerMin, rule.BotKey)
			}
			rule.RatePerMin = r.RatePerMin
		default:
			return nil, fmt.Errorf("invalid action %q: must be allow, deny or rate_limit", r.Action)
		}

		if seen[rule.BotKey] {
			return nil, fmt.Errorf("duplicate rule for %s", rule.BotKey)
		}
		seen[rule.BotKey] = true
		out = append(out, rule)
	}
	return out, nil
}

// MergeBotRules returns the ordered effective rules for one site.
//
// Server rules apply only when the server defaults are enabled and the site
// inherits them; a site rule replaces the server rule with the same key.
// Order matters — nginx takes the first matching regex — so it is: site rules
// before server rules, and "allow" first within each group. That lets
// "allow Googlebot" win over a broader custom rule such as "deny bot".
func MergeBotRules(serverEnabled, inherit bool, serverRules, siteRules []models.BotRule) []models.AgentBotRule {
	overridden := make(map[string]bool, len(siteRules))
	for _, r := range siteRules {
		overridden[r.BotKey] = true
	}

	out := appendBotGroup(nil, siteRules, nil)
	if serverEnabled && inherit {
		out = appendBotGroup(out, serverRules, overridden)
	}
	return out
}

func appendBotGroup(out []models.AgentBotRule, rules []models.BotRule, skip map[string]bool) []models.AgentBotRule {
	for _, allowPass := range []bool{true, false} {
		for _, r := range rules {
			if skip[r.BotKey] || (r.Action == models.BotActionAllow) != allowPass {
				continue
			}
			pattern := r.UserAgent
			if !strings.HasPrefix(r.BotKey, botCustomPrefix) {
				bot, ok := models.FindBot(r.BotKey)
				if !ok {
					continue // preset removed from the catalog
				}
				pattern = bot.Pattern
			}
			out = append(out, models.AgentBotRule{
				Name:       r.BotKey,
				Pattern:    pattern,
				Action:     r.Action,
				RatePerMin: r.RatePerMin,
			})
		}
	}
	return out
}

// HasEnforcedBotRule reports whether any rule actually restricts traffic.
func HasEnforcedBotRule(rules []models.AgentBotRule) bool {
	for _, r := range rules {
		if r.Action == models.BotActionDeny || r.Action == models.BotActionRateLimit {
			return true
		}
	}
	return false
}

// maxBotTaskBytes keeps the task payload inside task_queues.data (TEXT, 64 KB).
const maxBotTaskBytes = 60000

// BuildBotTask packs per-site effective rules into an agent task, sending each
// distinct rule list only once, and returns the JSON payload.
func BuildBotTask(sites []models.AgentBotSite) (string, error) {
	task := models.AgentBotRulesTask{Sites: make([]models.AgentBotSite, 0, len(sites))}
	index := map[string]int{}
	for _, site := range sites {
		key, _ := json.Marshal(site.Rules)
		i, ok := index[string(key)]
		if !ok {
			i = len(task.RuleSets)
			index[string(key)] = i
			rules := site.Rules
			if rules == nil {
				rules = []models.AgentBotRule{}
			}
			task.RuleSets = append(task.RuleSets, rules)
		}
		ref := i
		task.Sites = append(task.Sites, models.AgentBotSite{SiteID: site.SiteID, Domain: site.Domain, RuleSet: &ref})
	}
	data, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if len(data) > maxBotTaskBytes {
		return "", fmt.Errorf("bot rules for this server are too large to apply in one task — reduce the number of per-site overrides")
	}
	return string(data), nil
}
