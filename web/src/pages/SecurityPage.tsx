import { useState, useRef, useEffect } from 'react'
import {
  Shield, Search, Loader2, CheckCircle2, AlertTriangle, XCircle,
  Bug, ChevronLeft, ChevronRight, ScanLine, ShieldCheck, ShieldAlert,
  Download, RefreshCw, Server, Database, Clock, Calendar, Trash2,
  ToggleLeft, ToggleRight, Plus, ChevronDown, Eye, ShieldOff, ArrowLeft,
  Save, CheckSquare, Square,
} from 'lucide-react'
import {
  useMalwareScans, useStartMalwareScan, useClamAVStatus, useInstallClamAV,
  useScanSchedules, useCreateScanSchedule, useUpdateScanSchedule, useDeleteScanSchedule,
  useSiteWhitelist, useWhitelistThreat, useQuarantineThreat,
} from '@/hooks/useSecurity'
import { useFileContent, useFileDelete, useSaveFileContent } from '@/hooks/useFiles'
import { useSites } from '@/hooks/useSites'
import { useServers } from '@/hooks/useServers'
import { usePermissions } from '@/hooks/usePermissions'
import type { MalwareScan, MalwareScanSchedule, ThreatDetail, ClamAVStatus, Site } from '@/types'

// ─── Shared badge components ──────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { cls: string; icon: React.ReactNode; label: string }> = {
    running:   { cls: 'bg-blue-50 text-blue-700 border border-blue-200',     icon: <Loader2 size={11} className="animate-spin" />, label: 'Running' },
    completed: { cls: 'bg-emerald-50 text-emerald-700 border border-emerald-200', icon: <CheckCircle2 size={11} />, label: 'Completed' },
    failed:    { cls: 'bg-red-50 text-red-700 border border-red-200',        icon: <XCircle size={11} />, label: 'Failed' },
  }
  const s = map[status] || { cls: 'bg-gray-100 text-gray-600 border border-gray-200', icon: null, label: status }
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${s.cls}`}>
      {s.icon}{s.label}
    </span>
  )
}

function EngineBadge({ engine }: { engine: string }) {
  const map: Record<string, { cls: string; label: string }> = {
    wp_signature: { cls: 'bg-purple-50 text-purple-700 border border-purple-200', label: 'WP Signature' },
    clamav:       { cls: 'bg-orange-50 text-orange-700 border border-orange-200', label: 'ClamAV' },
    both:         { cls: 'bg-blue-50 text-blue-700 border border-blue-200',       label: 'WP + ClamAV' },
  }
  const s = map[engine] || { cls: 'bg-gray-100 text-gray-600 border border-gray-200', label: engine }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${s.cls}`}>
      {s.label}
    </span>
  )
}

function SeverityBadge({ severity }: { severity: string }) {
  const map: Record<string, string> = {
    critical: 'bg-red-100 text-red-700 border border-red-200',
    high:     'bg-orange-100 text-orange-700 border border-orange-200',
    medium:   'bg-yellow-100 text-yellow-700 border border-yellow-200',
    low:      'bg-blue-100 text-blue-700 border border-blue-200',
  }
  return (
    <span className={`px-2 py-0.5 rounded-md text-xs font-semibold uppercase tracking-wide ${map[severity] || 'bg-gray-100 text-gray-600 border border-gray-200'}`}>
      {severity}
    </span>
  )
}

// ─── Searchable site selector ─────────────────────────────────────────────────

interface SearchableSiteSelectProps {
  sites: Site[]
  value: number
  onChange: (id: number, site: Site | null) => void
  placeholder?: string
}

function SearchableSiteSelect({ sites, value, onChange, placeholder = 'Type domain to search...' }: SearchableSiteSelectProps) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const selectedSite = sites.find(s => s.id === value) || null

  // Close on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const filtered = sites
    .filter(s => s.status === 'active')
    .filter(s => !query || s.domain.toLowerCase().includes(query.toLowerCase()))
    .slice(0, 20)

  const displayValue = open ? query : (selectedSite?.domain ?? '')

  const handleSelect = (site: Site) => {
    onChange(site.id, site)
    setQuery('')
    setOpen(false)
  }

  const handleClear = () => {
    onChange(0, null)
    setQuery('')
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      <div className="relative">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
        <input
          type="text"
          value={displayValue}
          onChange={e => { setQuery(e.target.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          placeholder={placeholder}
          className="w-full pl-9 pr-8 py-2.5 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none transition-all"
        />
        {value ? (
          <button onClick={handleClear} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600">
            <XCircle size={14} />
          </button>
        ) : (
          <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
        )}
      </div>

      {open && (
        <div className="absolute z-50 mt-1 w-full bg-white border border-gray-200 rounded-xl shadow-lg overflow-hidden">
          {filtered.length === 0 ? (
            <p className="px-4 py-3 text-sm text-gray-400">
              {query ? `No active sites matching "${query}"` : 'No active sites'}
            </p>
          ) : (
            <ul className="max-h-52 overflow-y-auto divide-y divide-gray-50">
              {filtered.map(site => (
                <li key={site.id}>
                  <button
                    onClick={() => handleSelect(site)}
                    className={`w-full text-left px-4 py-2.5 text-sm hover:bg-blue-50 transition-colors flex items-center justify-between ${
                      site.id === value ? 'bg-blue-50 text-blue-700' : 'text-gray-800'
                    }`}
                  >
                    <span className="font-medium">{site.domain}</span>
                    {site.id === value && <CheckCircle2 size={13} className="text-blue-600 shrink-0" />}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}

// ─── ClamAV per-server card ───────────────────────────────────────────────────

function ClamAVServerCard({ serverId, serverName }: { serverId: number; serverName: string }) {
  const { can } = usePermissions()
  const statusMutation = useClamAVStatus(serverId)
  const installMutation = useInstallClamAV(serverId)
  const [status, setStatus] = useState<ClamAVStatus | null>(null)
  const [checked, setChecked] = useState(false)

  const handleCheck = () => {
    statusMutation.mutate(undefined, {
      onSuccess: (data) => { setStatus(data); setChecked(true) },
    })
  }

  const handleInstall = () => {
    installMutation.mutate(undefined, {
      onSuccess: () => handleCheck(),
    })
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-xl bg-orange-50 flex items-center justify-center">
            <Server size={17} className="text-orange-600" />
          </div>
          <div>
            <p className="font-semibold text-gray-900 text-sm">{serverName}</p>
            <p className="text-xs text-gray-400">Server #{serverId}</p>
          </div>
        </div>
        {checked && status && (
          <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${
            status.installed ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' : 'bg-gray-100 text-gray-600 border border-gray-200'
          }`}>
            {status.installed ? <CheckCircle2 size={11} /> : <XCircle size={11} />}
            {status.installed ? 'Installed' : 'Not Installed'}
          </span>
        )}
      </div>

      {checked && status?.installed && (
        <div className="mb-4 space-y-1.5 text-xs text-gray-500">
          <div className="flex items-center gap-2"><Database size={12} className="text-gray-400" /><span>{status.version}</span></div>
          {status.db_version && <div className="flex items-center gap-2"><Shield size={12} className="text-gray-400" /><span>{status.db_version}</span></div>}
          {status.last_updated && <div className="flex items-center gap-2"><Clock size={12} className="text-gray-400" /><span>DB updated: {status.last_updated}</span></div>}
        </div>
      )}
      {checked && !status?.installed && (
        <p className="text-xs text-gray-500 mb-4">ClamAV is not installed. Install it to enable ClamAV engine for malware scans.</p>
      )}

      <div className="flex gap-2">
        <button
          onClick={handleCheck}
          disabled={statusMutation.isPending}
          className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors disabled:opacity-50"
        >
          {statusMutation.isPending ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
          Check Status
        </button>
        {can('servers', 'manage') && checked && !status?.installed && (
          <button
            onClick={handleInstall}
            disabled={installMutation.isPending}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-semibold bg-orange-600 text-white hover:bg-orange-700 rounded-xl transition-colors disabled:opacity-50 shadow-sm"
          >
            {installMutation.isPending ? <><Loader2 size={13} className="animate-spin" /> Installing...</> : <><Download size={13} /> Install ClamAV</>}
          </button>
        )}
      </div>

      {installMutation.isError && <p className="mt-2 text-xs text-red-600">{(installMutation.error as Error).message}</p>}
      {installMutation.isSuccess && (
        <p className="mt-2 text-xs text-emerald-600 flex items-center gap-1"><CheckCircle2 size={12} /> ClamAV installed successfully!</p>
      )}
    </div>
  )
}

// ─── Scan schedule section ────────────────────────────────────────────────────

const INTERVAL_OPTIONS = [
  { label: 'Every 6 hours',  value: 6 },
  { label: 'Every 12 hours', value: 12 },
  { label: 'Daily',          value: 24 },
  { label: 'Every 2 days',   value: 48 },
  { label: 'Every 3 days',   value: 72 },
  { label: 'Weekly',         value: 168 },
]

function intervalLabel(h: number) {
  return INTERVAL_OPTIONS.find(o => o.value === h)?.label ?? `Every ${h}h`
}

function formatRelative(dt?: string | null) {
  if (!dt) return '—'
  const d = new Date(dt)
  const diff = d.getTime() - Date.now()
  const abs = Math.abs(diff)
  const mins  = Math.floor(abs / 60000)
  const hours = Math.floor(abs / 3600000)
  const days  = Math.floor(abs / 86400000)
  const past  = diff < 0
  if (mins < 60)  return past ? `${mins}m ago` : `in ${mins}m`
  if (hours < 48) return past ? `${hours}h ago` : `in ${hours}h`
  return past ? `${days}d ago` : `in ${days}d`
}

interface ScanScheduleSectionProps {
  sites: Site[]
}

function ScanScheduleSection({ sites }: ScanScheduleSectionProps) {
  const { can } = usePermissions()
  const [selectedSiteId, setSelectedSiteId] = useState<number>(0)
  const [showCreate, setShowCreate] = useState(false)

  const { data: schedules = [], isLoading } = useScanSchedules(selectedSiteId)
  const updateSchedule = useUpdateScanSchedule()
  const deleteSchedule = useDeleteScanSchedule()

  const schedule = schedules[0] ?? null

  const handleToggle = (enabled: boolean) => {
    if (!schedule) return
    updateSchedule.mutate({ siteId: selectedSiteId, id: schedule.id, enabled })
  }

  const handleDelete = () => {
    if (!schedule) return
    deleteSchedule.mutate({ siteId: selectedSiteId, id: schedule.id }, {
      onSuccess: () => setSelectedSiteId(0),
    })
  }

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide">Scheduled Scans</h2>
        {can('sites', 'manage') && (
          <button
            onClick={() => setShowCreate(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold bg-blue-600 text-white rounded-xl hover:bg-blue-700 transition-colors shadow-sm"
          >
            <Plus size={13} /> New Schedule
          </button>
        )}
      </div>

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
        {/* Site picker */}
        <div className="mb-4">
          <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wide mb-1.5">Select site to view / manage schedule</label>
          <div className="max-w-xs">
            <SearchableSiteSelect
              sites={sites}
              value={selectedSiteId}
              onChange={(id) => setSelectedSiteId(id)}
              placeholder="Search site by domain..."
            />
          </div>
        </div>

        {!selectedSiteId && (
          <p className="text-sm text-gray-400 py-4 text-center">Select a site above to view or configure its scan schedule.</p>
        )}

        {selectedSiteId && isLoading && (
          <div className="flex items-center gap-2 text-gray-400 py-4"><Loader2 size={16} className="animate-spin" /> Loading...</div>
        )}

        {selectedSiteId && !isLoading && !schedule && (
          <div className="text-center py-6">
            <div className="w-12 h-12 rounded-2xl bg-gray-100 flex items-center justify-center mx-auto mb-3">
              <Calendar size={22} className="text-gray-300" />
            </div>
            <p className="text-sm font-semibold text-gray-600">No schedule configured</p>
            <p className="text-xs text-gray-400 mt-0.5 mb-4">Automatic scans are not enabled for this site</p>
            {can('sites', 'manage') && (
              <button
                onClick={() => setShowCreate(true)}
                className="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-semibold bg-blue-600 text-white rounded-xl hover:bg-blue-700 transition-colors"
              >
                <Plus size={14} /> Set Up Schedule
              </button>
            )}
          </div>
        )}

        {selectedSiteId && !isLoading && schedule && (
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-2">
              <div className="flex items-center gap-2 flex-wrap">
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${
                  schedule.enabled ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' : 'bg-gray-100 text-gray-500 border border-gray-200'
                }`}>
                  {schedule.enabled ? <CheckCircle2 size={11} /> : <XCircle size={11} />}
                  {schedule.enabled ? 'Active' : 'Paused'}
                </span>
                <EngineBadge engine={schedule.engine} />
                <span className="text-xs text-gray-500 capitalize">{schedule.scan_type} scan</span>
              </div>
              <div className="grid grid-cols-2 gap-x-8 gap-y-1 text-xs text-gray-500">
                <div className="flex items-center gap-1.5">
                  <Clock size={12} className="text-gray-400" />
                  <span>{intervalLabel(schedule.interval_h)}</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <Calendar size={12} className="text-gray-400" />
                  <span>Next: {formatRelative(schedule.next_run_at)}</span>
                </div>
                {schedule.last_run_at && (
                  <div className="flex items-center gap-1.5">
                    <CheckCircle2 size={12} className="text-gray-400" />
                    <span>Last: {formatRelative(schedule.last_run_at)}</span>
                  </div>
                )}
              </div>
            </div>

            {can('sites', 'manage') && (
              <div className="flex items-center gap-2 shrink-0">
                <button
                  onClick={() => handleToggle(!schedule.enabled)}
                  disabled={updateSchedule.isPending}
                  title={schedule.enabled ? 'Pause schedule' : 'Resume schedule'}
                  className="p-2 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors disabled:opacity-50"
                >
                  {schedule.enabled
                    ? <ToggleRight size={18} className="text-emerald-600" />
                    : <ToggleLeft size={18} className="text-gray-400" />
                  }
                </button>
                <button
                  onClick={() => setShowCreate(true)}
                  title="Edit schedule"
                  className="p-2 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors text-xs text-gray-600 font-medium px-3"
                >
                  Edit
                </button>
                <button
                  onClick={handleDelete}
                  disabled={deleteSchedule.isPending}
                  title="Delete schedule"
                  className="p-2 rounded-xl border border-red-100 text-red-500 hover:bg-red-50 transition-colors disabled:opacity-50"
                >
                  {deleteSchedule.isPending ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {showCreate && (
        <ScanScheduleModal
          sites={sites}
          defaultSiteId={selectedSiteId}
          existing={schedule}
          onClose={() => setShowCreate(false)}
          onSaved={(siteId) => { setSelectedSiteId(siteId); setShowCreate(false) }}
        />
      )}
    </div>
  )
}

// ─── Schedule modal (create or edit) ─────────────────────────────────────────

interface ScanScheduleModalProps {
  sites: Site[]
  defaultSiteId: number
  existing: MalwareScanSchedule | null
  onClose: () => void
  onSaved: (siteId: number) => void
}

function ScanScheduleModal({ sites, defaultSiteId, existing, onClose, onSaved }: ScanScheduleModalProps) {
  const [siteId, setSiteId] = useState(defaultSiteId)
  const [scanType, setScanType] = useState<'full' | 'quick'>(existing?.scan_type ?? 'full')
  const [engine, setEngine] = useState<'wp_signature' | 'clamav' | 'both'>(existing?.engine ?? 'wp_signature')
  const [intervalH, setIntervalH] = useState(existing?.interval_h ?? 24)
  const createSchedule = useCreateScanSchedule()
  const updateSchedule = useUpdateScanSchedule()

  const isEdit = !!existing

  const handleSubmit = () => {
    if (!siteId) return
    if (isEdit) {
      updateSchedule.mutate(
        { siteId, id: existing.id, scan_type: scanType, engine, interval_h: intervalH },
        { onSuccess: () => onSaved(siteId) }
      )
    } else {
      createSchedule.mutate(
        { siteId, scan_type: scanType, engine, interval_h: intervalH },
        { onSuccess: () => onSaved(siteId) }
      )
    }
  }

  const isPending = createSchedule.isPending || updateSchedule.isPending
  const error = (createSchedule.error || updateSchedule.error) as Error | null

  const engineOptions = [
    { value: 'wp_signature' as const, label: 'WP Signature', desc: 'Fast regex-based, WP-aware patterns', badge: 'bg-purple-50 text-purple-700 border-purple-200' },
    { value: 'clamav' as const,       label: 'ClamAV',       desc: 'Full antivirus (requires ClamAV installed)', badge: 'bg-orange-50 text-orange-700 border-orange-200' },
    { value: 'both' as const,         label: 'Both',         desc: 'WP Signature + ClamAV (most thorough)', badge: 'bg-blue-50 text-blue-700 border-blue-200' },
  ]

  return (
    <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-2xl w-full max-w-md">
        <div className="p-6 border-b border-gray-100">
          <div className="flex items-center gap-3 mb-1">
            <div className="w-10 h-10 rounded-xl bg-blue-100 flex items-center justify-center">
              <Calendar size={20} className="text-blue-600" />
            </div>
            <div>
              <h3 className="text-lg font-bold text-gray-900">{isEdit ? 'Edit' : 'New'} Scan Schedule</h3>
              <p className="text-sm text-gray-500">Automatic periodic malware scanning</p>
            </div>
          </div>
        </div>

        <div className="p-6 space-y-5">
          {/* Site selector */}
          {!isEdit && (
            <div>
              <label className="block text-sm font-semibold text-gray-700 mb-1.5">Site</label>
              <SearchableSiteSelect
                sites={sites}
                value={siteId}
                onChange={(id) => setSiteId(id)}
              />
            </div>
          )}

          {/* Interval */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Frequency</label>
            <div className="grid grid-cols-3 gap-2">
              {INTERVAL_OPTIONS.map(opt => (
                <button
                  key={opt.value}
                  onClick={() => setIntervalH(opt.value)}
                  className={`py-2.5 px-2 border-2 rounded-xl text-xs text-center transition-all ${
                    intervalH === opt.value ? 'border-blue-600 bg-blue-50 text-blue-700 font-semibold' : 'border-gray-200 text-gray-600 hover:border-gray-300 hover:bg-gray-50'
                  }`}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          {/* Scan type */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Scan Scope</label>
            <div className="grid grid-cols-2 gap-3">
              {(['full', 'quick'] as const).map(t => (
                <button key={t} onClick={() => setScanType(t)}
                  className={`p-4 border-2 rounded-xl text-sm text-left transition-all ${
                    scanType === t ? 'border-blue-600 bg-blue-50' : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                  }`}>
                  <p className="font-semibold text-gray-900 capitalize">{t} Scan</p>
                  <p className="text-xs text-gray-500 mt-0.5">{t === 'full' ? 'PHP, JS, HTML, htaccess' : 'Uploads & plugins only'}</p>
                </button>
              ))}
            </div>
          </div>

          {/* Engine */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Scan Engine</label>
            <div className="space-y-2">
              {engineOptions.map(opt => (
                <button key={opt.value} onClick={() => setEngine(opt.value)}
                  className={`w-full flex items-center gap-3 p-3.5 border-2 rounded-xl text-left transition-all ${
                    engine === opt.value ? 'border-blue-600 bg-blue-50' : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                  }`}>
                  <span className={`px-2 py-0.5 rounded-md text-xs font-semibold border shrink-0 ${opt.badge}`}>{opt.label}</span>
                  <span className="text-xs text-gray-500">{opt.desc}</span>
                  {engine === opt.value && <CheckCircle2 size={15} className="text-blue-600 ml-auto shrink-0" />}
                </button>
              ))}
            </div>
          </div>

          {error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700 flex items-center gap-2">
              <XCircle size={15} className="shrink-0" />{error.message}
            </div>
          )}
        </div>

        <div className="px-6 pb-6 flex gap-3">
          <button onClick={onClose} className="flex-1 px-4 py-2.5 text-sm font-medium bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors">
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!siteId || isPending}
            className="flex-1 px-4 py-2.5 text-sm font-semibold bg-blue-600 text-white hover:bg-blue-700 rounded-xl disabled:opacity-50 flex items-center justify-center gap-2 transition-colors shadow-sm"
          >
            {isPending ? <Loader2 size={15} className="animate-spin" /> : <Calendar size={15} />}
            {isEdit ? 'Save Changes' : 'Create Schedule'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Scan detail modal ────────────────────────────────────────────────────────

// Editor so users can inspect AND fix a flagged file (e.g. strip injected code) in place.
function ThreatFileEditor({ siteId, filePath, onBack }: { siteId: number; filePath: string; onBack: () => void }) {
  const { data, isLoading, error } = useFileContent(siteId, filePath, true)
  const saveMut = useSaveFileContent()
  const [content, setContent] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)
  const [note, setNote] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  useEffect(() => { if (data?.content != null) { setContent(data.content); setDirty(false) } }, [data?.content])

  const save = async () => {
    if (content == null) return
    setNote(null)
    try {
      await saveMut.mutateAsync({ siteId, req: { path: filePath, content } })
      setDirty(false)
      setNote({ type: 'success', text: 'Saved' })
    } catch (e) {
      setNote({ type: 'error', text: (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to save' })
    }
  }

  return (
    <div className="flex flex-col min-h-0">
      <div className="flex items-center justify-between mb-3 shrink-0">
        <button onClick={onBack} className="flex items-center gap-1.5 text-sm text-blue-600 hover:underline">
          <ArrowLeft size={14} /> Back to threats
        </button>
        <div className="flex items-center gap-2">
          {note && <span className={`text-xs font-medium ${note.type === 'success' ? 'text-emerald-600' : 'text-red-600'}`}>{note.text}</span>}
          <button onClick={save} disabled={!dirty || saveMut.isPending || content == null}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
            {saveMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Save size={13} />} Save
          </button>
        </div>
      </div>
      <code className="text-xs text-gray-500 break-all mb-2 shrink-0">{filePath}</code>
      {isLoading ? (
        <div className="flex items-center justify-center py-10 text-gray-400"><Loader2 size={18} className="animate-spin" /></div>
      ) : error ? (
        <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">Could not open file — it may have been quarantined or deleted.</div>
      ) : (
        <textarea
          value={content ?? ''}
          onChange={e => { setContent(e.target.value); setDirty(true) }}
          spellCheck={false}
          className="w-full text-xs font-mono bg-gray-900 text-gray-100 p-4 rounded-xl whitespace-pre resize-none max-h-[50vh] min-h-[40vh] overflow-auto focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      )}
    </div>
  )
}

function ScanDetailModal({ scan, onClose }: { scan: MalwareScan; onClose: () => void }) {
  const siteId = scan.site_id
  let threats: ThreatDetail[] = []
  try {
    // threats_detail is "null" for clean scans (Go marshals a nil slice as null),
    // so guard against JSON.parse returning null before using array methods.
    const parsed = scan.threats_detail ? JSON.parse(scan.threats_detail) : null
    if (Array.isArray(parsed)) threats = parsed
  } catch { /* empty */ }

  const { data: whitelist = [] } = useSiteWhitelist(siteId)
  const whitelistMut = useWhitelistThreat()
  const quarantineMut = useQuarantineThreat()
  const deleteMut = useFileDelete()

  const [handled, setHandled] = useState<Record<string, 'safe' | 'quarantined' | 'deleted'>>({})
  const [busy, setBusy] = useState<string | null>(null)
  const [openFile, setOpenFile] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<ThreatDetail | null>(null)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)

  const flash = (type: 'success' | 'error', text: string) => { setMsg({ type, text }); setTimeout(() => setMsg(null), 4000) }
  const errText = (e: unknown, fallback: string) => (e as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback
  const toPublicPath = (f: string) => '/public/' + f.replace(/^\/+/, '')

  // Hide threats already whitelisted server-side or handled in this session.
  const whitelistedPaths = new Set(whitelist.map(w => w.file_path))
  const visible = threats.filter(t => !whitelistedPaths.has(t.file) && handled[t.file] !== 'safe')

  const markSafe = async (t: ThreatDetail) => {
    setBusy(t.file)
    try {
      await whitelistMut.mutateAsync({ siteId, file: t.file, threat_type: t.threat_type })
      setHandled(h => ({ ...h, [t.file]: 'safe' }))
      flash('success', `Marked safe — future scans will skip this file`)
    } catch (e) { flash('error', errText(e, 'Failed to mark safe')) } finally { setBusy(null) }
  }

  const quarantine = async (t: ThreatDetail) => {
    setBusy(t.file)
    try {
      await quarantineMut.mutateAsync({ siteId, file: t.file, threat_type: t.threat_type })
      setHandled(h => ({ ...h, [t.file]: 'quarantined' }))
      flash('success', `Quarantined ${t.file.split('/').pop()}`)
    } catch (e) { flash('error', errText(e, 'Failed to quarantine')) } finally { setBusy(null) }
  }

  const doDelete = async (t: ThreatDetail) => {
    setBusy(t.file); setConfirmDelete(null)
    try {
      await deleteMut.mutateAsync({ siteId, req: { path: toPublicPath(t.file) } })
      setHandled(h => ({ ...h, [t.file]: 'deleted' }))
      flash('success', `Deleted ${t.file.split('/').pop()}`)
    } catch (e) { flash('error', errText(e, 'Failed to delete')) } finally { setBusy(null) }
  }

  // --- Bulk selection / actions ---
  // Selectable = visible threats not yet handled in this session.
  const actionable = visible.filter(t => !handled[t.file])
  const allSelected = actionable.length > 0 && actionable.every(t => selected.has(t.file))
  const selCount = actionable.filter(t => selected.has(t.file)).length

  const toggleSel = (f: string) => setSelected(s => { const n = new Set(s); n.has(f) ? n.delete(f) : n.add(f); return n })
  const toggleAll = () => setSelected(allSelected ? new Set() : new Set(actionable.map(t => t.file)))

  const runBulk = async (label: string, fn: (t: ThreatDetail) => Promise<unknown>, mark: 'safe' | 'quarantined' | 'deleted') => {
    const targets = actionable.filter(t => selected.has(t.file))
    if (targets.length === 0) return
    setBulkBusy(true)
    const results = await Promise.allSettled(targets.map(fn))
    const okFiles = targets.filter((_, i) => results[i].status === 'fulfilled').map(t => t.file)
    const failed = targets.length - okFiles.length
    setHandled(h => { const n = { ...h }; okFiles.forEach(f => { n[f] = mark }); return n })
    setSelected(new Set()); setBulkBusy(false)
    flash(failed ? 'error' : 'success', `${label}: ${okFiles.length} done${failed ? `, ${failed} failed` : ''}`)
  }

  const bulkMarkSafe = () => runBulk('Marked safe', t => whitelistMut.mutateAsync({ siteId, file: t.file, threat_type: t.threat_type }), 'safe')
  const bulkQuarantine = () => runBulk('Quarantined', t => quarantineMut.mutateAsync({ siteId, file: t.file, threat_type: t.threat_type }), 'quarantined')
  const bulkDelete = () => { setConfirmBulkDelete(false); runBulk('Deleted', t => deleteMut.mutateAsync({ siteId, req: { path: toPublicPath(t.file) } }), 'deleted') }

  const isClean = scan.status === 'completed' && threats.length === 0

  return (
    <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-2xl w-full max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="p-6 border-b border-gray-100 flex items-start justify-between">
          <div>
            <h3 className="text-lg font-bold text-gray-900">Scan #{scan.id} — {scan.site?.domain || `Site #${scan.site_id}`}</h3>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-sm text-gray-500 capitalize">{scan.scan_type} scan</span>
              <EngineBadge engine={scan.engine || 'wp_signature'} />
            </div>
          </div>
          <StatusBadge status={scan.status} />
        </div>

        <div className="px-6 pt-5 pb-4 grid grid-cols-3 gap-3">
          <div className="text-center p-4 bg-gray-50 rounded-xl border border-gray-100">
            <p className="text-2xl font-bold text-gray-900">{scan.files_scanned.toLocaleString()}</p>
            <p className="text-xs text-gray-500 mt-0.5">Files Scanned</p>
          </div>
          <div className={`text-center p-4 rounded-xl border ${scan.threats_found > 0 ? 'bg-red-50 border-red-100' : 'bg-emerald-50 border-emerald-100'}`}>
            <p className={`text-2xl font-bold ${scan.threats_found > 0 ? 'text-red-600' : 'text-emerald-600'}`}>{scan.threats_found}</p>
            <p className="text-xs text-gray-500 mt-0.5">Threats Found</p>
          </div>
          <div className="text-center p-4 bg-gray-50 rounded-xl border border-gray-100">
            <p className="text-sm font-semibold text-gray-700">{new Date(scan.started_at).toLocaleString()}</p>
            <p className="text-xs text-gray-500 mt-0.5">Started</p>
          </div>
        </div>

        <div className="px-6 pb-6 flex-1 overflow-y-auto space-y-4">
          {msg && (
            <div className={`p-3 rounded-xl text-sm border ${msg.type === 'success' ? 'bg-emerald-50 border-emerald-200 text-emerald-700' : 'bg-red-50 border-red-200 text-red-700'}`}>{msg.text}</div>
          )}
          {scan.error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700 flex items-start gap-2">
              <XCircle size={15} className="mt-0.5 shrink-0" />{scan.error}
            </div>
          )}
          {openFile ? (
            <ThreatFileEditor siteId={siteId} filePath={toPublicPath(openFile)} onBack={() => setOpenFile(null)} />
          ) : isClean ? (
            <div className="text-center py-6">
              <div className="w-14 h-14 rounded-full bg-emerald-100 flex items-center justify-center mx-auto mb-3">
                <ShieldCheck size={28} className="text-emerald-600" />
              </div>
              <p className="text-emerald-700 font-semibold">No threats detected</p>
              <p className="text-sm text-gray-500 mt-1">Your site appears to be clean</p>
            </div>
          ) : threats.length > 0 ? (
            <div>
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-sm font-semibold text-gray-700 flex items-center gap-2">
                  <AlertTriangle size={14} className="text-orange-500" />
                  {visible.length} Threat{visible.length !== 1 ? 's' : ''} {visible.length !== threats.length ? `· ${threats.length - visible.length} handled` : 'Detected'}
                </h4>
                {actionable.length > 0 && (
                  <button onClick={toggleAll} className="flex items-center gap-1.5 text-xs font-medium text-gray-600 hover:text-blue-600">
                    {allSelected ? <CheckSquare size={14} className="text-blue-600" /> : <Square size={14} />} Select all
                  </button>
                )}
              </div>
              {selCount > 0 && (
                <div className="flex flex-wrap items-center gap-2 mb-3 px-3 py-2 bg-blue-50 border border-blue-200 rounded-xl sticky top-0 z-10">
                  <span className="text-sm font-medium text-blue-800">{selCount} selected</span>
                  <div className="flex-1" />
                  <button onClick={bulkMarkSafe} disabled={bulkBusy}
                    className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-emerald-700 bg-white border border-emerald-200 rounded-lg hover:bg-emerald-50 disabled:opacity-50">
                    {bulkBusy ? <Loader2 size={13} className="animate-spin" /> : <ShieldCheck size={13} />} Mark safe
                  </button>
                  <button onClick={bulkQuarantine} disabled={bulkBusy}
                    className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-amber-700 bg-white border border-amber-200 rounded-lg hover:bg-amber-50 disabled:opacity-50">
                    {bulkBusy ? <Loader2 size={13} className="animate-spin" /> : <ShieldOff size={13} />} Quarantine
                  </button>
                  <button onClick={() => setConfirmBulkDelete(true)} disabled={bulkBusy}
                    className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-red-700 bg-white border border-red-200 rounded-lg hover:bg-red-50 disabled:opacity-50">
                    <Trash2 size={13} /> Delete
                  </button>
                  <button onClick={() => setSelected(new Set())} className="p-1 text-blue-400 hover:text-blue-600"><XCircle size={14} /></button>
                </div>
              )}
              {visible.length === 0 ? (
                <div className="text-center py-6 text-sm text-emerald-700 font-medium flex items-center justify-center gap-2">
                  <ShieldCheck size={16} /> All threats handled
                </div>
              ) : (
                <div className="space-y-2">
                  {visible.map((threat, idx) => {
                    const state = handled[threat.file]
                    const isBusy = busy === threat.file
                    return (
                      <div key={idx} className={`p-4 border rounded-xl ${selected.has(threat.file) ? 'bg-blue-50 border-blue-200' : 'bg-red-50 border-red-100'}`}>
                        <div className="flex items-start gap-2 mb-1.5">
                          {!state && (
                            <button onClick={() => toggleSel(threat.file)} className="mt-0.5 shrink-0" title="Select">
                              {selected.has(threat.file) ? <CheckSquare size={15} className="text-blue-600" /> : <Square size={15} className="text-gray-400" />}
                            </button>
                          )}
                          <code className="text-sm font-mono text-red-800 break-all flex-1">{threat.file}</code>
                          <SeverityBadge severity={threat.severity} />
                        </div>
                        <p className="text-xs text-gray-600 mb-1">{threat.description}</p>
                        <span className="inline-block text-xs text-red-600 font-semibold bg-red-100 px-2 py-0.5 rounded-md">{threat.threat_type}</span>
                        {state === 'quarantined' ? (
                          <div className="mt-3 text-xs text-amber-700 font-medium flex items-center gap-1.5"><ShieldOff size={13} /> Quarantined — manage under Security → Quarantine</div>
                        ) : (
                          <div className="mt-3 flex flex-wrap items-center gap-2">
                            <button onClick={() => setOpenFile(threat.file)} disabled={isBusy}
                              className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50">
                              <Eye size={13} /> Open
                            </button>
                            <button onClick={() => markSafe(threat)} disabled={isBusy}
                              className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-emerald-700 bg-white border border-emerald-200 rounded-lg hover:bg-emerald-50 disabled:opacity-50">
                              {isBusy ? <Loader2 size={13} className="animate-spin" /> : <ShieldCheck size={13} />} Mark safe
                            </button>
                            <button onClick={() => quarantine(threat)} disabled={isBusy}
                              className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-amber-700 bg-white border border-amber-200 rounded-lg hover:bg-amber-50 disabled:opacity-50">
                              {isBusy ? <Loader2 size={13} className="animate-spin" /> : <ShieldOff size={13} />} Quarantine
                            </button>
                            <button onClick={() => setConfirmDelete(threat)} disabled={isBusy}
                              className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-red-700 bg-white border border-red-200 rounded-lg hover:bg-red-50 disabled:opacity-50">
                              <Trash2 size={13} /> Delete
                            </button>
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          ) : null}
        </div>

        <div className="px-6 py-4 border-t border-gray-100">
          <button onClick={onClose} className="w-full px-4 py-2.5 text-sm font-medium bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors">
            Close
          </button>
        </div>
      </div>

      {confirmDelete && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4" onClick={() => setConfirmDelete(null)}>
          <div className="bg-white rounded-xl p-6 max-w-sm w-full shadow-xl" onClick={e => e.stopPropagation()}>
            <h4 className="text-base font-semibold text-gray-900 mb-2 flex items-center gap-2"><AlertTriangle size={18} className="text-red-500" /> Delete file?</h4>
            <p className="text-sm text-gray-600 mb-1">This permanently deletes:</p>
            <code className="block text-xs font-mono text-red-700 bg-red-50 p-2 rounded mb-3 break-all">{confirmDelete.file}</code>
            <p className="text-xs text-gray-500 mb-4">This cannot be undone. If unsure, use Quarantine instead.</p>
            <div className="flex gap-3">
              <button onClick={() => setConfirmDelete(null)} className="flex-1 px-4 py-2 text-sm bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200">Cancel</button>
              <button onClick={() => doDelete(confirmDelete)} className="flex-1 px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700">Delete</button>
            </div>
          </div>
        </div>
      )}

      {confirmBulkDelete && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4" onClick={() => setConfirmBulkDelete(false)}>
          <div className="bg-white rounded-xl p-6 max-w-sm w-full shadow-xl" onClick={e => e.stopPropagation()}>
            <h4 className="text-base font-semibold text-gray-900 mb-2 flex items-center gap-2"><AlertTriangle size={18} className="text-red-500" /> Delete {selCount} file{selCount !== 1 ? 's' : ''}?</h4>
            <p className="text-xs text-gray-500 mb-4">This permanently deletes the selected files. This cannot be undone. If unsure, use Quarantine instead.</p>
            <div className="flex gap-3">
              <button onClick={() => setConfirmBulkDelete(false)} className="flex-1 px-4 py-2 text-sm bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200">Cancel</button>
              <button onClick={bulkDelete} className="flex-1 px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700">Delete {selCount}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function SecurityPage() {
  const [page, setPage] = useState(1)
  const [showStartScan, setShowStartScan] = useState(false)
  const [viewScan, setViewScan] = useState<MalwareScan | null>(null)
  const { data, isLoading } = useMalwareScans(page)
  const { can } = usePermissions()

  const scans: MalwareScan[] = data?.data || []
  const meta = data?.meta

  const totalScans  = meta?.total ?? scans.length
  const running     = scans.filter(s => s.status === 'running').length
  const withThreats = scans.filter(s => s.threats_found > 0).length
  const clean       = scans.filter(s => s.status === 'completed' && s.threats_found === 0).length

  const { data: serversData } = useServers()
  const { data: sitesData } = useSites()
  const servers = serversData?.data || []
  const sites   = sitesData?.data || []

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Shield className="text-blue-600" size={28} />
          <h1 className="text-2xl font-bold text-gray-900">Security</h1>
        </div>
        {can('sites', 'manage') && (
          <button
            onClick={() => setShowStartScan(true)}
            className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 text-sm font-semibold shadow-sm transition-colors"
          >
            <ScanLine size={16} />New Scan
          </button>
        )}
      </div>

      {/* ClamAV Section */}
      <div className="mb-6">
        <h2 className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-3">ClamAV — Antivirus Engine</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {servers.map(s => (
            <ClamAVServerCard key={s.id} serverId={s.id} serverName={s.name || s.ip_address} />
          ))}
          {servers.length === 0 && <p className="text-sm text-gray-400 col-span-3">No servers found.</p>}
        </div>
      </div>

      {/* Scheduled Scans Section */}
      <ScanScheduleSection sites={sites} />

      {/* Stats Row */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-500 font-medium uppercase tracking-wide">Total Scans</p>
          <p className="text-3xl font-bold text-gray-900 mt-1">{totalScans}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-500 font-medium uppercase tracking-wide">Clean</p>
          <p className="text-3xl font-bold text-emerald-600 mt-1">{clean}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-500 font-medium uppercase tracking-wide">With Threats</p>
          <p className={`text-3xl font-bold mt-1 ${withThreats > 0 ? 'text-red-600' : 'text-gray-900'}`}>{withThreats}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-500 font-medium uppercase tracking-wide">Running</p>
          <p className={`text-3xl font-bold mt-1 ${running > 0 ? 'text-blue-600' : 'text-gray-900'}`}>{running}</p>
        </div>
      </div>

      {/* Scans Table */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-gray-400">
            <Loader2 size={24} className="animate-spin mr-2" />Loading scans...
          </div>
        ) : scans.length === 0 ? (
          <div className="text-center py-16">
            <div className="w-16 h-16 rounded-2xl bg-gray-100 flex items-center justify-center mx-auto mb-4">
              <Bug size={32} className="text-gray-300" />
            </div>
            <p className="font-semibold text-gray-700">No malware scans yet</p>
            <p className="text-sm text-gray-400 mt-1">Run a scan to check your sites for threats</p>
            {can('sites', 'manage') && (
              <button
                onClick={() => setShowStartScan(true)}
                className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-xl text-sm font-medium hover:bg-blue-700 transition-colors"
              >
                <ScanLine size={15} />Start First Scan
              </button>
            )}
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-gray-100">
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">ID</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Site</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Type</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Engine</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Status</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Files</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Threats</th>
                    <th className="text-left px-5 py-3.5 text-xs font-semibold text-gray-500 uppercase tracking-wide">Started</th>
                    <th className="px-5 py-3.5"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {scans.map((scan: MalwareScan) => (
                    <tr key={scan.id} className="hover:bg-gray-50/60 transition-colors">
                      <td className="px-5 py-4 text-sm font-mono text-gray-500">#{scan.id}</td>
                      <td className="px-5 py-4 text-sm font-semibold text-gray-900">{scan.site?.domain || `Site #${scan.site_id}`}</td>
                      <td className="px-5 py-4 text-sm text-gray-600 capitalize">{scan.scan_type}</td>
                      <td className="px-5 py-4"><EngineBadge engine={scan.engine || 'wp_signature'} /></td>
                      <td className="px-5 py-4"><StatusBadge status={scan.status} /></td>
                      <td className="px-5 py-4 text-sm text-gray-700">{scan.files_scanned.toLocaleString()}</td>
                      <td className="px-5 py-4">
                        {scan.status === 'running' ? (
                          <span className="text-sm text-gray-400">—</span>
                        ) : scan.threats_found > 0 ? (
                          <span className="inline-flex items-center gap-1 text-sm font-semibold text-red-600">
                            <ShieldAlert size={14} />{scan.threats_found}
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-sm font-semibold text-emerald-600">
                            <ShieldCheck size={14} />Clean
                          </span>
                        )}
                      </td>
                      <td className="px-5 py-4 text-sm text-gray-500">
                        {new Date(scan.started_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                      </td>
                      <td className="px-5 py-4 text-right">
                        <button onClick={() => setViewScan(scan)} className="text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline">
                          View
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {meta && meta.total_pages > 1 && (
              <div className="flex items-center justify-between px-5 py-4 border-t border-gray-100">
                <p className="text-sm text-gray-500">Page {meta.page} of {meta.total_pages} ({meta.total} total)</p>
                <div className="flex gap-2">
                  <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1} className="p-2 border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-40 transition-colors">
                    <ChevronLeft size={16} />
                  </button>
                  <button onClick={() => setPage(p => p + 1)} disabled={page >= meta.total_pages} className="p-2 border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-40 transition-colors">
                    <ChevronRight size={16} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {showStartScan && <StartScanModal sites={sites} servers={servers} onClose={() => setShowStartScan(false)} />}
      {viewScan && <ScanDetailModal scan={viewScan} onClose={() => setViewScan(null)} />}
    </div>
  )
}

// ─── Start scan modal (with searchable site selector) ─────────────────────────

function StartScanModal({ sites, servers, onClose }: { sites: Site[]; servers: { id: number; name?: string; ip_address: string }[]; onClose: () => void }) {
  const [siteId, setSiteId] = useState(0)
  const [selectedSite, setSelectedSite] = useState<Site | null>(null)
  const [scanType, setScanType] = useState<'full' | 'quick'>('full')
  const [engine, setEngine] = useState<'wp_signature' | 'clamav' | 'both'>('wp_signature')
  const startScan = useStartMalwareScan()

  const selectedServer = servers.find(s => s.id === selectedSite?.server_id)

  const handleSubmit = () => {
    if (!siteId) return
    startScan.mutate(
      { siteId, scan_type: scanType, engine },
      { onSuccess: () => onClose() }
    )
  }

  const engineOptions = [
    { value: 'wp_signature' as const, label: 'WP Signature', desc: 'Fast regex-based scan, WP-aware patterns',            badge: 'bg-purple-50 text-purple-700 border-purple-200' },
    { value: 'clamav' as const,       label: 'ClamAV',       desc: 'Full antivirus scan (requires ClamAV installed)',     badge: 'bg-orange-50 text-orange-700 border-orange-200' },
    { value: 'both' as const,         label: 'Both',         desc: 'WP Signature + ClamAV combined (most thorough)',      badge: 'bg-blue-50 text-blue-700 border-blue-200' },
  ]

  return (
    <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-2xl w-full max-w-md">
        <div className="p-6 border-b border-gray-100">
          <div className="flex items-center gap-3 mb-1">
            <div className="w-10 h-10 rounded-xl bg-blue-100 flex items-center justify-center">
              <ScanLine size={20} className="text-blue-600" />
            </div>
            <div>
              <h3 className="text-lg font-bold text-gray-900">New Malware Scan</h3>
              <p className="text-sm text-gray-500">Scan a WordPress site for threats</p>
            </div>
          </div>
        </div>

        <div className="p-6 space-y-5">
          {/* Searchable site selector */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Site</label>
            <SearchableSiteSelect
              sites={sites}
              value={siteId}
              onChange={(id, site) => { setSiteId(id); setSelectedSite(site) }}
            />
            {selectedServer && (
              <p className="text-xs text-gray-400 mt-1 flex items-center gap-1">
                <Server size={11} /> Server: {selectedServer.name || selectedServer.ip_address}
              </p>
            )}
          </div>

          {/* Scan type */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Scan Scope</label>
            <div className="grid grid-cols-2 gap-3">
              {(['full', 'quick'] as const).map(t => (
                <button key={t} onClick={() => setScanType(t)}
                  className={`p-4 border-2 rounded-xl text-sm text-left transition-all ${
                    scanType === t ? 'border-blue-600 bg-blue-50' : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                  }`}>
                  <p className="font-semibold text-gray-900 capitalize">{t} Scan</p>
                  <p className="text-xs text-gray-500 mt-0.5">{t === 'full' ? 'PHP, JS, HTML, htaccess' : 'Uploads & plugins only'}</p>
                </button>
              ))}
            </div>
          </div>

          {/* Engine selector */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">Scan Engine</label>
            <div className="space-y-2">
              {engineOptions.map(opt => (
                <button key={opt.value} onClick={() => setEngine(opt.value)}
                  className={`w-full flex items-center gap-3 p-3.5 border-2 rounded-xl text-left transition-all ${
                    engine === opt.value ? 'border-blue-600 bg-blue-50' : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                  }`}>
                  <span className={`px-2 py-0.5 rounded-md text-xs font-semibold border shrink-0 ${opt.badge}`}>{opt.label}</span>
                  <span className="text-xs text-gray-500">{opt.desc}</span>
                  {engine === opt.value && <CheckCircle2 size={15} className="text-blue-600 ml-auto shrink-0" />}
                </button>
              ))}
            </div>
            {(engine === 'clamav' || engine === 'both') && (
              <p className="mt-2 text-xs text-orange-600 flex items-center gap-1.5">
                <AlertTriangle size={12} />
                ClamAV must be installed on the target server. Check status in the ClamAV section above.
              </p>
            )}
          </div>

          {startScan.isError && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700 flex items-center gap-2">
              <XCircle size={15} className="shrink-0" />
              {(startScan.error as Error).message || 'Failed to start scan'}
            </div>
          )}
        </div>

        <div className="px-6 pb-6 flex gap-3">
          <button onClick={onClose} className="flex-1 px-4 py-2.5 text-sm font-medium bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors">
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!siteId || startScan.isPending}
            className="flex-1 px-4 py-2.5 text-sm font-semibold bg-blue-600 text-white hover:bg-blue-700 rounded-xl disabled:opacity-50 flex items-center justify-center gap-2 transition-colors shadow-sm"
          >
            {startScan.isPending ? <Loader2 size={15} className="animate-spin" /> : <ScanLine size={15} />}
            Start Scan
          </button>
        </div>
      </div>
    </div>
  )
}
