package main

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/gofiber-team/wp-manage/internal/api/handlers"
	mobileHandlers "github.com/gofiber-team/wp-manage/internal/api/handlers/mobile"
	appMiddleware "github.com/gofiber-team/wp-manage/internal/api/middleware"
	"github.com/gofiber-team/wp-manage/internal/api/routes"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	ws "github.com/gofiber-team/wp-manage/internal/api/websocket"
	"github.com/gofiber-team/wp-manage/internal/api/workers"
	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/database"
	pkglogger "github.com/gofiber-team/wp-manage/pkg/logger"
)

// Version is set at build time via -ldflags="-X main.Version=..."
var Version = "dev"

func main() {
	cfg := config.Load()
	pkglogger.Init(cfg.Server.Mode)

	// Database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}
	database.RunManualMigrations(db)

	// Services
	authService := services.NewAuthService(db, cfg)
	serverService := services.NewServerService(db)
	siteService := services.NewSiteService(db)
	deployService := services.NewDeployService(db, cfg)
	fileService := services.NewFileService(db)
	userService := services.NewUserService(db)
	apiKeyService := services.NewAPIKeyService(db)
	backupService := services.NewBackupService(db)
	securityService := services.NewSecurityService(db, fileService)
	rolloutService := services.NewRolloutService(db)
	performanceService := services.NewPerformanceService(db, cfg)
	multisiteService := services.NewMultisiteService(db)
	imageOptService := services.NewImageOptService(db)
	teamService := services.NewTeamService(db)
	activityService := services.NewActivityService(db)
	cronService := services.NewCronService(db)
	dnsService := services.NewDNSService(db)
	wordpressService := services.NewWordPressService(db, dnsService)
	domainService := services.NewDomainService(db, dnsService)
	redisService := services.NewRedisService(db)
	infraService := services.NewInfrastructureService(db)
	sshService := services.NewSSHService(db)
	sftpService := services.NewSFTPService(db)
	hardeningService := services.NewHardeningService(db)
	uptimeService := services.NewUptimeService(db)
	alertService := services.NewAlertService(db)
	wpcliService := services.NewWPCLIService(db)
	cacheService := services.NewCacheService(db)
	importService := services.NewImportService(db)
	migrationService := services.NewMigrationService(db, dnsService)
	sslService := services.NewSSLService(db)
	storageService := services.NewStorageService(db)
	fail2banService := services.NewFail2BanService(db)
	panelUpdateService := services.NewPanelUpdateService(db, Version)
	ionCubeService := services.NewIonCubeService(db)
	transferService := services.NewTransferService(db)

	// WebSocket Hub
	hub := ws.NewHub()
	go hub.Run()

	// Rate Limiter
	rateLimiter := appMiddleware.NewRateLimiter()

	// Handlers
	authHandler := handlers.NewAuthHandler(authService)
	serverHandler := handlers.NewServerHandler(serverService, siteService, backupService, cfg, hub)
	siteHandler := handlers.NewSiteHandler(siteService, serverService)
	deployHandler := handlers.NewDeployHandler(deployService, hub)
	fileHandler := handlers.NewFileHandler(fileService, userService)
	userHandler := handlers.NewUserHandler(userService)
	apiKeyHandler := handlers.NewAPIKeyHandler(apiKeyService)
	backupHandler := handlers.NewBackupHandler(backupService)
	migrationHandler := handlers.NewMigrationHandler(migrationService)
	securityHandler := handlers.NewSecurityHandler(securityService)
	wordpressHandler := handlers.NewWordPressHandler(wordpressService)
	rolloutHandler := handlers.NewRolloutHandler(rolloutService)
	performanceHandler := handlers.NewPerformanceHandler(performanceService)
	multisiteHandler := handlers.NewMultisiteHandler(multisiteService)
	imageOptHandler := handlers.NewImageOptHandler(imageOptService)
	teamHandler := handlers.NewTeamHandler(teamService)
	activityHandler := handlers.NewActivityHandler(activityService)
	cronHandler := handlers.NewCronHandler(cronService)
	domainHandler := handlers.NewDomainHandler(domainService)
	redisHandler := handlers.NewRedisHandler(redisService)
	infraHandler := handlers.NewInfrastructureHandler(infraService)
	sshHandler := handlers.NewSSHHandler(sshService)
	sftpHandler := handlers.NewSFTPHandler(sftpService)
	hardeningHandler := handlers.NewHardeningHandler(hardeningService)
	uptimeHandler := handlers.NewUptimeHandler(uptimeService)
	alertHandler := handlers.NewAlertHandler(alertService, siteService)
	wpcliHandler := handlers.NewWPCLIHandler(wpcliService, siteService)
	cacheHandler := handlers.NewCacheHandler(cacheService)
	importHandler := handlers.NewImportHandler(importService)
	dnsHandler := handlers.NewDNSHandler(dnsService)
	sslHandler := handlers.NewSSLHandler(sslService)
	storageHandler := handlers.NewStorageHandler(storageService)
	fail2banHandler := handlers.NewFail2BanHandler(fail2banService)
	panelUpdateHandler := handlers.NewPanelUpdateHandler(panelUpdateService)
	updateCheckHandler := handlers.NewUpdateCheckHandler(panelUpdateService, rolloutService, serverService)
	ionCubeHandler := handlers.NewIonCubeHandler(ionCubeService)
	transferHandler := handlers.NewTransferHandler(transferService, hub)

	// ── Mobile API services & handlers ───────────────────────────────────────
	fcmKey := cfg.FCMServerKey     // Set FCM_SERVER_KEY in env
	fcmProject := cfg.FCMProjectID // Set FCM_PROJECT_ID in env
	mobileAuthService := services.NewMobileAuthService(db, cfg)
	pushService := services.NewPushService(db, fcmKey, fcmProject)
	_ = pushService // Used by workers below

	mobileAuthHandler := mobileHandlers.NewAuthHandler(mobileAuthService, db)
	mobileDashboardHandler := mobileHandlers.NewDashboardHandler(
		db, serverService, siteService, uptimeService,
		alertService, activityService, backupService, migrationService, securityService,
	)
	mobileServerHandler := mobileHandlers.NewServerHandler(db, serverService, alertService)
	mobileSiteHandler := mobileHandlers.NewSiteHandler(db, siteService, uptimeService, backupService)
	mobileMonitoringHandler := mobileHandlers.NewMonitoringHandler(db, uptimeService, alertService, activityService, serverService)
	mobileOperationsHandler := mobileHandlers.NewOperationsHandler(db, backupService, securityService, migrationService, siteService, fileService)

	// Fiber app
	app := fiber.New(fiber.Config{
		AppName:                      "WP Manage API v1.0",
		BodyLimit:                    3 * 1024 * 1024 * 1024, // 3GB for file uploads
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
		ErrorHandler:                 errorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New())

	// Determine allowed CORS origins.
	// In production set CORS_ORIGINS to the exact panel domain (e.g. "https://panel.example.com").
	// Wildcard "*" is used only when CORS_ORIGINS is not configured AND we are in development mode.
	corsOrigins := cfg.Panel.CORSOrigins
	if corsOrigins == "" {
		if cfg.Server.Mode == "production" {
			// Production default: restrict to the panel domain
			corsOrigins = "https://" + cfg.Panel.Domain
		} else {
			// Development: allow all origins for convenience
			corsOrigins = "*"
		}
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins,
		AllowMethods: "GET,POST,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Agent-Key,X-API-Key",
	}))

	// Activity logging middleware for mutating requests
	app.Use(appMiddleware.ActivityMiddleware(activityService))

	// WebSocket endpoint
	app.Get("/ws", hub.Handler())

	// API routes
	routes.Setup(app, cfg, authHandler, serverHandler, siteHandler, deployHandler, fileHandler,
		userHandler, apiKeyHandler, backupHandler, migrationHandler, securityHandler, wordpressHandler, rolloutHandler, performanceHandler, multisiteHandler, imageOptHandler, teamHandler, infraHandler, activityHandler, cronHandler, domainHandler, redisHandler, sshHandler, sftpHandler, hardeningHandler, uptimeHandler, alertHandler, wpcliHandler, cacheHandler, importHandler, dnsHandler, sslHandler, storageHandler, fail2banHandler, panelUpdateHandler, ionCubeHandler, updateCheckHandler, transferHandler, userService, apiKeyService, rateLimiter)

	// Mobile API routes — separate prefix /api/mobile/v1/
	routes.SetupMobile(app, cfg,
		mobileAuthHandler, mobileDashboardHandler,
		mobileServerHandler, mobileSiteHandler,
		mobileMonitoringHandler, mobileOperationsHandler,
	)

	// Background workers
	uptimeWorker := workers.NewUptimeWorker(uptimeService, alertService, hub)
	go uptimeWorker.Start()

	alertWorker := workers.NewAlertWorker(db, alertService)
	_ = alertWorker // Used via ServerHandler on heartbeat

	sslExpiryWorker := workers.NewSSLExpiryWorker(db, alertService)
	go sslExpiryWorker.Start()

	backupRotationWorker := workers.NewBackupRotationWorker(db)
	backupRotationWorker.Start()

	backupScheduleWorker := workers.NewBackupScheduleWorker(db, backupService)
	go backupScheduleWorker.Start()

	scanScheduleWorker := workers.NewScanScheduleWorker(db, securityService)
	go scanScheduleWorker.Start()

	activityCleanupWorker := workers.NewActivityCleanupWorker(activityService)
	activityCleanupWorker.Start()

	autoUpdateWorker := workers.NewAutoUpdateWorker(db)
	autoUpdateWorker.Start()

	transferSSLWorker := workers.NewTransferSSLWorker(db)
	transferSSLWorker.Start()

	cloudflareIPWorker := workers.NewCloudflareIPWorker(db, infraService)
	cloudflareIPWorker.Start()

	diskUsageWorker := workers.NewDiskUsageWorker(db, siteService, alertService)
	go diskUsageWorker.Start()

	// Integrate alert worker with server handler
	serverHandler.SetAlertWorker(alertWorker)

	// Self-heal: ensure nginx never caches index.html so browsers always load the
	// newest asset hashes after an update. Idempotent + non-fatal; runs on every boot
	// so panels installed before this guard get fixed on their next restart/update.
	go func() {
		if err := services.EnsurePanelNginxNoCacheIndex(); err != nil {
			slog.Warn("nginx index.html no-cache check failed (non-fatal)", "error", err)
		}
	}()

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "version": Version})
	})

	// Start
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	slog.Info("Starting WP Manage API", "address", addr, "mode", cfg.Server.Mode)
	log.Fatal(app.Listen(addr))
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   err.Error(),
	})
}
