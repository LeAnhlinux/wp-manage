import { useState, useEffect } from 'react'
import { Loader2, Layers, AlertCircle, CheckCircle } from 'lucide-react'
import { useStagingSites, useCreateStaging, usePushStaging, useDeleteStaging, useCheckDNSProvider } from '@/hooks/useWordPress'
import { formatDate } from '@/lib/utils'
import type { StagingSite } from '@/types'

function CreateStagingModal({ defaultDomain, onConfirm, onClose }: {
  defaultDomain: string
  onConfirm: (domain: string) => void
  onClose: () => void
}) {
  const [domain, setDomain] = useState(defaultDomain)
  const [debouncedDomain, setDebouncedDomain] = useState(defaultDomain)

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedDomain(domain), 500)
    return () => clearTimeout(timer)
  }, [domain])

  const { data: dnsCheck, isLoading: dnsLoading } = useCheckDNSProvider(debouncedDomain)
  const dnsData = dnsCheck?.data

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md mx-4 p-6">
        <h3 className="text-lg font-semibold mb-4">Create Staging Environment</h3>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Staging Domain</label>
            <input
              type="text"
              value={domain}
              onChange={e => setDomain(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="staging-example.com"
            />
            <p className="text-xs text-gray-400 mt-1">Default: staging-{'{production domain}'}</p>
          </div>

          {debouncedDomain.length > 3 && (
            <div className={`flex items-start gap-2 p-3 rounded-lg text-sm ${
              dnsLoading ? 'bg-gray-50 text-gray-500' :
              dnsData?.found ? 'bg-green-50 text-green-700' : 'bg-yellow-50 text-yellow-700'
            }`}>
              {dnsLoading ? (
                <><Loader2 size={14} className="animate-spin mt-0.5 shrink-0" /><span>Checking DNS providers…</span></>
              ) : dnsData?.found ? (
                <><CheckCircle size={14} className="mt-0.5 shrink-0" /><div><p className="font-medium">DNS auto-configure via {dnsData.provider_name}</p><p className="text-xs mt-0.5">An A record for <strong>{debouncedDomain}</strong> will be created automatically.</p></div></>
              ) : (
                <><AlertCircle size={14} className="mt-0.5 shrink-0" /><div><p className="font-medium">DNS not managed</p><p className="text-xs mt-0.5">No DNS provider found for this domain. You'll need to configure DNS manually.</p></div></>
              )}
            </div>
          )}
        </div>

        <div className="flex gap-3 mt-6">
          <button
            onClick={onClose}
            className="flex-1 px-4 py-2 text-sm border border-gray-300 rounded-lg hover:bg-gray-50"
          >Cancel</button>
          <button
            onClick={() => onConfirm(domain)}
            disabled={!domain.trim()}
            className="flex-1 px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >Create Staging</button>
        </div>
      </div>
    </div>
  )
}

export function StagingPanel({ siteId, canManage, siteDomain }: { siteId: number; canManage: boolean; siteDomain: string }) {
  const { data: stagingData, isLoading } = useStagingSites(siteId)
  const createStaging = useCreateStaging()
  const pushStaging = usePushStaging()
  const deleteStaging = useDeleteStaging()
  const [showModal, setShowModal] = useState(false)

  const stagings: StagingSite[] = stagingData?.data || []
  const activeStagings = stagings.filter(s => s.status !== 'deleted')

  // Collect any error from mutations
  const mutationError = createStaging.error || pushStaging.error || deleteStaging.error
  const errorMsg = mutationError
    ? ((mutationError as any)?.response?.data?.error || (mutationError as Error)?.message || 'Unknown error')
    : null

  return (
    <div className="space-y-4">
      {showModal && (
        <CreateStagingModal
          defaultDomain={`staging-${siteDomain}`}
          onConfirm={(domain) => {
            createStaging.mutate({ siteId, stagingDomain: domain })
            setShowModal(false)
          }}
          onClose={() => setShowModal(false)}
        />
      )}

      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Staging Environments</h2>
          <p className="text-sm text-gray-500">Clone your production site for testing</p>
        </div>
        {canManage && activeStagings.length === 0 && (
          <button
            onClick={() => setShowModal(true)}
            disabled={createStaging.isPending}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
          >
            {createStaging.isPending ? <Loader2 size={14} className="animate-spin" /> : <Layers size={14} />}
            {createStaging.isPending ? 'Creating...' : 'Create Staging'}
          </button>
        )}
      </div>

      {/* Error banner */}
      {errorMsg && (
        <div className="flex items-start gap-2 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">
          <AlertCircle size={16} className="mt-0.5 shrink-0" />
          <div>
            <p className="font-medium">Operation failed</p>
            <p className="text-red-600">{errorMsg}</p>
          </div>
        </div>
      )}

      {/* Loading banners for in-progress operations */}
      {createStaging.isPending && (
        <div className="flex items-center gap-2 p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-700">
          <Loader2 size={14} className="animate-spin shrink-0" />
          <span>Creating staging environment… this may take a minute.</span>
        </div>
      )}
      {pushStaging.isPending && (
        <div className="flex items-center gap-2 p-3 bg-yellow-50 border border-yellow-200 rounded-lg text-sm text-yellow-700">
          <Loader2 size={14} className="animate-spin shrink-0" />
          <span>Pushing staging to production… do not close this page.</span>
        </div>
      )}
      {deleteStaging.isPending && (
        <div className="flex items-center gap-2 p-3 bg-gray-50 border border-gray-200 rounded-lg text-sm text-gray-700">
          <Loader2 size={14} className="animate-spin shrink-0" />
          <span>Deleting staging environment…</span>
        </div>
      )}

      {isLoading ? (
        <div className="text-center py-8 text-gray-500">Loading...</div>
      ) : activeStagings.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-lg border">
          <Layers size={40} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500">No staging environment</p>
          <p className="text-sm text-gray-400">Create a staging copy to test changes safely</p>
        </div>
      ) : (
        <div className="space-y-3">
          {activeStagings.map((staging) => (
            <div key={staging.id} className="bg-white rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">{staging.staging_site?.domain || `Staging #${staging.staging_site_id}`}</p>
                  <p className="text-sm text-gray-500">Created {formatDate(staging.created_at)}</p>
                  {staging.status === 'creating' && (
                    <p className="text-xs text-blue-500 mt-1 flex items-center gap-1">
                      <Loader2 size={10} className="animate-spin" /> Setting up environment…
                    </p>
                  )}
                  {staging.status === 'pushing' && (
                    <p className="text-xs text-yellow-600 mt-1 flex items-center gap-1">
                      <Loader2 size={10} className="animate-spin" /> Pushing to production…
                    </p>
                  )}
                  {staging.status === 'error' && (
                    <p className="text-xs text-red-500 mt-1 flex items-center gap-1">
                      <AlertCircle size={10} /> Failed — check agent logs
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                    staging.status === 'active'   ? 'bg-green-100 text-green-800' :
                    staging.status === 'creating' ? 'bg-blue-100 text-blue-800' :
                    staging.status === 'pushing'  ? 'bg-yellow-100 text-yellow-800' :
                    staging.status === 'error'    ? 'bg-red-100 text-red-800' :
                    'bg-gray-100 text-gray-800'
                  }`}>{staging.status}</span>
                  {canManage && staging.status === 'active' && (
                    <>
                      <button
                        onClick={() => { if (confirm('Push staging to production? This will overwrite production files and database.')) pushStaging.mutate(staging.id) }}
                        disabled={pushStaging.isPending}
                        className="px-3 py-1 text-xs bg-yellow-50 text-yellow-700 rounded hover:bg-yellow-100 disabled:opacity-50"
                      >Push to Production</button>
                      <button
                        onClick={() => { if (confirm('Delete this staging environment?')) deleteStaging.mutate(staging.id) }}
                        disabled={deleteStaging.isPending}
                        className="px-3 py-1 text-xs bg-red-50 text-red-700 rounded hover:bg-red-100 disabled:opacity-50"
                      >Delete</button>
                    </>
                  )}
                  {canManage && staging.status === 'error' && (
                    <button
                      onClick={() => { if (confirm('Delete this failed staging environment?')) deleteStaging.mutate(staging.id) }}
                      disabled={deleteStaging.isPending}
                      className="px-3 py-1 text-xs bg-red-50 text-red-700 rounded hover:bg-red-100 disabled:opacity-50"
                    >Delete</button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
