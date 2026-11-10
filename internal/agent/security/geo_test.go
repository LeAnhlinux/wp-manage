package security

import (
	"strings"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
)

func TestBuildGeoBaseConfig(t *testing.T) {
	got := BuildGeoBaseConfig(map[string][]string{
		"google":     {"8.8.4.0/24", "2001:4860::/32"},
		"cloudflare": {"173.245.48.0/20", "8.8.4.0/24"}, // duplicate of a google range
		"unknown":    {"9.9.9.0/24"},                    // not in the catalog: ignored
		"meta":       {"31.13.24.0/21", `1.1.1.0/24; return 200`},
	}, []string{"203.0.113.10", "2001:db8::1", "garbage"})

	want := `# Managed by WP Manage — country access (shared by all sites). Do not edit.
geoip2 /var/lib/wp-manage/geoip/country.mmdb {
    auto_reload 60m;
    $wpm_geo_country country iso_code;
}
geo $wpm_geo_provider {
    default "";
    127.0.0.0/8 self;
    ::1/128 self;
    203.0.113.10/32 self;
    2001:db8::1/128 self;
    173.245.48.0/20 cloudflare;
    8.8.4.0/24 cloudflare;
    2001:4860::/32 google;
    31.13.24.0/21 meta;
}
map $request_uri $wpm_geo_exempt {
    default 0;
    "~^/\.well-known/acme-challenge/" 1;
}
`
	if got != want {
		t.Errorf("base conf:\n%s\nwant:\n%s", got, want)
	}
	if n := strings.Count(BuildGeoBaseConfig(nil, nil), " self;"); n != 2 {
		t.Errorf("no data: want only the 2 loopback entries, got %d", n)
	}
}

func TestBuildGeoSiteConfigWhitelist(t *testing.T) {
	httpConf, snippet, err := BuildGeoSiteConfig(42, "example.com", models.GeoRuleSet{
		Mode:      "whitelist",
		Countries: []string{"VN", "US"},
		Providers: map[string]string{"meta": "deny", "google": "allow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantHTTP := `# Managed by WP Manage — country access for site 42 (example.com). Do not edit.
map $wpm_geo_country $wpm_geo_c_42 {
    default 1;
    "" 0;
    US 0;
    VN 0;
}
map $wpm_geo_provider $wpm_geo_p_42 {
    default "";
    self allow;
    google allow;
    meta deny;
}
map "$wpm_geo_exempt:$wpm_geo_p_42:$wpm_geo_c_42" $wpm_geo_block_42 {
    default 0;
    "0:deny:0" 1;
    "0:deny:1" 1;
    "0::1" 1;
}
`
	wantSnippet := "# Managed by WP Manage — country access. Do not edit.\nif ($wpm_geo_block_42) { return 403; }\n"
	if httpConf != wantHTTP {
		t.Errorf("http conf:\n%s\nwant:\n%s", httpConf, wantHTTP)
	}
	if snippet != wantSnippet {
		t.Errorf("snippet: %q", snippet)
	}
}

func TestBuildGeoSiteConfigModes(t *testing.T) {
	bl, _, err := BuildGeoSiteConfig(7, "example.com", models.GeoRuleSet{Mode: "blacklist", Countries: []string{"RU", "CN"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bl, "    default 0;\n    CN 1;\n    RU 1;\n}") {
		t.Errorf("blacklist map wrong:\n%s", bl)
	}
	if strings.Contains(bl, `"" 0;`) {
		t.Error("blacklist needs no unknown-country entry")
	}

	// mode off + a provider deny still blocks that provider
	off, snippet, err := BuildGeoSiteConfig(7, "example.com", models.GeoRuleSet{Mode: "off", Providers: map[string]string{"meta": "deny"}})
	if err != nil || off == "" || snippet == "" {
		t.Fatalf("provider deny must be enforced: %q %v", off, err)
	}
	if !strings.Contains(off, "$wpm_geo_c_7 {\n    default 0;\n}") {
		t.Errorf("off mode must never block by country:\n%s", off)
	}

	for _, r := range []models.GeoRuleSet{
		{Mode: "off"},
		{Mode: "off", Providers: map[string]string{"google": "allow"}},
	} {
		h, s, err := BuildGeoSiteConfig(7, "example.com", r)
		if err != nil || h != "" || s != "" {
			t.Errorf("nothing enforced: want empty config, got %q %q %v", h, s, err)
		}
	}
}

func TestBuildGeoSiteConfigRejectsBadInput(t *testing.T) {
	ok := models.GeoRuleSet{Mode: "blacklist", Countries: []string{"VN"}}
	cases := []struct {
		id     uint64
		domain string
		rule   models.GeoRuleSet
	}{
		{0, "example.com", ok},
		{1, "bad_domain", ok},
		{1, "../etc", ok},
		{1, "example.com", models.GeoRuleSet{Mode: "block", Countries: []string{"VN"}}},
		{1, "example.com", models.GeoRuleSet{Mode: "blacklist", Countries: []string{"vn"}}},
		{1, "example.com", models.GeoRuleSet{Mode: "blacklist", Countries: []string{"VN 1; } #"}}},
		{1, "example.com", models.GeoRuleSet{Mode: "whitelist"}},
		{1, "example.com", models.GeoRuleSet{Mode: "off", Providers: map[string]string{"aws": "deny"}}},
		{1, "example.com", models.GeoRuleSet{Mode: "off", Providers: map[string]string{"google": "deny; }"}}},
	}
	for i, c := range cases {
		if _, _, err := BuildGeoSiteConfig(c.id, c.domain, c.rule); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestGeoIncludeIsIndependentOfBotInclude(t *testing.T) {
	v := botInclude.set(twoBlockVhost, 42, true)
	v = geoInclude.set(v, 42, true)
	if !strings.Contains(v, "wpm-bots/site-42[.]conf;") || !strings.Contains(v, "wpm-geo/site-42[.]conf;") {
		t.Fatalf("both includes must coexist:\n%s", v)
	}
	if strings.Index(v, "wpm-geo/") > strings.Index(v, "listen 80;") {
		t.Error("geo include must be in the first server block")
	}
	v = geoInclude.set(v, 42, false)
	if strings.Contains(v, "wpm-geo/") || !strings.Contains(v, "wpm-bots/site-42[.]conf;") {
		t.Errorf("removing geo must leave the bot include:\n%s", v)
	}
	if botInclude.set(v, 42, false) != twoBlockVhost {
		t.Error("removing both must restore the original vhost")
	}
	if buildRealIPConfig([]string{"173.245.48.0/20", "nope"}) != "# Cloudflare Real IP — auto-generated by wp-manage, do not edit\nset_real_ip_from 173.245.48.0/20;\nreal_ip_header CF-Connecting-IP;\n" {
		t.Error("real ip config wrong")
	}
	if buildRealIPConfig(nil) != "" {
		t.Error("no cloudflare ranges: no real-ip file")
	}
}
