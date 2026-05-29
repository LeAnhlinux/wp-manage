import { useEffect, useState } from 'react'
import { Loader2, HardDrive, Cpu, RefreshCw, AlertTriangle } from 'lucide-react'
import { useResourceLimits, useUpdateResourceLimits, useScanDisk } from '@/hooks/useResourceLimits'

export function ResourceLimitsPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: limits, isLoading } = useResourceLimits(siteId)
  const update = useUpdateResourceLimits(siteId)
  const scan = useScanDisk(siteId)

  // Disk form
  const [diskGB, setDiskGB] = useState('')
  const [action, setAction] = useState<'none' | 'alert' | 'suspend'>('alert')
  // CPU/RAM form
  const [cores, setCores] = useState('')
  const [memMB, setMemMB] = useState('')
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  useEffect(() => {
    if (!limits) return
    setDiskGB(limits.disk_limit_mb ? (limits.disk_limit_mb / 1024).toString() : '')
    setAction(limits.over_limit_action || 'alert')
    setCores(limits.cpu_quota_percent ? (limits.cpu_quota_percent / 100).toString() : '')
    setMemMB(limits.memory_max_mb ? limits.memory_max_mb.toString() : '')
  }, [limits])

  if (isLoading || !limits) {
    return (
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      </div>
    )
  }

  const usedMB = limits.disk_used_mb || 0
  const limitMB = limits.disk_limit_mb || 0
  const pct = limitMB > 0 ? Math.min(100, Math.round((usedMB / limitMB) * 100)) : 0
  const stateColor = limits.disk_state === 'over' ? 'bg-red-500'
    : limits.disk_state === 'warning' ? 'bg-amber-500' : 'bg-blue-500'
  const stateBadge = limits.disk_state === 'over' ? 'bg-red-100 text-red-700'
    : limits.disk_state === 'warning' ? 'bg-amber-100 text-amber-700' : 'bg-green-100 text-green-700'

  const fmtMB = (mb: number) => mb >= 1024 ? `${(mb / 1024).toFixed(2)} GB` : `${mb} MB`

  const saveDisk = () => {
    setMsg(null)
    const gb = parseFloat(diskGB)
    update.mutate(
      { disk_limit_mb: isNaN(gb) || gb <= 0 ? 0 : Math.round(gb * 1024), over_limit_action: action },
      { onSuccess: () => setMsg({ ok: true, text: 'Disk limit saved' }), onError: (e: any) => setMsg({ ok: false, text: e?.response?.data?.error || 'Error' }) }
    )
  }

  const saveCompute = () => {
    setMsg(null)
    const c = parseFloat(cores)
    const m = parseInt(memMB)
    update.mutate(
      {
        cpu_quota_percent: isNaN(c) || c <= 0 ? 0 : Math.round(c * 100),
        memory_max_mb: isNaN(m) || m <= 0 ? 0 : m,
      },
      { onSuccess: () => setMsg({ ok: true, text: 'CPU/RAM limit applied (dedicated PHP-FPM)' }), onError: (e: any) => setMsg({ ok: false, text: e?.response?.data?.error || 'Error' }) }
    )
  }

  return (
    <div className="space-y-5">
      {msg && (
        <div className={`text-sm rounded-lg px-4 py-2.5 border ${msg.ok ? 'bg-green-50 border-green-200 text-green-700' : 'bg-red-50 border-red-200 text-red-700'}`}>
          {msg.text}
        </div>
      )}

      {/* DISK */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <HardDrive size={20} className="text-blue-500" /> Disk Limit
          </h2>
          <span className={`text-xs font-semibold px-2.5 py-1 rounded-full ${stateBadge}`}>{(limits.disk_state || 'ok').toUpperCase()}</span>
        </div>
        <p className="text-sm text-gray-500 mb-4">Files + database usage. Scanned every 6 hours; exceeding the limit triggers an alert or auto-suspends the site depending on the action below.</p>

        <div className="flex items-center justify-between text-sm mb-1.5">
          <span className="font-medium text-gray-700">{fmtMB(usedMB)} {limitMB > 0 && <span className="text-gray-400">/ {fmtMB(limitMB)}</span>}</span>
          {limitMB > 0 && <span className="text-gray-400">{pct}%</span>}
        </div>
        <div className="w-full bg-gray-100 rounded-full h-2 mb-1">
          <div className={`${stateColor} h-2 rounded-full transition-all duration-500`} style={{ width: `${limitMB > 0 ? pct : 0}%` }} />
        </div>
        <div className="flex items-center justify-between text-xs text-gray-400 mb-4">
          <span>{limits.disk_checked_at ? `Last scanned: ${new Date(limits.disk_checked_at).toLocaleString()}` : 'Not scanned yet'}</span>
          {canManage && (
            <button onClick={() => scan.mutate()} disabled={scan.isPending}
              className="flex items-center gap-1 text-blue-600 hover:text-blue-700 disabled:opacity-50">
              {scan.isPending ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />} Scan now
            </button>
          )}
        </div>

        {canManage && (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 items-end border-t border-gray-100 pt-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Limit (GB)</label>
              <input value={diskGB} onChange={e => setDiskGB(e.target.value)} type="number" min="0" step="0.5" placeholder="0 = unlimited"
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">When over limit</label>
              <select value={action} onChange={e => setAction(e.target.value as any)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                <option value="none">Do nothing</option>
                <option value="alert">Alert only</option>
                <option value="suspend">Auto-suspend site</option>
              </select>
            </div>
            <button onClick={saveDisk} disabled={update.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 flex items-center justify-center gap-2">
              {update.isPending && <Loader2 size={14} className="animate-spin" />} Save disk
            </button>
          </div>
        )}
      </div>

      {/* CPU / RAM */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Cpu size={20} className="text-purple-500" /> CPU &amp; RAM Limit
          {limits.fpm_isolated && <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-purple-100 text-purple-700">Dedicated PHP-FPM</span>}
        </h2>
        <p className="text-sm text-gray-500 mb-4">Hard limit via cgroup. Setting a limit runs a dedicated PHP-FPM for this site (~a few MB RAM for the master). Leave empty for unlimited.</p>

        {canManage ? (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 items-end">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">CPU (cores)</label>
                <input value={cores} onChange={e => setCores(e.target.value)} type="number" min="0" step="0.5" placeholder="e.g. 2"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
                {parseFloat(cores) > 0 && <p className="text-xs text-gray-400 mt-1">= {Math.round(parseFloat(cores) * 100)}% CPU</p>}
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Max RAM (MB)</label>
                <input value={memMB} onChange={e => setMemMB(e.target.value)} type="number" min="0" step="128" placeholder="e.g. 512"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
              </div>
              <button onClick={saveCompute} disabled={update.isPending}
                className="px-4 py-2 text-sm bg-purple-600 text-white rounded-lg hover:bg-purple-700 disabled:opacity-50 flex items-center justify-center gap-2">
                {update.isPending && <Loader2 size={14} className="animate-spin" />} Apply
              </button>
            </div>
            <div className="flex items-start gap-2 mt-3 text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2">
              <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
              <span>Changing CPU/RAM splits (or merges back) the site's PHP-FPM — expect a ~1-2 second interruption when applied.</span>
            </div>
          </>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            <div className="p-3 bg-gray-50 rounded-lg">
              <p className="text-xs text-gray-500 uppercase font-medium mb-0.5">CPU</p>
              <p className="text-base font-semibold text-gray-900">{limits.cpu_quota_percent ? `${limits.cpu_quota_percent / 100} cores` : 'Unlimited'}</p>
            </div>
            <div className="p-3 bg-gray-50 rounded-lg">
              <p className="text-xs text-gray-500 uppercase font-medium mb-0.5">RAM</p>
              <p className="text-base font-semibold text-gray-900">{limits.memory_max_mb ? `${limits.memory_max_mb} MB` : 'Unlimited'}</p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
