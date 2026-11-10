package phpfpm

import (
	"reflect"
	"testing"
)

func TestUpstreamVersion(t *testing.T) {
	cases := map[string]string{
		"8.3.36-1+ubuntu24.04.1+deb.sury.org+1":    "8.3.36",
		"1:7.4.33-31+ubuntu24.04.1+deb.sury.org+1": "7.4.33",
		"8.6.0~rc3-1+ubuntu24.04.1+deb.sury.org+1": "8.6.0~rc3",
		"8.3.6-0ubuntu0.24.04.5":                   "8.3.6",
		"8.2.35":                                   "8.2.35",
		"":                                         "",
	}
	for in, want := range cases {
		if got := upstreamVersion(in); got != want {
			t.Errorf("upstreamVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsPreRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"8.6.0~rc3": true, "8.6.0~alpha1": true, "8.6.0~beta2": true, "8.6.0RC1": true,
		"8.6.0": false, "8.3.36": false, "": false,
	} {
		if got := isPreRelease(v); got != want {
			t.Errorf("isPreRelease(%q) = %v", v, got)
		}
	}
}

func TestParseAptPolicy(t *testing.T) {
	out := `php8.3-fpm:
  Installed: 8.3.35-1+ubuntu24.04.1+deb.sury.org+1
  Candidate: 8.3.36-1+ubuntu24.04.1+deb.sury.org+1
  Version table:
     8.3.36-1+ubuntu24.04.1+deb.sury.org+1 500
`
	inst, cand := parseAptPolicy(out)
	if inst != "8.3.35-1+ubuntu24.04.1+deb.sury.org+1" || cand != "8.3.36-1+ubuntu24.04.1+deb.sury.org+1" {
		t.Errorf("got %q %q", inst, cand)
	}
	inst, cand = parseAptPolicy("php8.6-fpm:\n  Installed: (none)\n  Candidate: 8.6.0~rc3-1+x\n")
	if inst != "" || cand != "8.6.0~rc3-1+x" {
		t.Errorf("not installed: %q %q", inst, cand)
	}
	if inst, cand = parseAptPolicy(""); inst != "" || cand != "" {
		t.Error("unknown package must give empty values")
	}
}

func TestBuildStatus(t *testing.T) {
	// patch update
	s := buildStatus("8.3", "8.3.35-1+x", "8.3.36-1+x")
	if !s.Installed || s.InstalledVersion != "8.3.35" || s.CandidateVersion != "8.3.36" || !s.UpdateAvailable || s.PackageRebuild || s.PreRelease {
		t.Errorf("patch: %+v", s)
	}
	// same PHP version, new package revision (security rebuild)
	s = buildStatus("7.4", "1:7.4.33-30+x", "1:7.4.33-31+x")
	if !s.UpdateAvailable || !s.PackageRebuild {
		t.Errorf("rebuild: %+v", s)
	}
	// up to date
	if s = buildStatus("8.2", "8.2.35-1+x", "8.2.35-1+x"); s.UpdateAvailable {
		t.Errorf("up to date: %+v", s)
	}
	// not installed, release candidate in the repo
	s = buildStatus("8.6", "", "8.6.0~rc3-1+x")
	if s.Installed || s.UpdateAvailable || !s.PreRelease || s.CandidateVersion != "8.6.0~rc3" {
		t.Errorf("rc: %+v", s)
	}
	// not installed and not in the repo
	if s = buildStatus("8.0", "", ""); s.Installed || s.CandidateVersion != "" || s.UpdateAvailable {
		t.Errorf("absent: %+v", s)
	}
}

func TestParseInstalledPackages(t *testing.T) {
	out := `php8.3 un 
php8.3-bcmath ii 
php8.3-calendar un 
php8.3-cli ii 
php8.3-fpm ii 
php8.3-opcache rc 
php8.30-evil ii 
php8.3-redis ii 
`
	got := parseInstalledPackages(out, "8.3")
	want := []string{"php8.3-bcmath", "php8.3-cli", "php8.3-fpm", "php8.3-redis"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestParseDedicatedFPMEnv(t *testing.T) {
	if !envUsesPHP("FPM_BIN=/usr/sbin/php-fpm8.3\nFPM_CONF=/etc/wpm-fpm/a.com.conf\n", "8.3") {
		t.Error("must match 8.3")
	}
	if envUsesPHP("FPM_BIN=/usr/sbin/php-fpm8.3\n", "8.4") {
		t.Error("must not match another branch")
	}
	if envUsesPHP("FPM_CONF=/x/php-fpm8.3.conf\n", "8.3") {
		t.Error("only FPM_BIN counts")
	}
}
