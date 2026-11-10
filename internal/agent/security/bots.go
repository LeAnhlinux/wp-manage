package security

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// Bot control renders two nginx files per site, both keyed by SITE ID rather
// than domain: a domain change then overwrites the same files, so two files can
// never define the same map variable (which would break `nginx -t` for the
// whole server).
const (
	botHTTPConfDir = "/etc/nginx/conf.d"
	botBurst       = 5
)

func botHTTPConfPath(siteID uint64) string {
	return fmt.Sprintf("%s/wpm-bots-site-%d.conf", botHTTPConfDir, siteID)
}

func botSnippetPath(siteID uint64) string { return botInclude.snippetPath(siteID) }

func botIncludeLine(siteID uint64) string { return botInclude.line(siteID) }

// validateBotSite checks everything that ends up in a file path or inside
// nginx config. The API validates too; the agent must not rely on that.
func validateBotSite(site models.AgentBotSite) error {
	if site.SiteID == 0 {
		return fmt.Errorf("bot rules: missing site id for %q", site.Domain)
	}
	if err := utils.ValidateDomain(site.Domain); err != nil {
		return fmt.Errorf("bot rules: %w", err)
	}
	for _, r := range site.Rules {
		if !models.ValidBotPattern(r.Pattern) {
			return fmt.Errorf("bot rules: invalid user agent pattern %q", r.Pattern)
		}
		switch r.Action {
		case models.BotActionAllow, models.BotActionDeny:
		case models.BotActionRateLimit:
			if !models.ValidBotRate(r.RatePerMin) {
				return fmt.Errorf("bot rules: invalid rate %d for %q", r.RatePerMin, r.Pattern)
			}
		default:
			return fmt.Errorf("bot rules: invalid action %q", r.Action)
		}
	}
	return nil
}

// botRegex turns a validated literal pattern into an nginx map key. The
// allowed charset has a single regex metacharacter: the dot.
func botRegex(pattern string) string {
	return `"~*` + strings.ReplaceAll(pattern, ".", `\.`) + `"`
}

// botLimitKey reduces a rule name to [a-z0-9_] for use as the limit_req key.
func botLimitKey(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// BuildBotConfig renders the http-context file and the server-context snippet
// for one site. Both are empty when no rule restricts traffic, meaning the
// site's files should be removed.
//
// Every rule is written into every map, in task order: nginx uses the first
// matching regex, so an earlier "allow" shadows a later, broader rule.
func BuildBotConfig(site models.AgentBotSite) (httpConf, snippet string, err error) {
	if err := validateBotSite(site); err != nil {
		return "", "", err
	}

	hasDeny := false
	tierSet := map[int]bool{}
	for _, r := range site.Rules {
		switch r.Action {
		case models.BotActionDeny:
			hasDeny = true
		case models.BotActionRateLimit:
			tierSet[r.RatePerMin] = true
		}
	}
	if !hasDeny && len(tierSet) == 0 {
		return "", "", nil
	}
	tiers := make([]int, 0, len(tierSet))
	for t := range tierSet {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)

	id := site.SiteID
	var h, s strings.Builder
	fmt.Fprintf(&h, "# Managed by WP Manage — bot control for site %d (%s). Do not edit.\n", id, site.Domain)
	s.WriteString("# Managed by WP Manage — bot control. Do not edit.\n")

	if hasDeny {
		fmt.Fprintf(&h, "map $http_user_agent $wpm_bot_deny_%d {\n    default 0;\n", id)
		for _, r := range site.Rules {
			v := 0
			if r.Action == models.BotActionDeny {
				v = 1
			}
			fmt.Fprintf(&h, "    %s %d;\n", botRegex(r.Pattern), v)
		}
		h.WriteString("}\n")
		fmt.Fprintf(&s, "if ($wpm_bot_deny_%d) { return 403; }\n", id)
	}

	for _, tier := range tiers {
		// Empty key = request is not counted against the zone.
		fmt.Fprintf(&h, "map $http_user_agent $wpm_bot_rl%d_%d {\n    default \"\";\n", tier, id)
		for _, r := range site.Rules {
			key := ""
			if r.Action == models.BotActionRateLimit && r.RatePerMin == tier {
				key = botLimitKey(r.Name)
			}
			fmt.Fprintf(&h, "    %s \"%s\";\n", botRegex(r.Pattern), key)
		}
		h.WriteString("}\n")
		fmt.Fprintf(&h, "limit_req_zone $wpm_bot_rl%d_%d zone=wpm_bot_%d_%d:1m rate=%dr/m;\n", tier, id, id, tier, tier)
		fmt.Fprintf(&s, "limit_req zone=wpm_bot_%d_%d burst=%d nodelay;\n", id, tier, botBurst)
	}
	if len(tiers) > 0 {
		s.WriteString("limit_req_status 429;\n")
	}
	return h.String(), s.String(), nil
}

func setBotInclude(vhost string, siteID uint64, want bool) string {
	return botInclude.set(vhost, siteID, want)
}

func botSiteIDFromVhost(vhost string) (uint64, bool) { return botInclude.siteID(vhost) }

// ApplyBotRules writes the bot-control config for every site in the task, then
// tests and reloads nginx once. It is all-or-nothing: on any error every file
// touched is put back, so a bad rule can never leave nginx unable to reload.
func ApplyBotRules(task models.AgentBotRulesTask) error {
	type rendered struct {
		site              models.AgentBotSite
		httpConf, snippet string
	}
	if err := task.Resolve(); err != nil {
		return err
	}
	// Validate and render everything before touching disk.
	items := make([]rendered, 0, len(task.Sites))
	for _, site := range task.Sites {
		httpConf, snippet, err := BuildBotConfig(site)
		if err != nil {
			return err
		}
		items = append(items, rendered{site, httpConf, snippet})
	}
	if len(items) == 0 {
		return nil
	}
	if err := os.MkdirAll(botInclude.dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", botInclude.dir, err)
	}

	var backups []fileBackup
	rollback := func() {
		for i := len(backups) - 1; i >= 0; i-- {
			backups[i].restore()
		}
	}

	for _, it := range items {
		vPath := vhostPath(it.site.Domain)
		vhostData, err := os.ReadFile(vPath)
		if err != nil {
			rollback()
			return fmt.Errorf("nginx vhost not found for %s", it.site.Domain)
		}
		enforce := it.httpConf != ""
		newVhost := setBotInclude(string(vhostData), it.site.SiteID, enforce)
		if enforce && !strings.Contains(newVhost, botIncludeLine(it.site.SiteID)) {
			rollback()
			return fmt.Errorf("could not find a server block in the nginx vhost for %s", it.site.Domain)
		}

		writes := []struct{ path, content string }{
			{botHTTPConfPath(it.site.SiteID), it.httpConf},
			{botSnippetPath(it.site.SiteID), it.snippet},
			{vPath, newVhost},
		}
		for _, w := range writes {
			backups = append(backups, backupFile(w.path))
			if err := writeOrRemoveFile(w.path, w.content); err != nil {
				rollback()
				return fmt.Errorf("write %s: %w", w.path, err)
			}
		}
	}

	if err := nginxTestAndReload(rollback); err != nil {
		return err
	}
	slog.Info("Bot rules applied", "sites", len(items))
	return nil
}
