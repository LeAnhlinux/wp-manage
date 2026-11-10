package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
)

func TestNormalizeBotRules(t *testing.T) {
	ok := []models.BotRuleInput{
		{BotKey: "gptbot", Action: "deny"},
		{BotKey: "ahrefsbot", Action: "rate_limit", RatePerMin: 30},
		{UserAgent: "MyScraper/1.0", Action: "deny"},
	}
	rules, err := NormalizeBotRules("site", 7, ok)
	if err != nil || len(rules) != 3 {
		t.Fatalf("got %v %v", rules, err)
	}
	if rules[2].BotKey != "custom:myscraper/1.0" || rules[2].UserAgent != "MyScraper/1.0" {
		t.Fatalf("custom key: %+v", rules[2])
	}
	if rules[0].ScopeType != "site" || rules[0].ScopeID != 7 {
		t.Fatal("scope not set")
	}
	if rules[0].RatePerMin != 0 || rules[1].RatePerMin != 30 {
		t.Fatalf("rate: %d %d", rules[0].RatePerMin, rules[1].RatePerMin)
	}

	bad := [][]models.BotRuleInput{
		{{BotKey: "nope", Action: "deny"}},
		{{BotKey: "gptbot", Action: "block"}},
		{{BotKey: "gptbot", Action: "rate_limit", RatePerMin: 45}},
		{{UserAgent: `x"; return 200;`, Action: "deny"}},
		{{UserAgent: "a", Action: "deny"}},
		{{BotKey: "gptbot", Action: "deny"}, {BotKey: "gptbot", Action: "allow"}},
		{{UserAgent: "Foo", Action: "deny"}, {UserAgent: "foo", Action: "allow"}},
	}
	for i, in := range bad {
		if _, err := NormalizeBotRules("site", 1, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}

	many := make([]models.BotRuleInput, 0, 101)
	for i := 0; i < 101; i++ {
		many = append(many, models.BotRuleInput{UserAgent: fmt.Sprintf("bot-%03d", i), Action: "deny"})
	}
	if _, err := NormalizeBotRules("site", 1, many[:100]); err != nil {
		t.Errorf("100 rules must be accepted: %v", err)
	}
	if _, err := NormalizeBotRules("site", 1, many); err == nil {
		t.Error("101 rules must be rejected")
	}
}

func TestMergeBotRules(t *testing.T) {
	server := []models.BotRule{
		{BotKey: "gptbot", Action: "deny"},
		{BotKey: "googlebot", Action: "allow"},
		{BotKey: "ahrefsbot", Action: "rate_limit", RatePerMin: 30},
	}
	site := []models.BotRule{
		{BotKey: "gptbot", Action: "allow"},
		{BotKey: "custom:bot", UserAgent: "bot", Action: "deny"},
	}
	got := MergeBotRules(true, true, server, site)
	names := []string{}
	for _, r := range got {
		names = append(names, r.Name+":"+r.Action)
	}
	want := "gptbot:allow,custom:bot:deny,googlebot:allow,ahrefsbot:rate_limit"
	if strings.Join(names, ",") != want {
		t.Fatalf("got %s", strings.Join(names, ","))
	}
	if got[0].Pattern != "GPTBot" || got[1].Pattern != "bot" {
		t.Fatal("pattern not resolved")
	}
	if got[3].RatePerMin != 30 {
		t.Fatal("rate lost")
	}

	if n := len(MergeBotRules(false, true, server, site)); n != 2 {
		t.Fatalf("server disabled: %d", n)
	}
	if n := len(MergeBotRules(true, false, server, site)); n != 2 {
		t.Fatalf("inherit off: %d", n)
	}
	if HasEnforcedBotRule(MergeBotRules(true, true, server[1:2], nil)) {
		t.Fatal("allow-only must not be enforced")
	}
	if !HasEnforcedBotRule(got) {
		t.Fatal("deny must be enforced")
	}
	// A rule whose catalog key no longer exists is dropped, not sent with an empty pattern.
	if n := len(MergeBotRules(true, true, []models.BotRule{{BotKey: "gone", Action: "deny"}}, nil)); n != 0 {
		t.Fatalf("unknown key kept: %d", n)
	}
}

func TestBuildBotTask(t *testing.T) {
	shared := []models.AgentBotRule{{Name: "gptbot", Pattern: "GPTBot", Action: "deny"}}
	own := []models.AgentBotRule{{Name: "curl", Pattern: "curl/", Action: "rate_limit", RatePerMin: 10}}
	sites := []models.AgentBotSite{
		{SiteID: 1, Domain: "a.com", Rules: shared},
		{SiteID: 2, Domain: "b.com", Rules: own},
		{SiteID: 3, Domain: "c.com", Rules: shared},
		{SiteID: 4, Domain: "d.com", Rules: nil}, // rules removed: agent must delete its files
	}
	payload, err := BuildBotTask(sites)
	if err != nil {
		t.Fatal(err)
	}
	var task models.AgentBotRulesTask
	if err := json.Unmarshal([]byte(payload), &task); err != nil {
		t.Fatal(err)
	}
	if len(task.RuleSets) != 3 {
		t.Fatalf("want 3 distinct rule sets, got %d", len(task.RuleSets))
	}
	if err := task.Resolve(); err != nil {
		t.Fatal(err)
	}
	for i, want := range sites {
		got := task.Sites[i]
		if got.SiteID != want.SiteID || got.Domain != want.Domain || len(got.Rules) != len(want.Rules) {
			t.Errorf("site %d did not round-trip: %+v", i, got)
		}
	}
	if task.Sites[2].Rules[0].Pattern != "GPTBot" {
		t.Error("shared rule set lost")
	}

	// 500 sites on the server defaults must fit in the 64 KB payload
	// (the limit is roughly 850 sites with typical domain lengths).
	big := make([]models.AgentBotSite, 500)
	for i := range big {
		big[i] = models.AgentBotSite{SiteID: uint64(i + 1), Domain: fmt.Sprintf("site-%04d.example.com", i), Rules: shared}
	}
	if _, err := BuildBotTask(big); err != nil {
		t.Errorf("500 sites sharing rules must fit: %v", err)
	}

	bad := models.AgentBotRulesTask{Sites: []models.AgentBotSite{{SiteID: 1, RuleSet: new(int)}}}
	if err := bad.Resolve(); err == nil {
		t.Error("out-of-range rule set must be rejected")
	}
}
