package main

import (
	"flag"
	"log"
	"log/slog"
	"os"
	"time"

	agentgrpc "github.com/gofiber-team/wp-manage/internal/agent/grpc"
	pkglogger "github.com/gofiber-team/wp-manage/pkg/logger"
)

func main() {
	apiURL := flag.String("api-url", os.Getenv("WPM_API_URL"), "Central API URL")
	apiKey := flag.String("api-key", os.Getenv("WPM_API_KEY"), "Agent API key")
	provision := flag.Bool("provision", false, "Run initial server provisioning")
	flag.Parse()

	if *apiURL == "" || *apiKey == "" {
		log.Fatal("--api-url and --api-key are required")
	}

	pkglogger.Init("production")
	slog.Info("Starting WP Manage Agent", "version", agentgrpc.Version)

	agent := agentgrpc.NewAgentServer(*apiURL, *apiKey)

	// Initial provisioning if requested
	if *provision {
		slog.Info("Starting server provisioning...")
		if err := agent.Provision(); err != nil {
			log.Fatalf("Provisioning failed: %v", err)
		}
		slog.Info("Provisioning completed successfully")
	}

	// Register system info
	if err := agent.RegisterSystemInfo(); err != nil {
		slog.Error("Failed to register system info", "error", err)
	}

	// Ensure required firewall ports are open, then re-check every 5 minutes.
	// Self-heals if UFW rules are reset or the firewall is reconfigured.
	agentgrpc.StartFirewallWatcher([]int{
		8443, // agent HTTPS server — file manager, health check
		80,   // HTTP — WordPress sites
		443,  // HTTPS — WordPress sites + Let's Encrypt
		22,   // SSH — management access (safety net)
	}, 5*time.Minute)

	// Start HTTP server for file manager and direct operations (port 8443)
	go agent.StartHTTPServer(8443)

	// Start heartbeat loop
	slog.Info("Starting heartbeat loop", "interval", "30s")
	agent.StartHeartbeat(30 * time.Second)
}
