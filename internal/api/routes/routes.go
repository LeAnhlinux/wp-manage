package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/handlers"
	"github.com/gofiber-team/wp-manage/internal/api/middleware"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/config"
)

func Setup(
	app *fiber.App,
	cfg *config.Config,
	authHandler *handlers.AuthHandler,
	serverHandler *handlers.ServerHandler,
	siteHandler *handlers.SiteHandler,
	deployHandler *handlers.DeployHandler,
	fileHandler *handlers.FileHandler,
	userHandler *handlers.UserHandler,
	apiKeyHandler *handlers.APIKeyHandler,
	backupHandler *handlers.BackupHandler,
	migrationHandler *handlers.MigrationHandler,
	securityHandler *handlers.SecurityHandler,
	wordpressHandler *handlers.WordPressHandler,
	rolloutHandler *handlers.RolloutHandler,
	performanceHandler *handlers.PerformanceHandler,
	multisiteHandler *handlers.MultisiteHandler,
	imageOptHandler *handlers.ImageOptHandler,
	teamHandler *handlers.TeamHandler,
	infraHandler *handlers.InfrastructureHandler,
	activityHandler *handlers.ActivityHandler,
	cronHandler *handlers.CronHandler,
	domainHandler *handlers.DomainHandler,
	redisHandler *handlers.RedisHandler,
	sshHandler *handlers.SSHHandler,
	sftpHandler *handlers.SFTPHandler,
	hardeningHandler *handlers.HardeningHandler,
	uptimeHandler *handlers.UptimeHandler,
	alertHandler *handlers.AlertHandler,
	wpcliHandler *handlers.WPCLIHandler,
	cacheHandler *handlers.CacheHandler,
	importHandler *handlers.ImportHandler,
	dnsHandler *handlers.DNSHandler,
	sslHandler *handlers.SSLHandler,
	storageHandler *handlers.StorageHandler,
	fail2banHandler *handlers.Fail2BanHandler,
	panelUpdateHandler *handlers.PanelUpdateHandler,
	updateCheckHandler *handlers.UpdateCheckHandler,
	userSvc *services.UserService,
	apiKeySvc *services.APIKeyService,
	rateLimiter *middleware.RateLimiter,
) {
	api := app.Group("/api/v1")

	// Public routes
	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/2fa/validate", authHandler.Validate2FA)

	// Agent routes (authenticated by agent API key)
	agent := api.Group("/agent", middleware.AgentAuthRequired())
	agent.Post("/heartbeat", serverHandler.Heartbeat)
	agent.Post("/system-info", serverHandler.SystemInfo)
	agent.Get("/tasks", serverHandler.Tasks)
	agent.Post("/tasks/complete", serverHandler.TaskComplete)
	agent.Post("/backups/complete", serverHandler.BackupComplete)
	agent.Post("/migrations/update", migrationHandler.Update)
	agent.Post("/migrations/:id/setup-ssh", migrationHandler.SetupSSH)
	agent.Post("/migrations/:id/cleanup-ssh", migrationHandler.CleanupSSH)
	agent.Post("/scans/complete", securityHandler.ScanComplete)
	agent.Post("/staging/complete", wordpressHandler.StagingComplete)
	agent.Post("/updates/complete", wordpressHandler.UpdateCheckComplete)
	agent.Post("/db-optimize/complete", wordpressHandler.DBOptimizeComplete)
	agent.Post("/rollouts/complete", rolloutHandler.UpdateComplete)
	agent.Post("/images/optimize/complete", imageOptHandler.OptimizeComplete)
	agent.Post("/multisite/enable/complete", multisiteHandler.EnableComplete)
	agent.Post("/multisite/subsite/complete", multisiteHandler.CreateSubsiteComplete)
	agent.Post("/multisite/sync/complete", multisiteHandler.SyncSubsitesComplete)
	agent.Post("/wpcli/complete", wpcliHandler.Complete)
	agent.Post("/import/complete", importHandler.Complete)
	agent.Get("/files/tmp/:token", siteHandler.GetTempFile)

	// Protected routes (JWT or API Key auth)
	protected := api.Group("", middleware.APIKeyOrAuthRequired(cfg, apiKeySvc), rateLimiter.Handler())
	protected.Get("/auth/me", authHandler.Me)
	protected.Post("/auth/2fa/enable", authHandler.Enable2FA)
	protected.Post("/auth/2fa/verify", authHandler.Verify2FA)
	protected.Post("/auth/2fa/disable", authHandler.Disable2FA)

	// User management routes (admin only)
	users := protected.Group("/users", middleware.PermissionRequired("users", "manage"))
	users.Post("/", userHandler.Create)
	users.Get("/", userHandler.List)
	users.Get("/:id", userHandler.Get)
	users.Put("/:id", userHandler.Update)
	users.Delete("/:id", userHandler.Delete)
	users.Post("/:id/servers", userHandler.AssignServer)
	users.Delete("/:id/servers/:serverId", userHandler.UnassignServer)
	users.Get("/:id/servers", userHandler.GetAssignedServers)

	// API Key routes (any authenticated user can manage their own keys)
	apiKeys := protected.Group("/api-keys")
	apiKeys.Post("/", apiKeyHandler.Create)
	apiKeys.Get("/", apiKeyHandler.List)
	apiKeys.Delete("/:id", apiKeyHandler.Delete)

	// Server routes (with RBAC)
	serverAccess := middleware.ServerAccessRequired(userSvc)
	servers := protected.Group("/servers")
	servers.Post("/", middleware.PermissionRequired("servers", "create"), serverHandler.Create)
	servers.Get("/", serverHandler.List) // filtered in service layer by role
	servers.Get("/:id", serverAccess, serverHandler.Get)
	servers.Delete("/:id", middleware.PermissionRequired("servers", "delete"), serverHandler.Delete)
	servers.Post("/deploy", middleware.PermissionRequired("servers", "deploy"), deployHandler.AutoDeploy)
	// DNS check for site creation (no siteId exists yet — uses serverID)
	servers.Get("/:id/check-dns", serverAccess, domainHandler.CheckDNSByServer)

	// Site routes (with RBAC)
	sites := protected.Group("/sites")
	sites.Post("/", middleware.PermissionRequired("sites", "create"), siteHandler.Create)
	sites.Get("/", siteHandler.List) // filtered in service layer by role
	sites.Get("/:id", siteHandler.Get)
	sites.Delete("/:id", middleware.PermissionRequired("sites", "delete"), siteHandler.Delete)
	sites.Put("/:id/php", middleware.PermissionRequired("sites", "manage"), siteHandler.UpdatePHP)
	sites.Post("/:id/restart", middleware.PermissionRequired("sites", "manage"), siteHandler.Restart)
	sites.Post("/:id/suspend", middleware.PermissionRequired("sites", "manage"), siteHandler.Suspend)
	sites.Post("/:id/resume", middleware.PermissionRequired("sites", "manage"), siteHandler.Resume)
	sites.Post("/:id/ssl", middleware.PermissionRequired("sites", "manage"), siteHandler.IssueSSL)
	sites.Get("/:id/credentials", siteHandler.Credentials)
	sites.Post("/:id/autologin", siteHandler.Autologin)
	sites.Post("/:id/autologin/reset", middleware.PermissionRequired("sites", "manage"), siteHandler.ResetAutologin)
	sites.Post("/:siteId/open-basedir/toggle", middleware.PermissionRequired("sites", "manage"), siteHandler.ToggleOpenBasedir)
	sites.Post("/:id/upload-source", middleware.PermissionRequired("sites", "manage"), siteHandler.UploadSource)
	sites.Post("/:id/import-db", middleware.PermissionRequired("sites", "manage"), siteHandler.ImportDB)

	// Backup routes (with RBAC)
	backups := protected.Group("/backups")
	backups.Get("/", backupHandler.ListAll) // all backups (filtered by role)
	backups.Get("/:id", backupHandler.Get)
	backups.Delete("/:id", middleware.PermissionRequired("backups", "manage"), backupHandler.Delete)
	// Restore by backup ID — works for both normal and orphaned (site deleted) backups
	backups.Post("/:id/restore", middleware.PermissionRequired("backups", "restore"), backupHandler.RestoreByID)

	// Server-level backup archive (includes orphaned backups from deleted sites)
	servers.Get("/:id/backups", backupHandler.ListByServer)

	siteBackups := sites.Group("/:siteId/backups")
	siteBackups.Post("/", middleware.PermissionRequired("backups", "create"), backupHandler.Create)
	siteBackups.Get("/", backupHandler.ListBySite)
	siteBackups.Post("/restore", middleware.PermissionRequired("backups", "restore"), backupHandler.Restore)

	siteSchedules := sites.Group("/:siteId/backup-schedules")
	siteSchedules.Post("/", middleware.PermissionRequired("backups", "create"), backupHandler.CreateSchedule)
	siteSchedules.Get("/", backupHandler.ListSchedules)

	schedules := protected.Group("/backup-schedules")
	schedules.Put("/:id", middleware.PermissionRequired("backups", "manage"), backupHandler.UpdateSchedule)
	schedules.Delete("/:id", middleware.PermissionRequired("backups", "manage"), backupHandler.DeleteSchedule)

	// Migration routes (with RBAC)
	migrations := protected.Group("/migrations")
	migrations.Get("/", migrationHandler.List) // filtered by role
	migrations.Get("/:id", migrationHandler.Get)
	migrations.Post("/:id/rollback", middleware.PermissionRequired("sites", "manage"), migrationHandler.Rollback)
	migrations.Post("/:id/cancel", middleware.PermissionRequired("sites", "manage"), migrationHandler.Cancel)
	migrations.Post("/:id/cleanup-source", middleware.PermissionRequired("sites", "manage"), migrationHandler.CleanupSource)
	migrations.Get("/:id/logs", migrationHandler.Logs)

	siteMigrations := sites.Group("/:siteId/migrate")
	siteMigrations.Post("/", middleware.PermissionRequired("sites", "manage"), migrationHandler.Start)

	// Security routes (WAF + Malware)
	siteWAF := sites.Group("/:siteId/waf")
	siteWAF.Get("/", securityHandler.GetWAFConfig)
	siteWAF.Put("/", middleware.PermissionRequired("sites", "manage"), securityHandler.UpdateWAFConfig)

	scans := protected.Group("/scans")
	scans.Get("/", securityHandler.ListAllScans) // all scans filtered by role
	scans.Get("/:id", securityHandler.GetScan)

	siteScans := sites.Group("/:siteId/scans")
	siteScans.Post("/", middleware.PermissionRequired("sites", "manage"), securityHandler.StartScan)
	siteScans.Get("/", securityHandler.ListSiteScans)
	// Scan schedules
	siteScans.Get("/schedule", securityHandler.ListSchedules)
	siteScans.Post("/schedule", middleware.PermissionRequired("sites", "manage"), securityHandler.CreateSchedule)
	siteScans.Put("/schedule/:schedId", middleware.PermissionRequired("sites", "manage"), securityHandler.UpdateSchedule)
	siteScans.Delete("/schedule/:schedId", middleware.PermissionRequired("sites", "manage"), securityHandler.DeleteSchedule)

	// ClamAV — server-scoped (ClamAV is installed on the server, not per site)
	serverClamAV := servers.Group("/:id/clamav")
	serverClamAV.Get("/status", securityHandler.GetClamAVStatus)
	serverClamAV.Post("/install", middleware.PermissionRequired("servers", "manage"), securityHandler.InstallClamAV)

	// WordPress Advanced routes (staging, auto-update, DB optimize)
	siteStaging := sites.Group("/:siteId/staging")
	siteStaging.Post("/", middleware.PermissionRequired("sites", "manage"), wordpressHandler.CreateStaging)
	siteStaging.Get("/", wordpressHandler.GetStaging)

	staging := protected.Group("/staging")
	staging.Post("/:stagingId/push", middleware.PermissionRequired("sites", "manage"), wordpressHandler.PushStaging)
	staging.Delete("/:stagingId", middleware.PermissionRequired("sites", "manage"), wordpressHandler.DeleteStaging)

	siteAutoUpdate := sites.Group("/:siteId/auto-update")
	siteAutoUpdate.Get("/", wordpressHandler.GetAutoUpdateConfig)
	siteAutoUpdate.Put("/", middleware.PermissionRequired("sites", "manage"), wordpressHandler.UpdateAutoUpdateConfig)

	siteUpdates := sites.Group("/:siteId/updates")
	siteUpdates.Post("/check", middleware.PermissionRequired("sites", "manage"), wordpressHandler.CheckUpdates)
	siteUpdates.Get("/logs", wordpressHandler.GetUpdateLogs)

	siteDB := sites.Group("/:siteId/db")
	siteDB.Post("/optimize", middleware.PermissionRequired("sites", "manage"), wordpressHandler.OptimizeDatabase)

	// Rollout routes (admin only)
	releases := protected.Group("/releases", middleware.PermissionRequired("settings", "manage"))
	releases.Post("/", rolloutHandler.CreateRelease)
	releases.Get("/", rolloutHandler.ListReleases)
	releases.Get("/:id", rolloutHandler.GetRelease)
	releases.Delete("/:id", rolloutHandler.DeleteRelease)

	rollouts := protected.Group("/rollouts", middleware.PermissionRequired("settings", "manage"))
	rollouts.Post("/", rolloutHandler.CreateRollout)
	rollouts.Get("/", rolloutHandler.ListRollouts)
	rollouts.Get("/:id", rolloutHandler.GetRollout)
	rollouts.Post("/:id/start", rolloutHandler.StartRollout)
	rollouts.Post("/:id/pause", rolloutHandler.PauseRollout)

	// Image Optimization routes
	siteImages := sites.Group("/:siteId/image-optimization")
	siteImages.Get("/", imageOptHandler.GetConfig)
	siteImages.Put("/", middleware.PermissionRequired("sites", "manage"), imageOptHandler.UpdateConfig)
	siteImages.Post("/toggle", middleware.PermissionRequired("sites", "manage"), imageOptHandler.Toggle)
	siteImages.Post("/run", middleware.PermissionRequired("sites", "manage"), imageOptHandler.RunOptimization)
	siteImages.Get("/results", imageOptHandler.GetResults)

	// Multisite routes
	siteMultisite := sites.Group("/:siteId/multisite")
	siteMultisite.Post("/enable", middleware.PermissionRequired("sites", "manage"), multisiteHandler.Enable)
	siteMultisite.Get("/subsites", multisiteHandler.ListSubsites)
	siteMultisite.Post("/subsites", middleware.PermissionRequired("sites", "manage"), multisiteHandler.CreateSubsite)
	siteMultisite.Delete("/subsites/:subsiteId", middleware.PermissionRequired("sites", "manage"), multisiteHandler.DeleteSubsite)
	siteMultisite.Post("/sync", middleware.PermissionRequired("sites", "manage"), multisiteHandler.SyncSubsites)

	// Performance routes
	sitePerf := sites.Group("/:siteId/performance")
	sitePerf.Post("/run", middleware.PermissionRequired("sites", "manage"), performanceHandler.RunAudit)
	sitePerf.Get("/", performanceHandler.GetMetrics)
	sitePerf.Get("/latest", performanceHandler.GetLatest)

	// Team routes
	teams := protected.Group("/teams")
	teams.Post("/", middleware.PermissionRequired("teams", "create"), teamHandler.Create)
	teams.Get("/", teamHandler.List) // filtered in handler by role
	teams.Get("/:id", teamHandler.Get) // access checked in handler
	teams.Put("/:id", teamHandler.Update) // team admin check in handler
	teams.Delete("/:id", middleware.PermissionRequired("teams", "delete"), teamHandler.Delete)
	teams.Post("/:id/members", teamHandler.AddMember) // team admin check in handler
	teams.Put("/:id/members/:userId", teamHandler.UpdateMember)
	teams.Delete("/:id/members/:userId", teamHandler.RemoveMember)
	teams.Post("/:id/servers", middleware.PermissionRequired("teams", "manage"), teamHandler.AssignServer)
	teams.Delete("/:id/servers/:serverId", middleware.PermissionRequired("teams", "manage"), teamHandler.UnassignServer)
	teams.Post("/migrate", middleware.PermissionRequired("teams", "manage"), teamHandler.MigrateFromUserServer)

	// Infrastructure routes (Firewall, Nginx, OPcache, Rebuild, HealthCheck)
	serverFirewall := servers.Group("/:id/firewall", serverAccess)
	serverFirewall.Get("/rules", infraHandler.ListFirewallRules)
	serverFirewall.Post("/rules", middleware.PermissionRequired("sites", "manage"), infraHandler.CreateFirewallRule)
	serverFirewall.Delete("/rules/:ruleId", middleware.PermissionRequired("sites", "manage"), infraHandler.DeleteFirewallRule)
	serverFirewall.Get("/status", middleware.PermissionRequired("infrastructure", "read"), infraHandler.GetFirewallStatus)

	serverNginx := servers.Group("/:id/nginx-configs", serverAccess)
	serverNginx.Get("/", infraHandler.ListNginxConfigs)
	serverNginx.Get("/:configId", infraHandler.GetNginxConfig)
	serverNginx.Post("/", middleware.PermissionRequired("sites", "manage"), infraHandler.CreateNginxConfig)
	serverNginx.Put("/:configId", middleware.PermissionRequired("sites", "manage"), infraHandler.UpdateNginxConfig)
	serverNginx.Delete("/:configId", middleware.PermissionRequired("sites", "manage"), infraHandler.DeleteNginxConfig)

	servers.Post("/:id/opcache", serverAccess, middleware.PermissionRequired("sites", "manage"), infraHandler.OPcacheAction)
	servers.Post("/:id/rebuild-config", serverAccess, middleware.PermissionRequired("sites", "manage"), infraHandler.RebuildConfig)
	servers.Post("/:id/health-check", serverAccess, infraHandler.RunHealthCheck)
	servers.Get("/:id/services", serverAccess, infraHandler.CheckServices)
	servers.Post("/:id/services/:service/install", serverAccess, middleware.PermissionRequired("infrastructure", "manage"), infraHandler.InstallService)

	// PHP version management — static routes MUST be before :siteId wildcard
	servers.Get("/:id/phpfpm/installed", serverAccess, middleware.PermissionRequired("infrastructure", "read"), infraHandler.ListInstalledPHPVersions)
	servers.Post("/:id/phpfpm/install", serverAccess, middleware.PermissionRequired("infrastructure", "manage"), infraHandler.InstallPHPVersion)
	// PHP-FPM pool routes (wildcard :siteId — must be after all static segments)
	servers.Get("/:id/phpfpm/:siteId", serverAccess, middleware.PermissionRequired("infrastructure", "read"), infraHandler.GetPHPFPMPool)
	servers.Put("/:id/phpfpm/:siteId", serverAccess, middleware.PermissionRequired("infrastructure", "manage"), infraHandler.UpdatePHPFPMPool)
	servers.Put("/:id/phpfpm/:siteId/version", serverAccess, middleware.PermissionRequired("infrastructure", "manage"), infraHandler.ChangeSitePHPVersion)

	// MariaDB routes
	serverDB := servers.Group("/:id/mariadb", serverAccess)
	serverDB.Get("/users", middleware.PermissionRequired("infrastructure", "read"), infraHandler.ListMariaDBUsers)
	serverDB.Post("/users", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.CreateMariaDBUser)
	serverDB.Delete("/users/:username", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.DeleteMariaDBUser)
	serverDB.Get("/databases", middleware.PermissionRequired("infrastructure", "read"), infraHandler.ListMariaDBDatabases)
	serverDB.Post("/databases", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.CreateMariaDBDatabase)
	serverDB.Delete("/databases/:dbName", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.DeleteMariaDBDatabase)
	serverDB.Post("/databases/:dbName/import", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.ImportMariaDBDatabase)
	serverDB.Get("/config", middleware.PermissionRequired("infrastructure", "read"), infraHandler.GetMariaDBConfig)
	serverDB.Put("/config", middleware.PermissionRequired("infrastructure", "manage"), infraHandler.UpdateMariaDBConfig)

	// Redis routes (server-scoped)
	serverRedis := servers.Group("/:id/redis", serverAccess)
	serverRedis.Get("/stats", middleware.PermissionRequired("infrastructure", "read"), redisHandler.GetServerStats)
	serverRedis.Post("/flush", middleware.PermissionRequired("infrastructure", "manage"), redisHandler.FlushServer)
	serverRedis.Get("/sites", middleware.PermissionRequired("infrastructure", "read"), redisHandler.GetServerSiteStatuses)

	// Cron job routes (server-scoped)
	serverCron := servers.Group("/:id/cron-jobs", serverAccess)
	serverCron.Get("/", middleware.PermissionRequired("infrastructure", "read"), cronHandler.ListByServer)
	serverCron.Post("/", middleware.PermissionRequired("infrastructure", "manage"), cronHandler.Create)
	serverCron.Put("/:jobId", middleware.PermissionRequired("infrastructure", "manage"), cronHandler.Update)
	serverCron.Post("/:jobId/toggle", middleware.PermissionRequired("infrastructure", "manage"), cronHandler.Toggle)
	serverCron.Post("/:jobId/run", middleware.PermissionRequired("infrastructure", "manage"), cronHandler.RunNow)
	serverCron.Get("/:jobId/logs", middleware.PermissionRequired("infrastructure", "read"), cronHandler.GetLogs)
	serverCron.Delete("/:jobId", middleware.PermissionRequired("infrastructure", "manage"), cronHandler.Delete)

	// Domain routes (site-scoped)
	siteDomains := sites.Group("/:siteId/domains")
	siteDomains.Get("/", domainHandler.List)
	siteDomains.Get("/check-dns", domainHandler.CheckDNS)
	siteDomains.Post("/", middleware.PermissionRequired("sites", "manage"), domainHandler.Create)
	siteDomains.Put("/:domainId", middleware.PermissionRequired("sites", "manage"), domainHandler.Update)
	siteDomains.Delete("/:domainId", middleware.PermissionRequired("sites", "manage"), domainHandler.Delete)
	siteDomains.Post("/:domainId/ssl", middleware.PermissionRequired("sites", "manage"), domainHandler.IssueSSL)

	// Redis routes (site-scoped)
	siteRedis := sites.Group("/:siteId/redis")
	siteRedis.Get("/", redisHandler.GetConfig)
	siteRedis.Put("/", middleware.PermissionRequired("sites", "manage"), redisHandler.UpdateConfig)
	siteRedis.Post("/toggle", middleware.PermissionRequired("sites", "manage"), redisHandler.Toggle)
	siteRedis.Get("/stats", redisHandler.GetStats)
	siteRedis.Post("/flush", middleware.PermissionRequired("sites", "manage"), redisHandler.Flush)

	// Cron job routes (site-scoped)
	siteCron := sites.Group("/:siteId/cron-jobs")
	siteCron.Get("/", cronHandler.ListBySite)

	// SFTP routes (site-scoped)
	siteSFTP := sites.Group("/:siteId/sftp")
	siteSFTP.Get("/", sftpHandler.List)
	siteSFTP.Post("/", middleware.PermissionRequired("sites", "manage"), sftpHandler.Create)
	siteSFTP.Delete("/:accountId", middleware.PermissionRequired("sites", "manage"), sftpHandler.Delete)

	// WordPress hardening routes (site-scoped)
	sites.Post("/:siteId/harden", middleware.PermissionRequired("sites", "manage"), hardeningHandler.Harden)
	sites.Post("/:siteId/harden/revert", middleware.PermissionRequired("sites", "manage"), hardeningHandler.RevertHarden)

	// Activity log routes
	activityLogs := protected.Group("/activity-logs")
	activityLogs.Get("/", activityHandler.List)
	activityLogs.Get("/actions", activityHandler.GetActions)
	activityLogs.Delete("/cleanup", middleware.PermissionRequired("settings", "manage"), activityHandler.Cleanup)

	// Global DNS providers (Option B — not server-scoped)
	protected.Get("/dns-providers", middleware.PermissionRequired("infrastructure", "read"), dnsHandler.ListAll)
	protected.Post("/dns-providers", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.CreateGlobal)
	protected.Delete("/dns-providers/:id", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.DeleteGlobal)
	protected.Post("/dns-providers/test", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.TestProvider)
	protected.Post("/dns-providers/lookup-zone", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.LookupZone)
	protected.Post("/dns-providers/auto-create-record", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.AutoCreateRecord)
	globalDNS := protected.Group("/dns-providers/:id", middleware.PermissionRequired("infrastructure", "read"))
	globalDNS.Get("/records", dnsHandler.ListRecordsGlobal)
	globalDNS.Post("/records", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.CreateRecordGlobal)
	globalDNS.Put("/records/:recordId", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.UpdateRecordGlobal)
	globalDNS.Delete("/records/:recordId", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.DeleteRecordGlobal)

	// Global monitoring routes (all sites)
	monitoringGroup := protected.Group("/monitoring")
	monitoringGroup.Get("/uptime", uptimeHandler.ListAll)

	// SSH Keys routes (server-scoped)
	serverSSH := servers.Group("/:id/ssh-keys", serverAccess)
	serverSSH.Get("/", sshHandler.List)
	serverSSH.Post("/", middleware.PermissionRequired("infrastructure", "manage"), sshHandler.Create)
	serverSSH.Delete("/:keyId", middleware.PermissionRequired("infrastructure", "manage"), sshHandler.Delete)

	// Server logs routes (tail system logs on the VPS)
	servers.Get("/:id/logs", serverAccess, infraHandler.GetServerLogs)
	servers.Post("/:id/logs/stream", serverAccess, middleware.PermissionRequired("infrastructure", "read"), serverHandler.StreamLogs)

	// Uptime monitoring routes (site-scoped)
	siteUptime := sites.Group("/:siteId/uptime")
	siteUptime.Get("/", uptimeHandler.List)
	siteUptime.Post("/", middleware.PermissionRequired("sites", "manage"), uptimeHandler.Create)
	siteUptime.Put("/:checkId", middleware.PermissionRequired("sites", "manage"), uptimeHandler.Update)
	siteUptime.Delete("/:checkId", middleware.PermissionRequired("sites", "manage"), uptimeHandler.Delete)
	siteUptime.Get("/:checkId/history", uptimeHandler.GetHistory)

	// Alert rules routes (server-scoped)
	serverAlerts := servers.Group("/:id/alerts", serverAccess)
	serverAlerts.Get("/", alertHandler.ListRules)
	serverAlerts.Post("/", middleware.PermissionRequired("infrastructure", "manage"), alertHandler.CreateRule)
	serverAlerts.Put("/:ruleId", middleware.PermissionRequired("infrastructure", "manage"), alertHandler.UpdateRule)
	serverAlerts.Delete("/:ruleId", middleware.PermissionRequired("infrastructure", "manage"), alertHandler.DeleteRule)
	serverAlerts.Get("/history", alertHandler.GetHistory)

	// PHP-FPM autotune (server-scoped)
	servers.Post("/:id/phpfpm-autotune", serverAccess, middleware.PermissionRequired("infrastructure", "manage"), alertHandler.PHPFPMAutotune)

	// Notification settings
	notifications := protected.Group("/settings/notifications", middleware.PermissionRequired("settings", "manage"))
	notifications.Get("/", alertHandler.GetNotificationConfig)
	notifications.Put("/", alertHandler.UpdateNotificationConfig)

	// WP-CLI routes (site-scoped)
	siteWPCLI := sites.Group("/:siteId/wpcli")
	siteWPCLI.Post("/", middleware.PermissionRequired("sites", "manage"), wpcliHandler.Execute)
	siteWPCLI.Get("/history", wpcliHandler.GetHistory)

	// Nginx Cache routes (site-scoped)
	siteCache := sites.Group("/:siteId/cache")
	siteCache.Get("/", cacheHandler.GetConfig)
	siteCache.Put("/", middleware.PermissionRequired("sites", "manage"), cacheHandler.UpdateConfig)
	siteCache.Post("/toggle", middleware.PermissionRequired("sites", "manage"), cacheHandler.Toggle)
	siteCache.Post("/purge", middleware.PermissionRequired("sites", "manage"), cacheHandler.Purge)

	// Nginx Vhost routes (site-scoped)
	siteNginx := sites.Group("/:siteId/nginx")
	siteNginx.Get("/vhost", infraHandler.ReadVhost)
	siteNginx.Put("/vhost", middleware.PermissionRequired("sites", "manage"), infraHandler.WriteVhost)

	// External Import routes (site-scoped)
	siteImport := sites.Group("/:siteId/import")
	siteImport.Post("/", middleware.PermissionRequired("sites", "manage"), importHandler.Start)
	siteImport.Get("/", importHandler.List)
	siteImport.Get("/:importId", importHandler.GetStatus)

	// DNS Provider routes (server-scoped)
	serverDNS := servers.Group("/:id/dns-providers", serverAccess)
	serverDNS.Get("/", dnsHandler.List)
	serverDNS.Post("/", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.Create)
	serverDNS.Delete("/:providerId", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.Delete)
	serverDNS.Post("/:providerId/update-record", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.UpdateRecord)
	serverDNS.Get("/:providerId/records", middleware.PermissionRequired("infrastructure", "read"), dnsHandler.ListRecords)
	serverDNS.Post("/:providerId/records", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.CreateRecord)
	serverDNS.Delete("/:providerId/records/:recordId", middleware.PermissionRequired("infrastructure", "manage"), dnsHandler.DeleteRecord)

	// SSL Management routes (site-scoped)
	siteSSL := sites.Group("/:siteId/ssl")
	siteSSL.Get("/status", sslHandler.GetStatus)
	siteSSL.Post("/enable", middleware.PermissionRequired("sites", "manage"), sslHandler.Enable)
	siteSSL.Post("/disable", middleware.PermissionRequired("sites", "manage"), sslHandler.Disable)
	siteSSL.Post("/renew", middleware.PermissionRequired("sites", "manage"), sslHandler.Renew)
	siteSSL.Post("/custom", middleware.PermissionRequired("sites", "manage"), sslHandler.UploadCustom)
	siteSSL.Post("/force-https", middleware.PermissionRequired("sites", "manage"), sslHandler.ForceHTTPS)

	// Storage Credentials routes (server-scoped)
	serverStorage := servers.Group("/:id/storage", serverAccess)
	serverStorage.Get("/", storageHandler.List)
	serverStorage.Post("/", middleware.PermissionRequired("infrastructure", "manage"), storageHandler.Create)
	serverStorage.Delete("/:credId", middleware.PermissionRequired("infrastructure", "manage"), storageHandler.Delete)

	// Fail2Ban routes (server-scoped)
	serverFail2Ban := servers.Group("/:id/fail2ban", serverAccess)
	serverFail2Ban.Get("/", fail2banHandler.GetStatus)
	serverFail2Ban.Post("/unban", middleware.PermissionRequired("infrastructure", "manage"), fail2banHandler.UnbanIP)
	serverFail2Ban.Put("/jail", middleware.PermissionRequired("infrastructure", "manage"), fail2banHandler.UpdateJail)

	// Panel Updates (admin only)
	settings := protected.Group("/settings/panel-update", middleware.PermissionRequired("settings", "manage"))
	settings.Get("/", panelUpdateHandler.GetStatus)
	settings.Get("/last-result", panelUpdateHandler.GetLastUpdateResult)
	settings.Post("/releases", panelUpdateHandler.CreateRelease)
	settings.Get("/releases", panelUpdateHandler.ListReleases)
	settings.Post("/releases/:id/apply", panelUpdateHandler.TriggerUpdate) // starts wp-manage-updater, API will restart

	// Simple auto-update endpoints (check mirror + one-click apply)
	updateSettings := protected.Group("/settings/update", middleware.PermissionRequired("settings", "manage"))
	updateSettings.Get("/check", updateCheckHandler.CheckUpdate)
	updateSettings.Post("/apply-panel", updateCheckHandler.ApplyPanelUpdate)
	updateSettings.Post("/apply-agents", updateCheckHandler.ApplyAgentUpdate)

	// File Manager routes (with RBAC)
	files := sites.Group("/:id/files")
	files.Get("/", middleware.PermissionRequired("files", "read"), fileHandler.List)
	files.Get("/content", middleware.PermissionRequired("files", "read"), fileHandler.GetContent)
	files.Put("/content", middleware.PermissionRequired("files", "write"), fileHandler.SaveContent)
	files.Post("/upload", middleware.PermissionRequired("files", "write"), fileHandler.Upload)
	files.Post("/upload-chunk", middleware.PermissionRequired("files", "write"), fileHandler.UploadChunk)
	files.Post("/upload-merge", middleware.PermissionRequired("files", "write"), fileHandler.UploadMerge)
	files.Post("/extract", middleware.PermissionRequired("files", "write"), fileHandler.Extract)
	files.Get("/download", middleware.PermissionRequired("files", "read"), fileHandler.Download)
	files.Post("/mkdir", middleware.PermissionRequired("files", "write"), fileHandler.Mkdir)
	files.Put("/rename", middleware.PermissionRequired("files", "write"), fileHandler.Rename)
	files.Post("/delete", middleware.PermissionRequired("files", "write"), fileHandler.Delete)
	files.Put("/permissions", middleware.PermissionRequired("files", "write"), fileHandler.Permissions)
	files.Post("/search", middleware.PermissionRequired("files", "read"), fileHandler.Search)
	files.Post("/copy", middleware.PermissionRequired("files", "write"), fileHandler.Copy)
	files.Post("/move", middleware.PermissionRequired("files", "write"), fileHandler.Move)
	files.Post("/delete-bulk", middleware.PermissionRequired("files", "write"), fileHandler.DeleteBulk)
}
