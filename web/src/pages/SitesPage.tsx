import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Plus, Trash2, ExternalLink, Lock, LockOpen, Search, RefreshCw, Pause,
  Play, CheckCircle, XCircle, Loader2, X, Globe, ChevronRight, Filter, Network,
} from 'lucide-react'
import { useSites, useCreateSite, useDeleteSite, useRestartSite, useSuspendSite, useResumeSite, useAutologin } from '@/hooks/useSites'
import { useServers, useServerInstalledPHP, useCheckDomainDNS } from '@/hooks/useServers'
import { useAllDNSProviders, useAutoCreateDNSRecord } from '@/hooks/useDNS'
import { StatusBadge } from '@/components/ui/Badge'
import api from '@/lib/api'
import type { Site, APIResponse } from '@/types'

const DOMAIN_RE = /^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)+$/

function validateDomain(val: string, existingSites: Site[]): string {
  if (!val) return ''
  const cleaned = val.replace(/^https?:\/\//i, '').replace(/\/.*$/, '')
  if (!DOMAIN_RE.test(cleaned)) return 'Invalid domain format (e.g. example.com, sub.example.com)'
  if (existingSites.some(s => s.domain.toLowerCase() === cleaned.toLowerCase())) return 'Domain already exists'
  return ''
}

const CREATION_STEPS = [
  'Provisioning Nginx & PHP-FPM',
  'Setting up MariaDB database',
  'Installing WordPress',
  'Configuring SSL certificate',
  'Finalizing setup',
]

function CreationProgressCard({
  siteId, domain, installType, onDismiss,
}: { siteId: number; domain: string; installType: string; onDismiss: () => void }) {
  const qc = useQueryClient()
  const [stepIdx, setStepIdx] = useState(0)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const [finalStatus, setFinalStatus] = useState<'active' | 'error' | null>(null)

  const { data: site } = useQuery({
    queryKey: ['site-progress', siteId],
    queryFn: async () => {
      const { data } = await api.get<APIResponse<Site>>(`/sites/${siteId}`)
      return data.data!
    },
    enabled: !!siteId && !finalStatus,
    refetchInterval: finalStatus ? false : 3000,
  })

  const currentStatus = finalStatus ?? site?.status
  const isDone = currentStatus === 'active' || currentStatus === 'error'

  useEffect(() => {
    if (!finalStatus && (site?.status === 'active' || site?.status === 'error')) {
      setFinalStatus(site.status as 'active' | 'error')
      if (timerRef.current) clearInterval(timerRef.current)
      qc.invalidateQueries({ queryKey: ['sites'] })
    }
  }, [site?.status, finalStatus])

  useEffect(() => {
    timerRef.current = setInterval(() => {
      setStepIdx(i => Math.min(i + 1, CREATION_STEPS.length - 1))
    }, 12000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [])

  const currentStep = isDone ? (currentStatus === 'active' ? CREATION_STEPS.length : stepIdx) : stepIdx
  const progressPct = Math.round((currentStep / CREATION_STEPS.length) * 100)

  const [tick, setTick] = useState(0)
  useEffect(() => {
    const t = setInterval(() => setTick(v => v + 1), 1000)
    return () => clearInterval(t)
  }, [])
  const elapsedDisplay = tick >= 60 ? `${Math.floor(tick / 60)}m ${tick % 60}s` : `${tick}s`

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 mb-6">
      <div className="flex items-start justify-between mb-5">
        <div>
          <h3 className={`text-base font-semibold ${
            isDone ? (currentStatus === 'active' ? 'text-emerald-700' : 'text-red-700') : 'text-gray-900'
          }`}>
            {isDone
              ? currentStatus === 'active' ? '✅ Site Created Successfully' : '❌ Site Creation Failed'
              : `Creating: ${domain}`}
          </h3>
          {!isDone && (
            <p className="text-sm text-gray-400 mt-0.5">Elapsed: {elapsedDisplay} — Takes 3–5 minutes</p>
          )}
        </div>
        {isDone && (
          <button onClick={onDismiss} className="text-gray-300 hover:text-gray-600 transition-colors">
            <X size={18} />
          </button>
        )}
      </div>

      {/* Progress bar */}
      {!isDone && (
        <div className="mb-5">
          <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
            <div
              className="h-full bg-blue-500 rounded-full transition-all duration-1000"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>
      )}

      <div className="space-y-2.5">
        {CREATION_STEPS.filter((_, i) => installType === 'manual' ? i !== 3 : true).map((step, i) => {
          const stepDone = isDone && site?.status === 'active' ? true : i < currentStep
          const stepActive = !isDone && i === currentStep
          const stepError = isDone && currentStatus === 'error' && i === currentStep
          return (
            <div key={i} className={`flex items-center gap-3 text-sm ${
              stepDone ? 'text-emerald-700' : stepActive ? 'text-blue-700' : stepError ? 'text-red-600' : 'text-gray-300'
            }`}>
              {stepDone
                ? <CheckCircle size={16} className="text-emerald-500 shrink-0" />
                : stepActive
                  ? <Loader2 size={16} className="animate-spin text-blue-500 shrink-0" />
                  : stepError
                    ? <XCircle size={16} className="text-red-500 shrink-0" />
                    : <div className="w-4 h-4 rounded-full border-2 border-gray-200 shrink-0" />}
              {step}
            </div>
          )
        })}
      </div>

      {isDone && currentStatus === 'active' && (
        <div className="mt-5 p-4 bg-emerald-50 border border-emerald-200 rounded-xl text-sm text-emerald-700">
          🎉 <strong>{domain}</strong> is live!{' '}
          <Link to={`/sites/${siteId}`} className="underline font-medium">View details →</Link>
        </div>
      )}
      {isDone && currentStatus === 'error' && (
        <div className="mt-5 p-4 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
          Site creation failed.{' '}
          <Link to={`/sites/${siteId}`} className="underline font-medium">View details</Link> for the error.
        </div>
      )}
    </div>
  )
}

export default function SitesPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({
    server_id: 0, domain: '', php_version: '8.2',
    ssl_enabled: true, redis_enabled: false,
    install_type: 'auto' as 'auto' | 'manual',
    wp_admin_user: 'admin', wp_admin_email: '',
  })
  const [domainError, setDomainError] = useState('')
  const [dnsStatus, setDnsStatus] = useState<{ pointing: boolean; message: string; checked: boolean } | null>(null)
  const [dnsChecking, setDnsChecking] = useState(false)
  const [dnsOffer, setDnsOffer] = useState<null | 'offered' | 'creating' | 'done' | 'skipped'>(null)
  const [dnsOfferMsg, setDnsOfferMsg] = useState('')
  const [dnsProviderLabel, setDnsProviderLabel] = useState('DNS provider')
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [creatingInfo, setCreatingInfo] = useState<{ id: number; domain: string; installType: string } | null>(null)

  const { data, isLoading } = useSites()
  const { data: serversData } = useServers()
  const { data: dnsProviders } = useAllDNSProviders()
  const createSite = useCreateSite()
  // Fetch installed PHP versions when a server is selected
  const { data: installedPHPData, isFetching: phpFetching } = useServerInstalledPHP(form.server_id)
  const checkDNS = useCheckDomainDNS()
  const autoCreateDNS = useAutoCreateDNSRecord()
  const deleteSite = useDeleteSite()
  const restartSite = useRestartSite()
  const suspendSite = useSuspendSite()
  const resumeSite = useResumeSite()
  const autologinMut = useAutologin()

  const sites = data?.data || []
  const servers = serversData?.data || []
  // PHP version availability for selected server
  const installedPHP = installedPHPData ?? []
  const phpVersionOK = form.server_id === 0 || installedPHP.length === 0
    ? true  // no server selected or still loading — allow all
    : installedPHP.some(p => p.version === form.php_version)

  const filteredSites = sites.filter(site => {
    const matchesSearch = searchTerm === '' ||
      site.domain.toLowerCase().includes(searchTerm.toLowerCase()) ||
      site.server?.name?.toLowerCase().includes(searchTerm.toLowerCase())
    const matchesStatus = statusFilter === 'all' || site.status === statusFilter
    return matchesSearch && matchesStatus
  })

  // When PHP version list loads and current selection is not installed, auto-pick the best available.
  useEffect(() => {
    if (installedPHP.length === 0) return
    const isInstalled = installedPHP.some(p => p.version === form.php_version)
    if (!isInstalled) {
      // Pick the highest installed version as default
      const best = [...installedPHP].sort((a, b) => parseFloat(b.version) - parseFloat(a.version))[0]
      setForm(f => ({ ...f, php_version: best.version }))
    }
  }, [installedPHP])

  // Find a matching DNS provider for the given domain
  const findMatchingProvider = (domain: string) => {
    const providers = dnsProviders || []
    return providers.find(p => domain === p.domain || domain.endsWith('.' + p.domain)) || null
  }

  // Trigger DNS check when both domain is valid and server is selected
  const triggerDNSCheck = async (domain: string, serverId: number) => {
    if (!domain || !serverId) return
    setDnsChecking(true)
    setDnsStatus(null)
    setDnsOffer(null)
    setDnsOfferMsg('')
    try {
      const res = await checkDNS.mutateAsync({ serverId, domain })
      setDnsStatus({ pointing: res.pointing, message: res.message, checked: true })
      // If DNS doesn't point correctly and SSL is enabled → auto-disable SSL with warning
      if (!res.pointing) {
        setForm(f => ({ ...f, ssl_enabled: false }))
        // Check if we have a matching DNS provider — if so, offer to auto-create record
        const matched = findMatchingProvider(domain)
        if (matched) {
          const server = servers.find(s => s.id === serverId)
          if (server) {
            const providerLabel = matched.provider_type === 'godaddy' ? 'GoDaddy'
              : matched.provider_type === 'route53' ? 'AWS Route53'
              : 'Cloudflare'
            setDnsProviderLabel(providerLabel)
            setDnsOffer('offered')
            setDnsOfferMsg(`Found a DNS provider for <strong>${matched.domain}</strong> (${providerLabel}). Create A record <code>${domain} → ${server.ip_address}</code> via ${providerLabel} now?`)
          }
        }
      } else {
        setDnsOffer(null)
      }
    } catch {
      setDnsStatus({ pointing: false, message: 'Could not check DNS', checked: true })
    } finally {
      setDnsChecking(false)
    }
  }

  const handleAutoCreateDNS = async () => {
    const server = servers.find(s => s.id === form.server_id)
    if (!server) return
    setDnsOffer('creating')
    try {
      const res = await autoCreateDNS.mutateAsync({ domain: form.domain, server_ip: server.ip_address })
      setDnsOffer('done')
      setDnsOfferMsg(`✅ A record ${res.action === 'created' ? 'created' : 'updated'}: <code>${form.domain} → ${server.ip_address}</code>. DNS may take a few minutes to propagate.`)
    } catch (err: any) {
      setDnsOffer('offered') // revert to offered state so user can retry
      setDnsOfferMsg(`❌ Error: ${err?.response?.data?.error || 'Could not create A record'}. Try again?`)
    }
  }

  const handleDomainChange = (val: string) => {
    const cleaned = val.replace(/^https?:\/\//i, '').replace(/\/.*$/, '')
    setForm(f => ({ ...f, domain: cleaned }))
    setDomainError(validateDomain(cleaned, sites))
    // Reset DNS status when domain changes
    setDnsStatus(null)
    setDnsOffer(null)
    setDnsOfferMsg('')
  }

  // Re-run DNS check when server changes (if domain already entered)
  useEffect(() => {
    if (form.server_id && form.domain && !validateDomain(form.domain, sites)) {
      triggerDNSCheck(form.domain, form.server_id)
    } else {
      setDnsStatus(null)
    }
  }, [form.server_id])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    const err = validateDomain(form.domain, sites)
    if (err) { setDomainError(err); return }
    try {
      const res = await createSite.mutateAsync(form)
      const siteId: number = res?.data?.site?.id
      setShowCreate(false)
      setForm({ server_id: 0, domain: '', php_version: '8.2', ssl_enabled: true, redis_enabled: false, install_type: 'auto', wp_admin_user: 'admin', wp_admin_email: '' })
      setDomainError('')
      setDnsStatus(null)
      if (siteId) {
        setCreatingInfo({ id: siteId, domain: form.domain, installType: form.install_type })
      }
    } catch {}
  }

  const inputCls = 'w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-gray-50 focus:bg-white transition-colors'
  const selectCls = 'w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-white'

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-7">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 leading-none">Sites</h1>
          <p className="text-gray-500 text-sm mt-1">{sites.length} site{sites.length !== 1 ? 's' : ''} managed</p>
        </div>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 transition-colors text-sm font-semibold shadow-sm shadow-blue-200"
        >
          <Plus size={16} /> New Site
        </button>
      </div>

      {/* Create Form */}
      {showCreate && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 mb-6">
          <div className="flex items-center justify-between mb-5">
            <h2 className="text-base font-semibold text-gray-900">Create New Site</h2>
            <button onClick={() => { setShowCreate(false); setDnsStatus(null); setDnsOffer(null) }} className="text-gray-400 hover:text-gray-600">
              <X size={18} />
            </button>
          </div>

          {/* Install type */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-5">
            {([
              { value: 'auto', label: '⚡ Auto Install WordPress', desc: 'Automatically installs WP, creates DB, Nginx, PHP-FPM + SSL' },
              { value: 'manual', label: '📦 Upload Source / Import DB', desc: 'Creates infrastructure only — you upload the source and import the DB' },
            ] as const).map(opt => (
              <button key={opt.value} type="button"
                onClick={() => setForm({ ...form, install_type: opt.value })}
                className={`p-4 rounded-xl border-2 text-left transition-all ${
                  form.install_type === opt.value
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-100 hover:border-gray-200 bg-gray-50'
                }`}>
                <div className="font-semibold text-sm text-gray-800">{opt.label}</div>
                <div className="text-xs text-gray-500 mt-1">{opt.desc}</div>
              </button>
            ))}
          </div>

          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">Server</label>
                <select value={form.server_id}
                  onChange={(e) => {
                    setForm(f => ({ ...f, server_id: Number(e.target.value) }))
                    setDnsStatus(null) // reset DNS when server changes (useEffect will re-check)
                  }}
                  className={selectCls} required>
                  <option value={0}>Select server…</option>
                  {servers.filter(s => s.status === 'active').map(s => (
                    <option key={s.id} value={s.id}>{s.name} ({s.ip_address})</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">Domain</label>
                <div className="flex gap-2">
                  <input
                    value={form.domain}
                    onChange={(e) => handleDomainChange(e.target.value)}
                    onBlur={() => {
                      if (form.domain && !domainError && form.server_id) {
                        triggerDNSCheck(form.domain, form.server_id)
                      }
                    }}
                    className={`flex-1 px-4 py-2.5 border rounded-xl outline-none text-sm transition-colors ${
                      domainError
                        ? 'border-red-400 bg-red-50 focus:ring-2 focus:ring-red-500/20 focus:border-red-500'
                        : 'border-gray-200 bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500'
                    }`}
                    placeholder="example.com"
                    required
                  />
                  {/* Manual DNS check button */}
                  {form.server_id > 0 && form.domain && !domainError && (
                    <button type="button"
                      onClick={() => triggerDNSCheck(form.domain, form.server_id)}
                      disabled={dnsChecking}
                      title="Check DNS"
                      className="px-3 py-2.5 border border-gray-200 rounded-xl text-sm text-gray-600 hover:bg-gray-50 disabled:opacity-50 flex items-center gap-1.5 shrink-0">
                      {dnsChecking
                        ? <Loader2 size={14} className="animate-spin" />
                        : <RefreshCw size={14} />}
                      DNS
                    </button>
                  )}
                </div>
                {domainError && <p className="mt-1.5 text-xs text-red-600">{domainError}</p>}
                {/* DNS check result */}
                {!domainError && dnsStatus && (
                  <div className={`mt-1.5 flex items-start gap-1.5 text-xs ${dnsStatus.pointing ? 'text-emerald-600' : 'text-amber-600'}`}>
                    {dnsStatus.pointing
                      ? <CheckCircle size={13} className="shrink-0 mt-0.5" />
                      : <XCircle size={13} className="shrink-0 mt-0.5" />}
                    <span>{dnsStatus.message}</span>
                  </div>
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">
                  PHP Version
                  {phpFetching && <span className="ml-2 text-xs text-gray-400">(checking...)</span>}
                </label>
                {/* If we have server-specific PHP info, show only installed versions */}
                {installedPHP.length > 0 ? (
                  <>
                    <select value={form.php_version}
                      onChange={(e) => setForm({ ...form, php_version: e.target.value })}
                      className={selectCls}>
                      {installedPHP.map(p => (
                        <option key={p.version} value={p.version}>
                          PHP {p.version}{p.fpm_active ? ' ✓' : ' (inactive)'}
                        </option>
                      ))}
                    </select>
                    {!phpVersionOK && (
                      <p className="mt-1.5 text-xs text-red-600">
                        ⚠️ PHP {form.php_version} is not installed on this server
                      </p>
                    )}
                  </>
                ) : (
                  <>
                    <select value={form.php_version}
                      onChange={(e) => setForm({ ...form, php_version: e.target.value })}
                      className={selectCls}>
                      {['7.4', '8.0', '8.1', '8.2', '8.3', '8.4', '8.5'].map(v => (
                        <option key={v} value={v}>PHP {v}</option>
                      ))}
                    </select>
                    {form.server_id > 0 && !phpFetching && (
                      <p className="mt-1.5 text-xs text-gray-400">Select a server to see installed PHP versions</p>
                    )}
                  </>
                )}
              </div>
              {form.install_type === 'auto' && (
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1.5">Admin Email</label>
                  <input type="email" value={form.wp_admin_email}
                    onChange={(e) => setForm({ ...form, wp_admin_email: e.target.value })}
                    className={inputCls} placeholder="admin@example.com" required />
                </div>
              )}
            </div>

            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-5">
                {[
                  { key: 'ssl_enabled', label: '🔒 Auto SSL (Let\'s Encrypt)' },
                  { key: 'redis_enabled', label: '⚡ Redis Cache' },
                ].map(({ key, label }) => (
                  <label key={key} className="flex items-center gap-2.5 cursor-pointer select-none">
                    <div className="relative">
                      <input
                        type="checkbox"
                        checked={form[key as keyof typeof form] as boolean}
                        onChange={(e) => setForm({ ...form, [key]: e.target.checked })}
                        className="sr-only"
                      />
                      <div className={`w-9 h-5 rounded-full transition-colors ${
                        form[key as keyof typeof form] ? 'bg-blue-600' : 'bg-gray-200'
                      }`}>
                        <div className={`absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow transition-transform ${
                          form[key as keyof typeof form] ? 'translate-x-4' : ''
                        }`} />
                      </div>
                    </div>
                    <span className="text-sm text-gray-700">{label}</span>
                  </label>
                ))}
              </div>
              {/* DNS warning for SSL */}
              {dnsStatus && !dnsStatus.pointing && (
                <div className="flex items-start gap-2 p-3 bg-amber-50 border border-amber-200 rounded-xl text-sm text-amber-700">
                  <XCircle size={15} className="shrink-0 mt-0.5" />
                  <div>
                    <p className="font-medium">DNS does not point to this server — Auto SSL disabled</p>
                    <p className="text-xs mt-0.5 text-amber-600">{dnsStatus.message}</p>
                    <p className="text-xs mt-0.5 text-amber-600">
                      Point DNS to the correct IP and re-enable SSL, or enable it manually after the site is created.
                    </p>
                  </div>
                </div>
              )}

              {/* DNS auto-create offer banner */}
              {dnsOffer && dnsOffer !== 'skipped' && (
                <div className={`flex items-start gap-3 p-4 rounded-xl border text-sm ${
                  dnsOffer === 'done'
                    ? 'bg-emerald-50 border-emerald-200 text-emerald-700'
                    : 'bg-blue-50 border-blue-200 text-blue-700'
                }`}>
                  <Network size={16} className="shrink-0 mt-0.5" />
                  <div className="flex-1 min-w-0">
                    <p
                      className="font-medium leading-snug"
                      dangerouslySetInnerHTML={{ __html: dnsOffer === 'done'
                        ? dnsOfferMsg
                        : dnsOfferMsg.replace(/<strong>/g, '<span class="font-semibold">').replace(/<\/strong>/g, '</span>')
                      }}
                    />
                    {dnsOffer === 'offered' && (
                      <div className="flex items-center gap-2 mt-3">
                        <button
                          type="button"
                          onClick={handleAutoCreateDNS}
                          className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-xs font-semibold hover:bg-blue-700 transition-colors"
                        >
                          <Network size={13} /> Create A record
                        </button>
                        <button
                          type="button"
                          onClick={() => setDnsOffer('skipped')}
                          className="px-3 py-1.5 text-blue-600 rounded-lg text-xs hover:bg-blue-100 transition-colors"
                        >
                          Skip, just create the site
                        </button>
                      </div>
                    )}
                    {dnsOffer === 'creating' && (
                      <div className="flex items-center gap-2 mt-2 text-xs text-blue-500">
                        <Loader2 size={13} className="animate-spin" />
                        Creating A record via {dnsProviderLabel}...
                      </div>
                    )}
                    {dnsOffer === 'done' && (
                      <p className="text-xs mt-1.5 text-emerald-600">
                        DNS propagation can take 1–5 minutes. You can still create the site now.
                      </p>
                    )}
                  </div>
                </div>
              )}
            </div>

            {form.install_type === 'manual' && (
              <div className="bg-blue-50 border border-blue-100 rounded-xl p-4 text-sm text-blue-700">
                💡 After creation, open the site details to <b>upload the source .zip</b> and <b>import the database .sql</b>
              </div>
            )}

            {createSite.error && (
              <div className="p-4 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
                {(createSite.error as any)?.response?.data?.error || 'Failed to create site'}
              </div>
            )}

            <div className="flex items-center gap-3">
              <button type="submit" disabled={createSite.isPending || !!domainError}
                className="flex items-center gap-2 px-5 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 disabled:opacity-50 text-sm font-semibold transition-colors">
                {createSite.isPending ? (
                  <><Loader2 size={15} className="animate-spin" /> Creating...</>
                ) : form.install_type === 'manual' ? 'Create Site (Manual)' : 'Create Site'}
              </button>
              <button type="button" onClick={() => setShowCreate(false)}
                className="px-5 py-2.5 text-gray-500 rounded-xl hover:bg-gray-100 text-sm transition-colors">
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Creation progress */}
      {creatingInfo && (
        <CreationProgressCard
          siteId={creatingInfo.id}
          domain={creatingInfo.domain}
          installType={creatingInfo.installType}
          onDismiss={() => setCreatingInfo(null)}
        />
      )}

      {/* Filters */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 mb-5">
        <div className="flex-1 relative">
          <Search size={15} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            placeholder="Search by domain or server…"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-9 pr-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 transition-colors"
          />
        </div>
        <div className="relative flex-shrink-0">
          <Filter size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="pl-8 pr-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 cursor-pointer appearance-none"
          >
            <option value="all">All Status</option>
            <option value="active">Active</option>
            <option value="suspended">Suspended</option>
            <option value="creating">Creating</option>
            <option value="error">Error</option>
          </select>
        </div>
      </div>

      {/* Desktop Table */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden hidden md:block">
        <table className="w-full">
          <thead>
            <tr className="border-b border-gray-50">
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Domain</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Server</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">PHP</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">SSL</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Status</th>
              <th className="text-right px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {isLoading && (
              <tr><td colSpan={6} className="px-6 py-10 text-center">
                <Loader2 size={22} className="animate-spin text-gray-300 mx-auto" />
              </td></tr>
            )}
            {filteredSites.map((site) => (
              <tr key={site.id} className="hover:bg-gray-50/60 transition-colors">
                <td className="px-6 py-4">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-violet-500 to-violet-700 flex items-center justify-center flex-shrink-0">
                      <Globe size={14} className="text-white" />
                    </div>
                    <Link to={`/sites/${site.id}`} className="font-mono font-medium text-[13px] text-gray-900 hover:text-blue-600 transition-colors">
                      {site.domain}
                    </Link>
                  </div>
                </td>
                <td className="px-6 py-4 text-sm text-gray-500">{site.server?.name || `Server #${site.server_id}`}</td>
                <td className="px-6 py-4">
                  <span className="px-2 py-0.5 rounded-md bg-gray-100 text-gray-600 text-xs font-mono">
                    PHP {site.php_version}
                  </span>
                </td>
                <td className="px-6 py-4">
                  {site.ssl_enabled
                    ? <div className="flex items-center gap-1.5 text-emerald-600 text-xs font-medium">
                        <Lock size={13} /> SSL
                      </div>
                    : <div className="flex items-center gap-1.5 text-gray-400 text-xs">
                        <LockOpen size={13} /> No SSL
                      </div>
                  }
                </td>
                <td className="px-6 py-4"><StatusBadge status={site.status} /></td>
                <td className="px-6 py-4 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <button
                      onClick={() => { if (confirm(`Restart site ${site.domain}?`)) restartSite.mutate(site.id) }}
                      disabled={restartSite.isPending}
                      title="Restart"
                      className="p-1.5 text-gray-300 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
                      <RefreshCw size={14} className={restartSite.isPending ? 'animate-spin' : ''} />
                    </button>
                    {site.status === 'active' ? (
                      <button onClick={() => { if(confirm(`Suspend site ${site.domain}?`)) suspendSite.mutate(site.id) }}
                        disabled={suspendSite.isPending}
                        title="Suspend"
                        className="p-1.5 text-gray-300 hover:text-amber-500 hover:bg-amber-50 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
                        {suspendSite.isPending ? <Loader2 size={14} className="animate-spin" /> : <Pause size={14} />}
                      </button>
                    ) : site.status === 'suspended' ? (
                      <button onClick={() => resumeSite.mutate(site.id)}
                        disabled={resumeSite.isPending}
                        title="Resume"
                        className="p-1.5 text-gray-300 hover:text-emerald-600 hover:bg-emerald-50 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
                        {resumeSite.isPending ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                      </button>
                    ) : null}
                    <button
                      onClick={async () => {
                        try {
                          const res = await autologinMut.mutateAsync(site.id)
                          if (res?.data?.url) window.open(res.data.url, '_blank')
                          else alert(res?.error || 'Autologin is being set up. Try again in 30 seconds.')
                        } catch (e: any) {
                          alert(e?.response?.data?.error || 'Could not open WP Admin.')
                        }
                      }}
                      disabled={autologinMut.isPending}
                      title="WP Admin"
                      className="p-1.5 text-gray-300 hover:text-violet-600 hover:bg-violet-50 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
                      {autologinMut.isPending ? <Loader2 size={14} className="animate-spin" /> : <ExternalLink size={14} />}
                    </button>
                    <button
                      onClick={() => { if (confirm(`Delete site ${site.domain}?`)) deleteSite.mutate(site.id) }}
                      disabled={deleteSite.isPending}
                      title="Delete"
                      className="p-1.5 text-gray-300 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed">
                      {deleteSite.isPending ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {!isLoading && filteredSites.length === 0 && (
              <tr>
                <td colSpan={6} className="px-6 py-14 text-center">
                  <Globe size={32} className="text-gray-200 mx-auto mb-3" />
                  <p className="text-gray-400 text-sm">
                    {sites.length === 0 ? 'No sites yet.' : 'No sites match your filters.'}
                  </p>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Mobile Cards */}
      <div className="space-y-3 md:hidden">
        {isLoading && (
          <div className="bg-white rounded-2xl border border-gray-100 p-6 text-center">
            <Loader2 size={22} className="animate-spin text-gray-300 mx-auto" />
          </div>
        )}
        {filteredSites.map((site) => (
          <div key={site.id} className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
            <div className="flex items-center gap-3 p-4">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-violet-500 to-violet-700 flex items-center justify-center flex-shrink-0">
                <Globe size={18} className="text-white" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="font-semibold text-gray-900 truncate">{site.domain}</p>
                  <StatusBadge status={site.status} />
                </div>
                <p className="text-xs text-gray-400 mt-0.5">
                  PHP {site.php_version} · {site.server?.name || `Server #${site.server_id}`}
                </p>
              </div>
              <Link to={`/sites/${site.id}`} className="text-gray-300 hover:text-blue-600 flex-shrink-0">
                <ChevronRight size={18} />
              </Link>
            </div>
            <div className="flex items-center gap-2 px-4 py-3 border-t border-gray-50">
              {site.ssl_enabled
                ? <span className="flex items-center gap-1 text-xs text-emerald-600"><Lock size={11} /> SSL</span>
                : <span className="flex items-center gap-1 text-xs text-gray-400"><LockOpen size={11} /> No SSL</span>
              }
              <div className="ml-auto flex items-center gap-1">
                <button onClick={() => { if (confirm(`Restart site ${site.domain}?`)) restartSite.mutate(site.id) }}
                  disabled={restartSite.isPending}
                  title="Restart"
                  className="p-1.5 text-gray-300 hover:text-blue-600 rounded-lg transition-colors disabled:opacity-40">
                  <RefreshCw size={13} className={restartSite.isPending ? 'animate-spin' : ''} />
                </button>
                <button
                  onClick={async () => {
                    try {
                      const res = await autologinMut.mutateAsync(site.id)
                      if (res?.data?.url) window.open(res.data.url, '_blank')
                      else alert(res?.error || 'Autologin is being set up. Try again in 30 seconds.')
                    } catch (e: any) {
                      alert(e?.response?.data?.error || 'Could not open WP Admin.')
                    }
                  }}
                  disabled={autologinMut.isPending}
                  title="WP Admin"
                  className="p-1.5 text-gray-300 hover:text-violet-600 rounded-lg transition-colors disabled:opacity-40">
                  {autologinMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <ExternalLink size={13} />}
                </button>
                <button onClick={() => { if (confirm(`Delete site ${site.domain}?`)) deleteSite.mutate(site.id) }}
                  disabled={deleteSite.isPending}
                  title="Delete"
                  className="p-1.5 text-gray-300 hover:text-red-600 rounded-lg transition-colors disabled:opacity-40">
                  {deleteSite.isPending ? <Loader2 size={13} className="animate-spin" /> : <Trash2 size={13} />}
                </button>
              </div>
            </div>
          </div>
        ))}
        {!isLoading && filteredSites.length === 0 && (
          <div className="bg-white rounded-2xl border border-gray-100 p-10 text-center">
            <Globe size={32} className="text-gray-200 mx-auto mb-3" />
            <p className="text-gray-400 text-sm">
              {sites.length === 0 ? 'No sites yet.' : 'No sites match your filters.'}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
