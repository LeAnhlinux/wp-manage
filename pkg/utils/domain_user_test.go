package utils

import (
	"regexp"
	"testing"
)

func TestSiteLinuxUser(t *testing.T) {
	// Short names keep the historical form so existing sites are unaffected.
	if got := SiteLinuxUser("my-site.example.com"); got != "wp_my_site_example_com" {
		t.Fatalf("short name changed: %q", got)
	}

	valid := regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	long := "staging-shop.some-long-company-name.example.com"
	got := SiteLinuxUser(long)
	if len(got) > 32 {
		t.Fatalf("name %q is %d chars, useradd allows 32", got, len(got))
	}
	if !valid.MatchString(got) {
		t.Fatalf("name %q is not a valid Linux user name", got)
	}
	if got != SiteLinuxUser(long) {
		t.Fatal("name is not stable")
	}
	// Two long domains sharing the same prefix must not collide.
	if other := SiteLinuxUser("staging-shop.some-long-company-name.example.org"); other == got {
		t.Fatalf("collision: %q", got)
	}
	// Exactly 32 chars is still allowed untouched.
	b := "abcdefghijklmnopqrstuvw.co.uk" // 29 chars → 32 with prefix
	if u := SiteLinuxUser(b); u != "wp_abcdefghijklmnopqrstuvw_co_uk" {
		t.Fatalf("32-char boundary changed: %q", u)
	}
}
