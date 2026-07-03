import { useState } from 'react'
import {
  Globe, Plus, Trash2, RefreshCw, Loader2, CheckCircle2, XCircle,
  Wifi, Search, ChevronDown, ChevronUp, Pencil, X, Check
} from 'lucide-react'
import {
  useAllDNSProviders, useCreateGlobalDNSProvider, useDeleteGlobalDNSProvider,
  useTestDNSProvider, useLookupZone,
  useGlobalDNSRecords, useCreateGlobalDNSRecord, useDeleteGlobalDNSRecord, useUpdateGlobalDNSRecord,
} from '@/hooks/useDNS'
import { usePermissions } from '@/hooks/usePermissions'
import type { CreateDNSRecordRequest } from '@/types'

// ─── DNS Records Panel ───────────────────────────────────────────────────────

const TYPE_COLORS: Record<string, string> = {
  A: 'bg-blue-100 text-blue-700',
  AAAA: 'bg-indigo-100 text-indigo-700',
  CNAME: 'bg-purple-100 text-purple-700',
  MX: 'bg-orange-100 text-orange-700',
  TXT: 'bg-green-100 text-green-700',
  NS: 'bg-gray-100 text-gray-700',
}

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV']

function RecordRow({ record, canManage, providerId, providerType }: {
  record: import('@/types').DNSRecord
  canManage: boolean
  providerId: number
  providerType: string
}) {
  const isCloudflare = providerType === 'cloudflare'
  const deleteRecord = useDeleteGlobalDNSRecord(providerId)
  const updateRecord = useUpdateGlobalDNSRecord(providerId)
  const [editing, setEditing] = useState(false)
  const [edit, setEdit] = useState({ type: record.type, name: record.name, content: record.content, proxied: record.proxied ?? false, ttl: record.ttl || 300 })
  const [err, setErr] = useState('')

  const handleSave = async () => {
    setErr('')
    try {
      await updateRecord.mutateAsync({ recordId: record.id, ...edit, ttl: isCloudflare ? 1 : edit.ttl })
      setEditing(false)
    } catch (e: any) {
      setErr(e?.response?.data?.error || 'Update failed')
    }
  }

  const handleCancel = () => {
    setEdit({ type: record.type, name: record.name, content: record.content, proxied: record.proxied ?? false, ttl: record.ttl || 300 })
    setErr('')
    setEditing(false)
  }

  if (editing) {
    return (
      <div className="px-2 py-2 rounded-xl bg-blue-50 border border-blue-200 space-y-2">
        {err && <p className="text-xs text-red-600">{err}</p>}
        <div className="grid grid-cols-4 gap-2">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Type</label>
            <select value={edit.type} onChange={e => setEdit({ ...edit, type: e.target.value })}
              className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs bg-white">
              {RECORD_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Name</label>
            <input type="text" value={edit.name} onChange={e => setEdit({ ...edit, name: e.target.value })}
              className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" />
          </div>
          <div className={isCloudflare ? 'col-span-2' : ''}>
            <label className="block text-xs text-gray-500 mb-1">Content / Value</label>
            <input type="text" value={edit.content} onChange={e => setEdit({ ...edit, content: e.target.value })}
              className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" />
          </div>
          {!isCloudflare && (
            <div>
              <label className="block text-xs text-gray-500 mb-1">TTL (s)</label>
              <input type="number" value={edit.ttl} min={60} onChange={e => setEdit({ ...edit, ttl: parseInt(e.target.value) || 300 })}
                className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" />
            </div>
          )}
        </div>
        <div className="flex items-center justify-between">
          {isCloudflare ? (
            <label className="flex items-center gap-1.5 text-xs text-gray-600 cursor-pointer">
              <input type="checkbox" checked={edit.proxied} onChange={e => setEdit({ ...edit, proxied: e.target.checked })} />
              Proxied (Cloudflare CDN)
            </label>
          ) : (
            <span />
          )}
          <div className="flex gap-1.5">
            <button onClick={handleCancel}
              className="flex items-center gap-1 px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded-lg">
              <X size={11} /> Cancel
            </button>
            <button onClick={handleSave} disabled={updateRecord.isPending}
              className="flex items-center gap-1 px-2 py-1 text-xs text-white bg-blue-600 hover:bg-blue-700 rounded-lg disabled:opacity-50">
              {updateRecord.isPending ? <Loader2 size={11} className="animate-spin" /> : <Check size={11} />}
              {updateRecord.isPending ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-center justify-between px-2 py-1.5 rounded-lg hover:bg-gray-50 group">
      <div className="flex items-center gap-2 min-w-0">
        <span className={`flex-shrink-0 text-xs font-bold px-1.5 py-0.5 rounded ${TYPE_COLORS[record.type] || 'bg-gray-100 text-gray-700'}`}>
          {record.type}
        </span>
        <span className="text-xs font-medium text-gray-800 truncate">{record.name}</span>
        <span className="text-gray-300">→</span>
        <span className="text-xs text-gray-500 truncate">{record.content}</span>
        {record.proxied && isCloudflare && <span className="text-xs bg-orange-100 text-orange-600 px-1 rounded">Proxied</span>}
        {record.ttl > 0 && !isCloudflare && <span className="text-xs text-gray-400">TTL {record.ttl}s</span>}
      </div>
      {canManage && (
        <div className="flex items-center gap-1 flex-shrink-0 opacity-0 group-hover:opacity-100 ml-2">
          <button onClick={() => setEditing(true)}
            className="p-1 text-gray-400 hover:text-blue-500 hover:bg-blue-50 rounded transition-colors">
            <Pencil size={12} />
          </button>
          <button onClick={() => { if (confirm(`Delete ${record.type} record for ${record.name}?`)) deleteRecord.mutate(record.id) }}
            className="p-1 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded transition-colors">
            <Trash2 size={12} />
          </button>
        </div>
      )}
    </div>
  )
}

function DNSRecordsPanel({ providerId, providerDomain, providerType, canManage }: {
  providerId: number
  providerDomain: string
  providerType: string
  canManage: boolean
}) {
  const isCloudflare = providerType === 'cloudflare'
  const { data: records = [], isLoading, refetch } = useGlobalDNSRecords(providerId)
  const createRecord = useCreateGlobalDNSRecord(providerId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<CreateDNSRecordRequest>({ type: 'A', name: '', content: '', ttl: isCloudflare ? 1 : 300, proxied: false })
  const [err, setErr] = useState('')

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    try {
      await createRecord.mutateAsync(form)
      setForm({ type: 'A', name: '', content: '', ttl: isCloudflare ? 1 : 300, proxied: false })
      setShowForm(false)
    } catch (e: any) {
      setErr(e?.response?.data?.error || 'Failed to create record')
    }
  }

  return (
    <div className="mt-3 pl-3 border-l-2 border-blue-200">
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
          DNS Records — {providerDomain}
        </p>
        <div className="flex items-center gap-2">
          <button onClick={() => refetch()} className="p-1 text-gray-400 hover:text-blue-600 rounded transition-colors">
            <RefreshCw size={12} />
          </button>
          {canManage && (
            <button onClick={() => setShowForm(!showForm)}
              className="flex items-center gap-1 px-2 py-1 text-xs text-white bg-blue-600 rounded-lg hover:bg-blue-700">
              <Plus size={11} /> Add Record
            </button>
          )}
        </div>
      </div>

      {showForm && (
        <form onSubmit={handleCreate} className="mb-3 p-3 bg-blue-50 rounded-xl space-y-2 border border-blue-200">
          {err && <p className="text-xs text-red-600">{err}</p>}
          <div className="grid grid-cols-4 gap-2">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Type</label>
              <select value={form.type} onChange={e => setForm({ ...form, type: e.target.value })}
                className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs bg-white">
                {RECORD_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Name</label>
              <input type="text" placeholder="@ or subdomain" value={form.name}
                onChange={e => setForm({ ...form, name: e.target.value })}
                className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" required />
            </div>
            <div className={isCloudflare ? 'col-span-2' : ''}>
              <label className="block text-xs text-gray-500 mb-1">Content / Value</label>
              <input type="text" placeholder="IP address or target" value={form.content}
                onChange={e => setForm({ ...form, content: e.target.value })}
                className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" required />
            </div>
            {!isCloudflare && (
              <div>
                <label className="block text-xs text-gray-500 mb-1">TTL (s)</label>
                <input type="number" value={form.ttl} min={60}
                  onChange={e => setForm({ ...form, ttl: parseInt(e.target.value) || 300 })}
                  className="w-full px-2 py-1.5 border border-gray-200 rounded-lg text-xs" />
              </div>
            )}
          </div>
          <div className="flex items-center justify-between">
            {isCloudflare ? (
              <label className="flex items-center gap-1.5 text-xs text-gray-600 cursor-pointer">
                <input type="checkbox" checked={form.proxied} onChange={e => setForm({ ...form, proxied: e.target.checked })} />
                Proxied (Cloudflare CDN)
              </label>
            ) : (
              <span />
            )}
            <div className="flex gap-2">
              <button type="button" onClick={() => setShowForm(false)}
                className="px-2 py-1 text-xs text-gray-600 hover:bg-gray-200 rounded-lg">Cancel</button>
              <button type="submit" disabled={createRecord.isPending}
                className="px-3 py-1 text-xs text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {createRecord.isPending ? 'Adding...' : 'Add'}
              </button>
            </div>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="flex items-center gap-2 py-3 text-xs text-gray-400">
          <Loader2 size={12} className="animate-spin" /> Loading records...
        </div>
      ) : records.length === 0 ? (
        <p className="text-xs text-gray-400 py-2">No DNS records found.</p>
      ) : (
        <div className="space-y-1">
          {records.map(r => (
            <RecordRow key={r.id} record={r} canManage={canManage} providerId={providerId} providerType={providerType} />
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Add Provider Modal ──────────────────────────────────────────────────────

function AddProviderModal({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState({
    provider_type: 'cloudflare',
    api_key: '',
    api_email: '',
    zone_id: '',
    domain: '',
  })
  const createProvider = useCreateGlobalDNSProvider()
  const testProvider = useTestDNSProvider()
  const lookupZone = useLookupZone()
  const [err, setErr] = useState('')
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)
  const [lookupErr, setLookupErr] = useState('')

  const updateForm = (patch: Partial<typeof form>) => {
    setForm(f => ({ ...f, ...patch }))
    setTestResult(null)
    if (patch.provider_type) {
      // Reset credentials when switching provider type to avoid confusion
      setForm(f => ({ ...f, ...patch, api_key: '', api_email: '', zone_id: '' }))
    }
  }

  // Auto-lookup Zone ID from domain
  const handleLookupZone = async () => {
    if (!form.api_key || !form.domain) return
    setLookupErr('')
    try {
      const result = await lookupZone.mutateAsync({
        provider_type: form.provider_type,
        api_key: form.api_key,
        api_email: form.api_email || undefined,
        domain: form.domain,
      })
      updateForm({ zone_id: result.zone_id })
    } catch (e: any) {
      setLookupErr(e?.response?.data?.error || 'Zone not found')
    }
  }

  // Test connection
  const handleTest = async () => {
    // GoDaddy: zone_id = domain; others need explicit zone_id
    if (!form.api_key || (!form.zone_id && form.provider_type !== 'godaddy')) return
    setTestResult(null)
    const testPayload = form.provider_type === 'godaddy'
      ? { ...form, zone_id: form.domain }
      : form
    try {
      const result = await testProvider.mutateAsync(testPayload as any)
      setTestResult({ success: result.success, message: result.message })
    } catch (e: any) {
      setTestResult({ success: false, message: e?.response?.data?.error || 'Connection test failed' })
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    try {
      await createProvider.mutateAsync(form)
      onClose()
    } catch (e: any) {
      setErr(e?.response?.data?.error || 'Failed to add DNS provider')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-lg">
        <div className="px-6 py-4 border-b border-gray-100 flex items-center justify-between">
          <h3 className="text-base font-semibold text-gray-900 flex items-center gap-2">
            <Globe size={18} className="text-blue-500" /> Add DNS Provider
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-xl leading-none">&times;</button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          {err && <p className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded-xl">{err}</p>}

          {/* Provider Type */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1.5">Provider</label>
            <select value={form.provider_type} onChange={e => updateForm({ provider_type: e.target.value })}
              className="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none">
              <option value="cloudflare">Cloudflare</option>
              <option value="route53">AWS Route53</option>
              <option value="godaddy">GoDaddy</option>
            </select>
          </div>

          {/* API Key — label differs by provider */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1.5">
              {form.provider_type === 'route53' ? 'AWS Access Key ID'
                : form.provider_type === 'godaddy' ? 'GoDaddy API Key'
                : 'API Token / Key'}
            </label>
            <input
              type="password"
              placeholder={
                form.provider_type === 'route53' ? 'AKIAIOSFODNN7EXAMPLE'
                : form.provider_type === 'godaddy' ? '3mM44UoYLJmGaD7_xxxxxxxx'
                : 'Cloudflare API Token or Global API Key'
              }
              value={form.api_key}
              onChange={e => updateForm({ api_key: e.target.value })}
              className="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              required
            />
            {form.provider_type === 'godaddy' && (
              <p className="text-xs text-gray-400 mt-1">
                Get it at <a href="https://developer.godaddy.com/keys" target="_blank" rel="noreferrer"
                  className="text-blue-500 hover:underline">developer.godaddy.com/keys</a> → Production
              </p>
            )}
          </div>

          {/* API Email / Secret Key — label and requirement differ by provider */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1.5">
              {form.provider_type === 'route53' ? 'AWS Secret Access Key'
                : form.provider_type === 'godaddy' ? 'GoDaddy API Secret'
                : <>API Email <span className="text-gray-400 font-normal">(only needed with a Global API Key)</span></>}
            </label>
            <input
              type="password"
              placeholder={
                form.provider_type === 'route53' ? 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'
                : form.provider_type === 'godaddy' ? 'WKc9GBqDL1aAzEF_xxxxxxxx'
                : 'email@example.com'
              }
              value={form.api_email}
              onChange={e => updateForm({ api_email: e.target.value })}
              className="w-full px-3 py-2 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
              required={form.provider_type === 'route53' || form.provider_type === 'godaddy'}
            />
            {form.provider_type === 'route53' && (
              <p className="text-xs text-gray-400 mt-1">
                Create an IAM user with the <code className="bg-gray-100 px-1 rounded">AmazonRoute53FullAccess</code> permission
              </p>
            )}
          </div>

          {/* Domain */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1.5">Domain (root zone)</label>
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="example.com"
                value={form.domain}
                onChange={e => {
                  const d = e.target.value
                  // GoDaddy: zone_id = domain (no separate zone concept)
                  updateForm(form.provider_type === 'godaddy' ? { domain: d, zone_id: d } : { domain: d })
                }}
                className="flex-1 px-3 py-2 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none"
                required
              />
              {/* "Find Zone ID" only for Cloudflare and Route53 */}
              {form.provider_type !== 'godaddy' && (
                <button
                  type="button"
                  onClick={handleLookupZone}
                  disabled={!form.api_key || !form.domain || lookupZone.isPending || (form.provider_type === 'route53' && !form.api_email)}
                  title="Auto-find Zone ID from domain"
                  className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium border border-gray-200 rounded-xl bg-gray-50 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed text-gray-700 transition-colors whitespace-nowrap"
                >
                  {lookupZone.isPending
                    ? <><Loader2 size={13} className="animate-spin" /> Finding...</>
                    : <><Search size={13} /> Find Zone ID</>
                  }
                </button>
              )}
              {/* GoDaddy: "Verify" button confirms domain is in account */}
              {form.provider_type === 'godaddy' && (
                <button
                  type="button"
                  onClick={handleLookupZone}
                  disabled={!form.api_key || !form.api_email || !form.domain || lookupZone.isPending}
                  title="Verify the domain in your GoDaddy account"
                  className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium border border-gray-200 rounded-xl bg-gray-50 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed text-gray-700 transition-colors whitespace-nowrap"
                >
                  {lookupZone.isPending
                    ? <><Loader2 size={13} className="animate-spin" /> Verifying...</>
                    : <><Search size={13} /> Verify</>
                  }
                </button>
              )}
            </div>
            {lookupErr && <p className="text-xs text-red-500 mt-1">{lookupErr}</p>}
            {form.provider_type === 'godaddy' && form.domain && (
              <p className="text-xs text-gray-400 mt-1">GoDaddy uses the domain as the Zone ID — no separate input needed.</p>
            )}
          </div>

          {/* Zone ID + Test — hidden for GoDaddy (zone_id = domain, handled above) */}
          {form.provider_type !== 'godaddy' && (
            <div>
              <label className="block text-xs font-medium text-gray-700 mb-1.5">
                {form.provider_type === 'route53' ? 'Hosted Zone ID' : 'Zone ID'}
                <span className="ml-1.5 text-gray-400 font-normal">
                  {form.provider_type === 'route53'
                    ? '(AWS Console → Route53 → Hosted Zones)'
                    : '(Cloudflare Dashboard → Overview)'}
                </span>
              </label>
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder={form.provider_type === 'route53' ? 'Z1D633PJN98FT9' : '32-char hex from Cloudflare'}
                  value={form.zone_id}
                  onChange={e => updateForm({ zone_id: e.target.value })}
                  className="flex-1 px-3 py-2 border border-gray-200 rounded-xl text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 outline-none font-mono"
                  required
                />
                <button
                  type="button"
                  onClick={handleTest}
                  disabled={!form.api_key || !form.zone_id || testProvider.isPending || (form.provider_type === 'route53' && !form.api_email)}
                  title="Test connection"
                  className="flex items-center gap-1.5 px-3 py-2 text-sm font-medium border border-gray-200 rounded-xl bg-gray-50 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed text-gray-700 transition-colors whitespace-nowrap"
                >
                  {testProvider.isPending
                    ? <><Loader2 size={13} className="animate-spin" /> Testing...</>
                    : <><Wifi size={13} /> Test</>
                  }
                </button>
              </div>
            </div>
          )}

          {/* GoDaddy: Test button inline (zone_id field hidden) */}
          {form.provider_type === 'godaddy' && (
            <button
              type="button"
              onClick={handleTest}
              disabled={!form.api_key || !form.api_email || !form.domain || testProvider.isPending}
              className="w-full flex items-center justify-center gap-1.5 px-3 py-2 text-sm font-medium border border-gray-200 rounded-xl bg-gray-50 hover:bg-gray-100 disabled:opacity-40 disabled:cursor-not-allowed text-gray-700 transition-colors"
            >
              {testProvider.isPending
                ? <><Loader2 size={13} className="animate-spin" /> Testing...</>
                : <><Wifi size={13} /> Test Connection</>
              }
            </button>
          )}

          {/* Test result */}
          {testResult && (
            <div className={`flex items-start gap-2 px-3 py-2.5 rounded-xl text-sm ${
              testResult.success
                ? 'bg-emerald-50 border border-emerald-200 text-emerald-700'
                : 'bg-red-50 border border-red-200 text-red-700'
            }`}>
              {testResult.success
                ? <CheckCircle2 size={16} className="flex-shrink-0 mt-0.5 text-emerald-500" />
                : <XCircle size={16} className="flex-shrink-0 mt-0.5 text-red-500" />
              }
              <span>{testResult.message}</span>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose}
              className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-xl">Cancel</button>
            <button type="submit" disabled={createProvider.isPending}
              className="px-5 py-2 text-sm font-semibold text-white bg-blue-600 rounded-xl hover:bg-blue-700 disabled:opacity-50">
              {createProvider.isPending ? 'Adding...' : 'Add Provider'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ─── Provider Card ────────────────────────────────────────────────────────────

function ProviderCard({ provider, expanded, onToggle, canManage }: {
  provider: import('@/types').DNSProvider
  expanded: boolean
  onToggle: () => void
  canManage: boolean
}) {
  const deleteProvider = useDeleteGlobalDNSProvider()

  const providerLabel = provider.provider_type === 'cloudflare' ? 'Cloudflare'
    : provider.provider_type === 'route53' ? 'AWS Route53'
    : 'GoDaddy'
  const providerColor = provider.provider_type === 'cloudflare'
    ? 'bg-orange-100 text-orange-600'
    : provider.provider_type === 'route53'
    ? 'bg-yellow-100 text-yellow-600'
    : 'bg-green-100 text-green-700'

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
      <div className="flex items-center gap-3 px-5 py-4">
        <button onClick={onToggle} className="flex items-center gap-3 flex-1 text-left min-w-0">
          <div className={`w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0 ${providerColor}`}>
            <Globe size={18} />
          </div>
          <div className="min-w-0">
            <p className="font-semibold text-sm text-gray-900 truncate">{provider.domain}</p>
            <p className="text-xs text-gray-500">
              {providerLabel}
              {provider.zone_id && ` · Zone: ${provider.zone_id.slice(0, 8)}...`}
            </p>
          </div>
          <span className="ml-auto text-xs text-blue-500 flex-shrink-0 flex items-center gap-1">
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            {expanded ? 'Hide Records' : 'Show Records'}
          </span>
        </button>
        {canManage && (
          <button
            onClick={() => { if (confirm(`Remove DNS provider for ${provider.domain}?`)) deleteProvider.mutate(provider.id) }}
            className="p-2 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-xl flex-shrink-0 transition-colors">
            <Trash2 size={15} />
          </button>
        )}
      </div>
      {expanded && (
        <div className="px-5 pb-4 border-t border-gray-50">
          <DNSRecordsPanel
            providerId={provider.id}
            providerDomain={provider.domain}
            providerType={provider.provider_type}
            canManage={canManage}
          />
        </div>
      )}
    </div>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export default function DNSProvidersPage() {
  const { data: providers = [], isLoading } = useAllDNSProviders()
  const { can } = usePermissions()
  const [showAddModal, setShowAddModal] = useState(false)
  const [expandedProvider, setExpandedProvider] = useState<number | null>(null)

  const canWrite = can('infrastructure', 'manage')

  return (
    <div className="p-6 max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 flex items-center gap-2">
            <Globe size={24} className="text-blue-500" /> DNS Providers & Records
          </h1>
          <p className="text-sm text-gray-500 mt-1">
            Manage DNS providers and records for all domains
          </p>
        </div>
        {canWrite && (
          <button onClick={() => setShowAddModal(true)}
            className="flex items-center gap-1.5 px-4 py-2 text-sm font-semibold text-white bg-blue-600 rounded-xl hover:bg-blue-700">
            <Plus size={15} /> Add Provider
          </button>
        )}
      </div>

      {/* Stats */}
      {providers.length > 0 && (
        <div className="grid grid-cols-2 gap-4 mb-6">
          <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
            <p className="text-xs text-gray-500 mb-1">Total Providers</p>
            <p className="text-2xl font-bold text-gray-900">{providers.length}</p>
          </div>
          <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
            <p className="text-xs text-gray-500 mb-1">CF / R53 / GoDaddy</p>
            <p className="text-2xl font-bold text-gray-900">
              {providers.filter(p => p.provider_type === 'cloudflare').length}
              <span className="text-gray-400 text-base font-normal"> / </span>
              {providers.filter(p => p.provider_type === 'route53').length}
              <span className="text-gray-400 text-base font-normal"> / </span>
              {providers.filter(p => p.provider_type === 'godaddy').length}
            </p>
          </div>
        </div>
      )}

      {/* Content */}
      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 size={28} className="animate-spin text-blue-500" />
        </div>
      ) : providers.length === 0 ? (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-12 text-center">
          <Globe size={40} className="text-gray-300 mx-auto mb-3" />
          <p className="text-gray-600 font-semibold mb-1">No DNS providers yet</p>
          <p className="text-gray-400 text-sm mb-5">
            Add Cloudflare, AWS Route53, or GoDaddy to manage DNS records directly from here.
          </p>
          {canWrite && (
            <button onClick={() => setShowAddModal(true)}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-semibold text-white bg-blue-600 rounded-xl hover:bg-blue-700">
              <Plus size={14} /> Add Provider
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {providers.map(p => (
            <ProviderCard
              key={p.id}
              provider={p}
              expanded={expandedProvider === p.id}
              onToggle={() => setExpandedProvider(expandedProvider === p.id ? null : p.id)}
              canManage={canWrite}
            />
          ))}
        </div>
      )}

      {showAddModal && (
        <AddProviderModal onClose={() => setShowAddModal(false)} />
      )}
    </div>
  )
}
