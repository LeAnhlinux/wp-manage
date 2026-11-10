package security

import (
	"strings"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
)

func botSite(rules ...models.AgentBotRule) models.AgentBotSite {
	return models.AgentBotSite{SiteID: 42, Domain: "example.com", Rules: rules}
}

func TestBuildBotConfig(t *testing.T) {
	httpConf, snippet, err := BuildBotConfig(botSite(
		models.AgentBotRule{Name: "googlebot", Pattern: "Googlebot", Action: "allow"},
		models.AgentBotRule{Name: "gptbot", Pattern: "GPTBot", Action: "deny"},
		models.AgentBotRule{Name: "ahrefsbot", Pattern: "AhrefsBot", Action: "rate_limit", RatePerMin: 30},
	))
	if err != nil {
		t.Fatal(err)
	}
	wantHTTP := `# Managed by WP Manage — bot control for site 42 (example.com). Do not edit.
map $http_user_agent $wpm_bot_deny_42 {
    default 0;
    "~*Googlebot" 0;
    "~*GPTBot" 1;
    "~*AhrefsBot" 0;
}
map $http_user_agent $wpm_bot_rl30_42 {
    default "";
    "~*Googlebot" "";
    "~*GPTBot" "";
    "~*AhrefsBot" "ahrefsbot";
}
limit_req_zone $wpm_bot_rl30_42 zone=wpm_bot_42_30:1m rate=30r/m;
`
	wantSnippet := `# Managed by WP Manage — bot control. Do not edit.
if ($wpm_bot_deny_42) { return 403; }
limit_req zone=wpm_bot_42_30 burst=5 nodelay;
limit_req_status 429;
`
	if httpConf != wantHTTP {
		t.Errorf("http conf:\n%s\nwant:\n%s", httpConf, wantHTTP)
	}
	if snippet != wantSnippet {
		t.Errorf("snippet:\n%s\nwant:\n%s", snippet, wantSnippet)
	}
}

func TestBuildBotConfigTiersAndEscaping(t *testing.T) {
	httpConf, snippet, err := BuildBotConfig(botSite(
		models.AgentBotRule{Name: "b", Pattern: "Bbot", Action: "rate_limit", RatePerMin: 120},
		models.AgentBotRule{Name: "custom:python.requests", Pattern: "python.requests", Action: "rate_limit", RatePerMin: 10},
	))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(httpConf, "wpm_bot_deny") || strings.Contains(snippet, "return 403") {
		t.Error("no deny rule: deny map / if must be omitted")
	}
	if !strings.Contains(httpConf, `"~*python\.requests" "custom_python_requests";`) {
		t.Errorf("pattern not escaped / key not sanitized:\n%s", httpConf)
	}
	i10 := strings.Index(snippet, "zone=wpm_bot_42_10 ")
	i120 := strings.Index(snippet, "zone=wpm_bot_42_120 ")
	if i10 < 0 || i120 < 0 || i10 > i120 {
		t.Errorf("tiers missing or unsorted:\n%s", snippet)
	}
	if strings.Count(httpConf, "limit_req_zone") != 2 {
		t.Errorf("want 2 zones:\n%s", httpConf)
	}
	if strings.Count(snippet, "limit_req_status 429;") != 1 {
		t.Errorf("limit_req_status must appear once:\n%s", snippet)
	}
}

func TestBuildBotConfigNothingEnforced(t *testing.T) {
	for _, s := range []models.AgentBotSite{
		botSite(),
		botSite(models.AgentBotRule{Name: "googlebot", Pattern: "Googlebot", Action: "allow"}),
	} {
		h, sn, err := BuildBotConfig(s)
		if err != nil || h != "" || sn != "" {
			t.Errorf("want empty config, got %q %q %v", h, sn, err)
		}
	}
}

func TestBuildBotConfigRejectsBadInput(t *testing.T) {
	deny := func(p string) models.AgentBotRule {
		return models.AgentBotRule{Name: "x", Pattern: p, Action: "deny"}
	}
	bad := []models.AgentBotSite{
		botSite(deny(`a"b`)),
		botSite(deny("a;b")),
		botSite(deny("a{b")),
		botSite(deny("a$b")),
		botSite(deny("a\nb")),
		botSite(models.AgentBotRule{Name: "x", Pattern: "ok", Action: "rate_limit", RatePerMin: 45}),
		botSite(models.AgentBotRule{Name: "x", Pattern: "ok", Action: "block"}),
		// invalid input is rejected even on an allow rule, which is not rendered
		botSite(models.AgentBotRule{Name: "x", Pattern: `"`, Action: "allow"}),
		{SiteID: 42, Domain: "bad_domain", Rules: []models.AgentBotRule{deny("ok")}},
		{SiteID: 42, Domain: "../etc", Rules: []models.AgentBotRule{deny("ok")}},
		{SiteID: 0, Domain: "example.com", Rules: []models.AgentBotRule{deny("ok")}},
	}
	for i, s := range bad {
		if _, _, err := BuildBotConfig(s); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

const twoBlockVhost = `server {
    listen 443 ssl;
    server_name example.com;
    location / {
        try_files $uri $uri/ /index.php?$args;
    }
}

server {
    listen 80;
    server_name example.com;
    return 301 https://$host$request_uri;
}
`

func TestSetBotInclude(t *testing.T) {
	line := "    include /etc/nginx/snippets/wpm-bots/site-42[.]conf;\n"

	with := setBotInclude(twoBlockVhost, 42, true)
	idx := strings.Index(with, line)
	if idx < 0 {
		t.Fatalf("include not added:\n%s", with)
	}
	if idx > strings.Index(with, "listen 80;") {
		t.Error("include must be in the FIRST server block")
	}
	if strings.Count(with[:idx], "{") != strings.Count(with[:idx], "}")+1 {
		t.Error("include must be at server-block depth, not inside a location")
	}
	if again := setBotInclude(with, 42, true); again != with {
		t.Error("not idempotent")
	}
	if id, ok := botSiteIDFromVhost(with); !ok || id != 42 {
		t.Errorf("site id round trip: %d %v", id, ok)
	}
	if _, ok := botSiteIDFromVhost(twoBlockVhost); ok {
		t.Error("no include: must report not found")
	}
	if removed := setBotInclude(with, 42, false); removed != twoBlockVhost {
		t.Errorf("remove must restore the original:\n%s", removed)
	}
	// A stale include for another site id is replaced, never duplicated.
	moved := setBotInclude(with, 7, true)
	if strings.Contains(moved, "site-42[") || strings.Count(moved, "wpm-bots/") != 1 {
		t.Errorf("stale include kept:\n%s", moved)
	}
}
