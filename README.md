# WP Manage

WordPress VPS Management Platform — a hub-and-spoke system for managing WordPress sites across multiple VPS servers.

## Architecture

- **Central API** (`wp-manage-api`): Go (Fiber v2) REST API + React frontend embedded via `go:embed`
- **VPS Agent** (`wp-manage-agent`): Lightweight Go agent running on each managed VPS as a systemd service
- **Communication**: HTTP polling (agent → API every 30s), REST + WebSocket (panel → API)
- **Database**: MariaDB 10.11+ (central metadata)
- **Cache**: Redis 7+

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22+, Fiber v2, GORM |
| Frontend | React 18, TypeScript, Tailwind CSS, shadcn/ui, Zustand, TanStack Query |
| Database | MariaDB 10.11+ |
| Web Server | Nginx 1.24+ |
| PHP | PHP-FPM 7.4–8.3+ |
| SSL | Let's Encrypt (Certbot) |
| Security | UFW, Fail2Ban |

## Completed Features

### Phase 1 — Server Provisioning & WordPress Management
- **Server provisioning**: Connect VPS via IP, auto-install Nginx + PHP-FPM + MariaDB stack
- **Agent auto-deploy**: One-command agent installation on VPS with systemd service
- **WordPress CRUD**: Create/delete WordPress sites with auto-configured Nginx vhosts, PHP-FPM pools, and MariaDB databases
- **SSL management**: Let's Encrypt certificate issuance via Certbot
- **PHP version management**: Switch PHP-FPM version per site (7.4–8.3+)
- **Site lifecycle**: Suspend/resume/restart sites, auto-login to wp-admin via HMAC-signed URLs
- **Server monitoring**: Agent heartbeat with system info (CPU, RAM, disk, OS)

### Phase 2, Batch 1 — File Manager
- **File browser**: Navigate server filesystem via panel UI
- **File operations**: Create/rename/delete files and directories
- **File editor**: Edit files with syntax highlighting (CodeMirror)
- **File upload/download**: Upload files to server, download files from server
- **Permission viewer**: View file ownership and permissions

### Phase 3, Batch 1 — RBAC & API Access

#### Multi-user RBAC
- **Three roles**: Admin (full access), Manager (manage sites on assigned servers), Viewer (read-only on assigned servers)
- **User management**: Admin can create/update/delete users, assign/unassign servers
- **Server-level access control**: Non-admin users only see servers explicitly assigned to them
- **Permission enforcement**: Hardcoded permission matrix — no DB lookup overhead
- **Frontend guards**: Navigation items and action buttons hidden based on role

| Resource | Action | Admin | Manager | Viewer |
|----------|--------|-------|---------|--------|
| Servers | create/delete | Yes | No | No |
| Servers | read | All | Assigned | Assigned |
| Sites | create/delete/manage | Yes | Own servers | No |
| Sites | read | All | Own servers | Own servers |
| Files | write | Yes | Yes | No |
| Files | read | Yes | Yes | Yes |
| Users | manage | Yes | No | No |

#### API Key Access
- **Programmatic access**: Generate API keys with `wpm_` prefix for external integrations
- **SHA256 hashed storage**: Keys stored as SHA256 hashes (appropriate for high-entropy random keys)
- **Dual auth**: `X-API-Key` header or `Authorization: Bearer <JWT>` — API checks both
- **Per-key rate limiting**: Configurable rate limit per API key (default 100 req/min)
- **In-memory sliding window**: Rate limiter with automatic cleanup, X-RateLimit-* response headers
- **Key management UI**: Create/list/delete keys in Settings page, key shown once on creation

### Phase 3, Batch 2 — Backup & Restore
- **Full backup**: Archives site files (tar.gz) + database dump (mysqldump) in a single compressed archive
- **Database-only backup**: Exports database as compressed SQL dump (.sql.gz)
- **Incremental backup**: Captures files modified in last 24 hours + fresh database dump
- **Restore from backup**: Restores database and/or files from any completed backup with domain confirmation
- **Backup schedules**: Cron-based scheduling with configurable retention and storage type
- **Agent-side execution**: Backups and restores run on VPS agent with SHA256 checksum verification
- **RBAC integration**: Admin and Manager can create/restore backups; Viewer can only view backups
- **Backup management UI**: Create backups, view status, restore with confirmation dialog, delete old backups

### Phase 3, Batch 3 — VPS-to-VPS Migration
- **5-phase migration**: Preparation → Initial Sync → Delta Sync → Switchover → Cleanup
- **rsync-based file transfer**: Efficient file sync between source and target servers over SSH
- **Database migration**: mysqldump on source, SCP transfer, mysql import on target
- **Delta sync**: Quick re-sync of changed files to minimize downtime
- **Domain change support**: Optional `wp search-replace` for URL migration during switchover
- **Progress tracking**: Real-time phase/progress updates from agent to API with percentage completion
- **Migration logs**: Timestamped log entries per phase with severity levels (info/error/warn)
- **Rollback support**: Cancel running migrations or rollback completed ones
- **Auto-update site record**: Site's `server_id` automatically updates to target server on completion
- **RBAC integration**: Admin and Manager can start/rollback/cancel migrations; Viewer can only view
- **Migration management UI**: Start migration wizard, real-time progress bar, phase timeline, log viewer

### Phase 3, Batch 4 — Security (WAF + Malware Scan)

#### WAF (Web Application Firewall)
- **ModSecurity integration**: Auto-install ModSecurity + OWASP CRS on Nginx per site
- **Per-site WAF config**: Enable/disable WAF, configurable paranoid level (1-4)
- **IP whitelisting**: Comma-separated whitelist of trusted IPs bypassing WAF
- **Nginx integration**: Automatic modsecurity_rules_file injection in site's nginx vhost
- **Config validation**: `nginx -t` test before reload to prevent downtime

#### Malware Scanner
- **Custom WP signature scan**: 10+ regex patterns detecting eval/base64, shell_exec, webshells, obfuscated code
- **ClamAV integration**: Uses ClamAV if installed for deep virus scanning
- **Scan types**: Full (all PHP/JS/HTML files), Quick (uploads + plugins only)
- **Upload directory check**: Flags PHP files in wp-content/uploads (potential webshells)
- **Threat severity levels**: Critical, high, medium, low — with file path and description
- **Agent-side execution**: Scans run on VPS agent, results reported back to API
- **RBAC integration**: Admin and Manager can start scans; Viewer can only view results
- **Security management UI**: Start scan wizard, scan results table, threat detail viewer with severity badges

### Phase 3, Batch 5 — WordPress Advanced

#### Staging Environments
- **One-click staging**: Clone production site (files + database) to a staging subdomain
- **Push to production**: Overwrite production files from staging (excludes wp-config.php)
- **Staging cleanup**: Delete staging environment, files, database, and nginx config
- **Staging lifecycle**: creating → active → pushing → deleted with status tracking
- **RBAC integration**: Admin and Manager can create/push/delete staging; Viewer can only view

#### WordPress Auto-Update
- **Per-site auto-update config**: Enable/disable auto-updates for core, plugins, and themes independently
- **Backup before update**: Option to create backup before any auto-update runs
- **Update check**: On-demand WP-CLI update check via agent (core, plugins, themes)
- **Update logs**: Timestamped history of all updates with version changes and status
- **RBAC integration**: Admin and Manager can configure auto-updates; Viewer can only view settings

#### Database Optimization
- **Table optimization**: OPTIMIZE TABLE on all InnoDB tables to reclaim fragmented space
- **Revision cleanup**: Remove old post revisions older than 30 days
- **Transient cleanup**: Clear expired WordPress transients from wp_options
- **Optimization history**: Logged results showing tables optimized, space saved, items cleaned
- **Agent-side execution**: Runs on VPS agent with direct MySQL access for efficiency

### Phase 3, Batch 6 — Rollout Strategy (Agent Updates)

#### Agent Releases
- **Release management**: Create, list, and delete agent binary releases with version, download URL, and SHA256 checksum
- **File size tracking**: Store binary file size for download progress estimation
- **Release notes**: Attach markdown release notes for change documentation
- **Safety guards**: Cannot delete a release that is in use by an active rollout

#### Rollout Plans
- **Three strategies**: Canary (10% first, fail-fast on any failure), Rolling (configurable batch %, auto-advance), Immediate (100% at once)
- **Batch scheduling**: Servers automatically divided into numbered batches based on strategy percentage
- **Rollout lifecycle**: Pending → Running → Completed/Failed/Paused with start/pause controls
- **Progress tracking**: Real-time updated_count and failed_count with per-server status
- **Canary fail-fast**: If any server fails during canary rollout, entire rollout is marked as failed
- **Auto-advance batches**: After a batch completes, next batch is automatically queued
- **Concurrency control**: Only one active rollout allowed at a time

#### Agent Self-Update
- **Binary download**: Agent downloads new binary from release URL
- **SHA256 verification**: Checksum validation before replacing the running binary
- **Safe replacement**: Backup existing binary before overwriting
- **Systemd restart**: Agent restarts itself via `systemctl restart` after successful update
- **Result reporting**: Agent reports old_version → new_version transition with success/failure status
- **RBAC integration**: Admin only — releases and rollouts are restricted to admin users

### Phase 4, Batch 1 — Performance Metrics (PageSpeed Insights)
- **Google PageSpeed Insights integration**: Run Lighthouse audits for any managed site directly from the panel
- **Core Web Vitals tracking**: LCP (Largest Contentful Paint), CLS (Cumulative Layout Shift), FID (Max Potential First Input Delay), TTFB (Time to First Byte)
- **Performance & Accessibility scores**: 0-100 scores from Google Lighthouse for both mobile and desktop
- **Device toggle**: Run audits separately for mobile and desktop strategies
- **Measurement history**: Paginated history with 7d/30d/90d period selector and device type filter
- **Score visualization**: Color-coded gauge cards (green ≥90, yellow ≥50, red <50) with latest metrics dashboard
- **API-only architecture**: Runs entirely on API server (no agent needed) — calls Google PageSpeed API with 90s timeout
- **Configurable API key**: Supports `GOOGLE_PAGESPEED_API_KEY` env var for higher quota (25K free queries/day)
- **RBAC integration**: Admin and Manager can run audits; Viewer can only view metrics

### Phase 4, Batch 2 — WordPress Multisite
- **Enable Multisite**: Convert any WordPress site into a multisite network (subdomain or subfolder mode)
- **WP-CLI integration**: Uses `wp core multisite-install` for reliable multisite setup
- **Subdomain mode**: Auto-configures Nginx wildcard server_name for `*.domain.com` subdomains
- **Subfolder mode**: Standard WordPress subfolder multisite with `/sub/` URL paths
- **Subsite CRUD**: Create and delete subsites via WP-CLI (`wp site create`, `wp site delete`)
- **Sync from WordPress**: Pull subsite list from WP-CLI to keep panel in sync with actual WordPress state
- **Agent-side execution**: All WordPress operations run on VPS agent via WP-CLI
- **Nginx safety**: Config tested with `nginx -t` before reload; original restored on failure
- **RBAC integration**: Admin and Manager can enable multisite and manage subsites; Viewer can only view

### Phase 4, Batch 3 — Image Optimization
- **Per-site config**: Quality slider (1-100%), WebP generation toggle, max dimensions
- **Agent-side optimization**: Uses jpegoptim (JPEG), optipng (PNG), cwebp (WebP) on VPS agent
- **Auto-install tools**: Agent installs missing optimization tools via apt-get if needed
- **WebP generation**: Creates `.webp` variants for all optimized images
- **Skip tracking**: Marker files prevent re-optimizing already processed images
- **Optimization history**: Paginated results with total images, optimized/skipped/error counts, bytes saved
- **Batch processing**: Scans entire wp-content/uploads directory for JPEG/PNG files
- **RBAC integration**: Admin and Manager can configure and run; Viewer can only view config and results

### Phase 4, Batch 4 — Team Management
- **Team CRUD**: Create, list, update, and delete teams with name and description
- **Team ownership**: Team creator is automatically set as owner and admin member
- **Member management**: Add/remove users, assign admin or member roles within teams
- **Server assignment**: Assign servers to teams — all team members get access to those servers
- **Team-based RBAC**: Server access now uses TeamServer+TeamMember chain (backward compatible with legacy UserServer)
- **Cross-cutting RBAC update**: Server listing, site listing, backup listing, security scan listing, migration listing all use team-based access
- **Migration utility**: One-click migration from old UserServer assignments to team-based assignments
- **Permission matrix**: Admin can manage all teams; Manager can create teams and manage their own; Viewer can view teams they belong to
- **Frontend**: Dedicated Teams page with team grid, detail panel, member management, server assignment

| Resource | Action | Admin | Manager | Viewer |
|----------|--------|-------|---------|--------|
| Teams | create | Yes | Yes | No |
| Teams | read | All | Own teams | Own teams |
| Teams | manage/delete | Yes | No | No |
| Teams | members | Yes | Team admin | No |
| Teams | servers | Yes | No | No |

## API Endpoints

### Authentication
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/login` | Login with email/password |
| POST | `/api/v1/auth/register` | Register first admin |

### Servers
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/servers` | Authenticated (filtered by role) |
| POST | `/api/v1/servers` | Admin only |
| GET | `/api/v1/servers/:id` | Server access required |
| DELETE | `/api/v1/servers/:id` | Admin only |

### Sites
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/sites` | Authenticated (filtered by role) |
| POST | `/api/v1/sites` | Admin or Manager |
| GET | `/api/v1/sites/:id` | Authenticated |
| DELETE | `/api/v1/sites/:id` | Admin or Manager |
| PUT | `/api/v1/sites/:id/php` | Admin or Manager |
| POST | `/api/v1/sites/:id/restart` | Admin or Manager |
| POST | `/api/v1/sites/:id/suspend` | Admin or Manager |
| POST | `/api/v1/sites/:id/resume` | Admin or Manager |
| POST | `/api/v1/sites/:id/ssl` | Admin or Manager |
| GET | `/api/v1/sites/:id/credentials` | Admin or Manager |
| GET | `/api/v1/sites/:id/autologin` | Admin or Manager |

### Users (Admin only)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users` | List all users |
| POST | `/api/v1/users` | Create user |
| GET | `/api/v1/users/:id` | Get user |
| PUT | `/api/v1/users/:id` | Update user |
| DELETE | `/api/v1/users/:id` | Delete user |
| POST | `/api/v1/users/:id/servers` | Assign server |
| DELETE | `/api/v1/users/:id/servers/:serverId` | Unassign server |
| GET | `/api/v1/users/:id/servers` | List assigned servers |

### API Keys
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/api-keys` | List user's API keys |
| POST | `/api/v1/api-keys` | Create API key |
| DELETE | `/api/v1/api-keys/:id` | Delete API key |

### Files
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/files/:serverId/list` | Read access |
| GET | `/api/v1/files/:serverId/read` | Read access |
| PUT | `/api/v1/files/:serverId/write` | Write access |
| POST | `/api/v1/files/:serverId/create` | Write access |
| DELETE | `/api/v1/files/:serverId/delete` | Write access |
| PUT | `/api/v1/files/:serverId/rename` | Write access |
| POST | `/api/v1/files/:serverId/upload` | Write access |
| GET | `/api/v1/files/:serverId/download` | Read access |

### Backups
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/backups` | Authenticated (filtered by role) |
| GET | `/api/v1/backups/:id` | Authenticated |
| DELETE | `/api/v1/backups/:id` | Admin or Manager |
| POST | `/api/v1/sites/:siteId/backups` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/backups` | Authenticated |
| POST | `/api/v1/sites/:siteId/backups/restore` | Admin or Manager |

### Backup Schedules
| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/sites/:siteId/backup-schedules` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/backup-schedules` | Authenticated |
| PUT | `/api/v1/backup-schedules/:id` | Admin or Manager |
| DELETE | `/api/v1/backup-schedules/:id` | Admin or Manager |

### Migrations
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/migrations` | Authenticated (filtered by role) |
| GET | `/api/v1/migrations/:id` | Authenticated |
| POST | `/api/v1/migrations/:id/rollback` | Admin or Manager |
| POST | `/api/v1/migrations/:id/cancel` | Admin or Manager |
| GET | `/api/v1/migrations/:id/logs` | Authenticated |
| POST | `/api/v1/sites/:siteId/migrate` | Admin or Manager |

### Security (WAF + Malware)
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/sites/:siteId/waf` | Authenticated |
| PUT | `/api/v1/sites/:siteId/waf` | Admin or Manager |
| GET | `/api/v1/scans` | Authenticated (filtered by role) |
| GET | `/api/v1/scans/:id` | Authenticated |
| POST | `/api/v1/sites/:siteId/scans` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/scans` | Authenticated |

### WordPress Advanced (Staging, Updates, DB)
| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/sites/:siteId/staging` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/staging` | Authenticated |
| POST | `/api/v1/staging/:stagingId/push` | Admin or Manager |
| DELETE | `/api/v1/staging/:stagingId` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/auto-update` | Authenticated |
| PUT | `/api/v1/sites/:siteId/auto-update` | Admin or Manager |
| POST | `/api/v1/sites/:siteId/updates/check` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/updates/logs` | Authenticated |
| POST | `/api/v1/sites/:siteId/db/optimize` | Admin or Manager |

### Rollouts (Admin only)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/releases` | Create agent release |
| GET | `/api/v1/releases` | List all releases |
| GET | `/api/v1/releases/:id` | Get release details |
| DELETE | `/api/v1/releases/:id` | Delete release |
| POST | `/api/v1/rollouts` | Create rollout plan |
| GET | `/api/v1/rollouts` | List rollouts (paginated) |
| GET | `/api/v1/rollouts/:id` | Get rollout with server details |
| POST | `/api/v1/rollouts/:id/start` | Start rollout execution |
| POST | `/api/v1/rollouts/:id/pause` | Pause running rollout |

### Multisite
| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/sites/:siteId/multisite/enable` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/multisite/subsites` | Authenticated |
| POST | `/api/v1/sites/:siteId/multisite/subsites` | Admin or Manager |
| DELETE | `/api/v1/sites/:siteId/multisite/subsites/:subsiteId` | Admin or Manager |
| POST | `/api/v1/sites/:siteId/multisite/sync` | Admin or Manager |

### Performance Metrics
| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/sites/:siteId/performance/run` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/performance` | Authenticated |
| GET | `/api/v1/sites/:siteId/performance/latest` | Authenticated |

### Image Optimization
| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/sites/:siteId/image-optimization` | Authenticated |
| PUT | `/api/v1/sites/:siteId/image-optimization` | Admin or Manager |
| POST | `/api/v1/sites/:siteId/image-optimization/run` | Admin or Manager |
| GET | `/api/v1/sites/:siteId/image-optimization/results` | Authenticated |

### Teams
| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/teams` | Admin or Manager |
| GET | `/api/v1/teams` | Authenticated (filtered by membership) |
| GET | `/api/v1/teams/:id` | Team member or Admin |
| PUT | `/api/v1/teams/:id` | Team admin or Admin |
| DELETE | `/api/v1/teams/:id` | Admin only |
| POST | `/api/v1/teams/:id/members` | Team admin or Admin |
| PUT | `/api/v1/teams/:id/members/:userId` | Team admin or Admin |
| DELETE | `/api/v1/teams/:id/members/:userId` | Team admin or Admin |
| POST | `/api/v1/teams/:id/servers` | Admin only |
| DELETE | `/api/v1/teams/:id/servers/:serverId` | Admin only |
| POST | `/api/v1/teams/migrate` | Admin only |

## Development

```bash
# Build API server
go build -o bin/wp-manage-api ./cmd/api

# Build agent
go build -o bin/wp-manage-agent ./cmd/agent

# Build frontend
cd web && npm run build

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o bin/wp-manage-api-linux ./cmd/api
GOOS=linux GOARCH=amd64 go build -o bin/wp-manage-agent-linux ./cmd/agent
```

## Deployment

- **API Server**: Binary at `/opt/wp-manage/bin/wp-manage-api`, frontend at `/opt/wp-manage/web/`
- **Agent**: Binary at `/usr/local/bin/wp-manage-agent`, runs as systemd service

## License

Proprietary - GoFiber Team
