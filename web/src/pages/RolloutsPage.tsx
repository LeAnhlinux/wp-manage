import { useState } from 'react'
import { Rocket, Plus, Play, Pause, Loader2, CheckCircle2, XCircle, AlertTriangle, ChevronLeft, ChevronRight, Package, Trash2 } from 'lucide-react'
import { useReleases, useCreateRelease, useDeleteRelease, useRollouts, useRollout, useCreateRollout, useStartRollout, usePauseRollout } from '@/hooks/useRollouts'
import type { AgentRelease, RolloutPlan, RolloutServer } from '@/types'

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    pending: 'bg-gray-100 text-gray-800',
    running: 'bg-blue-100 text-blue-800',
    updating: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
    paused: 'bg-yellow-100 text-yellow-800',
    rolled_back: 'bg-orange-100 text-orange-800',
  }
  const icons: Record<string, React.ReactNode> = {
    running: <Loader2 size={12} className="animate-spin" />,
    updating: <Loader2 size={12} className="animate-spin" />,
    completed: <CheckCircle2 size={12} />,
    failed: <XCircle size={12} />,
    paused: <Pause size={12} />,
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${styles[status] || 'bg-gray-100 text-gray-800'}`}>
      {icons[status]}
      {status}
    </span>
  )
}

function StrategyBadge({ strategy }: { strategy: string }) {
  const styles: Record<string, string> = {
    canary: 'bg-yellow-100 text-yellow-800',
    rolling: 'bg-blue-100 text-blue-800',
    immediate: 'bg-red-100 text-red-800',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${styles[strategy] || 'bg-gray-100 text-gray-800'}`}>
      {strategy}
    </span>
  )
}

export default function RolloutsPage() {
  const [page, setPage] = useState(1)
  const [showNewRelease, setShowNewRelease] = useState(false)
  const [showNewRollout, setShowNewRollout] = useState(false)
  const [viewRollout, setViewRollout] = useState<number | null>(null)
  const { data: rolloutsData, isLoading: rolloutsLoading } = useRollouts(page)
  const { data: releasesData } = useReleases()

  const rollouts: RolloutPlan[] = rolloutsData?.data || []
  const meta = rolloutsData?.meta
  const releases: AgentRelease[] = releasesData?.data || []

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Rocket size={24} />
            Agent Rollouts
          </h1>
          <p className="text-gray-500 text-sm mt-1">Manage agent releases and rollout strategy</p>
        </div>
        <div className="flex gap-2">
          <button onClick={() => setShowNewRelease(true)}
            className="flex items-center gap-2 px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 text-sm">
            <Package size={16} /> New Release
          </button>
          <button onClick={() => setShowNewRollout(true)}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm">
            <Plus size={16} /> New Rollout
          </button>
        </div>
      </div>

      {/* Releases Summary */}
      {releases.length > 0 && (
        <div className="mb-6 bg-white rounded-lg border p-4">
          <h2 className="text-sm font-semibold mb-3">Available Releases</h2>
          <div className="flex flex-wrap gap-2">
            {releases.slice(0, 5).map(r => (
              <ReleaseChip key={r.id} release={r} />
            ))}
          </div>
        </div>
      )}

      {/* Rollouts Table */}
      {rolloutsLoading ? (
        <div className="text-center py-12 text-gray-500">Loading...</div>
      ) : rollouts.length === 0 ? (
        <div className="text-center py-12">
          <Rocket size={48} className="mx-auto text-gray-300 mb-4" />
          <p className="text-gray-500">No rollouts yet</p>
          <p className="text-gray-400 text-sm">Create a release and start a rollout to update agents</p>
        </div>
      ) : (
        <>
          <div className="bg-white rounded-lg border overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">ID</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Release</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Strategy</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Progress</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Created</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {rollouts.map((plan) => (
                  <tr key={plan.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-mono">#{plan.id}</td>
                    <td className="px-4 py-3 text-sm font-medium">{plan.release?.version || `Release #${plan.release_id}`}</td>
                    <td className="px-4 py-3"><StrategyBadge strategy={plan.strategy} /></td>
                    <td className="px-4 py-3"><StatusBadge status={plan.status} /></td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <div className="w-24 bg-gray-200 rounded-full h-2">
                          <div className="bg-blue-600 h-2 rounded-full" style={{ width: `${plan.total_servers > 0 ? (plan.updated_count / plan.total_servers) * 100 : 0}%` }} />
                        </div>
                        <span className="text-xs text-gray-500">
                          {plan.updated_count}/{plan.total_servers}
                          {plan.failed_count > 0 && <span className="text-red-500"> ({plan.failed_count} failed)</span>}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(plan.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => setViewRollout(plan.id)} className="text-sm text-blue-600 hover:underline">View</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <p className="text-sm text-gray-500">Page {meta.page} of {meta.total_pages}</p>
              <div className="flex gap-2">
                <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1} className="px-3 py-1 text-sm border rounded disabled:opacity-50"><ChevronLeft size={16} /></button>
                <button onClick={() => setPage(p => p + 1)} disabled={page >= meta.total_pages} className="px-3 py-1 text-sm border rounded disabled:opacity-50"><ChevronRight size={16} /></button>
              </div>
            </div>
          )}
        </>
      )}

      {showNewRelease && <NewReleaseModal onClose={() => setShowNewRelease(false)} />}
      {showNewRollout && <NewRolloutModal releases={releases} onClose={() => setShowNewRollout(false)} />}
      {viewRollout && <RolloutDetailModal rolloutId={viewRollout} onClose={() => setViewRollout(null)} />}
    </div>
  )
}

function ReleaseChip({ release }: { release: AgentRelease }) {
  const deleteRelease = useDeleteRelease()
  return (
    <div className="flex items-center gap-2 px-3 py-1.5 bg-gray-50 border rounded-lg text-sm">
      <Package size={14} className="text-gray-400" />
      <span className="font-mono font-medium">{release.version}</span>
      {release.file_size > 0 && (
        <span className="text-xs text-gray-400">{(release.file_size / 1024 / 1024).toFixed(1)} MB</span>
      )}
      <button onClick={() => { if (confirm(`Delete release ${release.version}?`)) deleteRelease.mutate(release.id) }}
        className="p-0.5 text-gray-300 hover:text-red-500" disabled={deleteRelease.isPending}>
        <Trash2 size={12} />
      </button>
    </div>
  )
}

function NewReleaseModal({ onClose }: { onClose: () => void }) {
  const [version, setVersion] = useState('')
  const [downloadUrl, setDownloadUrl] = useState('')
  const [checksum, setChecksum] = useState('')
  const [releaseNotes, setReleaseNotes] = useState('')
  const [fileSize, setFileSize] = useState(0)
  const createRelease = useCreateRelease()

  const handleSubmit = () => {
    if (!version || !downloadUrl || !checksum) return
    createRelease.mutate({
      version, download_url: downloadUrl, checksum, release_notes: releaseNotes, file_size: fileSize,
    }, { onSuccess: () => onClose() })
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md">
        <div className="p-6 border-b">
          <h3 className="text-lg font-semibold">New Agent Release</h3>
        </div>
        <div className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Version *</label>
            <input value={version} onChange={e => setVersion(e.target.value)} placeholder="e.g., 1.1.0" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Download URL *</label>
            <input value={downloadUrl} onChange={e => setDownloadUrl(e.target.value)} placeholder="https://..." className="w-full border rounded-lg px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">SHA256 Checksum *</label>
            <input value={checksum} onChange={e => setChecksum(e.target.value)} placeholder="sha256..." className="w-full border rounded-lg px-3 py-2 text-sm font-mono" />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Release Notes</label>
            <textarea value={releaseNotes} onChange={e => setReleaseNotes(e.target.value)} rows={3} className="w-full border rounded-lg px-3 py-2 text-sm" />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">File Size (bytes)</label>
            <input type="number" value={fileSize} onChange={e => setFileSize(Number(e.target.value))} className="w-full border rounded-lg px-3 py-2 text-sm" />
          </div>
          {createRelease.isError && <p className="text-sm text-red-600">{(createRelease.error as Error).message || 'Failed'}</p>}
        </div>
        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">Cancel</button>
          <button onClick={handleSubmit} disabled={!version || !downloadUrl || !checksum || createRelease.isPending}
            className="px-4 py-2 text-sm bg-blue-600 text-white hover:bg-blue-700 rounded-lg disabled:opacity-50">
            {createRelease.isPending ? 'Creating...' : 'Create Release'}
          </button>
        </div>
      </div>
    </div>
  )
}

function NewRolloutModal({ releases, onClose }: { releases: AgentRelease[]; onClose: () => void }) {
  const [releaseId, setReleaseId] = useState(0)
  const [strategy, setStrategy] = useState<'canary' | 'rolling' | 'immediate'>('rolling')
  const [batchPercent, setBatchPercent] = useState(20)
  const createRollout = useCreateRollout()

  const handleSubmit = () => {
    if (!releaseId) return
    createRollout.mutate({
      release_id: releaseId, strategy, batch_percent: batchPercent,
    }, { onSuccess: () => onClose() })
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md">
        <div className="p-6 border-b">
          <h3 className="text-lg font-semibold">New Rollout</h3>
          <p className="text-sm text-gray-500">Deploy agent update to servers</p>
        </div>
        <div className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Release</label>
            <select value={releaseId} onChange={e => setReleaseId(Number(e.target.value))} className="w-full border rounded-lg px-3 py-2 text-sm">
              <option value={0}>Select a release...</option>
              {releases.map(r => (
                <option key={r.id} value={r.id}>v{r.version}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Strategy</label>
            <div className="grid grid-cols-3 gap-2">
              {(['canary', 'rolling', 'immediate'] as const).map(s => (
                <button key={s} onClick={() => { setStrategy(s); setBatchPercent(s === 'canary' ? 10 : s === 'rolling' ? 20 : 100) }}
                  className={`p-3 border rounded-lg text-sm text-left ${strategy === s ? 'border-blue-600 bg-blue-50' : 'hover:bg-gray-50'}`}>
                  <p className="font-medium capitalize">{s}</p>
                  <p className="text-xs text-gray-500">{s === 'canary' ? '10% first' : s === 'rolling' ? '20% batches' : 'All at once'}</p>
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Batch Size: {batchPercent}%</label>
            <input type="range" min={5} max={100} step={5} value={batchPercent} onChange={e => setBatchPercent(Number(e.target.value))}
              className="w-full" />
          </div>
          {createRollout.isError && <p className="text-sm text-red-600">{(createRollout.error as Error).message || 'Failed'}</p>}
        </div>
        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">Cancel</button>
          <button onClick={handleSubmit} disabled={!releaseId || createRollout.isPending}
            className="px-4 py-2 text-sm bg-blue-600 text-white hover:bg-blue-700 rounded-lg disabled:opacity-50">
            {createRollout.isPending ? 'Creating...' : 'Create Rollout'}
          </button>
        </div>
      </div>
    </div>
  )
}

function RolloutDetailModal({ rolloutId, onClose }: { rolloutId: number; onClose: () => void }) {
  const { data } = useRollout(rolloutId)
  const startRollout = useStartRollout()
  const pauseRollout = usePauseRollout()

  const plan = data?.data
  if (!plan) return null

  const servers: RolloutServer[] = plan.servers || []
  const batches = servers.reduce((acc, s) => {
    acc[s.batch_number] = acc[s.batch_number] || []
    acc[s.batch_number].push(s)
    return acc
  }, {} as Record<number, RolloutServer[]>)

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-3xl max-h-[80vh] overflow-hidden">
        <div className="p-6 border-b flex items-center justify-between">
          <div>
            <h3 className="text-lg font-semibold">Rollout #{plan.id} — v{plan.release?.version}</h3>
            <div className="flex items-center gap-3 mt-1">
              <StrategyBadge strategy={plan.strategy} />
              <StatusBadge status={plan.status} />
              <span className="text-sm text-gray-500">Batch size: {plan.batch_percent}%</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {(plan.status === 'pending' || plan.status === 'paused') && (
              <button onClick={() => startRollout.mutate(plan.id)} disabled={startRollout.isPending}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50">
                <Play size={14} /> {plan.status === 'paused' ? 'Resume' : 'Start'}
              </button>
            )}
            {plan.status === 'running' && (
              <button onClick={() => pauseRollout.mutate(plan.id)} disabled={pauseRollout.isPending}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-yellow-600 text-white rounded-lg hover:bg-yellow-700 disabled:opacity-50">
                <Pause size={14} /> Pause
              </button>
            )}
          </div>
        </div>

        <div className="p-6 overflow-y-auto max-h-[55vh]">
          {/* Progress */}
          <div className="grid grid-cols-3 gap-4 mb-6">
            <div className="text-center p-3 bg-blue-50 rounded-lg">
              <p className="text-2xl font-bold">{plan.total_servers}</p>
              <p className="text-xs text-gray-500">Total Servers</p>
            </div>
            <div className="text-center p-3 bg-green-50 rounded-lg">
              <p className="text-2xl font-bold text-green-600">{plan.updated_count}</p>
              <p className="text-xs text-gray-500">Updated</p>
            </div>
            <div className={`text-center p-3 rounded-lg ${plan.failed_count > 0 ? 'bg-red-50' : 'bg-gray-50'}`}>
              <p className={`text-2xl font-bold ${plan.failed_count > 0 ? 'text-red-600' : ''}`}>{plan.failed_count}</p>
              <p className="text-xs text-gray-500">Failed</p>
            </div>
          </div>

          {/* Server Batches */}
          {Object.entries(batches).sort(([a], [b]) => Number(a) - Number(b)).map(([batch, srvs]) => (
            <div key={batch} className="mb-4">
              <h4 className="text-sm font-medium mb-2 text-gray-700">Batch {batch}</h4>
              <div className="space-y-1">
                {srvs.map((rs) => (
                  <div key={rs.id} className="flex items-center justify-between p-2 bg-gray-50 rounded text-sm">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{rs.server?.name || `Server #${rs.server_id}`}</span>
                      <span className="text-gray-400 font-mono text-xs">{rs.server?.ip_address}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      {rs.old_version && (
                        <span className="text-xs text-gray-400 font-mono">{rs.old_version} → {rs.new_version}</span>
                      )}
                      <StatusBadge status={rs.status} />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}

          {plan.status === 'failed' && (
            <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700 flex items-center gap-2">
              <AlertTriangle size={16} />
              Rollout failed. {plan.strategy === 'canary' ? 'Canary check detected failure — rollout stopped.' : 'Some servers failed to update.'}
            </div>
          )}
        </div>

        <div className="p-4 border-t flex justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">Close</button>
        </div>
      </div>
    </div>
  )
}
