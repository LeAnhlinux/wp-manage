package workers

import (
	"log/slog"
	"time"

	"github.com/gofiber-team/wp-manage/internal/api/services"
)

// GeoProviderWorker keeps the provider IP ranges used by country access
// (Google, Cloudflare, Meta, ...) fresh: once at startup if nothing is cached,
// then every Monday at 03:00, after which servers that use the feature are
// told to pull the new ranges and refresh their country database.
type GeoProviderWorker struct {
	svc *services.GeoService
}

func NewGeoProviderWorker(svc *services.GeoService) *GeoProviderWorker {
	return &GeoProviderWorker{svc: svc}
}

func (w *GeoProviderWorker) Start() {
	go w.loop()
}

func (w *GeoProviderWorker) loop() {
	w.svc.EnsureProviders()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		if now.Weekday() == time.Monday && now.Hour() == 3 {
			slog.Info("GeoProviderWorker: weekly refresh")
			w.svc.RefreshProviders()
			w.svc.QueueRefreshForActiveServers()
		}
	}
}
