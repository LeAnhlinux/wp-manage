package grpc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPHPFPMLogFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for name, age := range map[string]time.Duration{
		"php8.3-fpm.log": 3 * time.Hour,
		"php8.5-fpm.log": time.Minute,
		"php8.4-fpm.log": time.Hour,
		"syslog":         0,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, now.Add(-age), now.Add(-age))
	}

	got := phpFPMLogFiles(dir)
	want := []string{"php8.3-fpm.log", "php8.4-fpm.log", "php8.5-fpm.log"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if filepath.Base(got[i]) != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
