import { useState } from 'react'
import { Loader2, Globe, Plus, Network, CheckCircle2, AlertCircle, Lock, ExternalLink, CornerDownRight, Trash2, ShieldCheck, X } from 'lucide-react'
import { useDomains, useCreateDomain, useUpdateDomain, useDeleteDomain, useIssueDomainSSL, useCheckDNS } from '@/hooks/useDomains'

export function DomainsPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: domains = [], isLoading } = useDomains(siteId)
  const createDomain = useCreateDomain()
  const updateDomain = useUpdateDomain()
  const deleteDomain = useDeleteDomain()
  const issueDomainSSL = useIssueDomainSSL()
  const checkDNS = useCheckDNS(siteId)

  const [showForm, setShowForm] = useState(false)
  const [newDomain, setNewDomain] = useState('')
  const [newType, setNewType] = useState<'alias' | 'subdomain'>('alias')
  const [dnsResult, setDnsResult] = useState<{ domain: string; pointing: boolean; message: string } | null>(null)
  const [dnsChecking, setDnsChecking] = useState(false)

  // Redirect edit modal state
  const [redirectModal, setRedirectModal] = useState<{ domainId: number; domain: string; redirectTo: string; redirectType: number } | null>(null)

  // SSL issuing state per domain
  const [sslIssuingId, setSslIssuingId] = useState<number | null>(null)
  const [sslError, setSslError] = useState<string | null>(null)

  const handleCheckDNS = async () => {
    if (!newDomain.trim()) return
    setDnsChecking(true)
    setDnsResult(null)
    try {
      const res = await checkDNS.mutateAsync(newDomain.trim())
      setDnsResult({ domain: res.domain, pointing: res.pointing, message: res.message })
    } catch {
      setDnsResult({ domain: newDomain, pointing: false, message: 'DNS check failed' })
    } finally {
      setDnsChecking(false)
    }
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createDomain.mutateAsync({ siteId, domain: newDomain, type: newType })
    setNewDomain('')
    setDnsResult(null)
    setShowForm(false)
  }

  const handleIssueSSL = async (domainId: number) => {
    setSslIssuingId(domainId)
    setSslError(null)
    try {
      await issueDomainSSL.mutateAsync({ siteId, domainId })
    } catch (e: any) {
      setSslError(e?.response?.data?.error || 'SSL issuance failed')
    } finally {
      setSslIssuingId(null)
    }
  }

  const handleSaveRedirect = async () => {
    if (!redirectModal) return
    await updateDomain.mutateAsync({
      siteId,
      domainId: redirectModal.domainId,
      redirect_to: redirectModal.redirectTo,
      redirect_type: redirectModal.redirectType,
    })
    setRedirectModal(null)
  }

  const handleClearRedirect = async (domainId: number) => {
    await updateDomain.mutateAsync({ siteId, domainId, redirect_to: '', redirect_type: 0 })
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Globe size={20} className="text-blue-500" /> Domain Management
        </h2>
        {canManage && (
          <button onClick={() => setShowForm(!showForm)}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Add Domain
          </button>
        )}
      </div>

      {/* Add Domain Form */}
      {showForm && (
        <div className="mb-4 p-4 bg-gray-50 rounded-xl border border-gray-200 space-y-3">
          <div className="flex gap-3">
            <input type="text" placeholder="e.g. www.example.com" value={newDomain}
              onChange={e => { setNewDomain(e.target.value); setDnsResult(null) }}
              className="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-sm bg-white focus:outline-none focus:border-blue-500" />
            <select value={newType} onChange={e => setNewType(e.target.value as 'alias' | 'subdomain')}
              className="px-3 py-2 border border-gray-200 rounded-lg text-sm bg-white">
              <option value="alias">Alias</option>
              <option value="subdomain">Subdomain</option>
            </select>
            <button type="button" onClick={handleCheckDNS} disabled={!newDomain.trim() || dnsChecking}
              className="flex items-center gap-1.5 px-3 py-2 text-sm border border-gray-300 rounded-lg hover:bg-gray-100 disabled:opacity-50">
              {dnsChecking ? <Loader2 size={14} className="animate-spin" /> : <Network size={14} />}
              Check DNS
            </button>
          </div>

          {/* DNS check result */}
          {dnsResult && (
            <div className={`flex items-start gap-2 p-3 rounded-lg text-sm ${dnsResult.pointing ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700'}`}>
              {dnsResult.pointing
                ? <CheckCircle2 size={15} className="shrink-0 mt-0.5" />
                : <AlertCircle size={15} className="shrink-0 mt-0.5" />}
              <span>{dnsResult.message}</span>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button type="button" onClick={() => { setShowForm(false); setDnsResult(null) }}
              className="px-3 py-2 text-sm border rounded-lg hover:bg-gray-50">Cancel</button>
            <button type="button" onClick={handleCreate} disabled={!newDomain.trim() || createDomain.isPending}
              className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createDomain.isPending ? 'Adding...' : 'Add Domain'}
            </button>
          </div>
        </div>
      )}

      {sslError && (
        <div className="mb-3 flex items-center gap-2 p-3 bg-red-50 text-red-700 text-sm rounded-lg">
          <AlertCircle size={15} /> {sslError}
        </div>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><Loader2 className="animate-spin text-blue-600" size={24} /></div>
      ) : domains.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No domains configured</p>
      ) : (
        <div className="space-y-2">
          {domains.map(d => (
            <div key={d.id} className="border border-gray-200 rounded-xl overflow-hidden">
              {/* Domain row */}
              <div className="flex items-center justify-between p-3 bg-white">
                <div className="flex items-center gap-2 flex-wrap">
                  <Globe size={15} className="text-blue-500 shrink-0" />
                  <a href={`https://${d.domain}`} target="_blank" rel="noreferrer"
                    className="text-sm font-medium text-gray-900 hover:text-blue-600 flex items-center gap-1">
                    {d.domain} <ExternalLink size={11} />
                  </a>
                  {/* Type badge */}
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                    d.type === 'primary' ? 'bg-blue-100 text-blue-700' :
                    d.type === 'subdomain' ? 'bg-purple-100 text-purple-700' :
                    'bg-gray-100 text-gray-600'
                  }`}>{d.type}</span>
                  {/* SSL badge */}
                  {d.ssl_enabled ? (
                    <span className="flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-700">
                      <ShieldCheck size={11} /> SSL
                    </span>
                  ) : d.type !== 'primary' && (
                    <span className="px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-400">No SSL</span>
                  )}
                  {/* Redirect badge */}
                  {d.redirect_to && (
                    <span className="flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-700">
                      <CornerDownRight size={11} /> {d.redirect_type}→
                    </span>
                  )}
                </div>

                {/* Actions */}
                {canManage && d.type !== 'primary' && (
                  <div className="flex items-center gap-1 shrink-0">
                    {/* Issue SSL button */}
                    {!d.ssl_enabled && (
                      <button
                        onClick={() => handleIssueSSL(d.id)}
                        disabled={sslIssuingId === d.id}
                        title="Issue SSL (certbot expand)"
                        className="flex items-center gap-1 px-2 py-1 text-xs border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50 text-gray-600">
                        {sslIssuingId === d.id ? <Loader2 size={12} className="animate-spin" /> : <Lock size={12} />}
                        SSL
                      </button>
                    )}
                    {/* Configure redirect */}
                    <button
                      onClick={() => setRedirectModal({
                        domainId: d.id, domain: d.domain,
                        redirectTo: d.redirect_to || '', redirectType: d.redirect_type || 301
                      })}
                      title="Configure redirect"
                      className="p-1.5 text-gray-400 hover:text-blue-600 rounded hover:bg-gray-50">
                      <CornerDownRight size={14} />
                    </button>
                    {/* Delete */}
                    <button onClick={() => { if (confirm(`Remove the domain ${d.domain} from this site?`)) deleteDomain.mutate({ siteId, domainId: d.id }) }}
                      title="Remove domain"
                      className="p-1.5 text-gray-400 hover:text-red-600 rounded hover:bg-gray-50">
                      <Trash2 size={14} />
                    </button>
                  </div>
                )}
              </div>

              {/* Redirect info row */}
              {d.redirect_to && (
                <div className="flex items-center gap-2 px-3 py-2 bg-amber-50 border-t border-amber-100 text-xs text-amber-700">
                  <CornerDownRight size={12} />
                  <span className="font-medium">{d.redirect_type ?? 301}</span>
                  <span className="text-amber-400">→</span>
                  <span className="truncate max-w-[300px]">{d.redirect_to}</span>
                  {canManage && (
                    <button onClick={() => handleClearRedirect(d.id)}
                      className="ml-auto p-0.5 text-amber-400 hover:text-amber-700">
                      <X size={12} />
                    </button>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Redirect Config Modal */}
      {redirectModal && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-2xl shadow-xl w-full max-w-md p-6">
            <div className="flex items-center justify-between mb-4">
              <h3 className="font-semibold text-gray-900 flex items-center gap-2">
                <CornerDownRight size={18} className="text-amber-500" /> Redirect Config
              </h3>
              <button onClick={() => setRedirectModal(null)} className="text-gray-400 hover:text-gray-600">
                <X size={18} />
              </button>
            </div>
            <p className="text-sm text-gray-500 mb-4">
              Domain <strong>{redirectModal.domain}</strong> will return a redirect instead of serving the site.
            </p>
            <div className="space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-700 mb-1">Redirect Type</label>
                <select value={redirectModal.redirectType}
                  onChange={e => setRedirectModal(m => m ? { ...m, redirectType: Number(e.target.value) } : m)}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm bg-gray-50">
                  <option value={301}>301 — Permanent redirect</option>
                  <option value={302}>302 — Temporary redirect</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-700 mb-1">Redirect To</label>
                <input type="url" placeholder="https://www.example.com"
                  value={redirectModal.redirectTo}
                  onChange={e => setRedirectModal(m => m ? { ...m, redirectTo: e.target.value } : m)}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm bg-gray-50 focus:outline-none focus:border-blue-500" />
                <p className="text-xs text-gray-400 mt-1">$request_uri will be appended automatically</p>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-5">
              <button onClick={() => setRedirectModal(null)}
                className="px-4 py-2 text-sm border rounded-lg hover:bg-gray-50">Cancel</button>
              <button onClick={handleSaveRedirect}
                disabled={!redirectModal.redirectTo || updateDomain.isPending}
                className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {updateDomain.isPending ? 'Saving...' : 'Save Redirect'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
