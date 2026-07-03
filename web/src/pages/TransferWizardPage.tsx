import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Loader2, ChevronLeft, CheckCircle2, AlertCircle, XCircle, ArrowRight,
  Server, Globe, Shield, ChevronDown, ChevronUp, LayoutDashboard,
  RotateCcw, AlertTriangle, Clock, Wifi, ExternalLink,
} from 'lucide-react'
import {
  useTransferJob,
  useConnectTransfer,
  useStartTransfer,
  useCancelTransfer,
  useDeleteTransferJob,
  useCheckDomain,
  useTestConnection,
} from '@/hooks/useTransfer'
import type { TransferTestConnectionResult } from '@/types'
import { useServers } from '@/hooks/useServers'
import { useWebSocket } from '@/hooks/useWebSocket'
import { cn } from '@/lib/utils'
import type {
  TransferJob,
  TransferSiteResult,
  DiscoveredSiteInfo,
  SiteImportOption,
} from '@/types'

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatBytes(n: number): string {
  if (!n || n === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(n) / Math.log(k))
  return parseFloat((n / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '—'
  return new Date(dateStr).toLocaleString('vi-VN', {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

// ─── Step indicator ───────────────────────────────────────────────────────────

const STEPS = [
  { label: 'Connect' },
  { label: 'Select sites' },
  { label: 'Transfer' },
]

function StepIndicator({ current }: { current: 0 | 1 | 2 }) {
  return (
    <div className="flex items-center gap-0 mb-8">
      {STEPS.map((step, idx) => {
        const done = idx < current
        const active = idx === current
        return (
          <div key={step.label} className="flex items-center">
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold transition-colors',
                  done && 'bg-emerald-500 text-white',
                  active && 'bg-blue-600 text-white ring-4 ring-blue-100',
                  !done && !active && 'bg-gray-200 text-gray-400'
                )}
              >
                {done ? <CheckCircle2 size={16} /> : idx + 1}
              </div>
              <span
                className={cn(
                  'mt-1.5 text-xs font-medium whitespace-nowrap',
                  active && 'text-blue-600',
                  done && 'text-emerald-600',
                  !done && !active && 'text-gray-400'
                )}
              >
                {step.label}
              </span>
            </div>
            {idx < STEPS.length - 1 && (
              <div
                className={cn(
                  'h-0.5 w-16 sm:w-24 mx-1 mb-5 transition-colors',
                  idx < current ? 'bg-emerald-400' : 'bg-gray-200'
                )}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ─── Toast-like inline notification ──────────────────────────────────────────

function ErrorAlert({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
      <AlertCircle size={16} className="flex-shrink-0 mt-0.5" />
      <span>{message}</span>
    </div>
  )
}

// ─── Test Connection result banner ────────────────────────────────────────────

function TestConnectionBanner({
  result,
  isLoading,
}: {
  result: TransferTestConnectionResult | null
  isLoading: boolean
}) {
  if (isLoading) {
    return (
      <div className="flex items-center gap-2.5 p-3 bg-blue-50 border border-blue-200 rounded-xl text-sm text-blue-700">
        <Loader2 size={15} className="animate-spin flex-shrink-0" />
        <span>Testing SSH connection...</span>
      </div>
    )
  }
  if (!result) return null

  if (result.success) {
    return (
      <div className="flex items-start gap-2.5 p-3 bg-emerald-50 border border-emerald-200 rounded-xl text-sm text-emerald-800">
        <CheckCircle2 size={15} className="flex-shrink-0 mt-0.5 text-emerald-600" />
        <div className="min-w-0">
          <span className="font-semibold">Connection successful</span>
          <div className="flex flex-wrap gap-3 mt-0.5 text-xs text-emerald-700">
            {result.latency_ms !== undefined && (
              <span>⏱ {result.latency_ms}ms</span>
            )}
            {result.hostname && (
              <span className="font-mono">🖥 {result.hostname}</span>
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-start gap-2.5 p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
      <XCircle size={15} className="flex-shrink-0 mt-0.5" />
      <div className="min-w-0">
        <span className="font-semibold">Connection failed</span>
        <p className="text-xs mt-0.5 break-all">{result.error}</p>
      </div>
    </div>
  )
}

// ─── Step 1: Connect ──────────────────────────────────────────────────────────

type AuthMethod = 'key' | 'password'

function Step1Connect() {
  const navigate = useNavigate()
  const { data: serversData } = useServers()
  const connect = useConnectTransfer()
  const testConn = useTestConnection()

  const [targetServerId, setTargetServerId] = useState<number>(0)
  const [panelType] = useState<string>('aapanel')
  const [host, setHost] = useState('')
  const [port, setPort] = useState(22)
  const [user, setUser] = useState('root')
  const [authMethod, setAuthMethod] = useState<AuthMethod>('key')
  const [sshKey, setSshKey] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [testResult, setTestResult] = useState<TransferTestConnectionResult | null>(null)

  const servers = serversData?.data ?? []

  // Test button requires host + auth creds (no target server needed)
  const canTest =
    host.trim() !== '' &&
    (authMethod === 'key' ? sshKey.trim() !== '' : password.trim() !== '')

  const canSubmit =
    targetServerId > 0 && canTest

  const handleTest = async () => {
    setTestResult(null)
    try {
      const result = await testConn.mutateAsync({
        host: host.trim(),
        port,
        user: user.trim() || 'root',
        ...(authMethod === 'key' ? { ssh_key: sshKey } : { password }),
      })
      setTestResult(result)
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } }
      setTestResult({
        success: false,
        error: err?.response?.data?.error || 'Could not test connection',
      })
    }
  }

  const handleSubmit = async () => {
    setError('')
    try {
      const job = await connect.mutateAsync({
        target_server_id: targetServerId,
        panel_type: panelType,
        host: host.trim(),
        port,
        user: user.trim() || 'root',
        ...(authMethod === 'key' ? { ssh_key: sshKey } : { password }),
      })
      navigate(`/transfer/${job.id}`)
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string; message?: string } } }
      setError(
        err?.response?.data?.error ||
        err?.response?.data?.message ||
        'Connection failed. Check your connection details.'
      )
    }
  }

  return (
    <div className="max-w-xl mx-auto">
      <StepIndicator current={0} />

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
        <div className="p-6 border-b border-gray-50">
          <h2 className="text-lg font-bold text-gray-900">Connect to source server</h2>
          <p className="text-sm text-gray-500 mt-0.5">
            Enter SSH details to scan for WordPress sites to transfer
          </p>
        </div>

        <div className="p-6 space-y-5">
          {/* Target server */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-1.5">
              Target server <span className="text-red-500">*</span>
            </label>
            <div className="relative">
              <Server size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
              <select
                value={targetServerId}
                onChange={(e) => setTargetServerId(Number(e.target.value))}
                className="w-full border border-gray-200 rounded-xl pl-9 pr-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none appearance-none"
              >
                <option value={0}>Select target server…</option>
                {servers
                  .filter((s) => s.status === 'active')
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.ip_address})
                    </option>
                  ))}
              </select>
            </div>
          </div>

          {/* Panel type */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-2">
              Source panel type
            </label>
            <div className="grid grid-cols-3 gap-2">
              {[
                { value: 'aapanel', label: 'aaPanel', available: true },
                { value: 'cpanel', label: 'cPanel', available: false },
                { value: 'plesk', label: 'Plesk', available: false },
              ].map((opt) => (
                <div
                  key={opt.value}
                  className={cn(
                    'text-center px-3 py-2.5 rounded-xl border-2 text-sm font-medium transition-colors',
                    opt.value === panelType && opt.available
                      ? 'border-blue-500 bg-blue-50 text-blue-700'
                      : opt.available
                      ? 'border-gray-200 text-gray-700 cursor-pointer hover:border-gray-300'
                      : 'border-gray-100 text-gray-300 cursor-not-allowed bg-gray-50'
                  )}
                >
                  {opt.label}
                  {!opt.available && (
                    <span className="block text-xs font-normal text-gray-300 mt-0.5">coming soon</span>
                  )}
                  {opt.available && (
                    <span className="block text-xs font-normal text-emerald-500 mt-0.5">available</span>
                  )}
                </div>
              ))}
            </div>
          </div>

          {/* Host + Port */}
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                Host / IP <span className="text-red-500">*</span>
              </label>
              <div className="relative">
                <Globe size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                <input
                  type="text"
                  value={host}
                  onChange={(e) => setHost(e.target.value)}
                  placeholder="1.2.3.4 or hostname"
                  className="w-full border border-gray-200 rounded-xl pl-9 pr-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
                />
              </div>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">Port</label>
              <input
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
                min={1}
                max={65535}
                className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              />
            </div>
          </div>

          {/* User */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">SSH User</label>
            <input
              type="text"
              value={user}
              onChange={(e) => setUser(e.target.value)}
              placeholder="root"
              className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
            />
          </div>

          {/* Auth method */}
          <div>
            <label className="block text-sm font-semibold text-gray-700 mb-2">Authentication</label>
            <div className="grid grid-cols-2 gap-2 mb-3">
              {(
                [
                  { value: 'key', label: 'SSH Private Key' },
                  { value: 'password', label: 'Password' },
                ] as const
              ).map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setAuthMethod(opt.value)}
                  className={cn(
                    'px-3 py-2 rounded-xl border-2 text-sm font-medium transition-colors',
                    authMethod === opt.value
                      ? 'border-blue-500 bg-blue-50 text-blue-700'
                      : 'border-gray-200 text-gray-600 hover:border-gray-300'
                  )}
                >
                  {opt.label}
                </button>
              ))}
            </div>

            {authMethod === 'key' && (
              <div>
                <div className="flex items-start gap-2 p-2.5 bg-amber-50 border border-amber-200 rounded-xl text-xs text-amber-800 mb-2">
                  <Shield size={13} className="flex-shrink-0 mt-0.5" />
                  <span>
                    The SSH key is only used for this session and is never stored in the database.
                  </span>
                </div>
                <textarea
                  value={sshKey}
                  onChange={(e) => setSshKey(e.target.value)}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
                  rows={6}
                  className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-xs font-mono bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none resize-none"
                />
              </div>
            )}

            {authMethod === 'password' && (
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="SSH password"
                className="w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              />
            )}
          </div>

          {/* Test connection result */}
          <TestConnectionBanner result={testResult} isLoading={testConn.isPending} />

          {error && <ErrorAlert message={error} />}
        </div>

        <div className="p-4 border-t border-gray-50 flex justify-between items-center">
          <button
            onClick={() => navigate('/transfer')}
            className="flex items-center gap-1.5 px-4 py-2 text-sm text-gray-500 hover:bg-gray-100 rounded-xl transition-colors"
          >
            <ChevronLeft size={15} /> Cancel
          </button>
          <div className="flex items-center gap-2">
            <button
              onClick={handleTest}
              disabled={!canTest || testConn.isPending || connect.isPending}
              className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-xl disabled:opacity-50 transition-colors"
            >
              {testConn.isPending ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <Wifi size={14} />
              )}
              Test connection
            </button>
            <button
              onClick={handleSubmit}
              disabled={!canSubmit || connect.isPending}
              className="flex items-center gap-2 px-5 py-2 text-sm font-semibold bg-blue-600 text-white hover:bg-blue-700 rounded-xl disabled:opacity-50 transition-colors"
            >
              {connect.isPending ? (
                <>
                  <Loader2 size={15} className="animate-spin" /> Connecting...
                </>
              ) : (
                <>
                  Next <ArrowRight size={15} />
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Domain check indicator ───────────────────────────────────────────────────

type DomainCheckState = 'idle' | 'loading' | 'ok' | 'conflict' | 'error'

function DomainCheckIcon({ state }: { state: DomainCheckState }) {
  if (state === 'loading') return <Loader2 size={14} className="animate-spin text-gray-400" />
  if (state === 'ok') return <CheckCircle2 size={14} className="text-emerald-500" />
  if (state === 'conflict') return <AlertCircle size={14} className="text-red-500" />
  if (state === 'error') return <AlertTriangle size={14} className="text-amber-500" />
  return null
}

// ─── PHP badge ────────────────────────────────────────────────────────────────

function PHPBadge({
  version,
  available,
}: {
  version: string
  available: string[]
}) {
  if (!version) {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-400">
        ? Unknown
      </span>
    )
  }
  const isEOL = version.startsWith('7.')
  const isAvailable = available.includes(version)
  if (isEOL) {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-yellow-100 text-yellow-700">
        ⚠ PHP {version}
      </span>
    )
  }
  if (isAvailable) {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-emerald-100 text-emerald-700">
        ✓ PHP {version}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-red-100 text-red-700">
      ✗ PHP {version}
    </span>
  )
}

// ─── SSL mode selector (inline) ───────────────────────────────────────────────

const SSL_OPTIONS = [
  { value: 'reissue', label: 'Reissue SSL' },
  { value: 'copy', label: 'Copy cert' },
  { value: 'paid_ca', label: 'Paid CA' },
  { value: 'skip', label: 'Skip' },
] as const

// ─── Single site row in selection table ──────────────────────────────────────

interface SiteRowState {
  selected: boolean
  targetDomain: string
  sslMode: 'reissue' | 'copy' | 'paid_ca' | 'skip'
  phpOverride: string   // manual PHP version when auto-detection returned empty
  aliasExpanded: boolean
  domainCheckState: DomainCheckState
  conflictDetail?: string
}

interface SiteRowProps {
  site: DiscoveredSiteInfo
  state: SiteRowState
  jobId: number
  targetServerId: number
  availablePHP: string[]
  onChange: (patch: Partial<SiteRowState>) => void
}

function SiteRow({ site, state, jobId, targetServerId, availablePHP, onChange }: SiteRowProps) {
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [debouncedDomain, setDebouncedDomain] = useState(state.targetDomain)
  const [domainCheckEnabled, setDomainCheckEnabled] = useState(false)

  const { data: conflictResult, isFetching: isCheckingDomain } = useCheckDomain(
    debouncedDomain,
    targetServerId,
    jobId,
    domainCheckEnabled
  )

  // Sync check state from query
  useEffect(() => {
    if (isCheckingDomain) {
      onChange({ domainCheckState: 'loading' })
    } else if (conflictResult !== undefined) {
      if (conflictResult.available) {
        onChange({ domainCheckState: 'ok', conflictDetail: undefined })
      } else {
        onChange({
          domainCheckState: 'conflict',
          conflictDetail: conflictResult.conflict_detail,
        })
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isCheckingDomain, conflictResult])

  const handleDomainChange = useCallback(
    (value: string) => {
      onChange({ targetDomain: value, domainCheckState: 'idle' })
      setDomainCheckEnabled(false)
      if (debounceRef.current) clearTimeout(debounceRef.current)
      debounceRef.current = setTimeout(() => {
        setDebouncedDomain(value)
        setDomainCheckEnabled(true)
      }, 500)
    },
    [onChange]
  )

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

  const isHardConflict =
    state.domainCheckState === 'conflict' &&
    conflictResult?.conflict_type !== 'pending_transfer'

  return (
    <tr
      className={cn(
        'border-b border-gray-50 transition-colors',
        !state.selected && 'opacity-50',
        isHardConflict && 'bg-red-50/40'
      )}
    >
      {/* Checkbox */}
      <td className="px-4 py-3">
        <input
          type="checkbox"
          checked={state.selected}
          disabled={isHardConflict}
          onChange={(e) => onChange({ selected: e.target.checked })}
          className="w-4 h-4 rounded accent-blue-600 disabled:cursor-not-allowed"
        />
      </td>

      {/* Source domain */}
      <td className="px-3 py-3">
        <p className="text-sm font-semibold text-gray-800 font-mono leading-none">
          {site.primary_domain}
        </p>
        <p className="text-xs text-gray-400 mt-0.5">{formatBytes(site.size_bytes)}</p>
        {(site.aliases?.length ?? 0) > 0 && (
          <div className="mt-1">
            <button
              type="button"
              onClick={() => onChange({ aliasExpanded: !state.aliasExpanded })}
              className="inline-flex items-center gap-0.5 text-xs text-blue-500 hover:text-blue-700"
            >
              {site.aliases!.length} alias
              {state.aliasExpanded ? <ChevronUp size={11} /> : <ChevronDown size={11} />}
            </button>
            {state.aliasExpanded && (
              <ul className="mt-1 pl-1 space-y-0.5">
                {(site.aliases ?? []).map((a) => (
                  <li key={a} className="text-xs text-gray-500 font-mono">
                    {a}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </td>

      {/* PHP */}
      <td className="px-3 py-3">
        {site.php_version ? (
          <PHPBadge version={site.php_version} available={availablePHP} />
        ) : (
          <select
            value={state.phpOverride}
            onChange={(e) => onChange({ phpOverride: e.target.value })}
            className="border border-gray-200 rounded px-1.5 py-0.5 text-xs bg-white focus:outline-none focus:ring-1 focus:ring-blue-500 text-gray-600"
          >
            <option value="">? Select PHP</option>
            {availablePHP.map((v) => (
              <option key={v} value={v}>PHP {v}</option>
            ))}
          </select>
        )}
      </td>

      {/* Target domain */}
      <td className="px-3 py-3">
        <div className="relative">
          <input
            type="text"
            value={state.targetDomain}
            onChange={(e) => handleDomainChange(e.target.value)}
            className={cn(
              'w-full border rounded-lg px-2.5 py-1.5 text-xs font-mono bg-gray-50 focus:bg-white outline-none focus:ring-2 pr-7',
              state.domainCheckState === 'ok' &&
                'border-emerald-400 focus:border-emerald-500 focus:ring-emerald-500/20',
              state.domainCheckState === 'conflict' &&
                'border-red-400 focus:border-red-500 focus:ring-red-500/20',
              state.domainCheckState === 'loading' &&
                'border-gray-200 focus:border-blue-500 focus:ring-blue-500/20',
              state.domainCheckState === 'idle' &&
                'border-gray-200 focus:border-blue-500 focus:ring-blue-500/20'
            )}
          />
          <span className="absolute right-2 top-1/2 -translate-y-1/2">
            <DomainCheckIcon state={state.domainCheckState} />
          </span>
        </div>
        {state.domainCheckState === 'conflict' && state.conflictDetail && (
          <p className="text-xs text-red-600 mt-0.5">{state.conflictDetail}</p>
        )}
      </td>

      {/* SSL mode */}
      <td className="px-3 py-3">
        <select
          value={state.sslMode}
          onChange={(e) =>
            onChange({ sslMode: e.target.value as SiteRowState['sslMode'] })
          }
          className="border border-gray-200 rounded-lg px-2 py-1.5 text-xs bg-gray-50 focus:bg-white focus:border-blue-500 outline-none"
        >
          {SSL_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </td>
    </tr>
  )
}

// ─── Step 2: Select Sites ─────────────────────────────────────────────────────

function Step2Select({ job }: { job: TransferJob }) {
  const navigate = useNavigate()
  const startTransfer = useStartTransfer(job.id)
  const deleteJob = useDeleteTransferJob()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const sites = job.discovered_sites_parsed ?? []
  const availablePHP = job.target_php_versions_parsed ?? []

  // Per-row state, keyed by primary_domain
  const [rows, setRows] = useState<Record<string, SiteRowState>>(() => {
    const init: Record<string, SiteRowState> = {}
    for (const s of sites) {
      init[s.primary_domain] = {
        selected: true,
        targetDomain: s.primary_domain,
        sslMode: 'reissue',
        phpOverride: '',
        aliasExpanded: false,
        domainCheckState: 'idle',
      }
    }
    return init
  })

  const [error, setError] = useState('')

  const patchRow = useCallback(
    (domain: string, patch: Partial<SiteRowState>) => {
      setRows((prev) => ({
        ...prev,
        [domain]: { ...prev[domain], ...patch },
      }))
    },
    []
  )

  const hasHardConflict = Object.values(rows).some(
    (r) => r.selected && r.domainCheckState === 'conflict'
  )
  const selectedCount = Object.values(rows).filter((r) => r.selected).length

  // Warnings: PHP not available on target
  const phpWarnings = sites
    .filter((s) => {
      const r = rows[s.primary_domain]
      const ver = s.php_version || r?.phpOverride
      return r?.selected && ver && !availablePHP.includes(ver) && !ver.startsWith('7.')
    })
    .map((s) => {
      const ver = s.php_version || rows[s.primary_domain]?.phpOverride
      return `${s.primary_domain}: PHP ${ver} is not installed on the target server`
    })

  const eolWarnings = sites
    .filter((s) => {
      const r = rows[s.primary_domain]
      const ver = s.php_version || r?.phpOverride
      return r?.selected && ver?.startsWith('7.')
    })
    .map((s) => {
      const ver = s.php_version || rows[s.primary_domain]?.phpOverride
      return `${s.primary_domain}: PHP ${ver} is EOL — upgrade recommended`
    })

  // Error: selected site has no PHP version and no manual override chosen
  const missingPHPSites = sites
    .filter((s) => {
      const r = rows[s.primary_domain]
      return r?.selected && !s.php_version && !r?.phpOverride
    })
    .map((s) => s.primary_domain)

  const canStart = selectedCount > 0 && !hasHardConflict && missingPHPSites.length === 0

  const handleStart = async () => {
    setError('')
    const selectedSites: SiteImportOption[] = sites
      .filter((s) => rows[s.primary_domain]?.selected)
      .map((s) => {
        const r = rows[s.primary_domain]
        return {
          source_domain: s.primary_domain,
          target_domain: r.targetDomain || s.primary_domain,
          ssl_mode: r.sslMode,
          ...(r.phpOverride ? { php_override: r.phpOverride } : {}),
          include_aliases: s.aliases ?? [],
        }
      })

    try {
      await startTransfer.mutateAsync({ sites: selectedSites })
      // job query will refetch and re-render with Step3
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string; message?: string } } }
      setError(
        err?.response?.data?.error ||
        err?.response?.data?.message ||
        'Could not start transfer'
      )
    }
  }


  return (
    <div className="max-w-5xl mx-auto">
      <StepIndicator current={1} />

      {/* Header info */}
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-lg font-bold text-gray-900">
            Select sites to transfer
          </h2>
          <p className="text-sm text-gray-500 mt-0.5">
            Found <span className="font-semibold text-gray-700">{sites.length} sites</span> on{' '}
            <span className="font-mono text-gray-700">{job.source_host}</span>
          </p>
        </div>
        {availablePHP.length > 0 && (
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-gray-400">PHP on target:</span>
            {availablePHP.map((v) => (
              <span
                key={v}
                className="px-2 py-0.5 rounded text-xs bg-blue-50 text-blue-700 font-mono"
              >
                {v}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Missing PHP version error */}
      {missingPHPSites.length > 0 && (
        <div className="mb-4 flex items-start gap-2 p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
          <AlertCircle size={14} className="flex-shrink-0 mt-0.5" />
          <span>
            Could not detect the PHP version for:{' '}
            <strong>{missingPHPSites.join(', ')}</strong>. Select a PHP version manually.
          </span>
        </div>
      )}

      {/* Warnings */}
      {(phpWarnings.length > 0 || eolWarnings.length > 0) && (
        <div className="mb-4 space-y-2">
          {eolWarnings.map((w) => (
            <div
              key={w}
              className="flex items-start gap-2 p-3 bg-yellow-50 border border-yellow-200 rounded-xl text-sm text-yellow-800"
            >
              <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
              {w}
            </div>
          ))}
          {phpWarnings.map((w) => (
            <div
              key={w}
              className="flex items-start gap-2 p-3 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700"
            >
              <AlertCircle size={14} className="flex-shrink-0 mt-0.5" />
              {w}
            </div>
          ))}
        </div>
      )}

      {/* Table */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-x-auto">
        <table className="w-full min-w-[700px]">
          <thead>
            <tr className="border-b border-gray-50">
              <th className="px-4 py-3 text-left">
                <input
                  type="checkbox"
                  checked={selectedCount === sites.length && sites.length > 0}
                  onChange={(e) => {
                    setRows((prev) => {
                      const next = { ...prev }
                      for (const k of Object.keys(next)) {
                        next[k] = { ...next[k], selected: e.target.checked }
                      }
                      return next
                    })
                  }}
                  className="w-4 h-4 rounded accent-blue-600"
                />
              </th>
              <th className="px-3 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                Source domain / Size
              </th>
              <th className="px-3 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                PHP
              </th>
              <th className="px-3 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                Target domain
              </th>
              <th className="px-3 py-3 text-left text-xs font-semibold text-gray-400 uppercase tracking-wide">
                SSL
              </th>
            </tr>
          </thead>
          <tbody>
            {sites.map((site) => (
              <SiteRow
                key={site.primary_domain}
                site={site}
                state={rows[site.primary_domain]}
                jobId={job.id}
                targetServerId={job.target_server_id}
                availablePHP={availablePHP}
                onChange={(patch) => patchRow(site.primary_domain, patch)}
              />
            ))}
          </tbody>
        </table>
      </div>

      {error && <div className="mt-4"><ErrorAlert message={error} /></div>}

      {/* Delete confirm modal */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="bg-white rounded-2xl shadow-xl p-6 w-full max-w-sm mx-4">
            <div className="flex items-center gap-3 mb-3">
              <div className="w-10 h-10 rounded-full bg-red-100 flex items-center justify-center flex-shrink-0">
                <RotateCcw size={18} className="text-red-600" />
              </div>
              <h3 className="text-base font-semibold text-gray-900">Delete & Rescan?</h3>
            </div>
            <p className="text-sm text-gray-500 mb-5">
              This job will be deleted completely. You will be taken back to the new job
              screen to reconnect and rescan with improved detection.
            </p>
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-xl transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  await deleteJob.mutateAsync(job.id)
                  navigate('/transfer')
                }}
                disabled={deleteJob.isPending}
                className="flex items-center gap-2 px-4 py-2 text-sm font-semibold bg-red-600 text-white hover:bg-red-700 rounded-xl disabled:opacity-50 transition-colors"
              >
                {deleteJob.isPending ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <RotateCcw size={14} />
                )}
                Delete & Rescan
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Footer */}
      <div className="flex items-center justify-between mt-5">
        <div className="flex items-center gap-2">
          <button
            onClick={() => navigate('/transfer')}
            className="flex items-center gap-1.5 px-4 py-2 text-sm text-gray-500 hover:bg-gray-100 rounded-xl transition-colors"
          >
            <ChevronLeft size={15} /> Back to list
          </button>
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="flex items-center gap-1.5 px-4 py-2 text-sm text-red-500 hover:bg-red-50 rounded-xl transition-colors"
            title="Delete this job and start over"
          >
            <RotateCcw size={14} /> Rescan
          </button>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-500">
            Selected <span className="font-semibold text-gray-700">{selectedCount}</span>/{sites.length} sites
          </span>
          <button
            onClick={handleStart}
            disabled={!canStart || startTransfer.isPending}
            className="flex items-center gap-2 px-5 py-2.5 text-sm font-semibold bg-blue-600 text-white hover:bg-blue-700 rounded-xl disabled:opacity-50 transition-colors"
          >
            {startTransfer.isPending ? (
              <>
                <Loader2 size={15} className="animate-spin" /> Starting...
              </>
            ) : (
              <>
                Start transfer <ArrowRight size={15} />
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Step 3: Progress ─────────────────────────────────────────────────────────

type SiteStatus = TransferSiteResult['status']

const SITE_STATUS_CONFIG: Record<SiteStatus, { label: string; classes: string; icon: React.ReactNode }> = {
  pending: {
    label: 'Pending',
    classes: 'bg-gray-100 text-gray-500',
    icon: <Clock size={11} className="shrink-0" />,
  },
  running: {
    label: 'Running',
    classes: 'bg-blue-100 text-blue-700',
    icon: <Loader2 size={11} className="animate-spin shrink-0" />,
  },
  done: {
    label: 'Done',
    classes: 'bg-emerald-100 text-emerald-700',
    icon: <CheckCircle2 size={11} className="shrink-0" />,
  },
  failed: {
    label: 'Failed',
    classes: 'bg-red-100 text-red-700',
    icon: <XCircle size={11} className="shrink-0" />,
  },
  rolled_back: {
    label: 'Rolled back',
    classes: 'bg-orange-100 text-orange-700',
    icon: <RotateCcw size={11} className="shrink-0" />,
  },
  ssl_pending_reissue: {
    label: 'Awaiting SSL',
    classes: 'bg-yellow-100 text-yellow-700',
    icon: <Shield size={11} className="shrink-0" />,
  },
}

const PHASE_LABELS: Record<string, string> = {
  files: 'Sync files',
  db: 'Import DB',
  create_site: 'Create site',
  search_replace: 'Search-replace',
  ssl: 'SSL',
  verify: 'Verify',
}

function SiteProgressCard({ result }: { result: TransferSiteResult }) {
  const [logExpanded, setLogExpanded] = useState(false)
  const statusCfg = SITE_STATUS_CONFIG[result.status] ?? SITE_STATUS_CONFIG.pending
  const phaseLabel = PHASE_LABELS[result.phase] || result.phase || '—'

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
      <div className="flex items-start justify-between mb-3">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-gray-800 font-mono truncate">
            {result.target_domain}
          </p>
          {result.source_domain !== result.target_domain && (
            <p className="text-xs text-gray-400 font-mono truncate mt-0.5">
              ← {result.source_domain}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2 flex-shrink-0 ml-3">
          <span className="text-xs text-gray-400">{phaseLabel}</span>
          <span
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${statusCfg.classes}`}
          >
            {statusCfg.icon}
            {statusCfg.label}
          </span>
        </div>
      </div>

      {/* Progress bar */}
      <div className="mb-2">
        <div className="flex items-center justify-between text-xs text-gray-400 mb-1">
          <span>{phaseLabel}</span>
          <span>{result.progress}%</span>
        </div>
        <div className="w-full bg-gray-100 rounded-full h-1.5">
          <div
            className={cn(
              'h-1.5 rounded-full transition-all duration-500',
              result.status === 'done' || result.status === 'ssl_pending_reissue'
                ? 'bg-emerald-500'
                : result.status === 'failed' || result.status === 'rolled_back'
                ? 'bg-red-500'
                : 'bg-blue-500'
            )}
            style={{ width: `${result.progress}%` }}
          />
        </div>
      </div>

      {result.error && (
        <p className="text-xs text-red-600 mt-1 flex items-start gap-1">
          <AlertCircle size={12} className="shrink-0 mt-0.5" />
          {result.error}
        </p>
      )}

      {result.log_excerpt && (
        <div className="mt-2">
          <button
            type="button"
            onClick={() => setLogExpanded(!logExpanded)}
            className="inline-flex items-center gap-1 text-xs text-gray-400 hover:text-gray-600"
          >
            View log
            {logExpanded ? <ChevronUp size={11} /> : <ChevronDown size={11} />}
          </button>
          {logExpanded && (
            <pre className="mt-1.5 bg-[#0F172A] text-emerald-400 font-mono text-xs rounded-xl p-3 max-h-40 overflow-y-auto whitespace-pre-wrap break-all">
              {result.log_excerpt}
            </pre>
          )}
        </div>
      )}

      {/* Link to site in panel when done */}
      {(result.status === 'done' || result.status === 'ssl_pending_reissue') && result.site_id && (
        <div className="mt-3 flex items-center gap-2">
          <a
            href={`/sites/${result.site_id}`}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-emerald-700 bg-emerald-50 hover:bg-emerald-100 rounded-lg transition-colors"
          >
            <ExternalLink size={12} /> Manage site
          </a>
          <a
            href={`https://${result.target_domain}`}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-lg transition-colors"
          >
            <Globe size={12} /> Open website
          </a>
        </div>
      )}
    </div>
  )
}

function Step3Progress({ job }: { job: TransferJob }) {
  const navigate = useNavigate()
  const cancelTransfer = useCancelTransfer(job.id)

  // Subscribe to WebSocket channel for real-time updates
  useWebSocket(
    job.status === 'transferring' ? [`transfer:${job.id}`] : []
  )

  const results = job.results ?? []
  const total = job.total_sites || results.length
  const done = job.done_sites || results.filter((r) => r.status === 'done' || r.status === 'ssl_pending_reissue').length
  const failed = results.filter((r) => r.status === 'failed' || r.status === 'rolled_back').length
  const overall = total > 0 ? Math.round((done / total) * 100) : 0

  const isTransferring = job.status === 'transferring'
  const isDone = job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled'

  const handleCancel = async () => {
    if (!confirm('Cancel this transfer job? Sites currently transferring may end up broken.')) return
    await cancelTransfer.mutateAsync()
  }

  return (
    <div className="max-w-3xl mx-auto">
      <StepIndicator current={2} />

      {/* Overall progress */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 mb-5">
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-lg font-bold text-gray-900">
            {isTransferring
              ? 'Transferring...'
              : job.status === 'completed'
              ? 'Transfer completed!'
              : job.status === 'failed'
              ? 'Transfer failed'
              : job.status === 'cancelled'
              ? 'Cancelled'
              : 'Transfer'}
          </h2>
          <span className="text-sm font-semibold text-gray-700">
            {done}/{total} sites
          </span>
        </div>

        <div className="w-full bg-gray-100 rounded-full h-3 mb-2">
          <div
            className={cn(
              'h-3 rounded-full transition-all duration-700',
              job.status === 'completed'
                ? 'bg-emerald-500'
                : job.status === 'failed'
                ? 'bg-red-500'
                : 'bg-blue-500'
            )}
            style={{ width: `${overall}%` }}
          />
        </div>
        <div className="flex items-center justify-between text-xs text-gray-400">
          <span>
            {done} completed
            {failed > 0 && ` · ${failed} failed`}
            {total - done - failed > 0 && ` · ${total - done - failed} remaining`}
          </span>
          <span>{overall}%</span>
        </div>

        {isTransferring && (
          <div className="mt-1 flex items-center gap-1.5 text-xs text-blue-500">
            <Loader2 size={11} className="animate-spin" />
            Processing — this page updates automatically
          </div>
        )}

        {job.error && (
          <div className="mt-3">
            <ErrorAlert message={job.error} />
          </div>
        )}

        {/* Summary (when done) */}
        {isDone && (
          <div className="mt-4 grid grid-cols-3 gap-3">
            <div className="text-center p-3 bg-emerald-50 rounded-xl">
              <p className="text-2xl font-bold text-emerald-600">{done}</p>
              <p className="text-xs text-emerald-600 mt-0.5">Completed</p>
            </div>
            <div className="text-center p-3 bg-red-50 rounded-xl">
              <p className="text-2xl font-bold text-red-500">{failed}</p>
              <p className="text-xs text-red-500 mt-0.5">Failed</p>
            </div>
            <div className="text-center p-3 bg-gray-50 rounded-xl">
              <p className="text-2xl font-bold text-gray-500">{total}</p>
              <p className="text-xs text-gray-500 mt-0.5">Total</p>
            </div>
          </div>
        )}
      </div>

      {/* Per-site cards */}
      {results.length > 0 && (
        <div className="space-y-3">
          {results.map((r) => (
            <SiteProgressCard key={r.id} result={r} />
          ))}
        </div>
      )}

      {/* Pending sites (when no results yet but total > 0) */}
      {results.length === 0 && total > 0 && isTransferring && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-10 text-center">
          <Loader2 size={24} className="animate-spin text-gray-300 mx-auto mb-3" />
          <p className="text-sm text-gray-500">Initializing transfer tasks...</p>
        </div>
      )}

      {/* Action buttons */}
      <div className="flex items-center justify-between mt-6">
        <button
          onClick={() => navigate('/transfer')}
          className="flex items-center gap-1.5 px-4 py-2 text-sm text-gray-500 hover:bg-gray-100 rounded-xl transition-colors"
        >
          <ChevronLeft size={15} /> Back to list
        </button>

        <div className="flex items-center gap-3">
          {isTransferring && (
            <button
              onClick={handleCancel}
              disabled={cancelTransfer.isPending}
              className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-red-600 bg-red-50 hover:bg-red-100 rounded-xl transition-colors disabled:opacity-50"
            >
              {cancelTransfer.isPending ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <XCircle size={14} />
              )}
              Cancel job
            </button>
          )}

          {isDone && (
            <>
              <button
                onClick={() => navigate('/transfer/new')}
                className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-xl transition-colors"
              >
                <RotateCcw size={14} /> Transfer more
              </button>
              <button
                onClick={() => navigate('/sites')}
                className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-emerald-600 bg-emerald-50 hover:bg-emerald-100 rounded-xl transition-colors"
              >
                <ExternalLink size={14} /> View Sites
              </button>
              <button
                onClick={() => navigate('/')}
                className="flex items-center gap-2 px-5 py-2 text-sm font-semibold bg-emerald-600 text-white hover:bg-emerald-700 rounded-xl transition-colors"
              >
                <LayoutDashboard size={14} /> Back to Dashboard
              </button>
            </>
          )}
        </div>
      </div>

      {/* Timestamps */}
      <div className="mt-4 text-xs text-gray-400 flex items-center gap-4">
        {job.started_at && <span>Started: {formatDate(job.started_at)}</span>}
        {job.completed_at && <span>Completed: {formatDate(job.completed_at)}</span>}
      </div>
    </div>
  )
}

// ─── Error Boundary ───────────────────────────────────────────────────────────

interface EBState { hasError: boolean; message: string }
class TransferErrorBoundary extends React.Component<
  { children: React.ReactNode },
  EBState
> {
  constructor(props: { children: React.ReactNode }) {
    super(props)
    this.state = { hasError: false, message: '' }
  }
  static getDerivedStateFromError(err: unknown): EBState {
    return {
      hasError: true,
      message: err instanceof Error ? err.message : String(err),
    }
  }
  render() {
    if (this.state.hasError) {
      return (
        <div className="py-4 max-w-xl mx-auto">
          <div className="bg-white rounded-2xl border border-red-100 shadow-sm p-10 text-center">
            <AlertCircle size={32} className="mx-auto text-red-400 mb-3" />
            <p className="text-gray-700 font-semibold">Failed to render the Transfer page</p>
            <p className="text-gray-400 text-sm mt-1 font-mono break-all">{this.state.message}</p>
            <button
              onClick={() => { this.setState({ hasError: false, message: '' }); window.location.reload() }}
              className="mt-5 px-4 py-2 bg-blue-600 text-white rounded-xl text-sm font-semibold hover:bg-blue-700 transition-colors"
            >
              Reload page
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

// ─── Root Wizard page ─────────────────────────────────────────────────────────

function TransferWizardInner() {
  const { id } = useParams<{ id: string }>()
  const jobId = id && id !== 'new' ? Number(id) : null

  const { data: job, isLoading, isError } = useTransferJob(jobId)

  // Step 1: no id or /transfer/new → Connect form
  if (!jobId || isNaN(jobId)) {
    return (
      <div className="py-4">
        <Step1Connect />
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="py-4 max-w-3xl mx-auto">
        <StepIndicator current={1} />
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-16 text-center">
          <Loader2 size={28} className="animate-spin text-gray-300 mx-auto" />
        </div>
      </div>
    )
  }

  if (isError || !job) {
    return (
      <div className="py-4 max-w-xl mx-auto">
        <div className="bg-white rounded-2xl border border-red-100 shadow-sm p-10 text-center">
          <AlertCircle size={32} className="mx-auto text-red-400 mb-3" />
          <p className="text-gray-700 font-semibold">Transfer job not found</p>
          <p className="text-gray-400 text-sm mt-1">Job #{jobId} does not exist or you do not have access.</p>
        </div>
      </div>
    )
  }

  // Step 2a: pending / discovering → spinner only, do NOT mount Step2Select yet
  // (Step2Select initialises `rows` from discovered_sites on mount — if mounted
  //  while sites=[],  the state stays empty and crashes when sites arrive later)
  if (job.status === 'pending' || job.status === 'discovering') {
    return (
      <div className="py-4 max-w-3xl mx-auto">
        <StepIndicator current={1} />
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-16 text-center">
          <Loader2 size={32} className="animate-spin text-blue-400 mx-auto mb-4" />
          <p className="text-gray-700 font-semibold">
            {job.status === 'pending' ? 'Preparing connection...' : 'Scanning sites...'}
          </p>
          <p className="text-gray-400 text-sm mt-1">
            The system is connecting to the source server via SSH and listing all WordPress sites
          </p>
        </div>
      </div>
    )
  }

  // Step 2b: awaiting_selection → site table (Step2Select mounts fresh with full data)
  if (job.status === 'awaiting_selection') {
    return (
      <div className="py-4">
        <Step2Select job={job} />
      </div>
    )
  }

  // Step 3: transferring, completed, failed, cancelled
  return (
    <div className="py-4">
      <Step3Progress job={job} />
    </div>
  )
}

export default function TransferWizardPage() {
  return (
    <TransferErrorBoundary>
      <TransferWizardInner />
    </TransferErrorBoundary>
  )
}
