import React, { useState, useEffect } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft, Server, Globe, HardDrive, Cpu, MemoryStick, Clock, Wifi, WifiOff,
  Plus, Trash2, ExternalLink, RefreshCw, Activity, Shield, Settings, Zap, Wrench,
  HeartPulse, ToggleLeft, ToggleRight, Database, Terminal, FileText, Key, Copy, Check, Bell, AlertTriangle,
  Loader2, Save, CheckCircle2, XCircle, Play, Archive, ChevronLeft, ChevronRight, Upload
} from 'lucide-react'
import { useServer, useDeleteServer } from '@/hooks/useServers'
import {
  useFirewallRules, useCreateFirewallRule, useDeleteFirewallRule, useFirewallStatus,
  useCloudflareConfig, useEnableCloudflareFirewall, useDisableCloudflareFirewall,
  useNginxConfigs, useCreateNginxConfig, useDeleteNginxConfig,
  useOPcacheAction, useRebuildConfig, useRunHealthCheck, useServerLogs,
  usePHPFPMPool, useUpdatePHPFPMPool,
  useInstallPHPVersion, useChangeSitePHPVersion, useInstalledPHPVersions,
  useMariaDBUsers, useCreateMariaDBUser, useDeleteMariaDBUser,
  useMariaDBConfig, useUpdateMariaDBConfig,
  useMariaDBDatabases, useCreateMariaDBDatabase, useDeleteMariaDBDatabase, useImportMariaDBDatabase,
  useCheckServices, useInstallService,
} from '@/hooks/useInfrastructure'
import type { HealthCheckResult } from '@/hooks/useInfrastructure'
import { useSites } from '@/hooks/useSites'
import { useCronJobsByServer, useCreateCronJob, useUpdateCronJob, useToggleCronJob, useRunCronJob, useDeleteCronJob } from '@/hooks/useCronJobs'
import { useSSHKeys, useCreateSSHKey, useDeleteSSHKey } from '@/hooks/useSSHKeys'
import { useAlertRules, useCreateAlertRule, useDeleteAlertRule, useAlertHistory, usePHPFPMAutotune } from '@/hooks/useAlerts'
import { useStorageCredentials, useCreateStorageCredential, useDeleteStorageCredential } from '@/hooks/useStorage'
import { useFail2BanStatus, useFail2BanUnban, useFail2BanUpdateJail, useFail2BanToggleService, useFail2BanSetupJails } from '@/hooks/useFail2Ban'
import { useIonCubeStatus, useEnableIonCube, useDisableIonCube } from '@/hooks/useIonCube'
import { useServerRedisStats, useFlushServerRedis, useServerRedisSiteStatuses, useToggleRedis } from '@/hooks/useRedis'
import { useServerBackups, useDeleteBackup } from '@/hooks/useBackups'
import { usePermissions } from '@/hooks/usePermissions'
import { StatusBadge } from '@/components/ui/Badge'
import { formatDate } from '@/lib/utils'
import api from '@/lib/api'
import type { CronJob, CreateFirewallRuleRequest, CreateNginxConfigRequest, CreateCronJobRequest, CreateSSHKeyRequest, CreateAlertRuleRequest, CreateStorageCredentialRequest, PHPFPMPoolConfig, MariaDBUserRequest, MariaDBConfig, CreateDatabaseRequest, Backup } from '@/types'

export default function ServerDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = Number(id)
  const { data: server, isLoading, error, refetch } = useServer(serverId)
  const deleteServer = useDeleteServer()
  const { can } = usePermissions()
  const canManage = can('infrastructure', 'manage')
  const [activeTab, setActiveTab] = useState('overview')
  const [installingService, setInstallingService] = useState<string | null>(null)
  const [installError, setInstallError] = useState<string | null>(null)

  // These hooks must be called before any conditional returns (Rules of Hooks)
  const { data: servicesData, isLoading: servicesLoading, refetch: refetchServices, isFetching: servicesFetching } = useCheckServices(serverId)
  const installService = useInstallService(serverId)

  // Auto-check services on mount
  useEffect(() => {
    refetchServices()
  }, [serverId])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
      </div>
    )
  }

  if (error || !server) {
    return (
      <div className="text-center py-20">
        <p className="text-gray-500 mb-4">Server not found</p>
        <Link to="/servers" className="text-blue-600 hover:underline">Back to list</Link>
      </div>
    )
  }

  const isOnline = server.status === 'active'
  const lastHeartbeat = server.last_heartbeat ? formatDate(server.last_heartbeat) : 'N/A'

  const handleInstallService = async (service: string) => {
    setInstallingService(service)
    setInstallError(null)
    try {
      await installService.mutateAsync(service)
      // Auto re-check service status after install completes
      await refetchServices()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      setInstallError(`${service}: ${msg || 'Install failed'}`)
    } finally {
      setInstallingService(null)
    }
  }

  const handleDelete = async () => {
    if (!confirm(`Are you sure you want to delete server ${server.name}?`)) return
    try {
      await deleteServer.mutateAsync(server.id)
      navigate('/servers')
    } catch {}
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Link to="/servers" className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-lg">
            <ArrowLeft size={20} />
          </Link>
          <div>
            <div className="flex items-center gap-3">
              <Server size={22} className="text-blue-500" />
              <h1 className="text-2xl font-bold text-gray-900">{server.name}</h1>
              <StatusBadge status={server.status} />
            </div>
            <p className="text-sm text-gray-500 font-mono mt-0.5 ml-[34px]">{server.ip_address}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => refetch()} className="flex items-center gap-1.5 px-3 py-2 text-sm text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50">
            <RefreshCw size={14} /> Refresh
          </button>
          <Link to="/sites"
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Create Site
          </Link>
        </div>
      </div>

      {/* Info Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <InfoCard icon={<Server size={20} />} label="Operating System" value={server.os || 'Unknown'} />
        <InfoCard icon={<MemoryStick size={20} />} label="RAM" value={server.ram_mb ? `${Math.round(server.ram_mb / 1024)} GB` : '-'} />
        <InfoCard icon={<Cpu size={20} />} label="CPU" value={server.cpu_cores ? `${server.cpu_cores} cores` : '-'} />
        <InfoCard icon={<HardDrive size={20} />} label="Disk" value={server.disk_gb ? `${server.disk_gb} GB` : '-'} />
      </div>

      {/* Sidebar + Content */}
      <div className="flex gap-6 items-start">

        {/* Left sidebar nav */}
        <nav className="w-48 flex-shrink-0 bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden sticky top-4">
          {([
            { group: 'Server', items: [
              { id: 'overview', icon: <Server size={15} />,    label: 'Overview' },
              { id: 'logs',     icon: <FileText size={15} />,  label: 'Logs' },
            ]},
            { group: 'Security', items: [
              { id: 'firewall',  icon: <Shield size={15} />,   label: 'Firewall' },
              { id: 'fail2ban',  icon: <Shield size={15} />,   label: 'Fail2Ban' },
              { id: 'ssh-keys',  icon: <Key size={15} />,      label: 'SSH Keys' },
            ]},
            { group: 'Web Server', items: [
              { id: 'nginx',    icon: <Settings size={15} />,  label: 'Nginx' },
              { id: 'opcache',  icon: <Zap size={15} />,       label: 'OPcache' },
              { id: 'phpfpm',   icon: <Settings size={15} />,  label: 'PHP-FPM' },
              { id: 'ioncube',  icon: <Zap size={15} />,       label: 'ionCube' },
            ]},
            { group: 'Database', items: [
              { id: 'mariadb',  icon: <Database size={15} />,  label: 'MariaDB' },
              { id: 'redis',    icon: <Database size={15} />,  label: 'Redis' },
            ]},
            { group: 'System', items: [
              { id: 'cron',     icon: <Terminal size={15} />,  label: 'Cron Jobs' },
              { id: 'alerts',   icon: <Bell size={15} />,      label: 'Alerts' },
              { id: 'storage',  icon: <HardDrive size={15} />, label: 'Storage' },
              { id: 'backups',  icon: <Archive size={15} />,   label: 'Backups' },
              { id: 'config',   icon: <Wrench size={15} />,    label: 'Config' },
            ]},
          ] as { group: string; items: { id: string; icon: React.ReactNode; label: string }[] }[]).map(({ group, items }, gi) => (
            <div key={group} className={gi > 0 ? 'border-t border-gray-100' : ''}>
              <p className="px-3 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400">
                {group}
              </p>
              {items.map(({ id, icon, label }) => (
                <button
                  key={id}
                  onClick={() => setActiveTab(id)}
                  className={`w-full flex items-center gap-2.5 px-3 py-2 text-sm transition-colors ${
                    activeTab === id
                      ? 'bg-blue-50 text-blue-700 font-medium'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
                  }`}
                >
                  <span className={activeTab === id ? 'text-blue-600' : 'text-gray-400'}>{icon}</span>
                  {label}
                  {activeTab === id && (
                    <span className="ml-auto w-1 h-4 rounded-full bg-blue-600" />
                  )}
                </button>
              ))}
              {gi === 4 && <div className="pb-2" />}
            </div>
          ))}
        </nav>

        {/* Right content */}
        <div className="flex-1 min-w-0">
        {/* Tab Content */}
      {activeTab === 'overview' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Server Details */}
          <div className="lg:col-span-2 space-y-6">
            <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
              <h2 className="text-lg font-semibold text-gray-900 mb-4">Server Info</h2>
              <dl className="space-y-3">
                <DetailRow label="IP Address" value={<span className="font-mono">{server.ip_address}</span>} />
                <DetailRow label="Agent Version" value={server.agent_version || 'Not installed'} />
                <DetailRow label="Connection Status" value={
                  <span className="flex items-center gap-1.5">
                    {isOnline ? <Wifi size={14} className="text-green-500" /> : <WifiOff size={14} className="text-gray-400" />}
                    {isOnline ? 'Connected' : 'Disconnected'}
                  </span>
                } />
                <DetailRow label="Last Heartbeat" value={
                  <span className="flex items-center gap-1.5">
                    <Clock size={14} className="text-gray-400" />
                    {lastHeartbeat}
                  </span>
                } />
                <DetailRow label="Created" value={formatDate(server.created_at)} />
              </dl>
            </div>

            {/* Services Status */}
            <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-gray-900">Services</h2>
                {isOnline && (
                  <button
                    onClick={() => refetchServices()}
                    disabled={servicesFetching}
                    className="flex items-center gap-1.5 px-2.5 py-1 text-xs text-gray-500 hover:text-blue-600 border border-gray-200 rounded-lg hover:bg-blue-50 transition-colors disabled:opacity-50"
                  >
                    <RefreshCw size={11} className={servicesFetching ? 'animate-spin' : ''} />
                    Refresh
                  </button>
                )}
              </div>
              {installError && (
                <div className="mb-3 px-4 py-2.5 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700 whitespace-pre-wrap break-words">{installError}</div>
              )}
              <div className="grid grid-cols-2 gap-3">
                {(() => {
                  const SERVICES = [
                    { key: 'nginx',        installKey: 'nginx',     label: 'Nginx' },
                    { key: 'php8.3-fpm',   installKey: 'php',       label: 'PHP-FPM' },
                    { key: 'mariadb',      installKey: 'mariadb',   label: 'MariaDB' },
                    { key: 'redis-server', installKey: 'redis',     label: 'Redis' },
                    { key: 'certbot',      installKey: 'certbot',   label: 'Certbot (SSL)' },
                    { key: 'fail2ban',     installKey: 'fail2ban',  label: 'Fail2Ban' },
                    { key: 'ufw',          installKey: 'ufw',       label: 'UFW' },
                    { key: 'cron',         installKey: 'cron',      label: 'Cron' },
                    { key: 'wpcli',        installKey: 'wpcli',     label: 'WP-CLI' },
                  ]
                  return SERVICES.map(({ key, installKey, label }) => {
                    const svcData = servicesData?.find(s => s.service === key)
                    const isChecked = !!servicesData && servicesData.length > 0
                    const isRunning = svcData?.active === true
                    const isInstalling = installingService === installKey
                    return (
                      <div key={key} className="flex items-center gap-2 p-3 bg-gray-50 rounded-lg">
                        <Activity size={14} className={
                          isInstalling ? 'text-blue-400 animate-pulse' :
                          !isChecked ? 'text-gray-300' :
                          isRunning ? 'text-green-500' : 'text-red-400'
                        } />
                        <span className="text-sm font-medium text-gray-700">{label}</span>
                        {isInstalling ? (
                          <span className="ml-auto flex items-center gap-1 text-xs text-blue-600">
                            <Loader2 size={11} className="animate-spin" /> Installing...
                          </span>
                        ) : servicesLoading || servicesFetching ? (
                          <span className="ml-auto text-xs text-gray-400">...</span>
                        ) : isChecked && !isRunning ? (
                          <button
                            onClick={() => handleInstallService(installKey)}
                            disabled={!!installingService}
                            className="ml-auto flex items-center gap-1 text-xs px-2 py-0.5 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-40 transition-colors"
                          >
                            <Plus size={10} /> Install
                          </button>
                        ) : (
                          <span className={`ml-auto text-xs px-2 py-0.5 rounded-full ${
                            !isChecked ? 'bg-gray-100 text-gray-400' : 'bg-green-100 text-green-700'
                          }`}>
                            {!isChecked ? 'Unknown' : 'Running'}
                          </span>
                        )}
                      </div>
                    )
                  })
                })()}
              </div>
              {!servicesData && !servicesLoading && !servicesFetching && !isOnline && (
                <p className="text-xs text-gray-400 mt-3 text-center">Server offline — không thể kiểm tra service</p>
              )}
            </div>
          </div>

          {/* Right column */}
          <div className="space-y-6">
            <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-gray-900">Sites</h2>
                <span className="text-sm text-gray-500">{server.sites?.length || 0} site(s)</span>
              </div>
              {server.sites && server.sites.length > 0 ? (
                <div className="space-y-3">
                  {server.sites.map((site) => (
                    <Link key={site.id} to={`/sites/${site.id}`}
                      className="flex items-center justify-between p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors block">
                      <div className="flex items-center gap-2">
                        <Globe size={16} className="text-blue-500" />
                        <div>
                          <p className="text-sm font-medium text-gray-900">{site.domain}</p>
                          <p className="text-xs text-gray-500">PHP {site.php_version}</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <StatusBadge status={site.status} />
                        <a href={`https://${site.domain}`} target="_blank" rel="noopener"
                          onClick={(e) => e.stopPropagation()}
                          className="p-1 text-gray-400 hover:text-blue-600">
                          <ExternalLink size={14} />
                        </a>
                      </div>
                    </Link>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-gray-400 text-center py-6">No sites yet</p>
              )}
            </div>

            {can('servers', 'delete') && (
            <div className="bg-white rounded-xl shadow-sm border border-red-100 p-6">
              <h2 className="text-lg font-semibold text-red-600 mb-3">Danger Zone</h2>
              <p className="text-sm text-gray-500 mb-3">Deleting the server will remove the agent and all configurations.</p>
              <button onClick={handleDelete} disabled={deleteServer.isPending || (server.sites && server.sites.length > 0)}
                className="w-full flex items-center justify-center gap-2 px-4 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
                <Trash2 size={16} />
                {deleteServer.isPending ? 'Deleting...' : 'Delete Server'}
              </button>
              {server.sites && server.sites.length > 0 && (
                <p className="text-xs text-red-500 mt-2 text-center">Delete all sites before deleting this server</p>
              )}
            </div>
            )}
          </div>
        </div>
      )}

      {activeTab === 'firewall' && <FirewallTab serverId={serverId} />}
      {activeTab === 'nginx' && <NginxConfigTab serverId={serverId} />}
      {activeTab === 'opcache' && <OPcacheTab serverId={serverId} />}
      {activeTab === 'ioncube' && <IonCubeTab serverId={serverId} />}
      {activeTab === 'phpfpm' && <PHPFPMTab serverId={serverId} sites={server.sites || []} />}
      {activeTab === 'mariadb' && <MariaDBTab serverId={serverId} />}
      {activeTab === 'cron' && <CronJobsTab serverId={serverId} />}
      {activeTab === 'ssh-keys' && <SSHKeysTab serverId={serverId} />}
      {activeTab === 'alerts' && <AlertsTab serverId={serverId} />}
      {activeTab === 'logs' && <LogsTab serverId={serverId} />}
      {activeTab === 'config' && <ConfigTab serverId={serverId} />}
      {activeTab === 'storage' && <StorageTab serverId={serverId} />}
      {activeTab === 'fail2ban' && <Fail2BanTab serverId={serverId} />}
      {activeTab === 'redis' && <RedisServerTab serverId={serverId} canManage={canManage} />}
      {activeTab === 'backups' && <BackupArchivesTab serverId={serverId} canManage={canManage} />}
        </div> {/* end right content */}
      </div> {/* end flex layout */}
    </div>
  )
}

function InfoCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-4">
      <div className="flex items-center gap-2 text-gray-400 mb-2">
        {icon}
        <span className="text-xs font-medium uppercase">{label}</span>
      </div>
      <p className="text-lg font-semibold text-gray-900">{value}</p>
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-2 border-b border-gray-50 last:border-0">
      <dt className="text-sm text-gray-500">{label}</dt>
      <dd className="text-sm font-medium text-gray-900">{value}</dd>
    </div>
  )
}

// ==================== Firewall Tab ====================

function CloudflareProtectionPanel({ serverId, canManage }: { serverId: number; canManage: boolean }) {
  const { data: cfg, isLoading } = useCloudflareConfig(serverId)
  const enable = useEnableCloudflareFirewall()
  const disable = useDisableCloudflareFirewall()
  const isPending = enable.isPending || disable.isPending

  const handleToggle = async () => {
    try {
      if (cfg?.enabled) {
        await disable.mutateAsync(serverId)
      } else {
        await enable.mutateAsync(serverId)
      }
    } catch (e: any) {
      alert('Lỗi: ' + (e?.response?.data?.error ?? e?.message ?? 'Unknown error'))
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-orange-100 p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Shield size={20} className="text-orange-500" />
            Cloudflare Protection
          </h2>
          <p className="text-sm text-gray-500 mt-1">
            Chỉ cho phép traffic từ Cloudflare IPs vào port 80 và 443.
            IP Cloudflare tự động cập nhật hàng tuần (Thứ 2 lúc 02:00).
          </p>
          {cfg?.last_sync_at && (
            <p className="text-xs text-gray-400 mt-1">
              Lần đồng bộ cuối: {new Date(cfg.last_sync_at).toLocaleString('vi-VN')}
            </p>
          )}
        </div>
        {canManage ? (
          <button
            onClick={handleToggle}
            disabled={isLoading || isPending}
            title={cfg?.enabled ? 'Tắt Cloudflare Protection' : 'Bật Cloudflare Protection'}
            className={`relative inline-flex h-7 w-12 flex-shrink-0 items-center rounded-full transition-colors focus:outline-none disabled:opacity-50 ${
              cfg?.enabled ? 'bg-orange-500' : 'bg-gray-200'
            }`}
          >
            <span className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
              cfg?.enabled ? 'translate-x-6' : 'translate-x-1'
            }`} />
          </button>
        ) : (
          <span className={`px-2.5 py-1 rounded-full text-xs font-medium flex-shrink-0 ${
            cfg?.enabled ? 'bg-orange-100 text-orange-700' : 'bg-gray-100 text-gray-500'
          }`}>
            {cfg?.enabled ? 'Đang bật' : 'Đã tắt'}
          </span>
        )}
      </div>

      {isPending && (
        <div className="mt-3 flex items-center gap-2 text-sm text-orange-600">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-orange-500" />
          {enable.isPending ? 'Đang bật Cloudflare Protection...' : 'Đang tắt Cloudflare Protection...'}
          <span className="text-xs text-gray-400">(30–120s)</span>
        </div>
      )}

      {cfg?.enabled && !isPending && (
        <div className="mt-3 p-3 bg-orange-50 rounded-lg border border-orange-100 flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-orange-400 animate-pulse flex-shrink-0" />
          <p className="text-xs text-orange-700">
            <strong>Đang hoạt động:</strong> Port 80/443 chỉ nhận traffic từ Cloudflare IPs.
            Sites phải dùng Cloudflare proxy <span className="font-mono">☁️</span> mới truy cập được.
          </p>
        </div>
      )}

      <div className="mt-3 p-3 bg-amber-50 rounded-lg border border-amber-100">
        <p className="text-xs text-amber-700">
          ⚠️ <strong>Lưu ý:</strong> Chỉ bật khi <em>tất cả</em> sites trên server đã dùng Cloudflare proxy.
          Site dùng DNS trực tiếp (không qua Cloudflare) sẽ không truy cập được.
        </p>
      </div>
    </div>
  )
}

function FirewallTab({ serverId }: { serverId: number }) {
  const { can } = usePermissions()
  const canManage = can('infrastructure', 'manage')
  const { data: rules = [], isLoading } = useFirewallRules(serverId)
  const createRule = useCreateFirewallRule()
  const deleteRule = useDeleteFirewallRule()
  const getStatus = useFirewallStatus()
  const [showForm, setShowForm] = useState(false)
  const [ufwStatus, setUfwStatus] = useState('')
  const [statusLoading, setStatusLoading] = useState(false)
  const [form, setForm] = useState<CreateFirewallRuleRequest>({
    rule_type: 'allow', protocol: 'tcp', port: '', source: '', comment: ''
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createRule.mutateAsync({ serverId, ...form })
    setForm({ rule_type: 'allow', protocol: 'tcp', port: '', source: '', comment: '' })
    setShowForm(false)
  }

  const handleGetStatus = async () => {
    setStatusLoading(true)
    try {
      const result = await getStatus.mutateAsync(serverId)
      setUfwStatus(result)
    } catch (e: any) {
      setUfwStatus('Error: ' + (e?.response?.data?.error ?? e?.message ?? 'Failed to get status'))
    } finally {
      setStatusLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      {/* Cloudflare Protection Panel */}
      <CloudflareProtectionPanel serverId={serverId} canManage={canManage} />

      {/* UFW Status Panel */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Shield size={20} className="text-orange-500" /> UFW Firewall Status
          </h2>
          <button onClick={handleGetStatus} disabled={statusLoading}
            className="flex items-center gap-1.5 px-3 py-2 text-sm bg-orange-50 text-orange-700 border border-orange-200 rounded-lg hover:bg-orange-100 disabled:opacity-50">
            {statusLoading ? <><div className="animate-spin rounded-full h-3 w-3 border-b-2 border-orange-600" /> Checking...</> : <><RefreshCw size={13} /> Check UFW Status</>}
          </button>
        </div>
        <p className="text-xs text-gray-400 mb-3">UFW is automatically installed if not present. SSH (22), HTTP (80), HTTPS (443) are pre-allowed before enabling.</p>
        {ufwStatus ? (
          <pre className="text-xs font-mono bg-gray-900 text-green-400 p-3 rounded-lg overflow-x-auto whitespace-pre-wrap max-h-64">{ufwStatus}</pre>
        ) : (
          <div className="text-center py-6 text-gray-400">
            <Shield size={28} className="mx-auto mb-2 opacity-30" />
            <p className="text-sm">Click "Check UFW Status" to see live firewall rules from the server</p>
          </div>
        )}
      </div>

      {/* Managed Rules Panel */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Shield size={20} className="text-blue-500" /> Managed Rules ({rules.length})
          </h2>
          {canManage && (
            <button onClick={() => setShowForm(!showForm)}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
              <Plus size={14} /> Add Rule
            </button>
          )}
        </div>

        {showForm && (
          <form onSubmit={handleCreate} className="grid grid-cols-2 md:grid-cols-5 gap-3 mb-4 p-4 bg-blue-50 rounded-lg border border-blue-100">
            <select value={form.rule_type} onChange={e => setForm({...form, rule_type: e.target.value as 'allow' | 'deny'})}
              className="px-3 py-2 border rounded-lg text-sm">
              <option value="allow">Allow</option>
              <option value="deny">Deny</option>
            </select>
            <select value={form.protocol} onChange={e => setForm({...form, protocol: e.target.value})}
              className="px-3 py-2 border rounded-lg text-sm">
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
              <option value="both">Both</option>
            </select>
            <input type="text" placeholder="Port (e.g. 8080)" value={form.port}
              onChange={e => setForm({...form, port: e.target.value})}
              className="px-3 py-2 border rounded-lg text-sm font-mono" required />
            <input type="text" placeholder="Source IP (optional)" value={form.source || ''}
              onChange={e => setForm({...form, source: e.target.value})}
              className="px-3 py-2 border rounded-lg text-sm font-mono" />
            <div className="flex gap-2">
              <input type="text" placeholder="Comment" value={form.comment || ''}
                onChange={e => setForm({...form, comment: e.target.value})}
                className="px-3 py-2 border rounded-lg text-sm flex-1" />
              <button type="submit" disabled={createRule.isPending}
                className="px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                {createRule.isPending ? <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-white" /> : 'Add'}
              </button>
            </div>
          </form>
        )}

        {isLoading ? (
          <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
        ) : rules.length === 0 ? (
          <div className="text-center py-8 text-gray-400">
            <Shield size={28} className="mx-auto mb-2 opacity-30" />
            <p className="text-sm">No managed rules yet</p>
            <p className="text-xs mt-1">Add rules to manage via WP Manage (stored in DB + applied to UFW)</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b">
                  <th className="pb-2 font-medium">Action</th>
                  <th className="pb-2 font-medium">Protocol</th>
                  <th className="pb-2 font-medium">Port</th>
                  <th className="pb-2 font-medium">Source</th>
                  <th className="pb-2 font-medium">Comment</th>
                  <th className="pb-2 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {rules.map(rule => (
                  <tr key={rule.id} className="border-b border-gray-50 hover:bg-gray-50">
                    <td className="py-2.5">
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                        rule.rule_type === 'allow' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                      }`}>{rule.rule_type.toUpperCase()}</span>
                    </td>
                    <td className="py-2.5 font-mono text-gray-600">{rule.protocol.toUpperCase()}</td>
                    <td className="py-2.5 font-mono font-semibold">{rule.port}</td>
                    <td className="py-2.5 font-mono text-gray-500">{rule.source || <span className="text-gray-300">Anywhere</span>}</td>
                    <td className="py-2.5 text-gray-500 text-xs">{rule.comment}</td>
                    <td className="py-2.5">
                      <button onClick={() => deleteRule.mutate({ serverId, ruleId: rule.id })}
                        className="p-1 text-gray-400 hover:text-red-600 rounded hover:bg-red-50 transition-colors">
                        <Trash2 size={14} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== Nginx Config Tab ====================

function NginxConfigTab({ serverId }: { serverId: number }) {
  const { data: configs = [], isLoading } = useNginxConfigs(serverId)
  const createConfig = useCreateNginxConfig()
  const deleteConfig = useDeleteNginxConfig()
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateNginxConfigRequest>({
    config_type: 'redirect', name: '', content: ''
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createConfig.mutateAsync({ serverId, ...form })
    setForm({ config_type: 'redirect', name: '', content: '' })
    setShowForm(false)
  }

  return (
    <div className="space-y-6">
      {/* File Paths Info */}
      <div className="bg-gray-50 border border-gray-200 rounded-xl p-4">
        <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
          <FileText size={16} /> Nginx File Paths
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs font-mono text-gray-500">
          <div><span className="text-gray-400">Config:</span> /etc/nginx/nginx.conf</div>
          <div><span className="text-gray-400">Vhosts:</span> /etc/nginx/sites-available/*.conf</div>
          <div><span className="text-gray-400">Enabled:</span> /etc/nginx/sites-enabled/*.conf</div>
          <div><span className="text-gray-400">Snippets:</span> /etc/nginx/snippets/custom/</div>
          <div><span className="text-gray-400">Logs:</span> /var/log/nginx/access.log, error.log</div>
          <div><span className="text-gray-400">Cache:</span> /var/cache/nginx/fastcgi/</div>
        </div>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Settings size={20} className="text-blue-500" /> Nginx Custom Configs
          </h2>
          <button onClick={() => setShowForm(!showForm)}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Add Config
          </button>
        </div>

        {showForm && (
          <form onSubmit={handleCreate} className="space-y-3 mb-4 p-4 bg-gray-50 rounded-lg">
            <div className="grid grid-cols-2 gap-3">
              <select value={form.config_type}
                onChange={e => setForm({...form, config_type: e.target.value as CreateNginxConfigRequest['config_type']})}
                className="px-3 py-2 border rounded-lg text-sm">
                <option value="redirect">Redirect</option>
                <option value="custom_location">Custom Location</option>
                <option value="rate_limit">Rate Limit</option>
                <option value="security_header">Security Header</option>
              </select>
              <input type="text" placeholder="Config name" value={form.name}
                onChange={e => setForm({...form, name: e.target.value})}
                className="px-3 py-2 border rounded-lg text-sm" required />
            </div>
            <textarea placeholder="Nginx config content..." value={form.content}
              onChange={e => setForm({...form, content: e.target.value})}
              className="w-full px-3 py-2 border rounded-lg text-sm font-mono h-32" required />
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowForm(false)}
                className="px-4 py-2 text-sm text-gray-600 bg-gray-200 rounded-lg hover:bg-gray-300">Cancel</button>
              <button type="submit" disabled={createConfig.isPending}
                className="px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                {createConfig.isPending ? 'Creating...' : 'Create'}
              </button>
            </div>
          </form>
        )}

        {isLoading ? (
          <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
        ) : configs.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No custom Nginx configs</p>
        ) : (
          <div className="space-y-3">
            {configs.map(cfg => (
              <div key={cfg.id} className="p-4 border border-gray-200 rounded-lg">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-900">{cfg.name}</span>
                    <span className="px-2 py-0.5 rounded text-xs bg-blue-100 text-blue-700">{cfg.config_type}</span>
                    <span className={`px-2 py-0.5 rounded text-xs ${cfg.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                      {cfg.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </div>
                  <button onClick={() => deleteConfig.mutate({ serverId, configId: cfg.id })}
                    className="p-1 text-gray-400 hover:text-red-600">
                    <Trash2 size={14} />
                  </button>
                </div>
                <p className="text-xs text-gray-400 font-mono mb-2">/etc/nginx/snippets/custom/{cfg.name.replace(/ /g, '_')}_{cfg.config_type}.conf</p>
                <pre className="text-xs bg-gray-50 p-3 rounded font-mono text-gray-600 overflow-x-auto">{cfg.content}</pre>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== OPcache Tab ====================

function OPcacheTab({ serverId }: { serverId: number }) {
  const opcacheAction = useOPcacheAction()
  const { data: installedVersions = [], isLoading: versionsLoading, refetch: refetchVersions } = useInstalledPHPVersions(serverId)
  const [selectedVersion, setSelectedVersion] = useState<string>('')
  const [activeAction, setActiveAction] = useState<string | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Auto-select first active version
  const activeVersions = installedVersions.filter(v => v.fpm_active)
  const effectiveVersion = selectedVersion || activeVersions[0]?.version || installedVersions[0]?.version || '8.2'

  const handleAction = async (action: string) => {
    setActiveAction(action)
    setMsg(null)
    try {
      const result = await opcacheAction.mutateAsync({ serverId, action, phpVersion: effectiveVersion })
      setMsg({ type: 'success', text: result.message || `OPcache ${action}d on PHP ${effectiveVersion}` })
      // Refresh installed versions to update opcache_enabled badge
      if (action === 'enable' || action === 'disable') {
        refetchVersions()
      }
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to execute OPcache action' })
    } finally {
      setActiveAction(null)
    }
  }

  const isRunning = opcacheAction.isPending

  return (
    <div className="space-y-4">
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
          <Zap size={20} className="text-yellow-500" /> OPcache Management
        </h2>
        <p className="text-sm text-gray-500 mb-5">
          OPcache improves PHP performance by storing precompiled bytecode in memory.
          Select a PHP version then apply actions — takes effect immediately.
        </p>

        {/* PHP version selector */}
        <div className="mb-5 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
          <label className="text-xs font-semibold text-yellow-800 mb-2 block">Target PHP Version</label>
          {versionsLoading ? (
            <div className="flex items-center gap-2 text-sm text-yellow-700">
              <Loader2 size={14} className="animate-spin" /> Loading installed versions...
            </div>
          ) : installedVersions.length === 0 ? (
            <p className="text-sm text-yellow-700">No PHP versions detected on server yet.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {installedVersions.map(v => (
                <button key={v.version}
                  onClick={() => setSelectedVersion(v.version)}
                  className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium border-2 transition-colors ${
                    effectiveVersion === v.version
                      ? 'border-yellow-500 bg-yellow-100 text-yellow-800'
                      : 'border-gray-200 bg-white text-gray-600 hover:border-yellow-300'
                  }`}>
                  <span>PHP {v.version}</span>
                  <span className="flex items-center gap-1">
                    <span className={`w-2 h-2 rounded-full ${v.fpm_active ? 'bg-green-500' : 'bg-gray-300'}`} title={v.fpm_active ? 'FPM Active' : 'FPM Inactive'} />
                    <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${
                      v.opcache_enabled ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-600'
                    }`}>
                      OPcache {v.opcache_enabled ? 'ON' : 'OFF'}
                    </span>
                  </span>
                </button>
              ))}
            </div>
          )}
          {installedVersions.length > 0 && (
            <p className="text-xs text-yellow-600 mt-2">
              Targeting: <strong>PHP {effectiveVersion}</strong>
              {' '}({installedVersions.find(v => v.version === effectiveVersion)?.opcache_enabled ? '✓ OPcache enabled' : '✗ OPcache disabled'})
              {' '}— Actions apply to this version's php.ini immediately
            </p>
          )}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <button onClick={() => handleAction('enable')} disabled={isRunning}
            className="flex items-center justify-center gap-2 p-4 border-2 border-green-200 rounded-xl hover:bg-green-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
            {activeAction === 'enable' ? <Loader2 size={20} className="animate-spin text-green-600" /> : <ToggleRight size={20} className="text-green-600" />}
            <span className="text-sm font-medium text-green-700">{activeAction === 'enable' ? 'Enabling...' : 'Enable OPcache'}</span>
          </button>
          <button onClick={() => handleAction('disable')} disabled={isRunning}
            className="flex items-center justify-center gap-2 p-4 border-2 border-red-200 rounded-xl hover:bg-red-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
            {activeAction === 'disable' ? <Loader2 size={20} className="animate-spin text-red-600" /> : <ToggleLeft size={20} className="text-red-600" />}
            <span className="text-sm font-medium text-red-700">{activeAction === 'disable' ? 'Disabling...' : 'Disable OPcache'}</span>
          </button>
          <button onClick={() => handleAction('flush')} disabled={isRunning}
            className="flex items-center justify-center gap-2 p-4 border-2 border-blue-200 rounded-xl hover:bg-blue-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
            {activeAction === 'flush' ? <Loader2 size={20} className="animate-spin text-blue-600" /> : <RefreshCw size={20} className="text-blue-600" />}
            <span className="text-sm font-medium text-blue-700">{activeAction === 'flush' ? 'Flushing...' : 'Flush Cache'}</span>
          </button>
        </div>
        {isRunning && (
          <div className="mt-4 p-3 bg-blue-50 border border-blue-200 rounded-lg flex items-center gap-2">
            <Loader2 size={16} className="animate-spin text-blue-600" />
            <p className="text-sm text-blue-700">Applying OPcache action on PHP {effectiveVersion}...</p>
          </div>
        )}
        {msg && (
          <div className={`mt-4 p-3 rounded-lg border flex items-center gap-2 ${msg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
            {msg.type === 'success' ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" /> : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
            <p className={`text-sm ${msg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>{msg.text}</p>
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== ionCube Tab ====================

function IonCubeTab({ serverId }: { serverId: number }) {
  const { data: statuses = [], isLoading, refetch } = useIonCubeStatus(serverId)
  const enable = useEnableIonCube(serverId)
  const disable = useDisableIonCube(serverId)
  const [actionVersion, setActionVersion] = useState<string | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const isRunning = enable.isPending || disable.isPending

  const handleToggle = async (version: string, currentlyEnabled: boolean) => {
    setActionVersion(version)
    setMsg(null)
    try {
      if (currentlyEnabled) {
        await disable.mutateAsync(version)
        setMsg({ type: 'success', text: `ionCube disabled for PHP ${version}` })
      } else {
        await enable.mutateAsync(version)
        setMsg({ type: 'success', text: `ionCube enabled for PHP ${version}` })
      }
      refetch()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
        || (err instanceof Error ? err.message : null)
        || 'Operation failed'
      setMsg({ type: 'error', text: msg })
    } finally {
      setActionVersion(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Zap size={20} className="text-purple-500" /> ionCube Loader
        </h2>
        <p className="text-sm text-gray-500 mb-6">
          ionCube Loader decrypts and executes PHP files encoded with the ionCube PHP Encoder.
          Supported versions: PHP 7.1–8.3. PHP 8.4+ is not yet supported by ionCube.
        </p>

        {isLoading ? (
          <div className="flex items-center gap-2 text-sm text-gray-500 py-4">
            <Loader2 size={16} className="animate-spin" /> Querying ionCube status from server...
          </div>
        ) : statuses.length === 0 ? (
          <p className="text-sm text-gray-500 py-4">No PHP versions detected on this server.</p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {statuses.map(v => {
              const isActing = actionVersion === v.version && isRunning
              return (
                <div key={v.version}
                  className={`rounded-xl border-2 p-4 transition-all ${
                    !v.supported
                      ? 'border-gray-100 bg-gray-50 opacity-60'
                      : v.enabled
                      ? 'border-emerald-200 bg-emerald-50'
                      : 'border-gray-200 bg-white'
                  }`}>
                  {/* Header row */}
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-sm font-bold text-gray-800">PHP {v.version}</span>
                    {!v.supported ? (
                      <span className="text-xs px-2 py-0.5 bg-gray-200 text-gray-500 rounded-full">
                        Not Supported
                      </span>
                    ) : v.enabled ? (
                      <span className="text-xs px-2 py-0.5 bg-emerald-100 text-emerald-700 rounded-full font-medium">
                        Enabled
                      </span>
                    ) : (
                      <span className="text-xs px-2 py-0.5 bg-gray-100 text-gray-500 rounded-full">
                        Disabled
                      </span>
                    )}
                  </div>

                  {/* Loader file indicator */}
                  <p className="text-xs text-gray-400 mb-3">
                    {v.installed
                      ? <span className="text-emerald-600">✓ Loader installed</span>
                      : <span className="text-gray-400">✗ Loader not installed</span>}
                  </p>

                  {/* Action button */}
                  {v.supported ? (
                    <button
                      onClick={() => handleToggle(v.version, v.enabled)}
                      disabled={isRunning}
                      className={`w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                        v.enabled
                          ? 'bg-red-50 border border-red-200 text-red-700 hover:bg-red-100'
                          : 'bg-blue-600 text-white hover:bg-blue-700'
                      }`}>
                      {isActing
                        ? <Loader2 size={14} className="animate-spin" />
                        : v.enabled
                        ? <ToggleLeft size={14} />
                        : <ToggleRight size={14} />}
                      {isActing
                        ? (v.enabled ? 'Disabling...' : 'Enabling...')
                        : (v.enabled ? 'Disable' : 'Enable')}
                    </button>
                  ) : (
                    <div className="w-full px-3 py-2 rounded-lg text-sm text-center text-gray-400 bg-gray-100 border border-gray-200">
                      Not supported by ionCube
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}

        {msg && (
          <div className={`mt-4 p-3 rounded-lg border flex items-center gap-2 ${
            msg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'
          }`}>
            {msg.type === 'success'
              ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" />
              : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
            <p className={`text-sm ${msg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>
              {msg.text}
            </p>
          </div>
        )}
      </div>

      {/* Info card */}
      <div className="bg-purple-50 border border-purple-200 rounded-xl p-4">
        <h3 className="text-sm font-semibold text-purple-800 mb-2">About ionCube Loader</h3>
        <ul className="text-xs text-purple-700 space-y-1 list-disc list-inside">
          <li>ionCube is a Zend extension — must load <strong>before</strong> all other extensions (loaded as <code className="bg-purple-100 px-1 rounded">00-ioncube.ini</code>)</li>
          <li>The first Enable may take up to 60 seconds to download the ionCube package (~60 MB)</li>
          <li>Subsequent enables are instant — the package is cached on the server</li>
          <li>PHP-FPM is automatically reloaded after each change</li>
          <li>Disable removes the ini file only; the .so file stays cached for fast re-enable</li>
        </ul>
      </div>
    </div>
  )
}

// ==================== Config Tab ====================

const PHP_VERSIONS = ['7.4', '8.0', '8.1', '8.2', '8.3', '8.4', '8.5']

// ==================== PHP-FPM Tab ====================

function PHPFPMTab({ serverId, sites }: { serverId: number; sites: Array<{ id: number; domain: string; php_version: string }> }) {
  const [selectedSite, setSelectedSite] = useState<number | null>(null)
  const autotune = usePHPFPMAutotune(serverId)
  const installPHP = useInstallPHPVersion()
  const { data: installedVersions = [], isLoading: versionsLoading, refetch: refetchVersions } = useInstalledPHPVersions(serverId)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [installVersion, setInstallVersion] = useState('8.3')
  const [showInstall, setShowInstall] = useState(false)

  const handleAutotune = async () => {
    setMsg(null)
    try {
      await autotune.mutateAsync()
      setMsg({ type: 'success', text: 'PHP-FPM auto-tuned based on available RAM' })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Auto-tune failed' })
    }
  }

  const handleInstallPHP = async () => {
    setMsg(null)
    try {
      const result = await installPHP.mutateAsync({ serverId, version: installVersion })
      setMsg({ type: 'success', text: result.message || `PHP ${installVersion} installed successfully` })
      setShowInstall(false)
      refetchVersions()
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || `Failed to install PHP ${installVersion}` })
    }
  }

  return (
    <div className="space-y-6">
      {/* File Paths */}
      <div className="bg-gray-50 border border-gray-200 rounded-xl p-4">
        <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
          <FileText size={16} /> PHP-FPM File Paths
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs font-mono text-gray-500">
          <div><span className="text-gray-400">Pool configs:</span> /etc/php/*/fpm/pool.d/*.conf</div>
          <div><span className="text-gray-400">PHP config:</span> /etc/php/*/fpm/php.ini</div>
          <div><span className="text-gray-400">FPM config:</span> /etc/php/*/fpm/php-fpm.conf</div>
          <div><span className="text-gray-400">Error log:</span> /var/log/php*-fpm.log</div>
        </div>
      </div>

      {/* Installed PHP Versions */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Settings size={20} className="text-blue-500" /> Installed PHP Versions
          </h2>
          <button onClick={() => refetchVersions()} className="p-1.5 text-gray-400 hover:text-blue-600">
            <RefreshCw size={15} />
          </button>
        </div>
        {versionsLoading ? (
          <div className="flex items-center gap-2 py-3"><Loader2 size={16} className="animate-spin text-gray-400" /><span className="text-sm text-gray-500">Checking server...</span></div>
        ) : installedVersions.length === 0 ? (
          <p className="text-sm text-gray-400 py-3">No PHP versions detected. Click refresh or install below.</p>
        ) : (
          <div className="flex flex-wrap gap-3">
            {PHP_VERSIONS.map(v => {
              const info = installedVersions.find(iv => iv.version === v)
              const isInstalled = !!info
              return (
                <div key={v} className={`flex flex-col gap-1.5 px-4 py-3 rounded-xl border-2 min-w-[110px] ${
                  isInstalled
                    ? (info.fpm_active ? 'border-green-400 bg-green-50' : 'border-yellow-300 bg-yellow-50')
                    : 'border-gray-200 bg-gray-50'
                }`}>
                  <span className={`text-sm font-semibold ${isInstalled ? (info.fpm_active ? 'text-green-700' : 'text-yellow-700') : 'text-gray-400'}`}>
                    PHP {v}
                  </span>
                  {isInstalled ? (
                    <div className="flex flex-col gap-1">
                      <span className={`text-xs px-1.5 py-0.5 rounded font-medium w-fit ${info.fpm_active ? 'bg-green-100 text-green-700' : 'bg-yellow-100 text-yellow-700'}`}>
                        FPM: {info.fpm_active ? 'Running' : 'Stopped'}
                      </span>
                      <span className={`text-xs px-1.5 py-0.5 rounded font-medium w-fit ${info.opcache_enabled ? 'bg-blue-100 text-blue-700' : 'bg-gray-100 text-gray-500'}`}>
                        OPcache: {info.opcache_enabled ? 'ON' : 'OFF'}
                      </span>
                    </div>
                  ) : (
                    <span className="text-xs px-1.5 py-0.5 rounded bg-gray-200 text-gray-500 w-fit">Not installed</span>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* Install PHP Version */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
              <Plus size={20} className="text-blue-500" /> Install PHP Version
            </h2>
            <p className="text-sm text-gray-500 mt-1">Install additional PHP versions on this server.</p>
          </div>
          <button onClick={() => setShowInstall(!showInstall)}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Add PHP Version
          </button>
        </div>
        {showInstall && (
          <div className="flex items-center gap-3 p-4 bg-gray-50 rounded-lg">
            <div className="flex-1">
              <label className="text-xs text-gray-500 font-medium mb-1 block">PHP Version</label>
              <select value={installVersion} onChange={e => setInstallVersion(e.target.value)}
                className="w-full px-3 py-2 border rounded-lg text-sm">
                {PHP_VERSIONS.map(v => (
                  <option key={v} value={v}>PHP {v}</option>
                ))}
              </select>
            </div>
            <div className="pt-5">
              <button onClick={handleInstallPHP} disabled={installPHP.isPending}
                className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                {installPHP.isPending ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
                {installPHP.isPending ? `Installing PHP ${installVersion}...` : `Install PHP ${installVersion}`}
              </button>
            </div>
          </div>
        )}
        {installPHP.isPending && (
          <div className="mt-3 p-3 bg-blue-50 border border-blue-200 rounded-lg flex items-center gap-2">
            <Loader2 size={16} className="animate-spin text-blue-600" />
            <p className="text-sm text-blue-700">Installing PHP {installVersion}... This may take 2-5 minutes.</p>
          </div>
        )}
      </div>

      {msg && (
        <div className={`p-3 rounded-lg border ${msg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
          <p className={`text-sm flex items-center gap-2 ${msg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>
            {msg.type === 'success' ? <CheckCircle2 size={16} /> : <XCircle size={16} />}
            {msg.text}
          </p>
        </div>
      )}

      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
              <Settings size={20} className="text-indigo-500" /> PHP-FPM Pool Management
            </h2>
            <p className="text-sm text-gray-500 mt-1">
              Configure PHP-FPM pool settings and PHP version per site.
            </p>
          </div>
          <button onClick={handleAutotune} disabled={autotune.isPending}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-indigo-600 rounded-lg hover:bg-indigo-700 disabled:opacity-50">
            {autotune.isPending ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
            {autotune.isPending ? 'Tuning...' : 'Auto-tune by RAM'}
          </button>
        </div>
        {sites.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No sites on this server</p>
        ) : (
          <div className="space-y-3">
            {sites.map(site => (
              <div key={site.id}>
                <div className="p-4 border border-gray-200 rounded-lg cursor-pointer hover:bg-gray-50"
                  onClick={() => setSelectedSite(selectedSite === site.id ? null : site.id)}>
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm font-medium text-gray-900">{site.domain}</p>
                      <p className="text-xs text-gray-500 font-mono">/etc/php/{site.php_version}/fpm/pool.d/{site.domain}.conf</p>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-xs px-2 py-1 bg-indigo-100 text-indigo-700 rounded">PHP {site.php_version}</span>
                      <span className="text-xs text-gray-400">{selectedSite === site.id ? '▲' : '▼'}</span>
                    </div>
                  </div>
                </div>
                {selectedSite === site.id && (
                  <PHPFPMPoolEditor serverId={serverId} siteId={site.id} domain={site.domain} phpVersion={site.php_version} />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function PHPFPMPoolEditor({ serverId, siteId, domain, phpVersion }: { serverId: number; siteId: number; domain: string; phpVersion: string }) {
  const { data: pool, isLoading } = usePHPFPMPool(serverId, siteId)
  const updatePool = useUpdatePHPFPMPool(serverId)
  const changePHP = useChangeSitePHPVersion(serverId)
  const [form, setForm] = useState<PHPFPMPoolConfig | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [newPhpVersion, setNewPhpVersion] = useState(phpVersion)

  // Initialize form when pool data loads
  if (pool && !form) {
    setForm({ ...pool })
  }

  const handleSave = async () => {
    if (!form) return
    setMsg(null)
    try {
      await updatePool.mutateAsync({ siteId, ...form })
      setMsg({ type: 'success', text: 'Pool config updated. PHP-FPM will restart.' })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to update pool config' })
    }
  }

  const handleChangePHP = async () => {
    if (newPhpVersion === phpVersion) return
    setMsg(null)
    try {
      const result = await changePHP.mutateAsync({ siteId, newVersion: newPhpVersion })
      setMsg({ type: 'success', text: result.message || `PHP version changed to ${newPhpVersion}` })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to change PHP version' })
    }
  }

  if (isLoading || !form) {
    return (
      <div className="flex justify-center py-4 border border-t-0 border-gray-200 rounded-b-lg">
        <Loader2 size={20} className="animate-spin text-gray-400" />
      </div>
    )
  }

  return (
    <div className="border border-t-0 border-gray-200 rounded-b-lg p-4 bg-gray-50 space-y-4">
      {msg && (
        <div className={`p-2 rounded border text-sm flex items-center gap-2 ${msg.type === 'success' ? 'bg-green-50 border-green-200 text-green-700' : 'bg-red-50 border-red-200 text-red-700'}`}>
          {msg.type === 'success' ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
          {msg.text}
        </div>
      )}

      {/* PHP Version Change */}
      <div className="p-3 border border-indigo-100 bg-indigo-50 rounded-lg">
        <p className="text-xs font-semibold text-indigo-700 mb-2 flex items-center gap-1">
          <Settings size={12} /> Change PHP Version
        </p>
        <div className="flex items-center gap-3">
          <select value={newPhpVersion} onChange={e => setNewPhpVersion(e.target.value)}
            className="flex-1 px-3 py-1.5 border rounded-lg text-sm bg-white">
            {PHP_VERSIONS.map(v => (
              <option key={v} value={v}>PHP {v}</option>
            ))}
          </select>
          <button onClick={handleChangePHP}
            disabled={changePHP.isPending || newPhpVersion === phpVersion}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-white bg-indigo-600 rounded-lg hover:bg-indigo-700 disabled:opacity-50">
            {changePHP.isPending ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
            {changePHP.isPending ? 'Changing...' : 'Change'}
          </button>
        </div>
        {changePHP.isPending && (
          <p className="text-xs text-indigo-600 mt-2">Changing PHP version... may take 2-5 min if installing new version.</p>
        )}
      </div>

      <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
        <div>
          <label className="text-xs text-gray-500 font-medium">max_children</label>
          <input type="number" value={form.max_children} min={1} max={200}
            onChange={e => setForm({ ...form, max_children: parseInt(e.target.value) || 1 })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
        </div>
        <div>
          <label className="text-xs text-gray-500 font-medium">start_servers</label>
          <input type="number" value={form.start_servers} min={1}
            onChange={e => setForm({ ...form, start_servers: parseInt(e.target.value) || 1 })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
        </div>
        <div>
          <label className="text-xs text-gray-500 font-medium">min_spare_servers</label>
          <input type="number" value={form.min_spare_servers} min={1}
            onChange={e => setForm({ ...form, min_spare_servers: parseInt(e.target.value) || 1 })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
        </div>
        <div>
          <label className="text-xs text-gray-500 font-medium">max_spare_servers</label>
          <input type="number" value={form.max_spare_servers} min={1}
            onChange={e => setForm({ ...form, max_spare_servers: parseInt(e.target.value) || 1 })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
        </div>
        <div>
          <label className="text-xs text-gray-500 font-medium">max_requests</label>
          <input type="number" value={form.max_requests} min={0}
            onChange={e => setForm({ ...form, max_requests: parseInt(e.target.value) || 0 })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
        </div>
        <div>
          <label className="text-xs text-gray-500 font-medium">memory_limit</label>
          <input type="text" value={form.memory_limit}
            onChange={e => setForm({ ...form, memory_limit: e.target.value })}
            className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" placeholder="256M" />
        </div>
      </div>
      <div className="flex justify-end">
        <button onClick={handleSave} disabled={updatePool.isPending}
          className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
          {updatePool.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
          {updatePool.isPending ? 'Saving...' : 'Save & Restart PHP-FPM'}
        </button>
      </div>
    </div>
  )
}

// ==================== MariaDB Tab ====================

const DB_PRIVILEGES = ['ALL PRIVILEGES', 'SELECT', 'INSERT', 'UPDATE', 'DELETE', 'CREATE', 'DROP', 'INDEX', 'ALTER']

function MariaDBTab({ serverId }: { serverId: number }) {
  const { data: users = [], isLoading: usersLoading } = useMariaDBUsers(serverId)
  const { data: databases = [], isLoading: dbsLoading } = useMariaDBDatabases(serverId)
  const { data: config, isLoading: configLoading } = useMariaDBConfig(serverId)
  const createUser = useCreateMariaDBUser(serverId)
  const deleteUser = useDeleteMariaDBUser(serverId)
  const createDatabase = useCreateMariaDBDatabase(serverId)
  const deleteDatabase = useDeleteMariaDBDatabase(serverId)
  const updateConfig = useUpdateMariaDBConfig(serverId)
  const importDatabase = useImportMariaDBDatabase(serverId)

  const [showUserForm, setShowUserForm] = useState(false)
  const [userForm, setUserForm] = useState<MariaDBUserRequest>({
    username: '', password: '', database: '', privileges: ['ALL PRIVILEGES']
  })
  const [userMsg, setUserMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [showDbForm, setShowDbForm] = useState(false)
  const [dbForm, setDbForm] = useState<CreateDatabaseRequest>({ name: '', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' })
  const [dbMsg, setDbMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [configForm, setConfigForm] = useState<MariaDBConfig | null>(null)
  const [configMsg, setConfigMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [importTargetDb, setImportTargetDb] = useState<string | null>(null)
  const [importFile, setImportFile] = useState<File | null>(null)
  const [importMsg, setImportMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Initialize config form when data loads
  if (config && !configForm) {
    setConfigForm({ ...config })
  }

  const handleCreateDatabase = async (e: React.FormEvent) => {
    e.preventDefault()
    setDbMsg(null)
    try {
      await createDatabase.mutateAsync(dbForm)
      setDbMsg({ type: 'success', text: `Database "${dbForm.name}" created successfully` })
      setDbForm({ name: '', charset: 'utf8mb4', collation: 'utf8mb4_unicode_ci' })
      setShowDbForm(false)
    } catch (err: any) {
      setDbMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to create database' })
    }
  }

  const handleDeleteDatabase = async (dbName: string) => {
    if (!confirm(`Delete database "${dbName}"? ALL DATA WILL BE LOST and cannot be recovered!`)) return
    setDbMsg(null)
    try {
      await deleteDatabase.mutateAsync(dbName)
      setDbMsg({ type: 'success', text: `Database "${dbName}" deleted` })
    } catch (err: any) {
      setDbMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to delete database' })
    }
  }

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault()
    setUserMsg(null)
    try {
      await createUser.mutateAsync(userForm)
      setUserMsg({ type: 'success', text: `User "${userForm.username}" created successfully` })
      setUserForm({ username: '', password: '', database: '', privileges: ['ALL PRIVILEGES'] })
      setShowUserForm(false)
    } catch (err: any) {
      setUserMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to create user' })
    }
  }

  const handleDeleteUser = async (username: string) => {
    if (!confirm(`Delete MariaDB user "${username}"? This cannot be undone.`)) return
    setUserMsg(null)
    try {
      await deleteUser.mutateAsync(username)
      setUserMsg({ type: 'success', text: `User "${username}" deleted` })
    } catch (err: any) {
      setUserMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to delete user' })
    }
  }

  const handleImportDatabase = async () => {
    if (!importTargetDb || !importFile) return
    setImportMsg(null)
    try {
      await importDatabase.mutateAsync({ dbName: importTargetDb, file: importFile })
      setImportMsg({ type: 'success', text: `Database "${importTargetDb}" imported successfully` })
      setImportTargetDb(null)
      setImportFile(null)
    } catch (err: any) {
      setImportMsg({ type: 'error', text: err?.response?.data?.error || 'Import failed' })
    }
  }

  const handleSaveConfig = async () => {
    if (!configForm) return
    setConfigMsg(null)
    try {
      await updateConfig.mutateAsync(configForm)
      setConfigMsg({ type: 'success', text: 'MariaDB configuration saved. Service restarted.' })
    } catch (err: any) {
      setConfigMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to update config' })
    }
  }

  const togglePrivilege = (priv: string) => {
    if (priv === 'ALL PRIVILEGES') {
      setUserForm({ ...userForm, privileges: ['ALL PRIVILEGES'] })
      return
    }
    const current = (userForm.privileges ?? []).filter(p => p !== 'ALL PRIVILEGES')
    if (current.includes(priv)) {
      setUserForm({ ...userForm, privileges: current.filter(p => p !== priv) })
    } else {
      setUserForm({ ...userForm, privileges: [...current, priv] })
    }
  }

  return (
    <div className="space-y-6">
      {/* File Paths Info */}
      <div className="bg-gray-50 border border-gray-200 rounded-xl p-4">
        <h3 className="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
          <FileText size={16} /> MariaDB File Paths
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs font-mono text-gray-500">
          <div><span className="text-gray-400">Config:</span> /etc/mysql/mariadb.conf.d/</div>
          <div><span className="text-gray-400">Custom:</span> /etc/mysql/mariadb.conf.d/99-wp-manage.cnf</div>
          <div><span className="text-gray-400">Data:</span> /var/lib/mysql/</div>
          <div><span className="text-gray-400">Error log:</span> /var/log/mysql/error.log</div>
          <div><span className="text-gray-400">Slow log:</span> /var/log/mysql/slow-query.log</div>
        </div>
      </div>

      {/* MariaDB Configuration */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
          <Database size={20} className="text-orange-500" /> MariaDB Configuration
        </h2>

        {configLoading ? (
          <div className="flex justify-center py-6"><Loader2 size={20} className="animate-spin text-gray-400" /></div>
        ) : configForm ? (
          <div className="space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <div>
                <label className="text-xs text-gray-500 font-medium">Max Connections</label>
                <input type="number" min={10} max={1000} value={configForm.max_connections}
                  onChange={e => setConfigForm({ ...configForm, max_connections: parseInt(e.target.value) || 150 })}
                  className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium">InnoDB Buffer (MB)</label>
                <input type="number" min={64} max={65536} value={configForm.innodb_buffer_mb}
                  onChange={e => setConfigForm({ ...configForm, innodb_buffer_mb: parseInt(e.target.value) || 256 })}
                  className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium">Query Cache (MB)</label>
                <input type="number" min={0} max={2048} value={configForm.query_cache_mb}
                  onChange={e => setConfigForm({ ...configForm, query_cache_mb: parseInt(e.target.value) || 0 })}
                  className="w-full mt-1 px-3 py-2 border rounded-lg text-sm" />
              </div>
              <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg">
                <input type="checkbox" id="slow_query_log" checked={configForm.slow_query_log}
                  onChange={e => setConfigForm({ ...configForm, slow_query_log: e.target.checked })}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600" />
                <label htmlFor="slow_query_log" className="text-sm font-medium text-gray-700">Slow Query Log</label>
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium">Slow Query Time (sec)</label>
                <input type="number" min={1} max={60} value={configForm.slow_query_time}
                  onChange={e => setConfigForm({ ...configForm, slow_query_time: parseInt(e.target.value) || 2 })}
                  className="w-full mt-1 px-3 py-2 border rounded-lg text-sm"
                  disabled={!configForm.slow_query_log} />
              </div>
            </div>

            <div>
              <label className="text-xs text-gray-500 font-medium">Custom Config (appended to [mysqld])</label>
              <textarea value={configForm.custom_config || ''}
                onChange={e => setConfigForm({ ...configForm, custom_config: e.target.value })}
                placeholder="e.g. character-set-server = utf8mb4&#10;collation-server = utf8mb4_unicode_ci"
                className="w-full mt-1 px-3 py-2 border rounded-lg text-sm font-mono h-20" />
            </div>

            {configMsg && (
              <div className={`p-3 rounded-lg border flex items-center gap-2 ${configMsg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
                {configMsg.type === 'success' ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" /> : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
                <p className={`text-sm ${configMsg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>{configMsg.text}</p>
              </div>
            )}

            <div className="flex justify-end">
              <button onClick={handleSaveConfig} disabled={updateConfig.isPending}
                className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-orange-600 rounded-lg hover:bg-orange-700 disabled:opacity-50">
                {updateConfig.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
                {updateConfig.isPending ? 'Saving & Restarting...' : 'Save & Restart MariaDB'}
              </button>
            </div>
          </div>
        ) : (
          <p className="text-sm text-gray-400 text-center py-4">Failed to load config</p>
        )}
      </div>

      {/* Databases */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Database size={20} className="text-orange-500" /> Databases
          </h2>
          <button onClick={() => { setShowDbForm(!showDbForm); setDbMsg(null) }}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-orange-600 rounded-lg hover:bg-orange-700">
            <Plus size={14} /> Create Database
          </button>
        </div>

        {showDbForm && (
          <form onSubmit={handleCreateDatabase} className="space-y-3 mb-4 p-4 bg-orange-50 border border-orange-200 rounded-lg">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              <div>
                <label className="text-xs text-gray-500 font-medium mb-1 block">Database Name *</label>
                <input type="text" placeholder="e.g. wp_site1_db" value={dbForm.name}
                  onChange={e => setDbForm({ ...dbForm, name: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" required />
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium mb-1 block">Charset</label>
                <select value={dbForm.charset} onChange={e => setDbForm({ ...dbForm, charset: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm">
                  <option value="utf8mb4">utf8mb4 (Recommended)</option>
                  <option value="utf8">utf8</option>
                  <option value="latin1">latin1</option>
                </select>
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium mb-1 block">Collation</label>
                <select value={dbForm.collation} onChange={e => setDbForm({ ...dbForm, collation: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm">
                  <option value="utf8mb4_unicode_ci">utf8mb4_unicode_ci</option>
                  <option value="utf8mb4_general_ci">utf8mb4_general_ci</option>
                  <option value="utf8_general_ci">utf8_general_ci</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowDbForm(false)}
                className="px-4 py-2 text-sm text-gray-600 bg-gray-200 rounded-lg hover:bg-gray-300">Cancel</button>
              <button type="submit" disabled={createDatabase.isPending}
                className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-orange-600 rounded-lg hover:bg-orange-700 disabled:opacity-50">
                {createDatabase.isPending ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
                {createDatabase.isPending ? 'Creating...' : 'Create Database'}
              </button>
            </div>
          </form>
        )}

        {dbMsg && (
          <div className={`mb-4 p-3 rounded-lg border flex items-center gap-2 ${dbMsg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
            {dbMsg.type === 'success' ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" /> : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
            <p className={`text-sm ${dbMsg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>{dbMsg.text}</p>
          </div>
        )}

        {dbsLoading ? (
          <div className="flex justify-center py-8"><Loader2 size={20} className="animate-spin text-gray-400" /></div>
        ) : databases.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No databases found</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b">
                  <th className="pb-2 font-medium">Database</th>
                  <th className="pb-2 font-medium">Size</th>
                  <th className="pb-2 font-medium">Charset</th>
                  <th className="pb-2 font-medium">Collation</th>
                  <th className="pb-2 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {databases.map(db => (
                  <tr key={db.name} className="border-b border-gray-50">
                    <td className="py-3 font-mono font-medium text-gray-900">{db.name}</td>
                    <td className="py-3 text-gray-500 text-xs">{db.size}</td>
                    <td className="py-3 text-gray-500 text-xs font-mono">{db.charset}</td>
                    <td className="py-3 text-gray-500 text-xs font-mono">{db.collation}</td>
                    <td className="py-3">
                      <div className="flex items-center gap-1">
                        <button
                          onClick={() => { setImportTargetDb(importTargetDb === db.name ? null : db.name); setImportFile(null); setImportMsg(null) }}
                          title="Import SQL"
                          className={`p-1 rounded ${importTargetDb === db.name ? 'text-blue-600 bg-blue-50' : 'text-gray-400 hover:text-blue-600'}`}>
                          <Upload size={14} />
                        </button>
                        <button onClick={() => handleDeleteDatabase(db.name)}
                          disabled={deleteDatabase.isPending}
                          className="p-1 text-gray-400 hover:text-red-600 disabled:opacity-50">
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Import SQL Panel */}
        {importTargetDb && (
          <div className="mt-4 p-4 bg-blue-50 border border-blue-200 rounded-lg space-y-3">
            <h3 className="text-sm font-semibold text-blue-800 flex items-center gap-2">
              <Upload size={14} /> Import SQL into <span className="font-mono">{importTargetDb}</span>
            </h3>
            <p className="text-xs text-blue-600">Supported formats: <code>.sql</code>, <code>.sql.gz</code>, <code>.gz</code>, <code>.zip</code>. <strong>Existing data will NOT be dropped</strong> — the SQL will be applied on top. Max 512 MB.</p>
            <div className="flex items-center gap-3">
              <label className="flex-1">
                <input type="file" accept=".sql,.gz,.sql.gz,.zip" className="hidden"
                  onChange={e => { setImportFile(e.target.files?.[0] ?? null); setImportMsg(null) }} />
                <div className="flex items-center gap-2 px-3 py-2 border-2 border-dashed border-blue-300 rounded-lg cursor-pointer hover:border-blue-400 bg-white text-sm text-blue-600">
                  <Upload size={14} />
                  {importFile ? importFile.name : 'Click to choose file (.sql / .gz / .zip)'}
                  {importFile && <span className="text-xs text-gray-400 ml-auto">{(importFile.size / 1024 / 1024).toFixed(1)} MB</span>}
                </div>
              </label>
              <button
                onClick={handleImportDatabase}
                disabled={!importFile || importDatabase.isPending}
                className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 whitespace-nowrap">
                {importDatabase.isPending ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
                {importDatabase.isPending ? 'Importing...' : 'Import'}
              </button>
              <button onClick={() => { setImportTargetDb(null); setImportFile(null); setImportMsg(null) }}
                className="px-3 py-2 text-sm text-gray-600 bg-gray-200 rounded-lg hover:bg-gray-300">
                Cancel
              </button>
            </div>
            {importMsg && (
              <div className={`p-3 rounded-lg border flex items-center gap-2 ${importMsg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
                {importMsg.type === 'success' ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" /> : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
                <p className={`text-sm ${importMsg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>{importMsg.text}</p>
              </div>
            )}
            {importDatabase.isPending && (
              <p className="text-xs text-blue-500">Importing... This may take a while for large files (up to 5 minutes).</p>
            )}
          </div>
        )}
      </div>

      {/* Database Users */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Key size={20} className="text-orange-500" /> Database Users
          </h2>
          <button onClick={() => { setShowUserForm(!showUserForm); setUserMsg(null) }}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-orange-600 rounded-lg hover:bg-orange-700">
            <Plus size={14} /> Create User
          </button>
        </div>

        {showUserForm && (
          <form onSubmit={handleCreateUser} className="space-y-3 mb-4 p-4 bg-orange-50 border border-orange-200 rounded-lg">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-gray-500 font-medium mb-1 block">Username *</label>
                <input type="text" placeholder="e.g. wp_user" value={userForm.username}
                  onChange={e => setUserForm({ ...userForm, username: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" required />
              </div>
              <div>
                <label className="text-xs text-gray-500 font-medium mb-1 block">Password (leave blank to auto-generate)</label>
                <input type="password" placeholder="Auto-generate if empty" value={userForm.password || ''}
                  onChange={e => setUserForm({ ...userForm, password: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" />
              </div>
            </div>
            <div>
              <label className="text-xs text-gray-500 font-medium mb-1 block">Grant access to database (optional)</label>
              {databases.length > 0 ? (
                <select value={userForm.database || ''}
                  onChange={e => setUserForm({ ...userForm, database: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm">
                  <option value="">-- No database access --</option>
                  {databases.map(db => (
                    <option key={db.name} value={db.name}>{db.name}</option>
                  ))}
                </select>
              ) : (
                <input type="text" placeholder="e.g. wp_site1_db" value={userForm.database || ''}
                  onChange={e => setUserForm({ ...userForm, database: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" />
              )}
            </div>
            {userForm.database && (
              <div>
                <label className="text-xs text-gray-500 font-medium mb-2 block">Privileges</label>
                <div className="flex flex-wrap gap-2">
                  {DB_PRIVILEGES.map(priv => (
                    <label key={priv} className="flex items-center gap-1.5 cursor-pointer">
                      <input type="checkbox"
                        checked={(userForm.privileges ?? []).includes(priv)}
                        onChange={() => togglePrivilege(priv)}
                        className="h-3.5 w-3.5 rounded border-gray-300 text-orange-600" />
                      <span className="text-xs text-gray-700">{priv}</span>
                    </label>
                  ))}
                </div>
              </div>
            )}
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowUserForm(false)}
                className="px-4 py-2 text-sm text-gray-600 bg-gray-200 rounded-lg hover:bg-gray-300">Cancel</button>
              <button type="submit" disabled={createUser.isPending}
                className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-orange-600 rounded-lg hover:bg-orange-700 disabled:opacity-50">
                {createUser.isPending ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
                {createUser.isPending ? 'Creating...' : 'Create User'}
              </button>
            </div>
          </form>
        )}

        {userMsg && (
          <div className={`mb-4 p-3 rounded-lg border flex items-center gap-2 ${userMsg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
            {userMsg.type === 'success' ? <CheckCircle2 size={16} className="text-green-600 flex-shrink-0" /> : <XCircle size={16} className="text-red-600 flex-shrink-0" />}
            <p className={`text-sm ${userMsg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>{userMsg.text}</p>
          </div>
        )}

        {usersLoading ? (
          <div className="flex justify-center py-8"><Loader2 size={20} className="animate-spin text-gray-400" /></div>
        ) : users.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No database users found</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b">
                  <th className="pb-2 font-medium">Username</th>
                  <th className="pb-2 font-medium">Host</th>
                  <th className="pb-2 font-medium">Databases</th>
                  <th className="pb-2 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {users.map(user => (
                  <tr key={user.username} className="border-b border-gray-50">
                    <td className="py-3 font-mono font-medium text-gray-900">{user.username}</td>
                    <td className="py-3 font-mono text-gray-500">{user.host}</td>
                    <td className="py-3">
                      {user.databases && user.databases.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {user.databases.map(db => (
                            <span key={db} className="px-2 py-0.5 bg-orange-100 text-orange-700 rounded text-xs font-mono">{db}</span>
                          ))}
                        </div>
                      ) : (
                        <span className="text-xs text-gray-400">No database access</span>
                      )}
                    </td>
                    <td className="py-3">
                      <button onClick={() => handleDeleteUser(user.username)}
                        disabled={deleteUser.isPending}
                        className="p-1 text-gray-400 hover:text-red-600 disabled:opacity-50">
                        <Trash2 size={14} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== Cron Jobs Tab ====================

const CRON_PRESETS = [
  { label: 'Every minute', value: '* * * * *' },
  { label: 'Every 5 min', value: '*/5 * * * *' },
  { label: 'Every 15 min', value: '*/15 * * * *' },
  { label: 'Every 30 min', value: '*/30 * * * *' },
  { label: 'Hourly', value: '0 * * * *' },
  { label: 'Daily midnight', value: '0 0 * * *' },
  { label: 'Daily 2am', value: '0 2 * * *' },
  { label: 'Weekly (Sun)', value: '0 0 * * 0' },
  { label: 'Monthly', value: '0 0 1 * *' },
]

function CronJobCard({ job, serverId }: { job: CronJob; serverId: number }) {
  const toggleJob = useToggleCronJob()
  const runJob = useRunCronJob()
  const deleteJob = useDeleteCronJob()
  const updateJob = useUpdateCronJob()

  const [showLogs, setShowLogs] = useState(false)
  const [logs, setLogs] = useState('')
  const [logsLoading, setLogsLoading] = useState(false)
  const [runOutput, setRunOutput] = useState('')
  const [editing, setEditing] = useState(false)
  const [editForm, setEditForm] = useState({ schedule: job.schedule, command: job.command, user: job.user, comment: job.comment || '' })

  const fetchLogs = async () => {
    setLogsLoading(true)
    setShowLogs(true)
    try {
      const { data } = await api.get(`/servers/${serverId}/cron-jobs/${job.id}/logs?lines=100`)
      setLogs(data.data?.logs ?? '(no logs yet)')
    } catch {
      setLogs('Failed to fetch logs')
    } finally {
      setLogsLoading(false)
    }
  }

  const handleRunNow = async () => {
    setRunOutput('Running...')
    try {
      const result = await runJob.mutateAsync({ serverId, jobId: job.id })
      setRunOutput(result.data?.output ?? result.message ?? 'Done')
    } catch (e: any) {
      setRunOutput(e?.response?.data?.error ?? 'Failed')
    }
  }

  const handleSaveEdit = async (e: React.FormEvent) => {
    e.preventDefault()
    await updateJob.mutateAsync({ serverId, jobId: job.id, ...editForm })
    setEditing(false)
  }

  return (
    <div className={`border rounded-lg overflow-hidden ${job.enabled ? 'border-gray-200' : 'border-gray-100 bg-gray-50'}`}>
      {/* Header row */}
      <div className="p-3 flex items-center justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <code className="text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded font-mono border border-blue-100">{job.schedule}</code>
            <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${job.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-400'}`}>
              {job.enabled ? '● Active' : '○ Disabled'}
            </span>
            {job.comment && <span className="text-xs text-gray-400 italic">{job.comment}</span>}
          </div>
          <p className="text-sm font-mono text-gray-700 mt-1 truncate">{job.command}</p>
          <p className="text-xs text-gray-400 mt-0.5">User: <span className="font-mono">{job.user}</span></p>
        </div>

        {/* Action buttons */}
        <div className="flex items-center gap-1 shrink-0">
          {/* Toggle */}
          <button
            onClick={() => toggleJob.mutate({ serverId, jobId: job.id })}
            disabled={toggleJob.isPending}
            title={job.enabled ? 'Disable' : 'Enable'}
            className={`px-2 py-1 text-xs rounded font-medium transition-colors ${job.enabled ? 'bg-yellow-50 text-yellow-700 hover:bg-yellow-100' : 'bg-green-50 text-green-700 hover:bg-green-100'} disabled:opacity-50`}>
            {job.enabled ? 'Disable' : 'Enable'}
          </button>
          {/* Run Now */}
          <button
            onClick={handleRunNow}
            disabled={runJob.isPending}
            title="Run now"
            className="p-1.5 text-gray-400 hover:text-blue-600 hover:bg-blue-50 rounded transition-colors"
            aria-label="Run now">
            <Play size={14} />
          </button>
          {/* Edit */}
          <button
            onClick={() => { setEditing(!editing); setEditForm({ schedule: job.schedule, command: job.command, user: job.user, comment: job.comment || '' }) }}
            title="Edit"
            className="p-1.5 text-gray-400 hover:text-purple-600 hover:bg-purple-50 rounded transition-colors">
            <Settings size={14} />
          </button>
          {/* Logs */}
          <button
            onClick={() => showLogs ? setShowLogs(false) : fetchLogs()}
            title="View logs"
            className={`p-1.5 rounded transition-colors ${showLogs ? 'text-green-600 bg-green-50' : 'text-gray-400 hover:text-green-600 hover:bg-green-50'}`}>
            <FileText size={14} />
          </button>
          {/* Delete */}
          <button
            onClick={() => { if (confirm('Delete this cron job?')) deleteJob.mutate({ serverId, jobId: job.id }) }}
            title="Delete"
            className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded transition-colors">
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      {/* Edit form */}
      {editing && (
        <form onSubmit={handleSaveEdit} className="px-3 pb-3 border-t border-gray-100 pt-3 bg-purple-50 space-y-2">
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="text-xs text-gray-500 mb-1 block">Schedule</label>
              <input value={editForm.schedule} onChange={e => setEditForm({...editForm, schedule: e.target.value})}
                className="w-full px-2 py-1.5 border rounded text-xs font-mono" required />
              <div className="flex flex-wrap gap-1 mt-1">
                {CRON_PRESETS.map(p => (
                  <button key={p.value} type="button" onClick={() => setEditForm({...editForm, schedule: p.value})}
                    className="text-xs px-1.5 py-0.5 bg-white border rounded hover:bg-blue-50 hover:border-blue-300 text-gray-600">
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="text-xs text-gray-500 mb-1 block">User</label>
              <input value={editForm.user} onChange={e => setEditForm({...editForm, user: e.target.value})}
                className="w-full px-2 py-1.5 border rounded text-xs font-mono" />
            </div>
          </div>
          <div>
            <label className="text-xs text-gray-500 mb-1 block">Command</label>
            <input value={editForm.command} onChange={e => setEditForm({...editForm, command: e.target.value})}
              className="w-full px-2 py-1.5 border rounded text-xs font-mono" required />
          </div>
          <div>
            <label className="text-xs text-gray-500 mb-1 block">Comment</label>
            <input value={editForm.comment} onChange={e => setEditForm({...editForm, comment: e.target.value})}
              className="w-full px-2 py-1.5 border rounded text-xs" placeholder="Optional description" />
          </div>
          <div className="flex gap-2 justify-end">
            <button type="button" onClick={() => setEditing(false)} className="px-3 py-1 text-xs border rounded hover:bg-gray-50">Cancel</button>
            <button type="submit" disabled={updateJob.isPending} className="px-3 py-1 text-xs bg-purple-600 text-white rounded hover:bg-purple-700 disabled:opacity-50">
              {updateJob.isPending ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </form>
      )}

      {/* Run output */}
      {runOutput && (
        <div className="px-3 pb-3 border-t border-gray-100 pt-2">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-medium text-gray-500">Run output</span>
            <button onClick={() => setRunOutput('')} className="text-xs text-gray-400 hover:text-gray-600">✕</button>
          </div>
          <pre className="text-xs font-mono bg-gray-900 text-green-400 p-2 rounded overflow-x-auto max-h-32 whitespace-pre-wrap">{runOutput}</pre>
        </div>
      )}

      {/* Execution logs */}
      {showLogs && (
        <div className="px-3 pb-3 border-t border-gray-100 pt-2">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-medium text-gray-500">Execution logs (last 100 lines)</span>
            <div className="flex gap-2">
              <button onClick={fetchLogs} className="text-xs text-blue-500 hover:text-blue-700">Refresh</button>
              <button onClick={() => setShowLogs(false)} className="text-xs text-gray-400 hover:text-gray-600">✕</button>
            </div>
          </div>
          {logsLoading ? (
            <div className="flex justify-center py-4"><div className="animate-spin rounded-full h-4 w-4 border-b-2 border-green-600" /></div>
          ) : (
            <pre className="text-xs font-mono bg-gray-900 text-green-400 p-2 rounded overflow-x-auto max-h-48 whitespace-pre-wrap">{logs || '(no logs yet)'}</pre>
          )}
        </div>
      )}
    </div>
  )
}

function CronJobsTab({ serverId }: { serverId: number }) {
  const { data: jobs = [], isLoading } = useCronJobsByServer(serverId)
  const createJob = useCreateCronJob()
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateCronJobRequest>({
    command: '', schedule: '', user: 'root', comment: ''
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createJob.mutateAsync({ serverId, ...form })
    setForm({ command: '', schedule: '', user: 'root', comment: '' })
    setShowForm(false)
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Terminal size={20} className="text-green-600" /> Cron Jobs
          <span className="text-sm font-normal text-gray-400">({jobs.length} jobs, managed via /etc/cron.d/wp-manage)</span>
        </h2>
        <button onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
          <Plus size={14} /> Add Cron Job
        </button>
      </div>

      {showForm && (
        <form onSubmit={handleCreate} className="space-y-3 mb-5 p-4 bg-blue-50 rounded-lg border border-blue-100">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-gray-600 mb-1 block font-medium">Schedule *</label>
              <input type="text" placeholder="*/5 * * * *" value={form.schedule}
                onChange={e => setForm({...form, schedule: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm font-mono" required />
              {/* Presets */}
              <div className="flex flex-wrap gap-1 mt-1.5">
                {CRON_PRESETS.map(p => (
                  <button key={p.value} type="button" onClick={() => setForm({...form, schedule: p.value})}
                    className={`text-xs px-2 py-0.5 rounded border transition-colors ${form.schedule === p.value ? 'bg-blue-600 text-white border-blue-600' : 'bg-white border-gray-200 text-gray-600 hover:border-blue-400 hover:text-blue-600'}`}>
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="text-xs text-gray-600 mb-1 block font-medium">Run as User</label>
              <input type="text" placeholder="root" value={form.user || 'root'}
                onChange={e => setForm({...form, user: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm font-mono" />
            </div>
          </div>
          <div>
            <label className="text-xs text-gray-600 mb-1 block font-medium">Command *</label>
            <input type="text" placeholder="/usr/bin/php /var/www/mysite/public/cron.php" value={form.command}
              onChange={e => setForm({...form, command: e.target.value})}
              className="w-full px-3 py-2 border rounded-lg text-sm font-mono" required />
          </div>
          <div className="flex gap-3">
            <div className="flex-1">
              <label className="text-xs text-gray-600 mb-1 block font-medium">Comment (optional)</label>
              <input type="text" placeholder="e.g. WP Heartbeat" value={form.comment || ''}
                onChange={e => setForm({...form, comment: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div className="flex items-end gap-2">
              <button type="button" onClick={() => setShowForm(false)} className="px-3 py-2 text-sm border rounded-lg hover:bg-gray-50">Cancel</button>
              <button type="submit" disabled={createJob.isPending}
                className="px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                {createJob.isPending ? 'Creating...' : 'Create Job'}
              </button>
            </div>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : jobs.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          <Terminal size={32} className="mx-auto mb-2 opacity-30" />
          <p className="text-sm">No cron jobs configured</p>
          <p className="text-xs mt-1">Click "Add Cron Job" to schedule recurring tasks</p>
        </div>
      ) : (
        <div className="space-y-2">
          {jobs.map(job => (
            <CronJobCard key={job.id} job={job} serverId={serverId} />
          ))}
        </div>
      )}
    </div>
  )
}

// ==================== Config Tab ====================

function LogsTab({ serverId }: { serverId: number }) {
  const [logType, setLogType] = useState('nginx_access')
  const [lines, setLines] = useState(100)
  const fetchLogs = useServerLogs()

  const logTypes = [
    { id: 'nginx_access', label: 'Nginx Access' },
    { id: 'nginx_error', label: 'Nginx Error' },
    { id: 'php_error', label: 'PHP-FPM Error' },
    { id: 'mysql_error', label: 'MySQL/MariaDB Error' },
    { id: 'syslog', label: 'System Log' },
  ]

  const handleFetch = () => {
    fetchLogs.mutate({ serverId, logType, lines })
  }

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
          <FileText size={20} className="text-blue-500" /> System Logs
        </h2>

        <div className="flex items-center gap-3 mb-4 flex-wrap">
          <select
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm"
            value={logType}
            onChange={(e) => setLogType(e.target.value)}
          >
            {logTypes.map(t => (
              <option key={t.id} value={t.id}>{t.label}</option>
            ))}
          </select>
          <select
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm"
            value={lines}
            onChange={(e) => setLines(Number(e.target.value))}
          >
            <option value={50}>Last 50 lines</option>
            <option value={100}>Last 100 lines</option>
            <option value={200}>Last 200 lines</option>
            <option value={500}>Last 500 lines</option>
          </select>
          <button
            onClick={handleFetch}
            disabled={fetchLogs.isPending}
            className="flex items-center gap-2 px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            <RefreshCw size={14} className={fetchLogs.isPending ? 'animate-spin' : ''} />
            {fetchLogs.isPending ? 'Fetching...' : 'Fetch Logs'}
          </button>
          {fetchLogs.data && (
            <span className="text-xs text-gray-400">
              {fetchLogs.data.split('\n').length} lines loaded
            </span>
          )}
        </div>

        {fetchLogs.isError && (
          <div className="mb-3 p-3 bg-red-50 border border-red-200 rounded-lg">
            <p className="text-sm text-red-700">{String(fetchLogs.error)}</p>
          </div>
        )}

        <div className="bg-gray-900 rounded-lg p-4 min-h-[300px] max-h-[600px] overflow-auto">
          <pre className="text-xs text-green-400 font-mono whitespace-pre-wrap">
            {fetchLogs.isPending
              ? 'Fetching logs from server...'
              : fetchLogs.data
              ? fetchLogs.data
              : 'Click "Fetch Logs" to retrieve server logs...'}
          </pre>
        </div>
      </div>
    </div>
  )
}

// ==================== Alerts Tab ====================

function AlertsTab({ serverId }: { serverId: number }) {
  const { data: rules = [], isLoading } = useAlertRules(serverId)
  const { data: history = [] } = useAlertHistory(serverId)
  const createRule = useCreateAlertRule(serverId)
  const deleteRule = useDeleteAlertRule(serverId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateAlertRuleRequest>({
    metric: 'cpu', threshold: 90, channel: 'telegram', cooldown_min: 30
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createRule.mutateAsync(form)
      setForm({ metric: 'cpu', threshold: 90, channel: 'telegram', cooldown_min: 30 })
      setShowForm(false)
    } catch {}
  }

  const metricLabels: Record<string, string> = { cpu: 'CPU', ram: 'RAM', disk: 'Disk' }
  const channelLabels: Record<string, string> = { telegram: 'Telegram', email: 'Email' }

  return (
    <div className="space-y-6">
      {/* Alert Rules */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Bell size={20} className="text-orange-500" /> Alert Rules
          </h2>
          <button onClick={() => setShowForm(!showForm)}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Add Rule
          </button>
        </div>

        {showForm && (
          <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 space-y-3">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Metric</label>
                <select value={form.metric} onChange={e => setForm({ ...form, metric: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm">
                  <option value="cpu">CPU Usage</option>
                  <option value="ram">RAM Usage</option>
                  <option value="disk">Disk Usage</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Threshold (%)</label>
                <input type="number" min={1} max={100} value={form.threshold}
                  onChange={e => setForm({ ...form, threshold: Number(e.target.value) })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Channel</label>
                <select value={form.channel} onChange={e => setForm({ ...form, channel: e.target.value })}
                  className="w-full px-3 py-2 border rounded-lg text-sm">
                  <option value="telegram">Telegram</option>
                  <option value="email">Email</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Cooldown (min)</label>
                <input type="number" min={5} value={form.cooldown_min || 30}
                  onChange={e => setForm({ ...form, cooldown_min: Number(e.target.value) })}
                  className="w-full px-3 py-2 border rounded-lg text-sm" />
              </div>
            </div>
            {form.channel === 'email' && (
              <input type="email" placeholder="Email address" value={form.email || ''}
                onChange={e => setForm({ ...form, email: e.target.value })}
                className="w-full px-3 py-2 border rounded-lg text-sm" />
            )}
            <div className="flex gap-2">
              <button type="submit" disabled={createRule.isPending}
                className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {createRule.isPending ? 'Creating...' : 'Create Rule'}
              </button>
              <button type="button" onClick={() => setShowForm(false)}
                className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
                Cancel
              </button>
            </div>
          </form>
        )}

        {isLoading ? (
          <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
        ) : rules.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No alert rules configured</p>
        ) : (
          <div className="space-y-2">
            {rules.map(rule => (
              <div key={rule.id} className="flex items-center justify-between p-3 border border-gray-100 rounded-lg">
                <div className="flex items-center gap-3">
                  <AlertTriangle size={16} className="text-orange-500" />
                  <div>
                    <p className="text-sm font-medium">
                      {metricLabels[rule.metric] || rule.metric} &gt; {rule.threshold}%
                    </p>
                    <p className="text-xs text-gray-500">
                      via {channelLabels[rule.channel] || rule.channel} · cooldown {rule.cooldown_min}m
                      {rule.last_fired && ` · last fired ${new Date(rule.last_fired).toLocaleString()}`}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className={`px-2 py-0.5 rounded text-xs ${rule.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                    {rule.enabled ? 'Active' : 'Disabled'}
                  </span>
                  <button onClick={() => { if (confirm('Delete this alert rule?')) deleteRule.mutate(rule.id) }}
                    className="p-1 text-gray-400 hover:text-red-600">
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Alert History */}
      {history.length > 0 && (
        <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
          <h3 className="text-lg font-semibold text-gray-900 mb-3">Alert History</h3>
          <div className="space-y-2">
            {history.slice(0, 20).map(alert => (
              <div key={alert.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                <div>
                  <p className="text-sm text-gray-700">{alert.message}</p>
                  <p className="text-xs text-gray-400">{new Date(alert.created_at).toLocaleString()}</p>
                </div>
                <span className={`px-2 py-0.5 rounded text-xs ${alert.sent ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                  {alert.sent ? 'Sent' : 'Failed'}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

    </div>
  )
}

// ==================== SSH Keys Tab ====================

function SSHKeysTab({ serverId }: { serverId: number }) {
  const { can } = usePermissions()
  const canManage = can('infrastructure', 'manage')
  const { data: keys = [], isLoading } = useSSHKeys(serverId)
  const createKey = useCreateSSHKey(serverId)
  const deleteKey = useDeleteSSHKey(serverId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateSSHKeyRequest>({ name: '', public_key: '' })
  const [copied, setCopied] = useState<number | null>(null)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createKey.mutateAsync(form)
      setForm({ name: '', public_key: '' })
      setShowForm(false)
    } catch {}
  }

  const copyFingerprint = (id: number, fingerprint: string) => {
    navigator.clipboard.writeText(fingerprint)
    setCopied(id)
    setTimeout(() => setCopied(null), 2000)
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Key size={20} className="text-yellow-500" /> SSH Keys
        </h2>
        {canManage && (
        <button onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
          <Plus size={14} /> Add Key
        </button>
        )}
      </div>

      <p className="text-sm text-gray-500 mb-4">
        Manage SSH public keys for root access to this server. Keys are added to <code className="bg-gray-100 px-1 py-0.5 rounded text-xs">/root/.ssh/authorized_keys</code>.
      </p>

      {showForm && (
        <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 space-y-3">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Key Name</label>
            <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"
              placeholder="e.g. My Laptop" required />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Public Key</label>
            <textarea value={form.public_key} onChange={e => setForm({ ...form, public_key: e.target.value })}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"
              placeholder="ssh-rsa AAAAB3NzaC1yc2E... user@host"
              rows={3} required />
          </div>
          <div className="flex gap-2">
            <button type="submit" disabled={createKey.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createKey.isPending ? 'Adding...' : 'Add Key'}
            </button>
            <button type="button" onClick={() => setShowForm(false)}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : keys.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No SSH keys configured</p>
      ) : (
        <div className="space-y-3">
          {keys.map(key => (
            <div key={key.id} className="flex items-center justify-between p-4 border border-gray-100 rounded-lg hover:bg-gray-50">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <Key size={16} className="text-gray-400 flex-shrink-0" />
                  <p className="text-sm font-medium text-gray-900">{key.name}</p>
                </div>
                <div className="flex items-center gap-2 mt-1 ml-6">
                  <code className="text-xs text-gray-500 font-mono truncate">{key.fingerprint}</code>
                  <button onClick={() => copyFingerprint(key.id, key.fingerprint)}
                    className="p-0.5 text-gray-400 hover:text-gray-600 flex-shrink-0">
                    {copied === key.id ? <Check size={12} className="text-green-500" /> : <Copy size={12} />}
                  </button>
                </div>
                <p className="text-xs text-gray-400 mt-1 ml-6">Added {new Date(key.created_at).toLocaleDateString()}</p>
              </div>
              <button onClick={() => { if (confirm(`Remove SSH key "${key.name}"?`)) deleteKey.mutate(key.id) }}
                className="p-2 text-gray-400 hover:text-red-600 flex-shrink-0">
                <Trash2 size={16} />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ==================== Config Tab ====================

function ConfigTab({ serverId }: { serverId: number }) {
  const rebuildConfig = useRebuildConfig()
  const healthCheck = useRunHealthCheck()
  const { data: sitesData } = useSites(serverId)
  const sites = sitesData?.data ?? []

  // Selected site: undefined = All Sites
  const [selectedSiteId, setSelectedSiteId] = useState<number | undefined>(undefined)

  const selectedSite = sites.find(s => s.id === selectedSiteId)

  const handleRebuild = (types: string[]) => {
    rebuildConfig.mutate({
      serverId,
      config_types: types,
      site_id: selectedSiteId,
    })
  }

  // Parse rebuild output JSON
  const rebuildResults: { domain?: string; config_type: string; success: boolean; message: string }[] = (() => {
    const output = (rebuildConfig.data?.data as { output?: string } | undefined)?.output ?? ''
    if (!output) return []
    try { return JSON.parse(output) } catch { return [] }
  })()

  const hcResults: HealthCheckResult[] = healthCheck.data ?? []

  const configTypeLabel = (ct: string) =>
    ct === 'nginx_vhost' ? 'Nginx Vhost' : ct === 'php_fpm_pool' ? 'PHP-FPM Pool' : ct

  const statusIcon = (status: string) => {
    if (status === 'ok') return <CheckCircle2 size={15} className="text-green-500 flex-shrink-0 mt-0.5" />
    if (status === 'error') return <XCircle size={15} className="text-red-500 flex-shrink-0 mt-0.5" />
    return <AlertTriangle size={15} className="text-yellow-500 flex-shrink-0 mt-0.5" />
  }

  return (
    <div className="space-y-6">
      {/* Rebuild Config */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Wrench size={20} className="text-purple-500" /> Rebuild Config
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Rebuild Nginx vhost / PHP-FPM pool configs từ template. Tự động test (<code className="bg-gray-100 px-1 rounded">nginx -t</code>,{' '}
          <code className="bg-gray-100 px-1 rounded">php-fpm -t</code>) và rollback nếu lỗi.
        </p>

        {/* Site Selector */}
        <div className="mb-4">
          <label className="block text-xs font-medium text-gray-500 mb-1.5">Chọn site cần rebuild</label>
          <select
            className="border border-gray-200 rounded-lg px-3 py-2 text-sm w-full max-w-sm focus:outline-none focus:ring-2 focus:ring-purple-300"
            value={selectedSiteId ?? ''}
            onChange={e => setSelectedSiteId(e.target.value ? Number(e.target.value) : undefined)}
          >
            <option value="">🌐 Tất cả sites trên server ({sites.length} sites)</option>
            {sites.map(s => (
              <option key={s.id} value={s.id}>
                {s.domain} — PHP {s.php_version || '8.2'}
              </option>
            ))}
          </select>
          {selectedSite && (
            <p className="text-xs text-gray-400 mt-1">
              Domain: <span className="font-mono">{selectedSite.domain}</span> · PHP{' '}
              <span className="font-semibold text-blue-600">{selectedSite.php_version || '8.2'}</span>
            </p>
          )}
          {!selectedSiteId && sites.length > 0 && (
            <p className="text-xs text-amber-600 mt-1">
              ⚠ Rebuild all sẽ chạy lần lượt qua {sites.length} sites — có thể mất vài giây
            </p>
          )}
        </div>

        {/* Action Buttons */}
        <div className="flex flex-wrap gap-3">
          <button
            onClick={() => handleRebuild(['nginx_vhost'])}
            disabled={rebuildConfig.isPending}
            className="flex items-center gap-2 px-4 py-2.5 text-sm border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50 transition-colors"
          >
            <Globe size={15} className="text-green-600" />
            Rebuild Nginx Vhost
            {selectedSite && <span className="text-xs text-gray-400">({selectedSite.domain})</span>}
          </button>
          <button
            onClick={() => handleRebuild(['php_fpm_pool'])}
            disabled={rebuildConfig.isPending}
            className="flex items-center gap-2 px-4 py-2.5 text-sm border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50 transition-colors"
          >
            <Zap size={15} className="text-blue-600" />
            Rebuild PHP-FPM Pool
            {selectedSite && <span className="text-xs text-gray-400">(PHP {selectedSite.php_version || '8.2'})</span>}
          </button>
          <button
            onClick={() => handleRebuild(['nginx_vhost', 'php_fpm_pool'])}
            disabled={rebuildConfig.isPending}
            className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-purple-600 rounded-lg hover:bg-purple-700 disabled:opacity-50 transition-colors"
          >
            {rebuildConfig.isPending ? <Loader2 size={15} className="animate-spin" /> : <Wrench size={15} />}
            {rebuildConfig.isPending ? 'Rebuilding...' : 'Rebuild All Configs'}
          </button>
        </div>

        {/* Results */}
        {rebuildConfig.isSuccess && rebuildResults.length > 0 && (
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between">
              <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Kết quả rebuild</p>
              <span className="text-xs text-gray-400">
                {rebuildResults.filter(r => r.success).length}/{rebuildResults.length} thành công
              </span>
            </div>
            {rebuildResults.map((r, i) => (
              <div key={i} className={`flex items-start gap-2 p-2.5 rounded-lg border text-sm ${
                r.success ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'
              }`}>
                {r.success
                  ? <CheckCircle2 size={15} className="text-green-500 mt-0.5 flex-shrink-0" />
                  : <XCircle size={15} className="text-red-500 mt-0.5 flex-shrink-0" />}
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-gray-800">{configTypeLabel(r.config_type)}</span>
                    {r.domain && <span className="text-xs text-gray-400 font-mono">{r.domain}</span>}
                  </div>
                  <p className={`text-xs mt-0.5 ${r.success ? 'text-green-700' : 'text-red-700 font-mono break-all'}`}>
                    {r.message}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}

        {rebuildConfig.isSuccess && rebuildResults.length === 0 && (
          <div className="mt-4 p-3 bg-amber-50 border border-amber-200 rounded-lg">
            <p className="text-sm text-amber-700">Không có site nào để rebuild (server chưa có site nào).</p>
          </div>
        )}

        {rebuildConfig.isError && (
          <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded-lg">
            <p className="text-sm text-red-700">{String(rebuildConfig.error)}</p>
          </div>
        )}
      </div>

      {/* Health Check */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <HeartPulse size={20} className="text-red-500" /> Health Check
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Kiểm tra tất cả services: <code className="bg-gray-100 px-1 rounded text-xs">nginx -t</code>,{' '}
          <code className="bg-gray-100 px-1 rounded text-xs">php-fpm -t</code> (tất cả versions đã cài),
          và kết nối MariaDB.
        </p>
        <button
          onClick={() => healthCheck.mutate(serverId)}
          disabled={healthCheck.isPending}
          className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50"
        >
          {healthCheck.isPending ? <Loader2 size={16} className="animate-spin" /> : <HeartPulse size={16} />}
          {healthCheck.isPending ? 'Đang kiểm tra...' : 'Run Health Check'}
        </button>

        {hcResults.length > 0 && (
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between">
              <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Kết quả</p>
              <span className={`text-xs font-medium ${
                hcResults.every(r => r.status === 'ok') ? 'text-green-600' : 'text-red-600'
              }`}>
                {hcResults.filter(r => r.status === 'ok').length}/{hcResults.length} passed
              </span>
            </div>
            {hcResults.map((r, i) => (
              <div key={i} className={`flex items-start gap-2 p-2.5 rounded-lg border text-sm ${
                r.status === 'ok' ? 'bg-green-50 border-green-200' :
                r.status === 'error' ? 'bg-red-50 border-red-200' :
                'bg-yellow-50 border-yellow-200'
              }`}>
                {statusIcon(r.status)}
                <div className="min-w-0">
                  <span className="font-medium text-gray-800">{r.config_type}</span>
                  {r.file_path && <span className="text-gray-400 text-xs ml-1">({r.file_path})</span>}
                  <p className={`text-xs mt-0.5 ${
                    r.status === 'ok' ? 'text-green-700' :
                    r.status === 'error' ? 'text-red-700 font-mono break-all' :
                    'text-yellow-700'
                  }`}>{r.message}</p>
                </div>
              </div>
            ))}
          </div>
        )}

        {healthCheck.isError && (
          <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded-lg">
            <p className="text-sm text-red-700">{String(healthCheck.error)}</p>
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== Storage Tab ====================

function StorageTab({ serverId }: { serverId: number }) {
  const { data: creds = [], isLoading } = useStorageCredentials(serverId)
  const createCred = useCreateStorageCredential(serverId)
  const deleteCred = useDeleteStorageCredential(serverId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateStorageCredentialRequest>({
    provider: 's3', bucket: '', access_key: '', secret_key: '', region: '', endpoint: '', base_path: ''
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createCred.mutateAsync(form)
    setForm({ provider: 's3', bucket: '', access_key: '', secret_key: '', region: '', endpoint: '', base_path: '' })
    setShowForm(false)
  }

  const providerLabels: Record<string, string> = { s3: 'AWS S3', r2: 'Cloudflare R2', backblaze: 'Backblaze B2', gdrive: 'Google Drive' }
  const providerColors: Record<string, string> = { s3: 'bg-orange-100 text-orange-600', r2: 'bg-yellow-100 text-yellow-600', backblaze: 'bg-red-100 text-red-600', gdrive: 'bg-blue-100 text-blue-600' }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <HardDrive size={20} className="text-purple-500" /> Remote Backup Storage
        </h2>
        <button onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
          <Plus size={14} /> Add Storage
        </button>
      </div>

      {showForm && (
        <form onSubmit={handleCreate} className="space-y-3 mb-6 p-4 bg-gray-50 rounded-lg">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Provider</label>
              <select value={form.provider} onChange={e => setForm({...form, provider: e.target.value as CreateStorageCredentialRequest['provider']})}
                className="w-full px-3 py-2 border rounded-lg text-sm">
                <option value="s3">AWS S3</option>
                <option value="r2">Cloudflare R2</option>
                <option value="backblaze">Backblaze B2</option>
                <option value="gdrive">Google Drive</option>
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Bucket</label>
              <input type="text" value={form.bucket} onChange={e => setForm({...form, bucket: e.target.value})}
                placeholder="my-backup-bucket" className="w-full px-3 py-2 border rounded-lg text-sm" required />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Access Key</label>
              <input type="text" value={form.access_key} onChange={e => setForm({...form, access_key: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm" required />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Secret Key</label>
              <input type="password" value={form.secret_key} onChange={e => setForm({...form, secret_key: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm" required />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Region</label>
              <input type="text" value={form.region || ''} onChange={e => setForm({...form, region: e.target.value})}
                placeholder="us-east-1" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Endpoint (S3-compatible)</label>
              <input type="text" value={form.endpoint || ''} onChange={e => setForm({...form, endpoint: e.target.value})}
                placeholder="https://..." className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Base Path</label>
              <input type="text" value={form.base_path || ''} onChange={e => setForm({...form, base_path: e.target.value})}
                placeholder="/backups" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <button type="button" onClick={() => setShowForm(false)} className="px-3 py-2 text-sm text-gray-600 hover:bg-gray-200 rounded-lg">Cancel</button>
            <button type="submit" disabled={createCred.isPending}
              className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createCred.isPending ? 'Adding...' : 'Save Storage'}
            </button>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="text-center py-6"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600 mx-auto" /></div>
      ) : creds.length === 0 ? (
        <p className="text-center text-gray-400 py-6 text-sm">No remote storage configured. Add a provider to enable remote backups.</p>
      ) : (
        <div className="space-y-3">
          {creds.map(c => (
            <div key={c.id} className="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
              <div className="flex items-center gap-3">
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${providerColors[c.provider] || 'bg-gray-100'}`}>
                  <HardDrive size={20} />
                </div>
                <div>
                  <p className="font-medium text-sm">{providerLabels[c.provider] || c.provider}</p>
                  <p className="text-xs text-gray-500">{c.bucket}{c.base_path ? ` / ${c.base_path}` : ''} · {c.region || 'default region'}</p>
                </div>
              </div>
              <button onClick={() => { if (confirm(`Remove storage ${c.bucket}?`)) deleteCred.mutate(c.id) }}
                className="p-2 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-lg">
                <Trash2 size={16} />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

interface Fail2BanJail {
  name: string
  enabled: boolean
  filter: string
  ban_time: number
  max_retry: number
  banned_ips: string[]
  total_bans: number
}

interface Fail2BanStatusData {
  service_running: boolean
  jails: Fail2BanJail[]
}

function Fail2BanTab({ serverId }: { serverId: number }) {
  const { can } = usePermissions()
  const canManage = can('infrastructure', 'manage')
  const { data: status, isLoading, isError, error, refetch, isFetching } = useFail2BanStatus(serverId)
  const unban = useFail2BanUnban(serverId)
  const updateJail = useFail2BanUpdateJail(serverId)
  const toggleService = useFail2BanToggleService(serverId)
  const setupJails = useFail2BanSetupJails(serverId)
  const [unbanMsg, setUnbanMsg] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)
  const [editingJail, setEditingJail] = useState<string | null>(null)
  const [editBanTime, setEditBanTime] = useState(0)
  const [editMaxRetry, setEditMaxRetry] = useState(0)

  const f2bData = status as Fail2BanStatusData | null
  const serviceRunning = f2bData?.service_running ?? false
  const jails = f2bData?.jails ?? []
  const totalBanned = jails.reduce((sum, j) => sum + (j.banned_ips?.length ?? 0), 0)

  const handleToggleService = async () => {
    try {
      await toggleService.mutateAsync(!serviceRunning)
    } catch (e: unknown) {
      setUnbanMsg({ type: 'err', text: e instanceof Error ? e.message : 'Failed to toggle service' })
      setTimeout(() => setUnbanMsg(null), 4000)
    }
  }

  const handleUnbanIP = async (jail: string, ip: string) => {
    try {
      await unban.mutateAsync({ jail, ip })
      setUnbanMsg({ type: 'ok', text: `Unbanned ${ip} from ${jail}` })
    } catch (e: unknown) {
      setUnbanMsg({ type: 'err', text: e instanceof Error ? e.message : 'Unban failed' })
    }
    setTimeout(() => setUnbanMsg(null), 4000)
  }

  const handleSetupJails = async () => {
    try {
      await setupJails.mutateAsync()
      setUnbanMsg({ type: 'ok', text: 'Default jails configured (nginx + wordpress). Reloaded.' })
    } catch (e: unknown) {
      setUnbanMsg({ type: 'err', text: e instanceof Error ? e.message : 'Setup failed' })
    }
    setTimeout(() => setUnbanMsg(null), 6000)
  }

  const handleToggleJail = async (jail: Fail2BanJail) => {
    const enabled = !jail.enabled
    try {
      await updateJail.mutateAsync({ jail: jail.name, enabled })
    } catch {/* refetch will update state */ }
  }

  const handleSaveJail = async (jailName: string) => {
    try {
      await updateJail.mutateAsync({ jail: jailName, ban_time: editBanTime, max_retry: editMaxRetry })
      setEditingJail(null)
    } catch {/* ignore */ }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
              <Shield size={20} className="text-red-500" /> Fail2Ban Management
            </h2>
            {!isLoading && !isError && (
              <div className="flex gap-2 text-sm items-center">
                {/* Service toggle */}
                {canManage && (
                  <button
                    onClick={handleToggleService}
                    disabled={toggleService.isPending || isLoading}
                    title={serviceRunning ? 'Click to stop Fail2Ban service' : 'Click to start Fail2Ban service'}
                    className="flex items-center gap-1.5 disabled:opacity-50"
                  >
                    {toggleService.isPending
                      ? <Loader2 size={20} className="animate-spin text-gray-400" />
                      : serviceRunning
                        ? <ToggleRight size={24} className="text-emerald-500" />
                        : <ToggleLeft size={24} className="text-gray-400" />}
                    <span className={`font-medium ${serviceRunning ? 'text-emerald-600' : 'text-gray-500'}`}>
                      {serviceRunning ? 'Running' : 'Stopped'}
                    </span>
                  </button>
                )}
                {!canManage && (
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${serviceRunning ? 'bg-emerald-100 text-emerald-700' : 'bg-gray-100 text-gray-500'}`}>
                    {serviceRunning ? 'Running' : 'Stopped'}
                  </span>
                )}
                <span className="px-2 py-0.5 bg-gray-100 rounded-full text-gray-600">
                  {jails.length} jail{jails.length !== 1 ? 's' : ''}
                </span>
                {totalBanned > 0 && (
                  <span className="px-2 py-0.5 bg-red-100 text-red-700 rounded-full font-medium">
                    {totalBanned} banned IP{totalBanned !== 1 ? 's' : ''}
                  </span>
                )}
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            {canManage && (
              <button onClick={handleSetupJails} disabled={setupJails.isPending}
                title="Setup nginx + wordpress jails"
                className="flex items-center gap-1.5 px-3 py-2 text-sm text-emerald-700 bg-emerald-50 border border-emerald-200 rounded-lg hover:bg-emerald-100 disabled:opacity-50">
                {setupJails.isPending ? <Loader2 size={14} className="animate-spin" /> : <Wrench size={14} />}
                {setupJails.isPending ? 'Setting up...' : 'Setup Default Jails'}
              </button>
            )}
            <button onClick={() => refetch()} disabled={isFetching}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
              <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
              {isFetching ? 'Loading...' : 'Refresh'}
            </button>
          </div>
        </div>

        {unbanMsg && (
          <div className={`p-3 rounded-lg text-sm mb-4 ${unbanMsg.type === 'ok' ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'}`}>
            {unbanMsg.text}
          </div>
        )}

        {isLoading && (
          <div className="flex items-center justify-center py-12 text-gray-500 gap-2">
            <Loader2 size={18} className="animate-spin" />
            <span className="text-sm">Querying agent...</span>
          </div>
        )}

        {isError && (
          <div className="p-4 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {(error as Error)?.message ?? 'Failed to get Fail2Ban status'}
          </div>
        )}

        {!isLoading && !isError && jails.length === 0 && (
          <div className="text-center py-10 text-gray-500">
            <Shield size={32} className="mx-auto mb-2 text-gray-300" />
            <p className="text-sm">No jails found. Fail2Ban may not be running.</p>
          </div>
        )}

        {!isLoading && !isError && jails.length > 0 && (
          <div className="space-y-3">
            {jails.map(jail => (
              <div key={jail.name} className="border border-gray-200 rounded-xl overflow-hidden">
                {/* Jail header */}
                <div className="flex items-center justify-between px-4 py-3 bg-gray-50">
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => canManage && handleToggleJail(jail)}
                      disabled={!canManage || updateJail.isPending}
                      title={canManage ? (jail.enabled ? 'Disable jail' : 'Enable jail') : 'No permission'}
                    >
                      {jail.enabled
                        ? <ToggleRight size={22} className="text-emerald-500" />
                        : <ToggleLeft size={22} className="text-gray-400" />}
                    </button>
                    <span className="font-medium text-gray-800">{jail.name}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${jail.enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-gray-100 text-gray-500'}`}>
                      {jail.enabled ? 'Active' : 'Stopped'}
                    </span>
                  </div>
                  <div className="flex items-center gap-4 text-sm text-gray-500">
                    {editingJail === jail.name ? (
                      <div className="flex items-center gap-2">
                        <label className="text-xs">Ban time (s)</label>
                        <input type="number" value={editBanTime} onChange={e => setEditBanTime(Number(e.target.value))}
                          className="w-20 px-2 py-1 border rounded text-xs" />
                        <label className="text-xs">Max retry</label>
                        <input type="number" value={editMaxRetry} onChange={e => setEditMaxRetry(Number(e.target.value))}
                          className="w-16 px-2 py-1 border rounded text-xs" />
                        <button onClick={() => handleSaveJail(jail.name)} disabled={updateJail.isPending}
                          className="px-2 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
                          {updateJail.isPending ? '...' : 'Save'}
                        </button>
                        <button onClick={() => setEditingJail(null)}
                          className="px-2 py-1 text-xs bg-gray-200 text-gray-700 rounded hover:bg-gray-300">
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <>
                        <span>Ban: <strong>{jail.ban_time}s</strong></span>
                        <span>Max retry: <strong>{jail.max_retry}</strong></span>
                        <span className={`font-medium ${jail.banned_ips?.length > 0 ? 'text-red-600' : 'text-gray-400'}`}>
                          {jail.banned_ips?.length ?? 0} banned
                        </span>
                        {canManage && (
                          <button onClick={() => { setEditingJail(jail.name); setEditBanTime(jail.ban_time); setEditMaxRetry(jail.max_retry) }}
                            className="p-1 text-gray-400 hover:text-blue-600">
                            <Settings size={14} />
                          </button>
                        )}
                      </>
                    )}
                  </div>
                </div>

                {/* Banned IPs */}
                {jail.banned_ips?.length > 0 && (
                  <div className="px-4 py-3 bg-white">
                    <p className="text-xs text-gray-500 font-medium uppercase mb-2">Banned IPs</p>
                    <div className="flex flex-wrap gap-2">
                      {jail.banned_ips.map(ip => (
                        <div key={ip} className="flex items-center gap-1.5 px-2.5 py-1 bg-red-50 border border-red-200 rounded-lg text-xs">
                          <span className="text-red-700 font-mono">{ip}</span>
                          {canManage && (
                            <button onClick={() => handleUnbanIP(jail.name, ip)}
                              disabled={unban.isPending}
                              className="text-red-400 hover:text-red-700 disabled:opacity-50"
                              title={`Unban ${ip}`}>
                              <XCircle size={13} />
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Manual Unban */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h3 className="text-md font-semibold text-gray-900 mb-4">Manual Unban</h3>
        <UnbanFormFull serverId={serverId} unban={unban} jails={jails.map(j => j.name)} />
      </div>
    </div>
  )
}

function UnbanFormFull({ serverId, unban, jails }: {
  serverId: number
  unban: ReturnType<typeof useFail2BanUnban>
  jails: string[]
}) {
  const defaultJails = ['sshd', 'nginx-http-auth', 'nginx-botsearch', 'wordpress-login', 'wordpress-xmlrpc']
  const jailOptions = jails.length > 0 ? jails : defaultJails
  const [jail, setJail] = useState(jailOptions[0] ?? 'sshd')
  const [ip, setIP] = useState('')
  const [msg, setMsg] = useState<{ type: 'ok' | 'err'; text: string } | null>(null)

  const handleUnban = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await unban.mutateAsync({ jail, ip })
      setMsg({ type: 'ok', text: `Unbanned ${ip} from ${jail}` })
      setIP('')
    } catch (e: unknown) {
      setMsg({ type: 'err', text: e instanceof Error ? e.message : 'Unban failed' })
    }
    setTimeout(() => setMsg(null), 4000)
  }

  return (
    <form onSubmit={handleUnban} className="space-y-3">
      {msg && (
        <div className={`p-3 rounded-lg text-sm ${msg.type === 'ok' ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'}`}>
          {msg.text}
        </div>
      )}
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="block text-xs text-gray-500 mb-1">Jail</label>
          <select value={jail} onChange={e => setJail(e.target.value)}
            className="w-full px-3 py-2 border rounded-lg text-sm">
            {jailOptions.map(j => <option key={j} value={j}>{j}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-xs text-gray-500 mb-1">IP Address</label>
          <input type="text" value={ip} onChange={e => setIP(e.target.value)}
            placeholder="192.168.1.100" className="w-full px-3 py-2 border rounded-lg text-sm" required />
        </div>
        <div className="flex items-end">
          <button type="submit" disabled={unban.isPending}
            className="w-full px-4 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
            {unban.isPending ? 'Processing...' : 'Unban IP'}
          </button>
        </div>
      </div>
    </form>
  )
}

// ==================== Redis Server Tab ====================

function parseRedisInfo(raw: string): Record<string, string> {
  const result: Record<string, string> = {}
  for (const line of raw.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const idx = trimmed.indexOf(':')
    if (idx !== -1) result[trimmed.slice(0, idx).trim()] = trimmed.slice(idx + 1).trim()
  }
  return result
}

function RedisServerTab({ serverId, canManage }: { serverId: number; canManage: boolean }) {
  const { data: rawStats, refetch: fetchStats, isFetching: loadingStats } = useServerRedisStats(serverId)
  const flushServer = useFlushServerRedis()
  const { query: sitesQuery, invalidate: invalidateSites } = useServerRedisSiteStatuses(serverId)
  const toggleRedis = useToggleRedis()

  const [statsVisible, setStatsVisible] = useState(false)
  const [flushMsg, setFlushMsg] = useState<string | null>(null)

  const stats = rawStats ? parseRedisInfo(rawStats) : null
  const sites = sitesQuery.data ?? []

  const handleFetchStats = async () => {
    setStatsVisible(true)
    await fetchStats()
  }

  const handleFlush = () => {
    setFlushMsg(null)
    flushServer.mutate({ serverId }, {
      onSuccess: () => setFlushMsg('All Redis cache flushed successfully.'),
      onError: (e: Error) => setFlushMsg('Failed to flush: ' + e.message),
    })
  }

  return (
    <div className="space-y-6">
      {/* Stats & Actions */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Database size={20} className="text-red-500" /> Redis Server
        </h2>
        <p className="text-sm text-gray-500 mb-5">
          Server-wide Redis management — stats, flush all cache, and per-site object cache enable/disable.
        </p>

        <div className="flex gap-3 flex-wrap mb-4">
          <button onClick={handleFetchStats} disabled={loadingStats}
            className="flex items-center gap-2 px-4 py-2 text-sm bg-blue-50 text-blue-700 hover:bg-blue-100 rounded-lg font-medium disabled:opacity-50 transition-colors">
            {loadingStats ? <Loader2 size={14} className="animate-spin" /> : <Activity size={14} />}
            {loadingStats ? 'Fetching…' : 'View Stats'}
          </button>
          {canManage && (
            <button onClick={handleFlush} disabled={flushServer.isPending}
              className="flex items-center gap-2 px-4 py-2 text-sm bg-orange-50 text-orange-700 hover:bg-orange-100 rounded-lg font-medium disabled:opacity-50 transition-colors">
              {flushServer.isPending ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
              {flushServer.isPending ? 'Flushing…' : 'Flush All Cache'}
            </button>
          )}
        </div>

        {flushMsg && (
          <div className={`flex items-center gap-2 px-4 py-3 rounded-lg text-sm mb-4 ${
            flushMsg.includes('success') ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'
          }`}>
            {flushMsg.includes('success') ? <CheckCircle2 size={14} /> : <XCircle size={14} />}
            {flushMsg}
          </div>
        )}

        {statsVisible && (
          <div className="border border-gray-200 rounded-lg overflow-hidden">
            <div className="px-4 py-2 bg-gray-50 border-b border-gray-200 flex items-center justify-between">
              <span className="text-sm font-medium text-gray-700">Redis INFO</span>
              <button onClick={() => setStatsVisible(false)} className="text-xs text-gray-400 hover:text-gray-600">Hide</button>
            </div>
            {loadingStats ? (
              <div className="flex justify-center py-6"><Loader2 size={20} className="animate-spin text-blue-500" /></div>
            ) : stats ? (
              <div className="grid grid-cols-2 sm:grid-cols-4 divide-x divide-y divide-gray-100">
                {[
                  { label: 'Memory Used', value: stats['used_memory_human'] || '—' },
                  { label: 'Peak Memory', value: stats['used_memory_peak_human'] || '—' },
                  { label: 'Connected Clients', value: stats['connected_clients'] || '—' },
                  { label: 'OPS/sec', value: stats['instantaneous_ops_per_sec'] || '—' },
                  { label: 'Cache Hits', value: stats['keyspace_hits'] ? Number(stats['keyspace_hits']).toLocaleString() : '—' },
                  { label: 'Cache Misses', value: stats['keyspace_misses'] ? Number(stats['keyspace_misses']).toLocaleString() : '—' },
                  { label: 'Uptime (days)', value: stats['uptime_in_days'] || '—' },
                  { label: 'Redis Version', value: stats['redis_version'] || '—' },
                ].map(({ label, value }) => (
                  <div key={label} className="px-4 py-3">
                    <p className="text-xs text-gray-500 uppercase font-medium">{label}</p>
                    <p className="text-sm font-semibold text-gray-900 mt-0.5">{value}</p>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-gray-400 text-center py-6">No stats available — is Redis running?</p>
            )}
          </div>
        )}
      </div>

      {/* Per-site Redis status */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h3 className="text-base font-semibold text-gray-900 mb-4">Object Cache per Site</h3>
        {sitesQuery.isLoading ? (
          <div className="flex justify-center py-8"><Loader2 size={20} className="animate-spin text-blue-500" /></div>
        ) : sites.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No sites on this server.</p>
        ) : (
          <div className="space-y-2">
            {sites.map(site => (
              <div key={site.site_id}
                className="flex items-center justify-between p-3 rounded-lg border border-gray-100 hover:bg-gray-50">
                <div className="flex items-center gap-3">
                  <Globe size={15} className="text-blue-400 shrink-0" />
                  <span className="text-sm font-medium text-gray-900">{site.domain}</span>
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                    site.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'
                  }`}>{site.enabled ? 'Enabled' : 'Disabled'}</span>
                </div>
                {canManage && (
                  <button
                    onClick={() => toggleRedis.mutate({ siteId: site.site_id, enabled: !site.enabled }, {
                      onSuccess: () => invalidateSites(),
                    })}
                    disabled={toggleRedis.isPending}
                    className={`px-3 py-1.5 text-xs rounded-lg font-medium disabled:opacity-50 transition-colors ${
                      site.enabled
                        ? 'bg-red-50 text-red-700 hover:bg-red-100'
                        : 'bg-green-50 text-green-700 hover:bg-green-100'
                    }`}>
                    {site.enabled ? 'Disable' : 'Enable'}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ==================== Backup Archives Tab ====================

function formatBytes(bytes: number): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function BackupArchivesTab({ serverId, canManage }: { serverId: number; canManage: boolean }) {
  const [page, setPage] = useState(1)
  const { data, isLoading } = useServerBackups(serverId, page)
  const deleteBackup = useDeleteBackup()

  const backups: Backup[] = data?.data ?? []
  const meta = data?.meta
  const totalPages = meta ? Math.ceil(meta.total / meta.per_page) : 1

  const typeLabel = (type: string) => (({ full: 'Full', database_only: 'DB Only' } as Record<string, string>)[type] ?? type)
  const typeColor = (type: string) => (({ full: 'bg-blue-100 text-blue-700', database_only: 'bg-purple-100 text-purple-700' } as Record<string, string>)[type] ?? 'bg-gray-100 text-gray-600')
  const statusColor = (status: string) => (({ completed: 'bg-green-100 text-green-700', running: 'bg-amber-100 text-amber-700', failed: 'bg-red-100 text-red-700' } as Record<string, string>)[status] ?? 'bg-gray-100 text-gray-600')

  return (
    <div className="space-y-4">
      {/* Explainer banner */}
      <div className="bg-blue-50 border border-blue-200 rounded-xl p-4 flex gap-3 items-start">
        <Archive size={18} className="text-blue-500 mt-0.5 shrink-0" />
        <div className="text-sm text-blue-800">
          <p className="font-medium mb-0.5">All backups on this server</p>
          <p className="text-blue-600">
            Backups from <strong>active sites</strong> and <strong>deleted sites</strong> are both shown here.
            Orphaned backups are labelled <span className="inline-flex items-center px-1.5 py-0.5 bg-amber-100 text-amber-700 rounded text-xs font-medium">Site Deleted</span> — their files still exist on disk.
          </p>
        </div>
      </div>

      {/* Backup list */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
          <Archive size={20} className="text-blue-500" />
          Backup Archive
          {meta && <span className="ml-auto text-sm font-normal text-gray-400">{meta.total} total</span>}
        </h2>

        {isLoading ? (
          <div className="flex justify-center py-12">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" />
          </div>
        ) : backups.length === 0 ? (
          <div className="text-center py-12">
            <Archive size={40} className="text-gray-300 mx-auto mb-3" />
            <p className="text-gray-400 text-sm">No backups found on this server.</p>
            <p className="text-gray-400 text-xs mt-1">Create backups from individual site pages.</p>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-gray-500 border-b text-xs uppercase tracking-wide">
                    <th className="pb-3 font-medium">Domain</th>
                    <th className="pb-3 font-medium">Type</th>
                    <th className="pb-3 font-medium">Status</th>
                    <th className="pb-3 font-medium">Size</th>
                    <th className="pb-3 font-medium">Created</th>
                    <th className="pb-3 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {backups.map(backup => (
                    <tr key={backup.id} className="hover:bg-gray-50/50">
                      <td className="py-3 pr-4">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-medium text-gray-800">{backup.domain || '—'}</span>
                          {backup.site_id === null && (
                            <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-700">
                              Site Deleted
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="py-3 pr-4">
                        <span className={`inline-flex px-2 py-0.5 rounded-md text-xs font-medium ${typeColor(backup.type)}`}>
                          {typeLabel(backup.type)}
                        </span>
                      </td>
                      <td className="py-3 pr-4">
                        <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${statusColor(backup.status)}`}>
                          {backup.status === 'running' && <Loader2 size={10} className="animate-spin" />}
                          {backup.status === 'completed' ? 'Completed' : backup.status === 'running' ? 'Running' : 'Failed'}
                        </span>
                      </td>
                      <td className="py-3 pr-4 text-gray-600 tabular-nums">{formatBytes(backup.size_bytes)}</td>
                      <td className="py-3 pr-4 text-gray-500 text-xs whitespace-nowrap">
                        {new Date(backup.created_at).toLocaleString()}
                      </td>
                      <td className="py-3">
                        <div className="flex items-center gap-2 justify-end">
                          {backup.file_path && (
                            <span className="text-xs text-gray-400 font-mono max-w-[180px] truncate hidden lg:block" title={backup.file_path}>
                              {backup.file_path.split('/').pop()}
                            </span>
                          )}
                          {canManage && (
                            <button
                              onClick={() => { if (confirm(`Delete backup for "${backup.domain}"? This cannot be undone.`)) deleteBackup.mutate(backup.id) }}
                              className="p-1.5 text-gray-400 hover:text-red-600 transition-colors rounded hover:bg-red-50">
                              <Trash2 size={14} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between mt-5 pt-4 border-t border-gray-100 text-sm text-gray-500">
                <span>Page {page} of {totalPages} · {meta?.total} backups</span>
                <div className="flex gap-2">
                  <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronLeft size={14} />
                  </button>
                  <button onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page >= totalPages}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronRight size={14} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <div className="bg-gray-50 border border-gray-200 rounded-xl p-4 text-sm text-gray-500">
        <p className="font-medium text-gray-700 mb-1">Restore an orphaned backup</p>
        <ol className="list-decimal list-inside space-y-1">
          <li>Note the <strong>file path</strong> (hover the filename in the table)</li>
          <li>Re-create a site with the same domain on this server</li>
          <li>Go to the new site → <strong>Backups tab</strong> and use the Restore function</li>
        </ol>
      </div>
    </div>
  )
}
