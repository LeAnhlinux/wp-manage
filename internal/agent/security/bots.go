package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
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
	botSnippetDir  = "/etc/nginx/snippets/wpm-bots"
	botBurst       = 5
)

// The include uses a glob ("[.]" instead of "."). nginx fails `nginx -t` on a
// plain include of a missing file, but silently skips a glob with no match.
// Vhosts get copied verbatim to other servers (migration, backup restore) where
// the snippet does not exist yet; a plain include there would break nginx for
// every site on that server.
var botIncludeRegex = regexp.MustCompile(`(?m)^[ \t]*include /etc/nginx/snippets/wpm-bots/site-(\d+)\[\.\]conf;[ \t]*\n`)

func botIncludeLine(siteID uint64) string {
	return fmt.Sprintf("    include %s/site-%d[.]conf;\n", botSnippetDir, siteID)
}

func botHTTPConfPath(siteID uint64) string {
	return fmt.Sprintf("%s/wpm-bots-site-%d.conf", botHTTPConfDir, siteID)
}

func botSnippetPath(siteID uint64) string {
	return fmt.Sprintf("%s/site-%d.conf", botSnippetDir, siteID)
}

func botVhostPath(domain string) string {
	return fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
}

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

// setBotInclude removes any bot-control include from a vhost and, when want is
// true, adds the one for siteID at the end of the FIRST server block. The first
// block is the real site; with SSL the trailing block is only the port-80
// redirect, where rules would never see HTTPS traffic.
func setBotInclude(vhost string, siteID uint64, want bool) string {
	vhost = botIncludeRegex.ReplaceAllString(vhost, "")
	if !want {
		return vhost
	}
	end := firstServerBlockEnd(vhost)
	if end < 0 {
		return vhost
	}
	return vhost[:end] + botIncludeLine(siteID) + vhost[end:]
}

// botSiteIDFromVhost returns the site id of the bot-control include in a vhost.
func botSiteIDFromVhost(vhost string) (uint64, bool) {
	m := botIncludeRegex.FindStringSubmatch(vhost)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseUint(m[1], 10, 64)
	return id, err == nil
}

// botFileBackup remembers a file's previous state so a failed apply can undo it.
type botFileBackup struct {
	path    string
	data    []byte
	existed bool
}

func backupBotFile(path string) botFileBackup {
	data, err := os.ReadFile(path)
	return botFileBackup{path: path, data: data, existed: err == nil}
}

func (b botFileBackup) restore() {
	if b.existed {
		os.WriteFile(b.path, b.data, 0644)
	} else {
		os.Remove(b.path)
	}
}

// writeOrRemoveBotFile writes content, or removes the file when content is empty.
func writeOrRemoveBotFile(path, content string) error {
	if content == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

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
	if err := os.MkdirAll(botSnippetDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", botSnippetDir, err)
	}

	var backups []botFileBackup
	rollback := func() {
		for i := len(backups) - 1; i >= 0; i-- {
			backups[i].restore()
		}
	}

	for _, it := range items {
		vhostPath := botVhostPath(it.site.Domain)
		vhostData, err := os.ReadFile(vhostPath)
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
			{vhostPath, newVhost},
		}
		for _, w := range writes {
			backups = append(backups, backupBotFile(w.path))
			if err := writeOrRemoveBotFile(w.path, w.content); err != nil {
				rollback()
				return fmt.Errorf("write %s: %w", w.path, err)
			}
		}
	}

	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		rollback()
		return fmt.Errorf("nginx config test failed, changes rolled back: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload failed: %s", strings.TrimSpace(string(out)))
	}
	slog.Info("Bot rules applied", "sites", len(items))
	return nil
}

// BotSiteIDForDomain reports which site id's bot rules a domain's vhost
// includes. Call it BEFORE an operation that rewrites or deletes the vhost.
func BotSiteIDForDomain(domain string) (uint64, bool) {
	data, err := os.ReadFile(botVhostPath(domain))
	if err != nil {
		return 0, false
	}
	return botSiteIDFromVhost(string(data))
}

// RestoreBotInclude re-adds the bot-control include after a vhost was
// regenerated from the template (which drops it), then tests and reloads.
func RestoreBotInclude(domain string, siteID uint64) error {
	if _, err := os.Stat(botSnippetPath(siteID)); err != nil {
		return nil // rules were removed meanwhile
	}
	vhostPath := botVhostPath(domain)
	backup := backupBotFile(vhostPath)
	if !backup.existed {
		return fmt.Errorf("nginx vhost not found for %s", domain)
	}
	newVhost := setBotInclude(string(backup.data), siteID, true)
	if newVhost == string(backup.data) {
		return nil
	}
	if err := os.WriteFile(vhostPath, []byte(newVhost), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		backup.restore()
		return fmt.Errorf("nginx config test failed restoring bot rules: %s", strings.TrimSpace(string(out)))
	}
	return exec.Command("systemctl", "reload", "nginx").Run()
}

// RemoveBotFiles deletes a site's bot-control files (after the site's vhost is gone).
func RemoveBotFiles(siteID uint64) {
	os.Remove(botSnippetPath(siteID))
	os.Remove(botHTTPConfPath(siteID))
}
