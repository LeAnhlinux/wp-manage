import { useState, useEffect, useRef } from 'react'
import { HardDrive, Plus, RotateCcw, Trash2, Clock, Database, Archive, FileArchive, AlertCircle, CheckCircle2, Loader2, X } from 'lucide-react'
import { useBackups, useCreateBackup, useDeleteBackup, useRestoreBackupByID } from '@/hooks/useBackups'
import { useSites } from '@/hooks/useSites'
import { useServers } from '@/hooks/useServers'
import { usePermissions } from '@/hooks/usePermissions'
import { useQueryClient } from '@tanstack/react-query'
import type { Backup } from '@/types'

interface RestoreStatus {
  domain: string
  backupId: number
  status: 'pending' | 'success' | 'error'
  message?: string
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString()
}

function BackupTypeBadge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    full: 'bg-blue-100 text-blue-700',
    incremental: 'bg-purple-100 text-purple-700',
    database_only: 'bg-amber-100 text-amber-700',
  }
  const labels: Record<string, string> = {
    full: 'Full',
    incremental: 'Incremental',
    database_only: 'Database',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${styles[type] || 'bg-gray-100 text-gray-700'}`}>
      {labels[type] || type}
    </span>
  )
}

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return <span className="flex items-center gap-1 text-green-600 text-sm"><CheckCircle2 size={14} /> Completed</span>
    case 'running':
      return <span className="flex items-center gap-1 text-blue-600 text-sm"><Loader2 size={14} className="animate-spin" /> Running</span>
    case 'failed':
      return <span className="flex items-center gap-1 text-red-600 text-sm"><AlertCircle size={14} /> Failed</span>
    default:
      return <span className="text-gray-500 text-sm">{status}</span>
  }
}

function BackupTypeIcon({ type }: { type: string }) {
  switch (type) {
    case 'full': return <Archive size={16} className="text-blue-500" />
    case 'incremental': return <FileArchive size={16} className="text-purple-500" />
    case 'database_only': return <Database size={16} className="text-amber-500" />
    default: return <HardDrive size={16} />
  }
}

export default function BackupsPage() {
  const [page, setPage] = useState(1)
  const [showCreate, setShowCreate] = useState(false)
  const [restoreBackup, setRestoreBackup] = useState<Backup | null>(null)
  const [confirmDomain, setConfirmDomain] = useState('')
  const [selectedServerId, setSelectedServerId] = useState<number>(0)
  const [restoreStatus, setRestoreStatus] = useState<RestoreStatus | null>(null)
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const queryClient = useQueryClient()
  const { data: backupData, isLoading, refetch: refetchBackups } = useBackups(page)
  const { data: sitesData, refetch: refetchSites } = useSites()
  const { data: serversData } = useServers()
  const { can } = usePermissions()

  const createBackup = useCreateBackup()
  const deleteBackup = useDeleteBackup()
  // Use the new ID-based restore — works for both normal and orphaned backups
  const restoreBackupMut = useRestoreBackupByID()

  // Poll after task is queued: detect when backup.site_id becomes non-null (= site record created)
  const startPolling = (backupId: number, domain: string) => {
    if (pollIntervalRef.current) clearInterval(pollIntervalRef.current)
    let elapsed = 0
    const MAX_WAIT_MS = 5 * 60 * 1000 // 5 minutes max

    pollIntervalRef.current = setInterval(async () => {
      elapsed += 4000
      try {
        await refetchBackups()
        await queryClient.invalidateQueries({ queryKey: ['backups'] })
        // Check if the backup now has a site_id (site record created by TaskComplete)
        const updated = queryClient.getQueryData<{ data: Backup[] }>(['backups', page])
        const b = updated?.data?.find(b => b.id === backupId)
        if (b && b.site_id !== null && b.site_id !== undefined) {
          clearInterval(pollIntervalRef.current!)
          pollIntervalRef.current = null
          // Also refresh sites list so new site shows up immediately
          refetchSites()
          queryClient.invalidateQueries({ queryKey: ['sites'] })
          setRestoreStatus({ domain, backupId, status: 'success' })
          // Auto-dismiss success banner after 8s
          setTimeout(() => setRestoreStatus(s => s?.backupId === backupId ? null : s), 8000)
          return
        }
      } catch (_) {
        // ignore transient fetch errors, keep polling
      }
      if (elapsed >= MAX_WAIT_MS) {
        clearInterval(pollIntervalRef.current!)
        pollIntervalRef.current = null
        setRestoreStatus({ domain, backupId, status: 'error', message: 'Timed out after 5 minutes. Check agent logs.' })
      }
    }, 4000)
  }

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current)
    }
  }, [])

  // Create backup form state
  const [createSiteId, setCreateSiteId] = useState<number>(0)
  const [createType, setCreateType] = useState<'full' | 'incremental' | 'database_only'>('full')

  const handleCreate = () => {
    if (!createSiteId) return
    createBackup.mutate(
      { siteId: createSiteId, type: createType },
      {
        onSuccess: () => {
          setShowCreate(false)
          setCreateSiteId(0)
          setCreateType('full')
        },
      }
    )
  }

  const handleRestore = () => {
    if (!restoreBackup || !confirmDomain) return
    const backupId = restoreBackup.id
    const domain = restoreBackup.domain || `backup-${backupId}`
    restoreBackupMut.mutate(
      { backupId, confirmDomain, serverId: selectedServerId || undefined },
      {
        onSuccess: () => {
          setRestoreBackup(null)
          setConfirmDomain('')
          setSelectedServerId(0)
          // Show "in-progress" banner and start polling
          setRestoreStatus({ domain, backupId, status: 'pending' })
          startPolling(backupId, domain)
        },
        onError: (err: unknown) => {
          const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || String(err)
          setRestoreStatus({ domain, backupId, status: 'error', message: msg })
        },
      }
    )
  }

  const backups = backupData?.data || []
  const meta = backupData?.meta
  const sites = sitesData?.data || []
  const servers = serversData?.data || []
  const canManage = can('backups', 'create')

  // Domain to confirm is always backup.domain (captured at backup time, works for orphaned too)
  const restoreDomain = restoreBackup?.domain ?? ''
  const isOrphaned = restoreBackup ? restoreBackup.site_id === null : false
  // Legacy orphaned backups (created before server_id was stored) need the user to pick a server
  const needsServerSelection = isOrphaned && (restoreBackup?.server_id === 0 || !restoreBackup?.server_id)

  return (
    <div className="space-y-6">
      {/* Restore Status Banner */}
      {restoreStatus && (
        <div className={`flex items-start gap-3 p-4 rounded-xl border ${
          restoreStatus.status === 'pending'
            ? 'bg-blue-50 border-blue-200 text-blue-800'
            : restoreStatus.status === 'success'
            ? 'bg-green-50 border-green-200 text-green-800'
            : 'bg-red-50 border-red-200 text-red-800'
        }`}>
          <div className="mt-0.5 shrink-0">
            {restoreStatus.status === 'pending' && <Loader2 size={18} className="animate-spin text-blue-600" />}
            {restoreStatus.status === 'success' && <CheckCircle2 size={18} className="text-green-600" />}
            {restoreStatus.status === 'error' && <AlertCircle size={18} className="text-red-600" />}
          </div>
          <div className="flex-1 min-w-0">
            {restoreStatus.status === 'pending' && (
              <>
                <p className="font-medium">Đang restore <code className="bg-blue-100 px-1 rounded">{restoreStatus.domain}</code>...</p>
                <p className="text-sm mt-0.5 text-blue-700">
                  Agent đang xử lý trên server — có thể mất 1-3 phút. Site sẽ xuất hiện trong <strong>Sites</strong> khi hoàn thành.
                </p>
              </>
            )}
            {restoreStatus.status === 'success' && (
              <>
                <p className="font-medium">✅ Restore hoàn thành — <code className="bg-green-100 px-1 rounded">{restoreStatus.domain}</code></p>
                <p className="text-sm mt-0.5 text-green-700">
                  Site đã được recreate và đang active. Vào <strong>Sites</strong> để kiểm tra.
                </p>
              </>
            )}
            {restoreStatus.status === 'error' && (
              <>
                <p className="font-medium">❌ Restore thất bại — <code className="bg-red-100 px-1 rounded">{restoreStatus.domain}</code></p>
                {restoreStatus.message && (
                  <p className="text-sm mt-0.5 text-red-700">{restoreStatus.message}</p>
                )}
              </>
            )}
          </div>
          <button
            onClick={() => {
              setRestoreStatus(null)
              if (pollIntervalRef.current) { clearInterval(pollIntervalRef.current); pollIntervalRef.current = null }
            }}
            className="shrink-0 text-current opacity-60 hover:opacity-100"
          >
            <X size={16} />
          </button>
        </div>
      )}

      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <HardDrive size={28} /> Backups
          </h1>
          <p className="text-gray-500 mt-1">Manage backups across all your sites</p>
        </div>
        {canManage && (
          <button
            onClick={() => setShowCreate(true)}
            className="flex items-center gap-2 bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 transition"
          >
            <Plus size={18} /> Create Backup
          </button>
        )}
      </div>

      {/* Create Backup Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl p-6 w-full max-w-md">
            <h2 className="text-lg font-semibold mb-4">Create Backup</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Site</label>
                <select
                  value={createSiteId}
                  onChange={(e) => setCreateSiteId(Number(e.target.value))}
                  className="w-full border rounded-lg px-3 py-2"
                >
                  <option value={0}>Select a site...</option>
                  {sites.filter(s => s.status === 'active').map(s => (
                    <option key={s.id} value={s.id}>{s.domain}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Backup Type</label>
                <select
                  value={createType}
                  onChange={(e) => setCreateType(e.target.value as typeof createType)}
                  className="w-full border rounded-lg px-3 py-2"
                >
                  <option value="full">Full (Files + Database + Config)</option>
                  <option value="database_only">Database Only</option>
                  <option value="incremental">Incremental (Last 24h changes)</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button onClick={() => setShowCreate(false)} className="px-4 py-2 border rounded-lg hover:bg-gray-50">
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={!createSiteId || createBackup.isPending}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
              >
                {createBackup.isPending ? 'Creating...' : 'Create Backup'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Restore Confirmation Modal */}
      {restoreBackup && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl p-6 w-full max-w-md">
            <h2 className="text-lg font-semibold mb-2 text-amber-600 flex items-center gap-2">
              <AlertCircle size={20} /> Restore Backup
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              This will overwrite the current site data. Type the domain name to confirm.
            </p>

            {/* Orphaned backup warning */}
            {isOrphaned && (
              <div className="bg-amber-50 border border-amber-200 rounded-lg p-3 mb-4 text-sm text-amber-700">
                <p className="font-medium mb-1">⚠️ Original site no longer exists in the panel</p>
                <p>The site was deleted but this backup was preserved. Restore will <strong>recreate the site</strong> (Nginx vhost, PHP-FPM pool, database user) from the backup, then restore all files and data.</p>
              </div>
            )}

            <div className="bg-gray-50 rounded-lg p-3 mb-4 text-sm">
              <p><span className="font-medium">Backup:</span> {restoreBackup.type} — {formatDate(restoreBackup.created_at)}</p>
              <p><span className="font-medium">Size:</span> {formatBytes(restoreBackup.size_bytes)}</p>
              <p><span className="font-medium">Domain:</span> {restoreDomain}</p>
              {isOrphaned && <p className="text-amber-600 font-medium mt-1">⚠ Orphaned — site was deleted</p>}
            </div>
            {/* Server selector — only shown for legacy orphaned backups missing server_id */}
            {needsServerSelection && (
              <div className="mb-4">
                <label className="block text-sm font-medium mb-1">
                  Select Target Server <span className="text-red-500">*</span>
                </label>
                <p className="text-xs text-gray-500 mb-1">
                  This backup was created before server tracking was added. Choose which server to restore to.
                </p>
                <select
                  value={selectedServerId}
                  onChange={(e) => setSelectedServerId(Number(e.target.value))}
                  className="w-full border rounded-lg px-3 py-2"
                >
                  <option value={0}>Select a server...</option>
                  {servers.map(s => (
                    <option key={s.id} value={s.id}>{s.name} ({s.ip_address})</option>
                  ))}
                </select>
              </div>
            )}

            <div>
              <label className="block text-sm font-medium mb-1">
                Type <code className="text-red-600 bg-red-50 px-1 rounded">{restoreDomain}</code> to confirm
              </label>
              <input
                type="text"
                value={confirmDomain}
                onChange={(e) => setConfirmDomain(e.target.value)}
                placeholder={restoreDomain}
                className="w-full border rounded-lg px-3 py-2"
                autoFocus
              />
            </div>
            {/* Error from API */}
            {restoreBackupMut.isError && (
              <div className="mt-3 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">
                <span className="font-medium">Error: </span>
                {(restoreBackupMut.error as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Restore failed. Check agent logs.'}
              </div>
            )}

            <div className="flex justify-end gap-2 mt-4">
              <button
                onClick={() => { setRestoreBackup(null); setConfirmDomain(''); setSelectedServerId(0); restoreBackupMut.reset() }}
                className="px-4 py-2 border rounded-lg hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={handleRestore}
                disabled={
                  confirmDomain !== restoreDomain ||
                  restoreBackupMut.isPending ||
                  (needsServerSelection && !selectedServerId)
                }
                className="px-4 py-2 bg-amber-600 text-white rounded-lg hover:bg-amber-700 disabled:opacity-50"
              >
                {restoreBackupMut.isPending ? 'Restoring...' : isOrphaned ? 'Recreate & Restore' : 'Restore'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Backups Table */}
      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 size={24} className="animate-spin text-blue-600" />
        </div>
      ) : backups.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-xl border">
          <HardDrive size={48} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500">No backups yet</p>
          {canManage && (
            <button
              onClick={() => setShowCreate(true)}
              className="mt-3 text-blue-600 hover:underline text-sm"
            >
              Create your first backup
            </button>
          )}
        </div>
      ) : (
        <div className="bg-white rounded-xl border overflow-hidden">
          <table className="w-full">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-3 text-sm font-medium text-gray-500">Domain / Site</th>
                <th className="text-left px-4 py-3 text-sm font-medium text-gray-500">Type</th>
                <th className="text-left px-4 py-3 text-sm font-medium text-gray-500">Status</th>
                <th className="text-left px-4 py-3 text-sm font-medium text-gray-500">Size</th>
                <th className="text-left px-4 py-3 text-sm font-medium text-gray-500">Created</th>
                <th className="text-right px-4 py-3 text-sm font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {backups.map((backup) => (
                <tr key={backup.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <BackupTypeIcon type={backup.type} />
                      <div>
                        {/* Always use backup.domain — never backup.site?.domain */}
                        <span className="font-medium text-sm">
                          {backup.domain || `Backup #${backup.id}`}
                        </span>
                        {backup.site_id === null && (
                          <span className="ml-2 px-1.5 py-0.5 rounded text-xs bg-amber-100 text-amber-700">
                            Site deleted
                          </span>
                        )}
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <BackupTypeBadge type={backup.type} />
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={backup.status} />
                    {backup.error && (
                      <p className="text-xs text-red-500 mt-1 max-w-xs truncate" title={backup.error}>{backup.error}</p>
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    {backup.size_bytes > 0 ? formatBytes(backup.size_bytes) : '—'}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">
                    <div className="flex items-center gap-1">
                      <Clock size={14} />
                      {formatDate(backup.created_at)}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {canManage && backup.status === 'completed' && (
                        <button
                          onClick={() => { setRestoreBackup(backup); setConfirmDomain('') }}
                          className="p-1.5 text-amber-600 hover:bg-amber-50 rounded"
                          title={backup.site_id === null ? 'Recreate site from backup' : 'Restore from this backup'}
                        >
                          <RotateCcw size={16} />
                        </button>
                      )}
                      {canManage && (
                        <button
                          onClick={() => {
                            if (confirm('Delete this backup?')) {
                              deleteBackup.mutate(backup.id)
                            }
                          }}
                          className="p-1.5 text-red-500 hover:bg-red-50 rounded"
                          title="Delete backup"
                        >
                          <Trash2 size={16} />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {/* Pagination */}
          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 border-t bg-gray-50">
              <p className="text-sm text-gray-500">
                Page {meta.page} of {meta.total_pages} ({meta.total} backups)
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="px-3 py-1 border rounded text-sm disabled:opacity-50"
                >
                  Previous
                </button>
                <button
                  onClick={() => setPage(p => p + 1)}
                  disabled={page >= meta.total_pages}
                  className="px-3 py-1 border rounded text-sm disabled:opacity-50"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
