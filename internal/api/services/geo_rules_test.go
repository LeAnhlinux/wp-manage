package services

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
)

func TestNormalizeGeoInput(t *testing.T) {
	got, err := NormalizeGeoInput("whitelist", []string{" vn", "US", "vn"}, map[string]string{"google": "allow", "meta": "deny", "bing": ""})
	if err != nil {
		t.Fatal(err)
	}
	want := models.GeoRuleSet{Mode: "whitelist", Countries: []string{"US", "VN"}, Providers: map[string]string{"google": "allow", "meta": "deny"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}

	if r, err := NormalizeGeoInput("", nil, nil); err != nil || r.Mode != "off" {
		t.Fatalf("empty mode must mean off: %+v %v", r, err)
	}
	// "off" keeps the selection so switching the mode back does not lose it.
	if r, _ := NormalizeGeoInput("off", []string{"VN"}, nil); len(r.Countries) != 1 {
		t.Fatal("off must keep countries")
	}

	bad := []struct {
		mode      string
		countries []string
		providers map[string]string
	}{
		{"whitelist", nil, nil},                                             // would block everyone
		{"blacklist", []string{}, nil},                                      // nothing selected
		{"block", []string{"VN"}, nil},                                      // unknown mode
		{"blacklist", []string{"ZZ"}, nil},                                  // unknown country
		{"blacklist", []string{`V"`}, nil},                                  // injection attempt
		{"blacklist", []string{"VN"}, map[string]string{"aws": "deny"}},     // unknown provider
		{"blacklist", []string{"VN"}, map[string]string{"google": "block"}}, // unknown action
	}
	for i, b := range bad {
		if _, err := NormalizeGeoInput(b.mode, b.countries, b.providers); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestEffectiveGeoRule(t *testing.T) {
	server := models.GeoRuleSet{Mode: "blacklist", Countries: []string{"CN"}, Providers: map[string]string{"google": "allow"}}
	site := models.GeoRuleSet{Mode: "whitelist", Countries: []string{"VN"}}

	if r := EffectiveGeoRule(true, server, true, site); r.Mode != "blacklist" || r.Countries[0] != "CN" {
		t.Fatalf("inherit: %+v", r)
	}
	if r := EffectiveGeoRule(false, server, true, site); GeoRuleEnforced(r) {
		t.Fatalf("server disabled + inherit must enforce nothing: %+v", r)
	}
	if r := EffectiveGeoRule(true, server, false, site); r.Mode != "whitelist" || r.Countries[0] != "VN" || len(r.Providers) != 0 {
		t.Fatalf("own config must replace the server's entirely: %+v", r)
	}
	// mode off drops the (remembered) country list from what the agent gets
	off := EffectiveGeoRule(false, server, false, models.GeoRuleSet{Mode: "off", Countries: []string{"VN"}})
	if len(off.Countries) != 0 || GeoRuleEnforced(off) {
		t.Fatalf("off: %+v", off)
	}

	if GeoRuleEnforced(models.GeoRuleSet{Mode: "off", Providers: map[string]string{"google": "allow"}}) {
		t.Error("allow-only with mode off restricts nothing")
	}
	if !GeoRuleEnforced(models.GeoRuleSet{Mode: "off", Providers: map[string]string{"meta": "deny"}}) {
		t.Error("provider deny is enforced even with mode off")
	}
}

func TestBuildGeoTask(t *testing.T) {
	shared := models.GeoRuleSet{Mode: "blacklist", Countries: []string{"CN", "RU"}, Providers: map[string]string{"google": "allow"}}
	own := models.GeoRuleSet{Mode: "whitelist", Countries: []string{"VN"}}
	payload, err := BuildGeoTask([]GeoSiteRule{
		{SiteID: 1, Domain: "a.com", Rule: shared},
		{SiteID: 2, Domain: "b.com", Rule: own},
		{SiteID: 3, Domain: "c.com", Rule: shared},
	})
	if err != nil {
		t.Fatal(err)
	}
	var task models.AgentGeoRulesTask
	if err := json.Unmarshal([]byte(payload), &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Sets) != 2 || len(task.Sites) != 3 {
		t.Fatalf("sets=%d sites=%d", len(task.Sets), len(task.Sites))
	}
	r, err := task.RuleFor(task.Sites[2])
	if err != nil || r.Mode != "blacklist" || r.Providers["google"] != "allow" {
		t.Fatalf("site 3 rule: %+v %v", r, err)
	}
	if _, err := task.RuleFor(models.AgentGeoSite{Set: 9}); err == nil {
		t.Error("out-of-range set must be rejected")
	}

	big := make([]GeoSiteRule, 500)
	for i := range big {
		big[i] = GeoSiteRule{SiteID: uint64(i + 1), Domain: fmt.Sprintf("site-%04d.example.com", i), Rule: shared}
	}
	if _, err := BuildGeoTask(big); err != nil {
		t.Errorf("500 sites sharing rules must fit: %v", err)
	}
}

func TestParseProviderSources(t *testing.T) {
	js := `{"prefixes":[{"ipv4Prefix":"8.8.4.0/24"},{"ipv6Prefix":"2001:4860::/32"},{"ipv4Prefix":"not-a-cidr"}]}`
	if got := cleanCIDRs(parsePrefixJSON([]byte(js))); !reflect.DeepEqual(got, []string{"2001:4860::/32", "8.8.4.0/24"}) {
		t.Errorf("prefix json: %v", got)
	}
	lines := "173.245.48.0/20\n\n# comment\n103.21.244.0/22\n"
	if got := cleanCIDRs(parseCIDRLines(lines)); len(got) != 2 {
		t.Errorf("lines: %v", got)
	}
	radb := "route:      31.13.24.0/21\ndescr:      Facebook\norigin:     AS32934\n\nroute6:     2a03:2880::/32\norigin: AS32934\n"
	if got := cleanCIDRs(parseRADbRoutes(radb)); !reflect.DeepEqual(got, []string{"2a03:2880::/32", "31.13.24.0/21"}) {
		t.Errorf("radb: %v", got)
	}

	// Host bits are normalised, duplicates removed, and absurdly broad ranges
	// (which would allow or block most of the internet) are dropped.
	in := []string{"8.8.4.7/24", "8.8.4.0/24", "0.0.0.0/0", "10.0.0.0/7", "::/0", "2000::/3", `1.2.3.0/24; return 200`}
	if got := cleanCIDRs(in); !reflect.DeepEqual(got, []string{"8.8.4.0/24"}) {
		t.Errorf("clean: %v", got)
	}
	if strings.Contains(strings.Join(cleanCIDRs(in), " "), ";") {
		t.Error("unsafe text survived")
	}
}
