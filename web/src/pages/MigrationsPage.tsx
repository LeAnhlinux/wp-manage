import { useState } from 'react'
import { ArrowRightLeft, Play, RotateCcw, XCircle, ChevronLeft, ChevronRight, Eye, Loader2, CheckCircle2, AlertCircle, Clock } from 'lucide-react'
import { useMigrations, useMigration, useMigrationLogs, useStartMigration, useRollbackMigration, useCancelMigration } from '@/hooks/useMigrations'
import { useServers } from '@/hooks/useServers'
import { useSites } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import type { Migration, MigrationLog } from '@/types'

const phaseLabels: Record<string, string> = {
  preparation: 'Preparation',
  initial_sync: 'Initial Sync',
  delta_sync: 'Delta Sync',
  switchover: 'Switchover',
  cleanup: 'Cleanup',
}

const phaseOrder = ['preparation', 'initial_sync', 'delta_sync', 'switchover', 'cleanup']

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    running: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
    rolled_back: 'bg-yellow-100 text-yellow-800',
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
      {status.replace('_', ' ')}
    </span>
  )
}

function ProgressBar({ progress, phase }: { progress: number; phase: string }) {
  return (
    <div className="w-full">
      <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
        <span>{phaseLabels[phase] || phase}</span>
        <span>{progress}%</span>
      </div>
      <div className="w-full bg-gray-200 rounded-full h-2">
        <div
          className="bg-blue-600 h-2 rounded-full transition-all duration-500"
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
        if (status === 'completed') color = 'bg-green-500'
        else if (status === 'failed' && idx === currentIdx) color = 'bg-red-500'
        else if (idx < currentIdx) color = 'bg-green-500'
        else if (idx === currentIdx) color = 'bg-blue-500'
        return (
          <div key={phase} className="flex items-center gap-1">
            <div className={`w-3 h-3 rounded-full ${color}`} title={phaseLabels[phase]} />
            {idx < phaseOrder.length - 1 && <div className={`w-4 h-0.5 ${idx < currentIdx ? 'bg-green-500' : 'bg-gray-300'}`} />}
          </div>
        )
      })}
    </div>
  )
}

function MigrationDetailModal({ migrationId, onClose }: { migrationId: number; onClose: () => void }) {
  const { data: migration } = useMigration(migrationId)
  const { data: logs } = useMigrationLogs(migrationId)

  if (!migration) return null

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] overflow-hidden">
        <div className="p-6 border-b flex items-center justify-between">
          <div>
            <h3 className="text-lg font-semibold">Migration #{migration.id}</h3>
            <p className="text-sm text-gray-500">{migration.site?.domain || `Site #${migration.site_id}`}</p>
          </div>
          <StatusBadge status={migration.status} />
        </div>

        <div className="p-6 space-y-4 overflow-y-auto max-h-[50vh]">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-xs text-gray-500">Source Server</p>
              <p className="text-sm font-medium">{migration.source_server?.name || `#${migration.source_server_id}`}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Target Server</p>
              <p className="text-sm font-medium">{migration.target_server?.name || `#${migration.target_server_id}`}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Started</p>
              <p className="text-sm">{new Date(migration.started_at).toLocaleString()}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Completed</p>
              <p className="text-sm">{migration.completed_at ? new Date(migration.completed_at).toLocaleString() : '—'}</p>
            </div>
          </div>

          <ProgressBar progress={migration.progress} phase={migration.phase} />
          <PhaseTimeline currentPhase={migration.phase} status={migration.status} />

          {migration.error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
              {migration.error}
            </div>
          )}

          {/* Logs */}
          <div>
            <h4 className="text-sm font-medium mb-2">Migration Logs</h4>
            <div className="bg-gray-900 rounded-lg p-3 max-h-48 overflow-y-auto font-mono text-xs">
              {logs && logs.length > 0 ? (
                logs.map((log: MigrationLog) => (
                  <div key={log.id} className={`py-0.5 ${log.level === 'error' ? 'text-red-400' : 'text-green-400'}`}>
                    <span className="text-gray-500">{new Date(log.created_at).toLocaleTimeString()}</span>{' '}
                    <span className="text-blue-400">[{log.phase}]</span>{' '}
                    {log.message}
                  </div>
                ))
              ) : (
                <p className="text-gray-500">No logs yet...</p>
              )}
            </div>
          </div>
        </div>

        <div className="p-4 border-t flex justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">
            Close
          </button>
        </div>
      </div>
    </div>
  )
}

function StartMigrationModal({ onClose }: { onClose: () => void }) {
  const [siteId, setSiteId] = useState<number>(0)
  const [targetServerId, setTargetServerId] = useState<number>(0)
  const [changeDomain, setChangeDomain] = useState('')
  const { data: sitesData } = useSites()
  const { data: serversData } = useServers()
  const startMigration = useStartMigration()

  const sites = sitesData?.data || []
  const servers = serversData?.data || []
  const selectedSite = sites.find(s => s.id === siteId)

  const handleSubmit = () => {
    if (!siteId || !targetServerId) return
    startMigration.mutate(
      { siteId, target_server_id: targetServerId, change_domain: changeDomain || undefined },
      { onSuccess: () => onClose() }
    )
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md">
        <div className="p-6 border-b">
          <h3 className="text-lg font-semibold">Start Migration</h3>
          <p className="text-sm text-gray-500">Migrate a WordPress site to another server</p>
        </div>

        <div className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Source Site</label>
            <select
              value={siteId}
              onChange={e => setSiteId(Number(e.target.value))}
              className="w-full border rounded-lg px-3 py-2 text-sm"
            >
              <option value={0}>Select a site...</option>
              {sites.filter(s => s.status === 'active').map(site => (
                <option key={site.id} value={site.id}>{site.domain} (Server #{site.server_id})</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Target Server</label>
            <select
              value={targetServerId}
              onChange={e => setTargetServerId(Number(e.target.value))}
              className="w-full border rounded-lg px-3 py-2 text-sm"
            >
              <option value={0}>Select target server...</option>
              {servers
                .filter(s => s.status === 'active' && (!selectedSite || s.id !== selectedSite.server_id))
                .map(server => (
                  <option key={server.id} value={server.id}>{server.name} ({server.ip_address})</option>
                ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Change Domain (optional)</label>
            <input
              type="text"
              value={changeDomain}
              onChange={e => setChangeDomain(e.target.value)}
              placeholder="Leave empty to keep current domain"
              className="w-full border rounded-lg px-3 py-2 text-sm"
            />
            <p className="text-xs text-gray-400 mt-1">If set, wp search-replace will update all URLs</p>
          </div>

          {startMigration.isError && (
            <p className="text-sm text-red-600">
              {(startMigration.error as Error).message || 'Failed to start migration'}
            </p>
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!siteId || !targetServerId || startMigration.isPending}
            className="px-4 py-2 text-sm bg-blue-600 text-white hover:bg-blue-700 rounded-lg disabled:opacity-50 flex items-center gap-2"
          >
            {startMigration.isPending ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
            Start Migration
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

  const migrations: Migration[] = data?.data || []
  const meta = data?.meta

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <ArrowRightLeft size={24} />
            Migrations
          </h1>
          <p className="text-gray-500 text-sm mt-1">VPS-to-VPS WordPress site migrations</p>
        </div>
        {can('sites', 'manage') && (
          <button
            onClick={() => setShowStart(true)}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm"
          >
            <Play size={16} />
            New Migration
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading...</div>
      ) : migrations.length === 0 ? (
        <div className="text-center py-12">
          <ArrowRightLeft size={48} className="mx-auto text-gray-300 mb-4" />
          <p className="text-gray-500">No migrations yet</p>
          <p className="text-gray-400 text-sm">Start a migration to move a site between servers</p>
        </div>
      ) : (
        <>
          <div className="bg-white rounded-lg border overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">ID</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Site</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Source → Target</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Progress</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Started</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {migrations.map((m: Migration) => (
                  <tr key={m.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-mono">#{m.id}</td>
                    <td className="px-4 py-3 text-sm font-medium">{m.site?.domain || `Site #${m.site_id}`}</td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {m.source_server?.name || `#${m.source_server_id}`} → {m.target_server?.name || `#${m.target_server_id}`}
                    </td>
                    <td className="px-4 py-3 w-40">
                      <ProgressBar progress={m.progress} phase={m.phase} />
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={m.status} />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">
                      <div className="flex items-center gap-1">
                        <Clock size={12} />
                        {new Date(m.started_at).toLocaleDateString()}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          onClick={() => setViewId(m.id)}
                          className="p-1.5 text-gray-400 hover:text-blue-600 rounded"
                          title="View details"
                        >
                          <Eye size={16} />
                        </button>
                        {m.status === 'running' && can('sites', 'manage') && (
                          <button
                            onClick={() => { if (confirm('Cancel this migration?')) cancel.mutate(m.id) }}
                            className="p-1.5 text-gray-400 hover:text-orange-600 rounded"
                            title="Cancel"
                          >
                            <XCircle size={16} />
                          </button>
                        )}
                        {(m.status === 'completed' || m.status === 'failed') && can('sites', 'manage') && (
                          <button
                            onClick={() => { if (confirm('Rollback this migration?')) rollback.mutate(m.id) }}
                            className="p-1.5 text-gray-400 hover:text-yellow-600 rounded"
                            title="Rollback"
                          >
                            <RotateCcw size={16} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <p className="text-sm text-gray-500">
                Showing page {meta.page} of {meta.total_pages} ({meta.total} total)
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="px-3 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-50"
                >
                  <ChevronLeft size={16} />
                </button>
                <button
                  onClick={() => setPage(p => p + 1)}
                  disabled={page >= meta.total_pages}
                  className="px-3 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-50"
                >
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
