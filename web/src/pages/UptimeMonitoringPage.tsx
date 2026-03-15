import { useState } from 'react'
import {
  MonitorDot, Plus, Trash2, ChevronDown, ChevronUp,
  RefreshCw, Activity, Globe, Clock, CheckCircle2, XCircle, MinusCircle, Loader2
} from 'lucide-react'
import { useAllUptimeChecks, useCreateUptimeCheck, useDeleteUptimeCheck, useUptimeHistory, useUpdateUptimeCheck } from '@/hooks/useUptime'
import { useSites } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import { useQueryClient } from '@tanstack/react-query'
import type { UptimeCheck, UptimeLog } from '@/types'

// ── helpers ──────────────────────────────────────────────────────────────────

function statusInfo(check: UptimeCheck): { color: string; dot: string; label: string } {
  if (!check.last_check) return { color: 'text-gray-400', dot: 'bg-gray-300', label: 'No data' }
  if (check.last_status >= 200 && check.last_status < 300)
    return { color: 'text-emerald-600', dot: 'bg-emerald-500', label: 'Online' }
  if (check.last_status >= 400)
    return { color: 'text-red-600', dot: 'bg-red-500', label: 'Offline' }
  return { color: 'text-yellow-600', dot: 'bg-yellow-500', label: 'Warning' }
}

function statusBadge(code: number) {
  if (code === 0) return 'bg-gray-100 text-gray-500'
  if (code >= 200 && code < 300) return 'bg-emerald-100 text-emerald-700'
  if (code >= 400) return 'bg-red-100 text-red-700'
  return 'bg-yellow-100 text-yellow-700'
}

function fmtRelative(iso: string | null) {
  if (!iso) return '—'
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60_000) return 'just now'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`
  return new Date(iso).toLocaleDateString()
}

// ── History row ───────────────────────────────────────────────────────────────

function HistoryPanel({ siteId, checkId }: { siteId: number; checkId: number }) {
  const { data: logs = [], isLoading } = useUptimeHistory(siteId, checkId)

  if (isLoading) return (
    <div className="flex justify-center py-6">
      <Loader2 size={20} className="animate-spin text-blue-500" />
    </div>
  )

  if (logs.length === 0) return (
    <p className="text-sm text-gray-400 text-center py-4">No history recorded yet</p>
  )

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-gray-400 border-b border-gray-100">
            <th className="pb-2 font-medium pr-4">Time</th>
            <th className="pb-2 font-medium pr-4">Status</th>
            <th className="pb-2 font-medium pr-4">Response</th>
            <th className="pb-2 font-medium">Error</th>
          </tr>
        </thead>
        <tbody>
          {logs.slice(0, 30).map((log: UptimeLog) => (
            <tr key={log.id} className="border-b border-gray-50 hover:bg-gray-50">
              <td className="py-1.5 pr-4 text-gray-500 text-xs">{new Date(log.created_at).toLocaleString()}</td>
              <td className="py-1.5 pr-4">
                <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadge(log.status_code)}`}>
                  {log.status_code || 'ERR'}
                </span>
              </td>
              <td className="py-1.5 pr-4 text-gray-600 text-xs">{log.response_ms ? `${log.response_ms}ms` : '—'}</td>
              <td className="py-1.5 text-red-500 text-xs truncate max-w-[240px]">{log.error || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Check row ─────────────────────────────────────────────────────────────────

function CheckRow({
  check, canManage, expanded, onToggleExpand
}: {
  check: UptimeCheck
  canManage: boolean
  expanded: boolean
  onToggleExpand: () => void
}) {
  const deleteCheck = useDeleteUptimeCheck(check.site_id)
  const updateCheck = useUpdateUptimeCheck(check.site_id)
  const { dot, label, color } = statusInfo(check)

  return (
    <>
      <tr
        className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer group"
        onClick={onToggleExpand}
      >
        {/* Status */}
        <td className="py-3 pl-4 pr-3">
          <div className="flex items-center gap-2">
            <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${dot} ${!check.last_check ? '' : 'shadow-sm'}`} />
            <span className={`text-xs font-medium ${color}`}>{label}</span>
          </div>
        </td>

        {/* URL */}
        <td className="py-3 pr-3">
          <p className="text-sm font-medium text-gray-900 truncate max-w-[240px]">{check.url}</p>
        </td>

        {/* Site */}
        <td className="py-3 pr-3 hidden md:table-cell">
          <div className="flex items-center gap-1.5">
            <Globe size={12} className="text-gray-400" />
            <span className="text-sm text-gray-600 truncate max-w-[160px]">
              {check.site?.domain || `Site #${check.site_id}`}
            </span>
          </div>
        </td>

        {/* Last check */}
        <td className="py-3 pr-3 hidden lg:table-cell text-sm text-gray-500">
          {fmtRelative(check.last_check)}
        </td>

        {/* HTTP status */}
        <td className="py-3 pr-3 hidden lg:table-cell">
          {check.last_status > 0 ? (
            <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadge(check.last_status)}`}>
              {check.last_status}
            </span>
          ) : (
            <span className="text-xs text-gray-400">—</span>
          )}
        </td>

        {/* Interval */}
        <td className="py-3 pr-3 hidden xl:table-cell">
          <div className="flex items-center gap-1 text-xs text-gray-500">
            <Clock size={11} />
            {check.interval}s
          </div>
        </td>

        {/* Enabled toggle */}
        <td className="py-3 pr-3">
          <button
            onClick={e => {
              e.stopPropagation()
              if (canManage) updateCheck.mutate({ checkId: check.id, enabled: !check.enabled })
            }}
            disabled={!canManage || updateCheck.isPending}
            className={`px-2 py-0.5 rounded-full text-xs font-medium transition-colors ${
              check.enabled ? 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200' : 'bg-gray-100 text-gray-500 hover:bg-gray-200'
            } disabled:opacity-50`}
          >
            {check.enabled ? 'Active' : 'Paused'}
          </button>
        </td>

        {/* Actions */}
        <td className="py-3 pr-4 text-right">
          <div className="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
            {canManage && (
              <button
                onClick={e => {
                  e.stopPropagation()
                  if (confirm('Delete this uptime check?')) deleteCheck.mutate(check.id)
                }}
                className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors"
              >
                <Trash2 size={14} />
              </button>
            )}
            <button className="p-1.5 rounded-lg text-gray-400">
              {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            </button>
          </div>
        </td>
      </tr>

      {/* Expanded history */}
      {expanded && (
        <tr className="bg-gray-50">
          <td colSpan={8} className="px-6 py-4">
            <p className="text-xs font-semibold text-gray-500 mb-3 uppercase tracking-wide">Check History</p>
            <HistoryPanel siteId={check.site_id} checkId={check.id} />
          </td>
        </tr>
      )}
    </>
  )
}

// ── Add Check Modal ────────────────────────────────────────────────────────────

function AddCheckModal({ onClose }: { onClose: () => void }) {
  const { data: sitesPage } = useSites()
  const sites = sitesPage?.data || []

  const [siteId, setSiteId] = useState(sites[0]?.id ?? 0)
  const [url, setUrl] = useState('')
  const [interval, setIntervalVal] = useState(60)

  const createCheck = useCreateUptimeCheck(siteId)

  // Prefill URL when site changes
  const handleSiteChange = (id: number) => {
    setSiteId(id)
    const site = sites.find(s => s.id === id)
    if (site && !url) setUrl(`https://${site.domain}`)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!siteId) return
    try {
      await createCheck.mutateAsync({ url, interval })
      onClose()
    } catch {}
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white rounded-2xl shadow-xl w-full max-w-md p-6">
        <h3 className="text-lg font-semibold text-gray-900 mb-5">Add Uptime Check</h3>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Site */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Site</label>
            <select
              value={siteId}
              onChange={e => handleSiteChange(Number(e.target.value))}
              className="w-full px-3 py-2 border border-gray-200 rounded-xl bg-gray-50 text-sm focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              required
            >
              <option value={0} disabled>Select a site…</option>
              {sites.map(s => (
                <option key={s.id} value={s.id}>{s.domain}</option>
              ))}
            </select>
          </div>

          {/* URL */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">URL to Monitor</label>
            <input
              type="url"
              value={url}
              onChange={e => setUrl(e.target.value)}
              placeholder="https://example.com"
              required
              className="w-full px-3 py-2 border border-gray-200 rounded-xl bg-gray-50 text-sm focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
            />
          </div>

          {/* Interval */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Check Interval <span className="text-gray-400 font-normal">(seconds)</span>
            </label>
            <div className="flex gap-2">
              {[60, 120, 300, 600].map(v => (
                <button
                  key={v}
                  type="button"
                  onClick={() => setIntervalVal(v)}
                  className={`flex-1 py-1.5 text-xs rounded-lg border transition-colors ${
                    interval === v
                      ? 'border-blue-500 bg-blue-50 text-blue-700 font-medium'
                      : 'border-gray-200 text-gray-600 hover:bg-gray-50'
                  }`}
                >
                  {v < 60 ? `${v}s` : `${v / 60}m`}
                </button>
              ))}
            </div>
          </div>

          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2.5 text-sm text-gray-700 bg-gray-100 rounded-xl hover:bg-gray-200 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={createCheck.isPending || !siteId || !url}
              className="flex-1 py-2.5 text-sm text-white bg-blue-600 rounded-xl hover:bg-blue-700 disabled:opacity-50 transition-colors flex items-center justify-center gap-2"
            >
              {createCheck.isPending ? <><Loader2 size={14} className="animate-spin" /> Adding…</> : 'Add Check'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ── Main Page ──────────────────────────────────────────────────────────────────

export default function UptimeMonitoringPage() {
  const { data: checks = [], isLoading, refetch, isFetching } = useAllUptimeChecks()
  const { can } = usePermissions()
  const canManage = can('sites', 'manage')
  const queryClient = useQueryClient()

  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [showAddModal, setShowAddModal] = useState(false)
  const [filterSite, setFilterSite] = useState<number | 'all'>('all')

  // ── Stats ──
  const total = checks.length
  const online = checks.filter(c => c.last_check && c.last_status >= 200 && c.last_status < 300).length
  const offline = checks.filter(c => c.last_check && c.last_status >= 400).length
  const noData = checks.filter(c => !c.last_check).length

  // ── Unique sites from checks ──
  const sitesInChecks = Array.from(
    new Map(checks.filter(c => c.site).map(c => [c.site_id, c.site!])).values()
  )

  // ── Filtered list ──
  const displayed = filterSite === 'all' ? checks : checks.filter(c => c.site_id === filterSite)

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-blue-50 flex items-center justify-center">
            <MonitorDot size={20} className="text-blue-600" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-gray-900">Uptime Monitoring</h1>
            <p className="text-sm text-gray-500">HTTP availability monitoring across all sites</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => { refetch(); queryClient.invalidateQueries({ queryKey: ['uptime', 'all'] }) }}
            disabled={isFetching}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-gray-600 bg-white border border-gray-200 rounded-xl hover:bg-gray-50 disabled:opacity-50 transition-colors"
          >
            <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
            Refresh
          </button>
          {canManage && (
            <button
              onClick={() => setShowAddModal(true)}
              className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-blue-600 rounded-xl hover:bg-blue-700 transition-colors"
            >
              <Plus size={15} /> Add Check
            </button>
          )}
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        {[
          { label: 'Total Checks', value: total, icon: Activity, color: 'text-blue-600', bg: 'bg-blue-50' },
          { label: 'Online', value: online, icon: CheckCircle2, color: 'text-emerald-600', bg: 'bg-emerald-50' },
          { label: 'Offline', value: offline, icon: XCircle, color: 'text-red-600', bg: 'bg-red-50' },
          { label: 'No Data', value: noData, icon: MinusCircle, color: 'text-gray-500', bg: 'bg-gray-100' },
        ].map(stat => (
          <div key={stat.label} className="bg-white rounded-2xl border border-gray-100 p-4 flex items-center gap-3">
            <div className={`w-10 h-10 rounded-xl ${stat.bg} flex items-center justify-center flex-shrink-0`}>
              <stat.icon size={18} className={stat.color} />
            </div>
            <div>
              <p className="text-2xl font-bold text-gray-900">{stat.value}</p>
              <p className="text-xs text-gray-500">{stat.label}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Filter + Table */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
        {/* Filter bar */}
        {sitesInChecks.length > 1 && (
          <div className="flex items-center gap-2 px-5 py-3 border-b border-gray-100 overflow-x-auto">
            <button
              onClick={() => setFilterSite('all')}
              className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors ${
                filterSite === 'all' ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
              }`}
            >
              All Sites ({checks.length})
            </button>
            {sitesInChecks.map(site => (
              <button
                key={site.id}
                onClick={() => setFilterSite(site.id)}
                className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors ${
                  filterSite === site.id ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                }`}
              >
                {site.domain} ({checks.filter(c => c.site_id === site.id).length})
              </button>
            ))}
          </div>
        )}

        {isLoading ? (
          <div className="flex justify-center py-16">
            <Loader2 size={24} className="animate-spin text-blue-500" />
          </div>
        ) : displayed.length === 0 ? (
          <div className="text-center py-16">
            <MonitorDot size={40} className="text-gray-200 mx-auto mb-3" />
            <p className="text-gray-500 font-medium">No uptime checks yet</p>
            <p className="text-sm text-gray-400 mt-1">
              {canManage ? 'Click "Add Check" to start monitoring your sites.' : 'No checks have been configured.'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left text-xs text-gray-400 uppercase tracking-wide border-b border-gray-100">
                  <th className="pl-4 pr-3 py-3 font-medium">Status</th>
                  <th className="pr-3 py-3 font-medium">URL</th>
                  <th className="pr-3 py-3 font-medium hidden md:table-cell">Site</th>
                  <th className="pr-3 py-3 font-medium hidden lg:table-cell">Last Check</th>
                  <th className="pr-3 py-3 font-medium hidden lg:table-cell">HTTP</th>
                  <th className="pr-3 py-3 font-medium hidden xl:table-cell">Interval</th>
                  <th className="pr-3 py-3 font-medium">State</th>
                  <th className="pr-4 py-3" />
                </tr>
              </thead>
              <tbody>
                {displayed.map(check => (
                  <CheckRow
                    key={check.id}
                    check={check}
                    canManage={canManage}
                    expanded={expandedId === check.id}
                    onToggleExpand={() => setExpandedId(expandedId === check.id ? null : check.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Add Check Modal */}
      {showAddModal && <AddCheckModal onClose={() => setShowAddModal(false)} />}
    </div>
  )
}
