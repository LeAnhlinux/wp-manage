// wp-manage-updater is a tiny one-shot service that safely swaps the
// wp-manage-agent binary.
//
// It is started by the agent when a new version has been downloaded and staged.
// It runs as root (same as the agent) and is managed by its own systemd unit
// (wp-manage-updater.service, Type=oneshot).
//
// Config is read from the same EnvironmentFile as the agent:
//
//	/etc/wp-agent/agent.env   (WPM_API_URL, WPM_API_KEY)
//
// The updater binary itself is deployed once via update.sh and almost never
// needs to change.  When it does, update.sh handles it via SSH/SCP — the
// updater never updates itself.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/gofiber-team/wp-manage/internal/agent/updater"
	pkglogger "github.com/gofiber-team/wp-manage/pkg/logger"
)

func main() {
	apiURL := flag.String("api-url", os.Getenv("WPM_API_URL"), "Central API URL")
	apiKey := flag.String("api-key", os.Getenv("WPM_API_KEY"), "Agent API key")
	flag.Parse()

	// API URL + key are optional on the API server itself (panel update writes
	// to ResultFilePath instead of HTTP callback). They're required only on agent
	// servers where rollout callbacks are needed.
	if *apiURL == "" {
		slog.Warn("WPM_API_URL not set — rollout HTTP callbacks disabled (result written to file only)")
	}

	pkglogger.Init("production")
	slog.Info("wp-manage-updater started")

	updater.Run(*apiURL, *apiKey)
}
