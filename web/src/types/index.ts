export interface User {
  id: number
  email: string
  name: string
  role: 'admin' | 'manager' | 'viewer'
  two_factor_enabled: boolean
  created_at: string
}

export interface Server {
  id: number
  user_id: number
  name: string
  ip_address: string
  os: string
  ram_mb: number
  cpu_cores: number
  disk_gb: number
  agent_version: string
  status: 'active' | 'provisioning' | 'offline' | 'error'
  last_heartbeat: string | null
  created_at: string
  sites?: Site[]
}

export interface Site {
  id: number
  server_id: number
  domain: string
  php_version: string
  ssl_enabled: boolean
  ssl_expires_at: string | null
  db_name: string
  wp_version: string
  wp_admin_user: string
  wp_admin_email: string
  is_multisite: boolean
  multisite_type: string
  install_type: 'auto' | 'manual'
  linux_user?: string
  open_basedir_enabled?: boolean
  status: 'active' | 'suspended' | 'creating' | 'error' | 'deleting'
  created_at: string
  server?: Server
}

export interface Subsite {
  id: number
  site_id: number
  wp_blog_id: number
  domain: string
  path: string
  title: string
  status: string
  created_at: string
}

export interface EnableMultisiteRequest {
  type: 'subdomain' | 'subfolder'
}

export interface CreateSubsiteRequest {
  slug: string
  title: string
}

export interface Backup {
  id: number
  site_id: number | null  // null = orphaned (site was deleted, backup preserved)
  server_id: number       // always set — allows restore dispatch after site deletion
  domain: string          // stored at backup time, survives site deletion
  type: 'full' | 'database_only'
  storage: string
  file_path: string
  size_bytes: number
  checksum: string
  status: 'running' | 'completed' | 'failed'
  error?: string
  created_at: string
  completed_at: string | null
  site?: Site
}

export interface BackupSchedule {
  id: number
  site_id: number
  type: 'full' | 'database_only'
  storage: string
  cron_expr: string
  retention: number
  enabled: boolean
  created_at: string
  updated_at: string
  site?: Site
}

export interface CreateBackupRequest {
  type: 'full' | 'database_only'
  storage?: string
}

export interface RestoreRequest {
  backup_id: number
  confirm_domain: string
}

export interface CreateBackupScheduleRequest {
  type: 'full' | 'database_only'
  storage?: string
  cron_expr: string
  retention?: number
  enabled?: boolean
}

export interface APIResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
  error?: string
}

export interface PaginatedResponse<T> {
  success: boolean
  data: T[]
  meta: {
    page: number
    per_page: number
    total: number
    total_pages: number
  }
}

export interface LoginRequest {
  email: string
  password: string
}

export interface CreateServerRequest {
  name: string
  ip_address: string
}

export interface CreateSiteRequest {
  server_id: number
  domain: string
  php_version: string
  ssl_enabled: boolean
  redis_enabled: boolean
  install_type: 'auto' | 'manual'
  wp_admin_user: string
  wp_admin_email: string
}

export interface AutoDeployRequest {
  server_id: number
  ssh_user: string
  ssh_port: number
  auth_method: 'password' | 'key'
  password?: string
  private_key?: string
}

export interface DeployProgress {
  step: number
  total: number
  message: string
  status: 'running' | 'success' | 'error'
  error?: string
}

// File Manager types
export interface FileInfo {
  name: string
  path: string
  size: number
  is_dir: boolean
  permissions: string
  owner: string
  group: string
  mod_time: string
}

export interface FileContentRequest {
  path: string
  content: string
}

export interface FileMkdirRequest {
  path: string
}

export interface FileRenameRequest {
  old_path: string
  new_path: string
}

export interface FileDeleteRequest {
  path: string
}

export interface FilePermissionsRequest {
  path: string
  permissions: string
  recursive: boolean
}

export interface FileSearchRequest {
  path: string
  pattern: string
}

export interface FileSearchResult {
  file: string
  line: number
  content: string
}

export interface FileCopyMoveRequest {
  src_paths: string[]
  dest_dir: string
}

export interface FileDeleteBulkRequest {
  paths: string[]
}

// User management types
export interface CreateUserRequest {
  email: string
  password: string
  name: string
  role: 'admin' | 'manager' | 'viewer'
}

export interface UpdateUserRequest {
  email?: string
  password?: string
  name?: string
  role?: 'admin' | 'manager' | 'viewer'
}

export interface AssignServerRequest {
  server_id: number
}

// API Key types
export interface APIKey {
  id: number
  user_id: number
  name: string
  key_prefix: string
  rate_limit: number
  expires_at: string | null
  last_used_at: string | null
  created_at: string
}

export interface CreateAPIKeyRequest {
  name: string
  rate_limit?: number
  expires_in?: number // days, 0 = never
}

// Migration types
export interface Migration {
  id: number
  site_id: number
  source_server_id: number
  target_server_id: number
  mode: 'keep_domain' | 'change_domain'
  new_domain?: string
  update_dns: boolean
  source_cleaned_up: boolean
  phase: 'preparation' | 'initial_sync' | 'delta_sync' | 'switchover' | 'cleanup'
  status: 'running' | 'completed' | 'failed' | 'rolled_back'
  progress: number
  error?: string
  started_at: string
  completed_at: string | null
  site?: Site
  source_server?: Server
  target_server?: Server
}

export interface MigrationLog {
  id: number
  migration_id: number
  phase: string
  message: string
  level: string
  created_at: string
}

export interface MigrateRequest {
  target_server_id: number
  mode: 'keep_domain' | 'change_domain'
  new_domain?: string
  update_dns?: boolean
  cleanup_source?: boolean
}

// Security types
export interface WAFConfig {
  id: number
  site_id: number
  enabled: boolean
  rule_set: string
  paranoid_level: number
  whitelisted_ips: string
  custom_rules: string
  created_at: string
  updated_at: string
}

export interface UpdateWAFRequest {
  enabled?: boolean
  paranoid_level?: number
  whitelisted_ips?: string
  custom_rules?: string
}

export interface MalwareScan {
  id: number
  site_id: number
  scan_type: 'full' | 'quick' | 'custom'
  engine: 'wp_signature' | 'clamav' | 'both'
  status: 'running' | 'completed' | 'failed'
  files_scanned: number
  threats_found: number
  threats_detail: string // JSON array
  error?: string
  started_at: string
  completed_at: string | null
  site?: Site
}

export interface ThreatDetail {
  file: string
  threat_type: string
  description: string
  severity: 'low' | 'medium' | 'high' | 'critical'
}

export interface StartMalwareScanRequest {
  scan_type?: 'full' | 'quick' | 'custom'
  engine?: 'wp_signature' | 'clamav' | 'both'
  path?: string
}

export interface ClamAVStatus {
  installed: boolean
  version?: string
  db_version?: string
  last_updated?: string
  error?: string
}

export interface MalwareScanSchedule {
  id: number
  site_id: number
  site?: { id: number; domain: string; server_id: number }
  scan_type: 'full' | 'quick'
  engine: 'wp_signature' | 'clamav' | 'both'
  interval_h: number
  enabled: boolean
  last_run_at?: string
  next_run_at?: string
  created_at: string
  updated_at: string
}

// WordPress Advanced types

export interface StagingSite {
  id: number
  production_id: number
  staging_site_id: number
  status: 'creating' | 'active' | 'pushing' | 'deleted' | 'error'
  created_at: string
  updated_at: string
  production?: Site
  staging_site?: Site
}

export interface AutoUpdateConfig {
  id: number
  site_id: number
  core_updates: boolean
  plugin_updates: boolean
  theme_updates: boolean
  backup_before: boolean
  created_at: string
  updated_at: string
}

export interface UpdateAutoUpdateConfigRequest {
  core_updates?: boolean
  plugin_updates?: boolean
  theme_updates?: boolean
  backup_before?: boolean
}

export interface UpdateLog {
  id: number
  site_id: number
  update_type: string
  item_name: string
  from_version: string
  to_version: string
  status: string
  error?: string
  created_at: string
}

// Rollout types
export interface AgentRelease {
  id: number
  version: string
  download_url: string
  checksum: string
  release_notes: string
  file_size: number
  created_at: string
}

export interface CreateReleaseRequest {
  version: string
  download_url: string
  checksum: string
  release_notes?: string
  file_size?: number
}

export interface RolloutPlan {
  id: number
  release_id: number
  strategy: 'canary' | 'rolling' | 'immediate'
  batch_percent: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'paused'
  total_servers: number
  updated_count: number
  failed_count: number
  started_at: string | null
  completed_at: string | null
  created_at: string
  release?: AgentRelease
  servers?: RolloutServer[]
}

export interface RolloutServer {
  id: number
  rollout_plan_id: number
  server_id: number
  batch_number: number
  old_version: string
  new_version: string
  status: 'pending' | 'updating' | 'completed' | 'failed' | 'rolled_back'
  error?: string
  started_at: string | null
  completed_at: string | null
  server?: Server
}

export interface CreateRolloutRequest {
  release_id: number
  strategy: 'canary' | 'rolling' | 'immediate'
  batch_percent?: number
}

// Performance types
export interface PerformanceMetric {
  id: number
  site_id: number
  device_type: 'mobile' | 'desktop'
  performance_score: number
  accessibility_score: number
  lcp_ms: number
  cls: number
  fid_ms: number
  ttfb_ms: number
  measured_at: string
}

export interface RunPageSpeedRequest {
  device_type?: 'mobile' | 'desktop'
}

export interface ImageOptConfig {
  id: number
  site_id: number
  enabled: boolean
  quality: number
  webp_enabled: boolean
  nginx_webp: boolean
  max_width: number
  max_height: number
}

export interface ImageOptResult {
  id: number
  site_id: number
  total_images: number
  optimized_count: number
  skipped_count: number
  error_count: number
  bytes_saved: number
  status: string
  error?: string
  started_at: string
  completed_at: string | null
}

export interface UpdateImageOptConfigRequest {
  enabled?: boolean
  quality?: number
  webp_enabled?: boolean
  max_width?: number
  max_height?: number
}

// Team management types
export interface Team {
  id: number
  name: string
  description: string
  owner_id: number
  owner?: User
  members?: TeamMember[]
  servers?: TeamServerAssignment[]
  created_at: string
  updated_at: string
}

export interface TeamMember {
  id: number
  team_id: number
  user_id: number
  role: 'admin' | 'member'
  user?: User
  created_at: string
}

export interface TeamServerAssignment {
  id: number
  team_id: number
  server_id: number
  server?: Server
  created_at: string
}

export interface CreateTeamRequest {
  name: string
  description?: string
}

export interface UpdateTeamRequest {
  name?: string
  description?: string
}

export interface AddTeamMemberRequest {
  user_id: number
  role: 'admin' | 'member'
}

export interface UpdateTeamMemberRequest {
  role: 'admin' | 'member'
}

export interface AssignTeamServerRequest {
  server_id: number
}

// Infrastructure types (Batch 1)

export interface FirewallRule {
  id: number
  server_id: number
  rule_type: 'allow' | 'deny'
  protocol: string
  port: string
  source: string
  comment: string
  created_at: string
}

export interface CreateFirewallRuleRequest {
  rule_type: 'allow' | 'deny'
  protocol?: string
  port: string
  source?: string
  comment?: string
}

export interface NginxConfig {
  id: number
  server_id: number
  site_id: number | null
  config_type: 'redirect' | 'custom_location' | 'rate_limit' | 'security_header'
  name: string
  content: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateNginxConfigRequest {
  site_id?: number
  config_type: 'redirect' | 'custom_location' | 'rate_limit' | 'security_header'
  name: string
  content: string
  enabled?: boolean
}

export interface UpdateNginxConfigRequest {
  config_type?: string
  name?: string
  content?: string
  enabled?: boolean
}

export interface OPcacheActionRequest {
  action: 'enable' | 'disable' | 'flush'
}

export interface RebuildConfigRequest {
  config_types: string[]
  site_id?: number
}

export interface HealthCheckResult {
  service: string
  status: 'ok' | 'warning' | 'error'
  message: string
}

// ==================== Batch 2: Cron, Domain, Redis, PHP-FPM, MariaDB ====================

export interface CronJob {
  id: number
  server_id: number
  site_id: number | null
  command: string
  schedule: string
  user: string
  enabled: boolean
  comment: string
  created_at: string
  updated_at: string
}

export interface CreateCronJobRequest {
  site_id?: number
  command: string
  schedule: string
  user?: string
  comment?: string
}

export interface UpdateCronJobRequest {
  command?: string
  schedule?: string
  user?: string
  enabled?: boolean
  comment?: string
}

export interface DomainEntry {
  id: number
  site_id: number
  domain: string
  type: 'primary' | 'alias' | 'subdomain'
  ssl_enabled: boolean
  redirect_to?: string
  redirect_type?: number  // 301 or 302; 0/undefined = no redirect
  created_at: string
}

export interface CreateDomainRequest {
  domain: string
  type?: 'primary' | 'alias' | 'subdomain'
}

export interface UpdateDomainRequest {
  redirect_to: string
  redirect_type: number
}

export interface DNSCheckResult {
  domain: string
  server_ip: string
  resolved_ips: string[]
  pointing: boolean
  message: string
}

export interface RedisConfigData {
  id: number
  site_id: number
  enabled: boolean
  max_memory_mb: number
  prefix: string
  created_at: string
  updated_at: string
}

export interface RedisConfigRequest {
  enabled?: boolean
  max_memory_mb?: number
}

export interface RedisSiteStatus {
  site_id: number
  config_id?: number
  domain: string
  enabled: boolean
  max_memory_mb: number
  prefix?: string
}

export interface PHPFPMPoolConfig {
  max_children: number
  start_servers: number
  min_spare_servers: number
  max_spare_servers: number
  max_requests: number
  memory_limit: string
}

export interface VhostData {
  content: string
  file_path: string
}

export interface MariaDBUserRequest {
  username: string
  password?: string
  database?: string
  privileges?: string[]
}

// MariaDB user info returned from agent
export interface MariaDBUser {
  username: string
  host: string
  databases: string[]
}

export interface MariaDBConfig {
  max_connections: number
  innodb_buffer_mb: number
  query_cache_mb: number
  slow_query_log: boolean
  slow_query_time: number
  custom_config: string
}

export interface MariaDBDatabase {
  name: string
  size: string
  charset: string
  collation: string
}

export interface CreateDatabaseRequest {
  name: string
  charset?: string
  collation?: string
}

export interface InstalledPHPVersion {
  version: string
  installed: boolean
  fpm_active: boolean
  opcache_enabled: boolean
}

// ==================== ionCube ====================

export interface IonCubeVersionStatus {
  version: string
  installed: boolean // .so file present in PHP extension_dir
  enabled: boolean   // 00-ioncube.ini exists in fpm conf.d
  supported: boolean // ionCube officially supports this PHP version
}

export interface InstallPHPVersionRequest {
  version: string
}

export interface ChangePHPVersionRequest {
  new_version: string
}

// ==================== Batch 3: Activity Log, System Logs, Dashboard Realtime ====================

export interface ActivityLog {
  id: number
  user_id: number | null
  server_id: number | null
  site_id: number | null
  action: string
  details: string
  ip_address: string
  created_at: string
}

export interface ActivityLogFilters {
  user_id?: number
  server_id?: number
  site_id?: number
  action?: string
  from?: string
  to?: string
}

export interface ServerMetrics {
  server_id: number
  server_name?: string
  ip_address?: string
  cpu_usage: number
  ram_usage: number
  disk_usage: number
  uptime: number
  status?: string
}

export interface ServerLogRequest {
  type: 'nginx_access' | 'nginx_error' | 'php_error' | 'mysql_error' | 'syslog'
  lines?: number
}

export interface WebSocketMessage {
  type: string
  channel: string
  data: unknown
}

// ==================== Batch 4: Security & Authentication ====================

export interface TwoFactorSetup {
  secret: string
  qr_code_uri: string
  recovery_codes: string[]
}

export interface Validate2FARequest {
  email: string
  password: string
  code: string
}

export interface SSHKey {
  id: number
  server_id: number
  name: string
  public_key: string
  fingerprint: string
  created_at: string
}

export interface CreateSSHKeyRequest {
  name: string
  public_key: string
}

export interface SFTPAccount {
  id: number
  site_id: number
  username: string
  home_dir: string
  created_at: string
  // Only present in the Create response (one-time)
  host?: string
  port?: number
  password?: string
}

export interface CreateSFTPRequest {
  username: string
  password?: string
}

export interface HardenWordPressRequest {
  disable_xmlrpc: boolean
  hide_wp_version: boolean
  secure_file_perms: boolean
  security_headers: boolean
  disable_file_editor: boolean
  limit_login_attempts: boolean
}

// ==================== Batch 5: Monitoring & Alerts ====================

export interface UptimeCheck {
  id: number
  site_id: number
  url: string
  interval: number
  enabled: boolean
  last_status: number
  last_check: string | null
  created_at: string
  // Preloaded from ListAll endpoint
  site?: { id: number; domain: string; server_id: number }
}

export interface UptimeLog {
  id: number
  check_id: number
  status_code: number
  response_ms: number
  error: string
  created_at: string
}

export interface CreateUptimeCheckRequest {
  url: string
  interval?: number
}

export interface AlertRule {
  id: number
  server_id: number
  metric: string
  threshold: number
  channel: string
  webhook_url?: string
  email?: string
  enabled: boolean
  cooldown_min: number
  last_fired: string | null
  created_at: string
}

export interface CreateAlertRuleRequest {
  metric: string
  threshold: number
  channel: string
  webhook_url?: string
  email?: string
  cooldown_min?: number
}

export interface AlertHistory {
  id: number
  rule_id: number
  server_id: number
  metric: string
  value: number
  message: string
  channel: string
  sent: boolean
  created_at: string
}

export interface NotificationConfig {
  id: number
  telegram_token: string
  telegram_chat: string
  smtp_host: string
  smtp_port: number
  smtp_user: string
  smtp_from: string
}

export interface IntegrationConfig {
  id: number
  google_pagespeed_key: string
}

// ==================== Batch 6: WP-CLI, Cache, Import, DNS ====================

export interface WPCLIHistory {
  id: number
  site_id: number
  command: string
  output: string
  exit_code: number
  status: 'running' | 'completed' | 'failed'
  created_at: string
  completed_at: string | null
}

export interface WPCLIRequest {
  command: string
}

export interface CacheConfig {
  id: number
  site_id: number
  enabled: boolean
  cache_time: string
  excludes: string
  created_at: string
  updated_at: string
}

export interface UpdateCacheConfigRequest {
  enabled?: boolean
  cache_time?: string
  excludes?: string
}

export interface ExternalImport {
  id: number
  site_id: number
  domain: string
  source_ip: string
  ssh_user: string
  ssh_port: number
  status: 'pending' | 'running' | 'completed' | 'failed'
  progress: number
  error: string
  created_at: string
  completed_at: string | null
}

export interface ImportExternalRequest {
  domain: string
  source_ip: string
  ssh_user: string
  ssh_port?: number
}

export interface DNSProvider {
  id: number
  server_id: number
  provider_type: 'cloudflare' | 'route53' | 'godaddy'
  api_key: string
  api_email: string
  zone_id: string
  domain: string
  created_at: string
  server?: { id: number; name: string; ip_address: string }
}

export interface CreateDNSProviderRequest {
  provider_type: 'cloudflare' | 'route53' | 'godaddy'
  api_key: string
  api_email?: string
  zone_id: string
  domain: string
}


export interface DNSRecord {
  id: string
  type: string
  name: string
  content: string
  ttl: number
  proxied: boolean
  comment?: string
}

export interface CreateDNSRecordRequest {
  type: string
  name: string
  content: string
  ttl?: number
  proxied?: boolean
  comment?: string
}

// ==================== Batch 7: SSL, Storage, Site Wizard ====================

export interface SSLStatus {
  ssl_enabled: boolean
  ssl_expires_at: string | null
  domain: string
  force_https: boolean
}

export interface UploadCustomSSLRequest {
  cert_pem: string
  key_pem: string
  ca_pem: string
}

export interface StorageCredential {
  id: number
  server_id: number
  provider: 's3' | 'r2' | 'backblaze' | 'gdrive'
  bucket: string
  region: string
  access_key: string
  endpoint: string
  base_path: string
  created_at: string
}

export interface CreateStorageCredentialRequest {
  provider: 's3' | 'r2' | 'backblaze' | 'gdrive'
  bucket: string
  region?: string
  access_key: string
  secret_key: string
  endpoint?: string
  base_path?: string
}

// ==================== Batch 8: Fail2Ban, Panel Updates ====================

export interface Fail2BanJail {
  name: string
  enabled: boolean
  filter: string
  ban_time: number
  max_retry: number
  banned_ips: string[]
  total_bans: number
}

export interface Fail2BanStatus {
  jails: Fail2BanJail[]
}

export interface Fail2BanUnbanRequest {
  jail: string
  ip: string
}

export interface Fail2BanUpdateJailRequest {
  jail: string
  enabled?: boolean
  ban_time?: number
  max_retry?: number
}

export interface PanelRelease {
  id: number
  version: string
  download_url: string
  checksum: string
  release_notes: string
  file_size: number
  created_at: string
}

export interface PanelUpdateStatus {
  current_version: string
  update_available: boolean
  latest?: PanelRelease
}

export interface CreatePanelReleaseRequest {
  version: string
  download_url: string
  checksum: string
  release_notes?: string
  file_size?: number
}
