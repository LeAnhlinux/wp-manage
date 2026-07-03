import { useState, useEffect, useRef } from 'react'
import {
  ArrowRightLeft, Play, RotateCcw, XCircle, ChevronLeft, ChevronRight,
  Eye, Loader2, CheckCircle2, CheckCircle, AlertCircle, Clock, Globe, RefreshCw,
  Server, AlertTriangle, Info, Trash2, Wifi, WifiOff, Zap,
} from 'lucide-react'
import { useMigrations, useMigration, useMigrationLogs, useStartMigration, useRollbackMigration, useCancelMigration, useCleanupMigrationSource } from '@/hooks/useMigrations'
import { useServers } from '@/hooks/useServers'
import { useSites } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import { useAutoCreateDNSRecord } from '@/hooks/useDNS'
import api from '@/lib/api'
import type { Migration, MigrationLog } from '@/types'

const phaseLabels: Record<string, string> = {
  preparation: 'Preparation',
  initial_sync: 'Initial sync',
  delta_sync: 'Delta Sync',
  switchover: 'Switchover',
  cleanup: 'Cleanup',
}

const phaseOrder = ['preparation', 'initial_sync', 'delta_sync', 'switchover', 'cleanup']

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    running: 'bg-blue-100 text-blue-800',
    completed: 'bg-emerald-100 text-emerald-800',
    failed: 'bg-red-100 text-red-800',
    rolled_back: 'bg-amber-100 text-amber-800',
  }
  const icons: Record<string, React.ReactNode> = {
    running: <Loader2 size={12} className="animate-spin" />,
    completed: <CheckCircle2 size={12} />,
    failed: <AlertCircle size={12} />,
    rolled_back: <RotateCcw size={12} />,
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${styles[status] || 'bg-gray-100 text-gray-800'}`}>
      {icons[status]}
      {status === 'running' ? 'Running' : status === 'completed' ? 'Completed' : status === 'failed' ? 'Failed' : status === 'rolled_back' ? 'Rolled back' : status}
    </span>
  )
}

function ModeBadge({ mode }: { mode?: string }) {
  if (!mode || mode === 'keep_domain') {
    return <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-700"><Server size={10} />Keep domain</span>
  }
  return <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700"><Globe size={10} />Change domain</span>
}

function ProgressBar({ progress, phase }: { progress: number; phase: string }) {
  return (
    <div className="w-full">
      <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
        <span>{phaseLabels[phase] || phase}</span>
        <span>{progress}%</span>
      </div>
      <div className="w-full bg-gray-200 rounded-full h-1.5">
        <div
          className="bg-blue-600 h-1.5 rounded-full transition-all duration-500"
          style={{ width: `${progress}%` }}
        />
      </div>
    </div>
  )
}

function PhaseTimeline({ currentPhase, status }: { currentPhase: string; status: string }) {
  const currentIdx = phaseOrder.indexOf(currentPhase)
  return (
    <div className="flex items-center gap-1">
      {phaseOrder.map((phase, idx) => {
        let color = 'bg-gray-300'
        if (status === 'completed') color = 'bg-emerald-500'
        else if (status === 'failed' && idx === currentIdx) color = 'bg-red-500'
        else if (idx < currentIdx) color = 'bg-emerald-500'
        else if (idx === currentIdx) color = 'bg-blue-500 animate-pulse'
        return (
          <div key={phase} className="flex items-center gap-1">
            <div className={`w-3 h-3 rounded-full ${color}`} title={phaseLabels[phase]} />
            {idx < phaseOrder.length - 1 && (
              <div className={`w-5 h-0.5 ${idx < currentIdx ? 'bg-emerald-500' : 'bg-gray-200'}`} />
            )}
          </div>
        )
      })}
    </div>
  )
}

function MigrationDetailModal({ migrationId, onClose }: { migrationId: number; onClose: () => void }) {
  const { data: migration } = useMigration(migrationId)
  const { data: logs } = useMigrationLogs(migrationId, migration?.status === 'running')

  if (!migration) return null

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-2xl max-h-[85vh] overflow-hidden flex flex-col">
        <div className="p-6 border-b flex items-center justify-between flex-shrink-0">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <h3 className="text-lg font-bold text-gray-900">Migration #{migration.id}</h3>
              <StatusBadge status={migration.status} />
              <ModeBadge mode={migration.mode} />
            </div>
            <p className="text-sm text-gray-500 font-mono">{migration.site?.domain || `Site #${migration.site_id}`}
              {migration.mode === 'change_domain' && migration.new_domain && (
                <span className="text-indigo-600"> → {migration.new_domain}</span>
              )}
            </p>
          </div>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 p-1">✕</button>
        </div>

        <div className="p-6 space-y-5 overflow-y-auto flex-1">
          <div className="grid grid-cols-2 gap-4">
            <div className="bg-gray-50 rounded-xl p-3">
              <p className="text-xs text-gray-500 mb-1">Source server</p>
              <p className="text-sm font-semibold text-gray-800">{migration.source_server?.name || `#${migration.source_server_id}`}</p>
              <p className="text-xs text-gray-400 font-mono">{migration.source_server?.ip_address}</p>
            </div>
            <div className="bg-gray-50 rounded-xl p-3">
              <p className="text-xs text-gray-500 mb-1">Target server</p>
              <p className="text-sm font-semibold text-gray-800">{migration.target_server?.name || `#${migration.target_server_id}`}</p>
              <p className="text-xs text-gray-400 font-mono">{migration.target_server?.ip_address}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Started</p>
              <p className="text-sm">{new Date(migration.started_at).toLocaleString('vi-VN')}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Completed</p>
              <p className="text-sm">{migration.completed_at ? new Date(migration.completed_at).toLocaleString('vi-VN') : '—'}</p>
            </div>
          </div>

          <div className="space-y-2">
            <ProgressBar progress={migration.progress} phase={migration.phase} />
            <div className="flex items-center gap-2">
              <PhaseTimeline currentPhase={migration.phase} status={migration.status} />
              <span className="text-xs text-gray-400 ml-2">{phaseLabels[migration.phase] || migration.phase}</span>
            </div>
          </div>

          {migration.error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700 flex items-start gap-2">
              <AlertCircle size={16} className="flex-shrink-0 mt-0.5" />
              {migration.error}
            </div>
          )}

          {/* Migration Logs */}
          <div>
            <h4 className="text-sm font-semibold text-gray-800 mb-2">Migration Logs</h4>
            <div className="bg-[#0F172A] rounded-xl p-3 max-h-52 overflow-y-auto font-mono text-xs">
              {logs && logs.length > 0 ? (
                logs.map((log: MigrationLog) => (
                  <div key={log.id} className={`py-0.5 ${log.level === 'error' ? 'text-red-400' : log.level === 'warn' ? 'text-amber-400' : 'text-emerald-400'}`}>
                    <span className="text-slate-500">{new Date(log.created_at).toLocaleTimeString('vi-VN')}</span>{' '}
                    <span className="text-blue-400">[{phaseLabels[log.phase] || log.phase}]</span>{' '}
                    {log.message}
                  </div>
                ))
              ) : (
                <p className="text-slate-500">No logs yet...</p>
              )}
            </div>
          </div>
        </div>

        <div className="p-4 border-t flex justify-end flex-shrink-0">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-xl font-medium">
            Close
          </button>
        </div>
      </div>
    </div>
  )
}

type MigrationMode = 'keep_domain' | 'change_domain'

type DnsStatus = 'idle' | 'checking' | 'pointing' | 'not_pointing' | 'no_provider' | 'error'
interface DnsCheckResult {
  pointing: boolean
  resolved_ips?: string[]
  server_ip?: string
  message?: string
  provider_type?: string  // cloudflare | route53 | godaddy
  provider_name?: string  // "Cloudflare" | "AWS Route53" | "GoDaddy"
  can_auto_point?: boolean
}

function StartMigrationModal({ onClose }: { onClose: () => void }) {
  const [mode, setMode] = useState<MigrationMode>('keep_domain')
  const [siteId, setSiteId] = useState<number>(0)
  const [targetServerId, setTargetServerId] = useState<number>(0)
  const [newDomain, setNewDomain] = useState('')
  const [updateDNS, setUpdateDNS] = useState(false)
  const [cleanupSource, setCleanupSource] = useState(false)

  // DNS check state (change_domain only)
  const [dnsStatus, setDnsStatus] = useState<DnsStatus>('idle')
  const [dnsResult, setDnsResult] = useState<DnsCheckResult | null>(null)
  const [autoPointing, setAutoPointing] = useState(false)
  const [autoPointResult, setAutoPointResult] = useState<string | null>(null)
  const dnsDebounce = useRef<ReturnType<typeof setTimeout> | null>(null)
  const autoCreateDNS = useAutoCreateDNSRecord()

  const { data: sitesData } = useSites()
  const { data: serversData } = useServers()
  const startMigration = useStartMigration()

  const sites = sitesData?.data || []
  const servers = serversData?.data || []
  const selectedSite = sites.find(s => s.id === siteId)
  const targetServer = servers.find(s => s.id === targetServerId)

  // DNS check: debounce 800ms when newDomain or targetServerId changes (change_domain mode only)
  useEffect(() => {
    if (mode !== 'change_domain') return
    setAutoPointResult(null)
    const domain = newDomain.trim()
    const isValid = domain.length > 3 && domain.includes('.') && !domain.startsWith('.')
    if (!isValid || targetServerId === 0) { setDnsStatus('idle'); setDnsResult(null); return }

    setDnsStatus('checking')
    if (dnsDebounce.current) clearTimeout(dnsDebounce.current)
    dnsDebounce.current = setTimeout(async () => {
      try {
        const { data } = await api.get<{ data: DnsCheckResult }>(`/servers/${targetServerId}/check-dns`, { params: { domain } })
        const res = data.data
        setDnsResult(res)
        if (res.pointing) {
          setDnsStatus('pointing')
        } else if (!res.can_auto_point) {
          setDnsStatus('no_provider')
        } else {
          setDnsStatus('not_pointing')
        }
      } catch {
        setDnsStatus('error')
        setDnsResult(null)
      }
    }, 800)
    return () => { if (dnsDebounce.current) clearTimeout(dnsDebounce.current) }
  }, [newDomain, targetServerId, mode])

  const handleAutoPointDNS = async () => {
    if (!targetServer || !newDomain.trim()) return
    setAutoPointing(true)
    setAutoPointResult(null)
    try {
      const result = await autoCreateDNS.mutateAsync({ domain: newDomain.trim(), server_ip: targetServer.ip_address })
      const baseMsg = `✓ A record ${result.action === 'created' ? 'created' : 'updated'}: ${result.domain} → ${result.ip} (zone: ${result.zone_name})`
      const proxyWarn = result.proxied ? '\n⚠️ Cloudflare Proxy is ON — the DNS check will see Cloudflare IPs instead of the server IP. Turn off the proxy in Cloudflare (orange cloud icon) to check directly.' : ''
      setAutoPointResult(baseMsg + proxyWarn)
      // Re-check DNS after auto-point
      setTimeout(async () => {
        try {
          const { data } = await api.get<{ data: DnsCheckResult }>(`/servers/${targetServerId}/check-dns`, { params: { domain: newDomain.trim() } })
          setDnsResult(data.data)
          setDnsStatus(data.data.pointing ? 'pointing' : 'not_pointing')
        } catch { /* ignore */ }
      }, 2000)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'No DNS provider found managing this domain'
      setAutoPointResult(`✗ ${msg}`)
    } finally {
      setAutoPointing(false)
    }
  }

  const canSubmit = siteId > 0 && targetServerId > 0 && (mode === 'keep_domain' || newDomain.trim() !== '')

  const handleSubmit = () => {
    if (!canSubmit) return
    startMigration.mutate(
      {
        siteId,
        target_server_id: targetServerId,
        mode,
        new_domain: mode === 'change_domain' ? newDomain.trim() : undefined,
        update_dns: mode === 'keep_domain' ? updateDNS : false,
        cleanup_source: cleanupSource,
      },
      { onSuccess: () => onClose() }
    )
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-lg">
        {/* Header */}
        <div className="p-6 border-b">
          <h3 className="text-lg font-bold text-gray-900">Start Migration</h3>
          <p className="text-sm text-gray-500 mt-0.5">Move a WordPress site to another server</p>
        </div>

        <div className="p-6 space-y-5">
          {/* Mode selector */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-2">Migration mode</label>
            <div className="grid grid-cols-2 gap-3">
              {([
                {
                  value: 'keep_domain' as MigrationMode,
                  title: 'Keep domain',
                  desc: 'The site moves to the new server with the same domain. The source is suspended and DNS updates automatically.',
                  icon: <Server size={20} className="text-purple-500" />,
                },
                {
                  value: 'change_domain' as MigrationMode,
                  title: 'Change domain',
                  desc: 'The site is cloned to the new server under a different domain. The source keeps running normally.',
                  icon: <Globe size={20} className="text-indigo-500" />,
                },
              ] as const).map(opt => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setMode(opt.value)}
                  className={`text-left p-4 rounded-xl border-2 transition-all ${
                    mode === opt.value
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-200 hover:border-gray-300'
                  }`}
                >
                  <div className="flex items-center gap-2 mb-1.5">
                    {opt.icon}
                    <span className="text-sm font-semibold text-gray-800">{opt.title}</span>
                    {mode === opt.value && <CheckCircle2 size={14} className="text-blue-500 ml-auto" />}
                  </div>
                  <p className="text-xs text-gray-500 leading-relaxed">{opt.desc}</p>
                </button>
              ))}
            </div>
          </div>

          {/* Source site */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">Source site</label>
            <select
              value={siteId}
              onChange={e => setSiteId(Number(e.target.value))}
              className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
            >
              <option value={0}>Select site…</option>
              {sites.filter(s => s.status === 'active').map(site => (
                <option key={site.id} value={site.id}>{site.domain}</option>
              ))}
            </select>
          </div>

          {/* Target server */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">Target server</label>
            <select
              value={targetServerId}
              onChange={e => setTargetServerId(Number(e.target.value))}
              className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
            >
              <option value={0}>Select target server…</option>
              {servers
                .filter(s => s.status === 'active' && (!selectedSite || s.id !== selectedSite.server_id))
                .map(server => (
                  <option key={server.id} value={server.id}>{server.name} ({server.ip_address})</option>
                ))}
            </select>
          </div>

          {/* New domain (change_domain only) */}
          {mode === 'change_domain' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">New domain <span className="text-red-500">*</span></label>
              <div className="relative">
                <input
                  type="text"
                  value={newDomain}
                  onChange={e => { setNewDomain(e.target.value); setDnsStatus('idle') }}
                  placeholder="newdomain.com"
                  className={`w-full border rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:ring-2 outline-none pr-9 transition-colors ${
                    dnsStatus === 'pointing' ? 'border-emerald-400 focus:border-emerald-500 focus:ring-emerald-500/20'
                    : dnsStatus === 'not_pointing' ? 'border-amber-400 focus:border-amber-500 focus:ring-amber-500/20'
                    : dnsStatus === 'error' ? 'border-red-400 focus:border-red-500 focus:ring-red-500/20'
                    : 'border-gray-200 focus:border-blue-500 focus:ring-blue-500/20'
                  }`}
                />
                <span className="absolute right-3 top-1/2 -translate-y-1/2">
                  {dnsStatus === 'checking' && <Loader2 size={14} className="animate-spin text-gray-400" />}
                  {dnsStatus === 'pointing' && <Wifi size={14} className="text-emerald-500" />}
                  {dnsStatus === 'not_pointing' && <WifiOff size={14} className="text-amber-500" />}
                  {dnsStatus === 'error' && <AlertCircle size={14} className="text-red-500" />}
                </span>
              </div>

              {/* DNS status row */}
              {dnsStatus === 'checking' && (
                <p className="text-xs text-gray-400 mt-1.5 flex items-center gap-1"><Loader2 size={11} className="animate-spin" /> Checking DNS...</p>
              )}
              {dnsStatus === 'pointing' && dnsResult && (
                <p className="text-xs text-emerald-600 mt-1.5 flex items-center gap-1">
                  <CheckCircle size={11} /> DNS points correctly to {dnsResult.server_ip} — ready to migrate
                </p>
              )}
              {dnsStatus === 'not_pointing' && dnsResult && (
                <div className="mt-2 p-2.5 bg-amber-50 border border-amber-200 rounded-xl space-y-2">
                  <p className="text-xs text-amber-800 flex items-center gap-1.5">
                    <WifiOff size={11} className="flex-shrink-0" />
                    DNS does not point to {dnsResult.server_ip} yet.
                    {dnsResult.resolved_ips && dnsResult.resolved_ips.length > 0 && (
                      <span className="text-amber-600"> Currently points to: {dnsResult.resolved_ips.join(', ')}</span>
                    )}
                    {(!dnsResult.resolved_ips || dnsResult.resolved_ips.length === 0) && (
                      <span className="text-amber-600"> (domain has no A record yet)</span>
                    )}
                  </p>
                  <button
                    type="button"
                    onClick={handleAutoPointDNS}
                    disabled={autoPointing}
                    className="w-full flex items-center justify-center gap-1.5 px-3 py-1.5 bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold rounded-lg transition-colors disabled:opacity-60"
                  >
                    {autoPointing ? <><Loader2 size={12} className="animate-spin" /> Pointing DNS...</> : <><Zap size={12} /> Auto-point DNS to {dnsResult.server_ip}</>}
                  </button>
                  <p className="text-xs text-amber-600">
                    {dnsResult?.provider_name
                      ? <>Provider: <strong>{dnsResult.provider_name}</strong> ({dnsResult.provider_type})</>
                      : 'Requires the domain to use a configured DNS provider (Cloudflare, GoDaddy, Route53...)'}
                  </p>
                </div>
              )}
              {dnsStatus === 'error' && (
                <p className="text-xs text-red-500 mt-1.5 flex items-center gap-1">
                  <AlertCircle size={11} /> Could not look up DNS — check the domain name
                </p>
              )}
              {dnsStatus === 'idle' && targetServerId === 0 && newDomain.trim().includes('.') && (
                <p className="text-xs text-gray-400 mt-1">Select a target server to check DNS automatically</p>
              )}

              {/* Auto-point result */}
              {autoPointResult && (
                <div className={`text-xs mt-1.5 space-y-1 ${autoPointResult.startsWith('✓') ? 'text-emerald-600' : 'text-red-500'}`}>
                  {autoPointResult.split('\n').map((line, i) => (
                    <p key={i} className={i > 0 ? 'text-amber-600 bg-amber-50 border border-amber-200 rounded-lg px-2 py-1.5 leading-relaxed' : ''}>
                      {line}
                    </p>
                  ))}
                </div>
              )}

              {dnsStatus === 'idle' && !autoPointResult && (
                <p className="text-xs text-gray-400 mt-1">Enter a domain to check DNS automatically</p>
              )}
            </div>
          )}

          {/* DNS auto-update (keep_domain only) */}
          {mode === 'keep_domain' && (
            <label className="flex items-start gap-3 p-3 bg-purple-50 rounded-xl cursor-pointer">
              <input
                type="checkbox"
                checked={updateDNS}
                onChange={e => setUpdateDNS(e.target.checked)}
                className="mt-0.5"
              />
              <div>
                <p className="text-sm font-medium text-gray-700">Auto-update DNS</p>
                <p className="text-xs text-gray-500 mt-0.5">
                  After the migration completes, the system will point the domain A record to the new server IP
                  (requires a configured DNS Provider).
                </p>
              </div>
            </label>
          )}

          {/* Cleanup source */}
          <label className="flex items-start gap-3 p-3 bg-red-50 rounded-xl cursor-pointer border border-red-100">
            <input
              type="checkbox"
              checked={cleanupSource}
              onChange={e => setCleanupSource(e.target.checked)}
              className="mt-0.5"
            />
            <div>
              <p className="text-sm font-medium text-red-700">Delete the site from the source server after migrating</p>
              <p className="text-xs text-red-500 mt-0.5">
                Removes all files, the database, Nginx/PHP-FPM config, and the SSL cert of the site from the source server. <strong>This cannot be undone.</strong>
              </p>
            </div>
          </label>

          {/* Info boxes */}
          {mode === 'keep_domain' && (
            <div className="flex items-start gap-2 p-3 bg-amber-50 border border-amber-200 rounded-xl text-xs text-amber-800">
              <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
              <span>The site will be <strong>suspended</strong> during the database switchover. Downtime is usually under 1 minute.</span>
            </div>
          )}
          {mode === 'change_domain' && (
            <div className="flex items-start gap-2 p-3 bg-blue-50 border border-blue-200 rounded-xl text-xs text-blue-800">
              <Info size={14} className="flex-shrink-0 mt-0.5" />
              <span>The source site <strong>keeps running normally</strong>. You end up with 2 sites running in parallel on 2 different domains.</span>
            </div>
          )}

          {startMigration.isError && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
              {(startMigration.error as Error).message || 'Could not start migration'}
            </div>
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-xl transition-colors">
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!canSubmit || startMigration.isPending}
            className="px-5 py-2 text-sm bg-blue-600 text-white hover:bg-blue-700 rounded-xl disabled:opacity-50 flex items-center gap-2 font-semibold transition-colors"
          >
            {startMigration.isPending ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
            {startMigration.isPending ? 'Starting...' : 'Start Migration'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function MigrationsPage() {
  const [page, setPage] = useState(1)
  const [showStart, setShowStart] = useState(false)
  const [viewId, setViewId] = useState<number | null>(null)
  const { data, isLoading } = useMigrations(page)
  const { can } = usePermissions()
  const rollback = useRollbackMigration()
  const cancel = useCancelMigration()
  const cleanupSource = useCleanupMigrationSource()

  const migrations: Migration[] = data?.data || []
  const meta = data?.meta

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-7">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 leading-none flex items-center gap-2">
            <ArrowRightLeft size={22} className="text-blue-500" /> Migrations
          </h1>
          <p className="text-gray-500 text-sm mt-1">Move WordPress sites between VPS servers</p>
        </div>
        {can('sites', 'manage') && (
          <button
            onClick={() => setShowStart(true)}
            className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 text-sm font-semibold shadow-sm shadow-blue-200 transition-colors"
          >
            <Play size={15} /> New Migration
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="bg-white rounded-2xl border border-gray-100 p-10 text-center">
          <Loader2 size={24} className="animate-spin text-gray-300 mx-auto" />
        </div>
      ) : migrations.length === 0 ? (
        <div className="bg-white rounded-2xl border border-gray-100 p-14 text-center">
          <ArrowRightLeft size={40} className="mx-auto text-gray-200 mb-4" />
          <p className="text-gray-500 font-medium">No migrations yet</p>
          <p className="text-gray-400 text-sm mt-1">Click "New Migration" to move a site to a new server</p>
        </div>
      ) : (
        <>
          {/* Desktop Table */}
          <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden hidden md:block">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-50">
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">ID</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Site</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Servers</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Mode</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide w-44">Progress</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Status</th>
                  <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Time</th>
                  <th className="text-right px-5 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {migrations.map((m: Migration) => (
                  <tr key={m.id} className="hover:bg-gray-50/60 transition-colors">
                    <td className="px-5 py-3.5 text-sm font-mono text-gray-500">#{m.id}</td>
                    <td className="px-5 py-3.5">
                      <p className="text-sm font-semibold text-gray-900">{m.site?.domain || `Site #${m.site_id}`}</p>
                      {m.mode === 'change_domain' && m.new_domain && (
                        <p className="text-xs text-indigo-500 font-mono mt-0.5">→ {m.new_domain}</p>
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-sm text-gray-500">
                      <div className="flex items-center gap-1.5">
                        <span className="text-xs bg-gray-100 px-2 py-0.5 rounded font-mono">{m.source_server?.name || `#${m.source_server_id}`}</span>
                        <ArrowRightLeft size={12} className="text-gray-300" />
                        <span className="text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded font-mono">{m.target_server?.name || `#${m.target_server_id}`}</span>
                      </div>
                    </td>
                    <td className="px-5 py-3.5"><ModeBadge mode={m.mode} /></td>
                    <td className="px-5 py-3.5 w-44"><ProgressBar progress={m.progress} phase={m.phase} /></td>
                    <td className="px-5 py-3.5"><StatusBadge status={m.status} /></td>
                    <td className="px-5 py-3.5 text-sm text-gray-400">
                      <div className="flex items-center gap-1">
                        <Clock size={12} />
                        {new Date(m.started_at).toLocaleDateString('vi-VN')}
                      </div>
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button onClick={() => setViewId(m.id)} className="p-1.5 text-gray-300 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors" title="View details">
                          <Eye size={15} />
                        </button>
                        {m.status === 'running' && can('sites', 'manage') && (
                          <button
                            onClick={() => { if (confirm('Cancel this migration?')) cancel.mutate(m.id) }}
                            className="p-1.5 text-gray-300 hover:text-orange-600 hover:bg-orange-50 rounded-lg transition-colors"
                            title="Cancel"
                          >
                            <XCircle size={15} />
                          </button>
                        )}
                        {m.status === 'failed' && can('sites', 'manage') && (
                          m.mode === 'change_domain' ? (
                            <button
                              onClick={() => { if (confirm(`Cancel the domain change and clean up the new domain "${m.new_domain}"?`)) rollback.mutate(m.id) }}
                              className="p-1.5 text-gray-300 hover:text-amber-600 hover:bg-amber-50 rounded-lg transition-colors"
                              title="Cancel domain change / clean up new domain"
                            >
                              <RotateCcw size={15} />
                            </button>
                          ) : (
                            <button
                              onClick={() => { if (confirm('Restore the site to the source server?')) rollback.mutate(m.id) }}
                              className="p-1.5 text-gray-300 hover:text-amber-600 hover:bg-amber-50 rounded-lg transition-colors"
                              title="Restore site to source server"
                            >
                              <RotateCcw size={15} />
                            </button>
                          )
                        )}
                        {m.status === 'completed' && !m.source_cleaned_up && m.mode !== 'change_domain' && can('sites', 'manage') && (
                          <button
                            onClick={() => { if (confirm('Delete all site files, DB, and configs from the source server? This cannot be undone!')) cleanupSource.mutate(m.id) }}
                            className="p-1.5 text-gray-300 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
                            title="Clean up source server"
                          >
                            <Trash2 size={15} />
                          </button>
                        )}
                        {m.status === 'completed' && m.source_cleaned_up && m.mode !== 'change_domain' && (
                          <span className="p-1.5 text-emerald-400" title="Source server cleaned up">
                            <CheckCircle size={15} />
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile Cards */}
          <div className="space-y-3 md:hidden">
            {migrations.map((m: Migration) => (
              <div key={m.id} className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
                <div className="flex items-start justify-between mb-3">
                  <div>
                    <p className="font-semibold text-gray-900 text-sm">{m.site?.domain || `Site #${m.site_id}`}</p>
                    {m.mode === 'change_domain' && m.new_domain && (
                      <p className="text-xs text-indigo-500 font-mono">→ {m.new_domain}</p>
                    )}
                    <div className="flex items-center gap-1.5 mt-1">
                      <ModeBadge mode={m.mode} />
                      <StatusBadge status={m.status} />
                    </div>
                  </div>
                  <button onClick={() => setViewId(m.id)} className="p-1.5 text-gray-400 hover:text-blue-600 rounded-lg">
                    <Eye size={16} />
                  </button>
                </div>
                <ProgressBar progress={m.progress} phase={m.phase} />
                <div className="flex items-center justify-between mt-3 text-xs text-gray-400">
                  <span className="flex items-center gap-1"><Clock size={11} />{new Date(m.started_at).toLocaleDateString('vi-VN')}</span>
                  <div className="flex gap-1">
                    {m.status === 'running' && can('sites', 'manage') && (
                      <button onClick={() => { if (confirm('Cancel?')) cancel.mutate(m.id) }} className="p-1 text-orange-400 hover:text-orange-600">
                        <XCircle size={14} />
                      </button>
                    )}
                    {m.status === 'failed' && can('sites', 'manage') && (
                      m.mode === 'change_domain' ? (
                        <button onClick={() => { if (confirm(`Cancel the domain change and clean up the new domain "${m.new_domain}"?`)) rollback.mutate(m.id) }} className="p-1 text-amber-400 hover:text-amber-600" title="Cancel domain change">
                          <RotateCcw size={14} />
                        </button>
                      ) : (
                        <button onClick={() => { if (confirm('Restore the site to the source server?')) rollback.mutate(m.id) }} className="p-1 text-amber-400 hover:text-amber-600" title="Restore">
                          <RotateCcw size={14} />
                        </button>
                      )
                    )}
                    {m.status === 'completed' && !m.source_cleaned_up && m.mode !== 'change_domain' && can('sites', 'manage') && (
                      <button onClick={() => { if (confirm('Delete the site from the source server? This cannot be undone!')) cleanupSource.mutate(m.id) }} className="p-1 text-red-400 hover:text-red-600" title="Clean up source server">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>

          {/* Pagination */}
          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between mt-5">
              <p className="text-sm text-gray-500">
                Page {meta.page} / {meta.total_pages} ({meta.total} migrations)
              </p>
              <div className="flex gap-2">
                <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1}
                  className="p-2 border border-gray-200 rounded-xl hover:bg-gray-50 disabled:opacity-40 transition-colors">
                  <ChevronLeft size={16} />
                </button>
                <button onClick={() => setPage(p => p + 1)} disabled={page >= meta.total_pages}
                  className="p-2 border border-gray-200 rounded-xl hover:bg-gray-50 disabled:opacity-40 transition-colors">
                  <ChevronRight size={16} />
                </button>
              </div>
            </div>
          )}
        </>
      )}

      {showStart && <StartMigrationModal onClose={() => setShowStart(false)} />}
      {viewId && <MigrationDetailModal migrationId={viewId} onClose={() => setViewId(null)} />}
    </div>
  )
}
