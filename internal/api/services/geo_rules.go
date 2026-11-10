package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// NormalizeGeoInput validates user input and returns a canonical rule set:
// upper-cased, de-duplicated, sorted country codes and only known providers.
func NormalizeGeoInput(mode string, countries []string, providers map[string]string) (models.GeoRuleSet, error) {
	if mode == "" {
		mode = models.GeoModeOff
	}
	if !models.ValidGeoMode(mode) {
		return models.GeoRuleSet{}, fmt.Errorf("invalid mode %q: must be off, blacklist or whitelist", mode)
	}

	seen := map[string]bool{}
	codes := []string{}
	for _, c := range countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if !models.ValidCountryCode(c) {
			return models.GeoRuleSet{}, fmt.Errorf("unknown country code %q", c)
		}
		if !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)
	// An empty whitelist would block every visitor; an empty blacklist does nothing.
	if mode != models.GeoModeOff && len(codes) == 0 {
		return models.GeoRuleSet{}, fmt.Errorf("select at least one country")
	}

	rules := map[string]string{}
	for key, action := range providers {
		if action == "" {
			continue
		}
		if _, ok := models.FindGeoProvider(key); !ok {
			return models.GeoRuleSet{}, fmt.Errorf("unknown provider %q", key)
		}
		if action != models.GeoActionAllow && action != models.GeoActionDeny {
			return models.GeoRuleSet{}, fmt.Errorf("invalid action %q for %s: must be allow or deny", action, key)
		}
		rules[key] = action
	}
	return models.GeoRuleSet{Mode: mode, Countries: codes, Providers: rules}, nil
}

// EffectiveGeoRule returns what one site should enforce. A site that inherits
// uses the server's rules (only if the server defaults are enabled); otherwise
// its own rules replace the server's entirely.
func EffectiveGeoRule(serverEnabled bool, server models.GeoRuleSet, inherit bool, site models.GeoRuleSet) models.GeoRuleSet {
	var r models.GeoRuleSet
	switch {
	case !inherit:
		r = site
	case serverEnabled:
		r = server
	default:
		r = models.GeoRuleSet{Mode: models.GeoModeOff}
	}

	out := models.GeoRuleSet{Mode: r.Mode, Countries: []string{}, Providers: map[string]string{}}
	if out.Mode == "" {
		out.Mode = models.GeoModeOff
	}
	if out.Mode != models.GeoModeOff {
		out.Countries = append(out.Countries, r.Countries...)
	}
	for k, v := range r.Providers {
		out.Providers[k] = v
	}
	return out
}

// GeoRuleEnforced reports whether a rule set can block anything. Provider
// "allow" entries alone restrict nothing.
func GeoRuleEnforced(r models.GeoRuleSet) bool {
	if r.Mode == models.GeoModeBlacklist || r.Mode == models.GeoModeWhitelist {
		return true
	}
	for _, action := range r.Providers {
		if action == models.GeoActionDeny {
			return true
		}
	}
	return false
}

// GeoSiteRule pairs a site with the rule set it should enforce.
type GeoSiteRule struct {
	SiteID uint64
	Domain string
	Rule   models.GeoRuleSet
}

// BuildGeoTask packs per-site rules into the agent task payload, sending each
// distinct rule set once (task_queues.data is a 64 KB TEXT column).
func BuildGeoTask(sites []GeoSiteRule) (string, error) {
	task := models.AgentGeoRulesTask{Sets: []models.GeoRuleSet{}, Sites: make([]models.AgentGeoSite, 0, len(sites))}
	index := map[string]int{}
	for _, s := range sites {
		key, _ := json.Marshal(s.Rule) // map keys are marshalled sorted: stable
		i, ok := index[string(key)]
		if !ok {
			i = len(task.Sets)
			index[string(key)] = i
			task.Sets = append(task.Sets, s.Rule)
		}
		task.Sites = append(task.Sites, models.AgentGeoSite{SiteID: s.SiteID, Domain: s.Domain, Set: i})
	}
	data, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if len(data) > maxBotTaskBytes {
		return "", fmt.Errorf("country access rules for this server are too large to apply in one task — reduce the number of per-site overrides")
	}
	return string(data), nil
}
