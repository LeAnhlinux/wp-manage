package services

import (
	"os"
	"strings"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// TestFetchProvidersLive hits the real provider sources. It needs network
// access, so it only runs when WPM_LIVE=1. Set WPM_LIVE_OUT to a directory to
// also write each list there.
func TestFetchProvidersLive(t *testing.T) {
	if os.Getenv("WPM_LIVE") != "1" {
		t.Skip("set WPM_LIVE=1 to fetch the real provider IP ranges")
	}
	for _, p := range models.GeoProviders {
		cidrs, err := fetchProviderCIDRs(p.Key)
		if err != nil {
			t.Errorf("%s: %v", p.Key, err)
			continue
		}
		t.Logf("%-10s %5d ranges, e.g. %s", p.Key, len(cidrs), cidrs[0])
		if dir := os.Getenv("WPM_LIVE_OUT"); dir != "" {
			os.WriteFile(dir+"/"+p.Key+".txt", []byte(strings.Join(cidrs, "\n")), 0644)
		}
	}
}
