import { useState } from 'react'
import { Loader2, Archive, Database, Clock, AlertTriangle, Trash2, Plus, ChevronLeft, ChevronRight, Edit2, X } from 'lucide-react'
import { useSiteBackups, useCreateBackup, useDeleteBackup, useRestoreBackup, useSiteBackupSchedules, useCreateBackupSchedule, useUpdateBackupSchedule, useDeleteBackupSchedule } from '@/hooks/useBackups'
import type { Backup, BackupSchedule } from '@/types'

export const BACKUP_CRON_PRESETS = [
  { label: 'Daily at 3:00 AM', value: '0 3 * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Weekly (Sunday 3am)', value: '0 3 * * 0' },
  { label: 'Monthly (1st 3am)', value: '0 3 1 * *' },
  { label: 'Custom', value: 'custom' },
]

export function formatBytes(bytes: number): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export function BackupPanel({ siteId, domain, canManage }: { siteId: number; domain: string; canManage: boolean }) {
  const [page, setPage] = useState(1)
  const { data: backupsData, isLoading: loadingBackups } = useSiteBackups(siteId, page)
  const { data: schedules = [], isLoading: loadingSchedules } = useSiteBackupSchedules(siteId)
  const createBackup = useCreateBackup()
  const deleteBackup = useDeleteBackup()
  const restoreBackup = useRestoreBackup()
  const createSchedule = useCreateBackupSchedule()
  const updateSchedule = useUpdateBackupSchedule()
  const deleteSchedule = useDeleteBackupSchedule()

  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null)
  const [restoreConfirm, setRestoreConfirm] = useState('')
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Schedule form (create)
  const [showSchedForm, setShowSchedForm] = useState(false)
  const [schedForm, setSchedForm] = useState({
    type: 'full' as 'full' | 'database_only',
    cron_expr: '0 3 * * *',
    retention: 7,
  })
  const [customCron, setCustomCron] = useState(false)

  // Schedule edit modal
  const [editSched, setEditSched] = useState<BackupSchedule | null>(null)
  const [editForm, setEditForm] = useState({
    type: 'full' as 'full' | 'database_only',
    cron_expr: '0 3 * * *',
    retention: 7,
  })
  const [editCustomCron, setEditCustomCron] = useState(false)

  const openEditSched = (sched: BackupSchedule) => {
    setEditSched(sched)
    setEditForm({ type: sched.type, cron_expr: sched.cron_expr, retention: sched.retention })
    setEditCustomCron(!BACKUP_CRON_PRESETS.find(p => p.value === sched.cron_expr))
  }

  const handleUpdateSchedule = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editSched) return
    try {
      await updateSchedule.mutateAsync({ id: editSched.id, ...editForm })
      showMsg('success', 'Schedule updated.')
      setEditSched(null)
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to update schedule')
    }
  }

  const showMsg = (type: 'success' | 'error', text: string) => {
    setMsg({ type, text })
    setTimeout(() => setMsg(null), 5000)
  }

  const handleCreateBackup = async (type: 'full' | 'database_only') => {
    try {
      await createBackup.mutateAsync({ siteId, type })
      showMsg('success', 'Backup started — it will appear in the list once complete.')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to start backup')
    }
  }

  const handleRestore = async () => {
    if (!restoreTarget || restoreConfirm !== domain) return
    try {
      await restoreBackup.mutateAsync({ siteId, backup_id: restoreTarget.id, confirm_domain: domain })
      showMsg('success', 'Restore started successfully')
      setRestoreTarget(null)
      setRestoreConfirm('')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to restore backup')
    }
  }

  const handleCreateSchedule = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createSchedule.mutateAsync({ siteId, ...schedForm })
      setShowSchedForm(false)
      setSchedForm({ type: 'full', cron_expr: '0 3 * * *', retention: 7 })
      setCustomCron(false)
      showMsg('success', 'Backup schedule created')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to create schedule')
    }
  }

  const backups = backupsData?.data ?? []
  const meta = backupsData?.meta
  const totalPages = meta ? Math.ceil(meta.total / meta.per_page) : 1

  return (
    <div className="space-y-6">
      {/* Flash message */}
      {msg && (
        <div className={`p-3 rounded-lg text-sm border ${msg.type === 'success'
          ? 'bg-green-50 text-green-700 border-green-200'
          : 'bg-red-50 text-red-700 border-red-200'}`}>
          {msg.text}
        </div>
      )}

      {/* ── Manual Backup ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Archive size={20} className="text-blue-500" /> Create Backup
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Create an on-demand backup. Files are stored on the server and can be restored at any time.
        </p>
        {canManage ? (
          <div className="flex flex-wrap gap-3">
            <button
              onClick={() => handleCreateBackup('full')}
              disabled={createBackup.isPending}
              className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors">
              {createBackup.isPending
                ? <Loader2 size={16} className="animate-spin" />
                : <Archive size={16} />}
              Full Backup (Files + DB)
            </button>
            <button
              onClick={() => handleCreateBackup('database_only')}
              disabled={createBackup.isPending}
              className="flex items-center gap-2 px-4 py-2.5 bg-gray-100 text-gray-700 text-sm font-medium rounded-lg hover:bg-gray-200 disabled:opacity-50 transition-colors">
              {createBackup.isPending
                ? <Loader2 size={16} className="animate-spin" />
                : <Database size={16} />}
              Database Only
            </button>
          </div>
        ) : (
          <p className="text-sm text-gray-400">You don't have permission to create backups.</p>
        )}
      </div>

      {/* ── Backup History ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">Backup History</h2>

        {loadingBackups ? (
          <div className="flex justify-center py-10">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" />
          </div>
        ) : backups.length === 0 ? (
          <div className="text-center py-10">
            <Archive size={36} className="text-gray-300 mx-auto mb-2" />
            <p className="text-sm text-gray-400">No backups yet. Create one above.</p>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-gray-500 border-b">
                    <th className="pb-2 font-medium">Type</th>
                    <th className="pb-2 font-medium">Status</th>
                    <th className="pb-2 font-medium">Size</th>
                    <th className="pb-2 font-medium">Created</th>
                    <th className="pb-2 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {backups.map(backup => (
                    <tr key={backup.id} className="border-b border-gray-50 hover:bg-gray-50/50">
                      <td className="py-3">
                        <span className={`inline-flex px-2 py-0.5 rounded-md text-xs font-medium ${
                          backup.type === 'full'
                            ? 'bg-blue-100 text-blue-700'
                            : backup.type === 'database_only'
                            ? 'bg-purple-100 text-purple-700'
                            : 'bg-gray-100 text-gray-600'
                        }`}>
                          {backup.type === 'full' ? 'Full' : backup.type === 'database_only' ? 'DB Only' : 'Incremental'}
                        </span>
                      </td>
                      <td className="py-3">
                        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium ${
                          backup.status === 'completed'
                            ? 'bg-green-100 text-green-700'
                            : backup.status === 'running'
                            ? 'bg-amber-100 text-amber-700'
                            : 'bg-red-100 text-red-700'
                        }`}>
                          {backup.status === 'running' && <Loader2 size={10} className="animate-spin" />}
                          {backup.status === 'completed' ? 'Completed'
                            : backup.status === 'running' ? 'Running'
                            : 'Failed'}
                        </span>
                        {backup.error && (
                          <p className="text-xs text-red-500 mt-0.5 max-w-xs truncate" title={backup.error}>
                            {backup.error}
                          </p>
                        )}
                      </td>
                      <td className="py-3 text-gray-600 tabular-nums">{formatBytes(backup.size_bytes)}</td>
                      <td className="py-3 text-gray-500 text-xs">
                        {new Date(backup.created_at).toLocaleString()}
                      </td>
                      <td className="py-3">
                        <div className="flex items-center gap-2 justify-end">
                          {backup.status === 'completed' && canManage && (
                            <button
                              onClick={() => { setRestoreTarget(backup); setRestoreConfirm('') }}
                              className="px-2.5 py-1 text-xs font-medium text-blue-600 border border-blue-200 rounded hover:bg-blue-50 transition-colors">
                              Restore
                            </button>
                          )}
                          {canManage && (
                            <button
                              onClick={() => {
                                if (confirm('Delete this backup? This cannot be undone.'))
                                  deleteBackup.mutate(backup.id)
                              }}
                              className="p-1 text-gray-400 hover:text-red-600 transition-colors">
                              <Trash2 size={14} />
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
            {totalPages > 1 && (
              <div className="flex items-center justify-between mt-4 pt-4 border-t border-gray-100 text-sm text-gray-500">
                <span>Page {page} of {totalPages} · {meta?.total} total backups</span>
                <div className="flex gap-2">
                  <button
                    onClick={() => setPage(p => Math.max(1, p - 1))}
                    disabled={page === 1}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronLeft size={14} />
                  </button>
                  <button
                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                    disabled={page >= totalPages}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronRight size={14} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* ── Scheduled Backups ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Clock size={20} className="text-orange-500" /> Backup Schedules
          </h2>
          {canManage && (
            <button
              onClick={() => setShowSchedForm(v => !v)}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors">
              <Plus size={14} /> Add Schedule
            </button>
          )}
        </div>

        {/* Schedule creation form */}
        {showSchedForm && (
          <form onSubmit={handleCreateSchedule} className="bg-gray-50 rounded-lg p-4 mb-5 space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Backup Type</label>
                <select
                  value={schedForm.type}
                  onChange={e => setSchedForm({ ...schedForm, type: e.target.value as 'full' | 'database_only' })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                  <option value="full">Full (Files + DB)</option>
                  <option value="database_only">Database Only</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Schedule Preset</label>
                <select
                  value={customCron ? 'custom' : (BACKUP_CRON_PRESETS.find(p => p.value === schedForm.cron_expr) ? schedForm.cron_expr : 'custom')}
                  onChange={e => {
                    if (e.target.value === 'custom') {
                      setCustomCron(true)
                    } else {
                      setCustomCron(false)
                      setSchedForm({ ...schedForm, cron_expr: e.target.value })
                    }
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                  {BACKUP_CRON_PRESETS.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Keep Backups (số bản)</label>
                <input
                  type="number" min={1} max={365}
                  value={schedForm.retention}
                  onChange={e => setSchedForm({ ...schedForm, retention: Number(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Cron Expression</label>
              <input
                value={schedForm.cron_expr}
                onChange={e => { setCustomCron(true); setSchedForm({ ...schedForm, cron_expr: e.target.value }) }}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"
                placeholder="0 3 * * *"
                required />
              <p className="text-xs text-gray-400 mt-1">Format: minute hour day-of-month month day-of-week</p>
            </div>
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={createSchedule.isPending}
                className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {createSchedule.isPending ? 'Creating...' : 'Create Schedule'}
              </button>
              <button
                type="button"
                onClick={() => { setShowSchedForm(false); setCustomCron(false) }}
                className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
                Cancel
              </button>
            </div>
          </form>
        )}

        {loadingSchedules ? (
          <div className="flex justify-center py-6">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
          </div>
        ) : schedules.length === 0 ? (
          <div className="text-center py-8">
            <Clock size={30} className="text-gray-300 mx-auto mb-2" />
            <p className="text-sm text-gray-400">No backup schedules configured.</p>
            <p className="text-xs text-gray-400 mt-1">Add a schedule to automate backups.</p>
          </div>
        ) : (
          <div className="space-y-2">
            {schedules.map(sched => (
              <div key={sched.id}
                className="flex items-center justify-between p-3 border border-gray-200 rounded-lg hover:bg-gray-50/50">
                <div className="flex items-center gap-3 min-w-0">
                  <span className={`shrink-0 inline-flex px-2 py-0.5 rounded-md text-xs font-medium ${
                    sched.type === 'full' ? 'bg-blue-100 text-blue-700' : 'bg-purple-100 text-purple-700'
                  }`}>
                    {sched.type === 'full' ? 'Full' : 'DB Only'}
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm font-mono font-medium text-gray-800 truncate">{sched.cron_expr}</p>
                    <p className="text-xs text-gray-500">
                      {BACKUP_CRON_PRESETS.find(p => p.value === sched.cron_expr)?.label ?? sched.cron_expr}
                      {' · '}Giữ {sched.retention} bản
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-3 shrink-0 ml-4">
                  <span className={`text-xs font-medium ${sched.enabled ? 'text-green-600' : 'text-gray-400'}`}>
                    {sched.enabled ? 'Active' : 'Paused'}
                  </span>
                  {canManage && (
                    <button
                      onClick={() => openEditSched(sched)}
                      title="Edit schedule"
                      className="p-1 text-gray-400 hover:text-blue-600 transition-colors">
                      <Edit2 size={14} />
                    </button>
                  )}
                  {canManage && (
                    <button
                      onClick={() => updateSchedule.mutate({ id: sched.id, enabled: !sched.enabled })}
                      className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                        sched.enabled ? 'bg-green-500' : 'bg-gray-300'
                      }`}>
                      <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                        sched.enabled ? 'translate-x-4' : 'translate-x-0.5'
                      }`} />
                    </button>
                  )}
                  {canManage && (
                    <button
                      onClick={() => {
                        if (confirm('Delete this backup schedule?'))
                          deleteSchedule.mutate(sched.id)
                      }}
                      className="p-1 text-gray-400 hover:text-red-600 transition-colors">
                      <Trash2 size={14} />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── Edit Schedule Modal ── */}
      {editSched && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl max-w-lg w-full p-6 shadow-xl">
            <div className="flex items-center justify-between mb-5">
              <h3 className="text-base font-semibold text-gray-900 flex items-center gap-2">
                <Edit2 size={16} className="text-blue-600" /> Sửa lịch backup
              </h3>
              <button onClick={() => setEditSched(null)} className="p-1 text-gray-400 hover:text-gray-600">
                <X size={18} />
              </button>
            </div>
            <form onSubmit={handleUpdateSchedule} className="space-y-4">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Loại backup</label>
                  <select
                    value={editForm.type}
                    onChange={e => setEditForm({ ...editForm, type: e.target.value as 'full' | 'database_only' })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                    <option value="full">Full (Files + DB)</option>
                    <option value="database_only">Database Only</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-600 mb-1">Giữ lại (số bản)</label>
                  <input
                    type="number" min={1} max={365}
                    value={editForm.retention}
                    onChange={e => setEditForm({ ...editForm, retention: Number(e.target.value) })}
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
                  <p className="text-xs text-gray-400 mt-1">Bản cũ hơn sẽ bị xóa tự động</p>
                </div>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Schedule Preset</label>
                <select
                  value={editCustomCron ? 'custom' : (BACKUP_CRON_PRESETS.find(p => p.value === editForm.cron_expr) ? editForm.cron_expr : 'custom')}
                  onChange={e => {
                    if (e.target.value === 'custom') {
                      setEditCustomCron(true)
                    } else {
                      setEditCustomCron(false)
                      setEditForm({ ...editForm, cron_expr: e.target.value })
                    }
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                  {BACKUP_CRON_PRESETS.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Cron Expression</label>
                <input
                  value={editForm.cron_expr}
                  onChange={e => { setEditCustomCron(true); setEditForm({ ...editForm, cron_expr: e.target.value }) }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"
                  placeholder="0 3 * * *"
                  required />
                <p className="text-xs text-gray-400 mt-1">minute hour day-of-month month day-of-week</p>
              </div>
              <div className="flex gap-2 pt-1">
                <button
                  type="submit"
                  disabled={updateSchedule.isPending}
                  className="flex-1 px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
                  {updateSchedule.isPending ? 'Đang lưu...' : 'Lưu thay đổi'}
                </button>
                <button
                  type="button"
                  onClick={() => setEditSched(null)}
                  className="px-4 py-2 text-sm bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200">
                  Hủy
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ── Restore Confirmation Modal ── */}
      {restoreTarget && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl max-w-md w-full p-6 shadow-xl">
            <h3 className="text-lg font-semibold text-gray-900 mb-2 flex items-center gap-2">
              <AlertTriangle size={20} className="text-amber-500" /> Confirm Restore
            </h3>
            <div className="bg-amber-50 border border-amber-200 rounded-lg p-3 mb-4 text-sm text-amber-800">
              This will <strong>overwrite your live site</strong> with the backup from{' '}
              <strong>{new Date(restoreTarget.created_at).toLocaleString()}</strong>.{' '}
              All current files and database data will be replaced. This cannot be undone.
            </div>
            <p className="text-sm text-gray-700 mb-2">
              Type <code className="bg-gray-100 px-1.5 py-0.5 rounded text-sm">{domain}</code> to confirm:
            </p>
            <input
              autoFocus
              value={restoreConfirm}
              onChange={e => setRestoreConfirm(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-red-200 focus:border-red-400"
              placeholder={domain} />
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => { setRestoreTarget(null); setRestoreConfirm('') }}
                className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-lg transition-colors">
                Cancel
              </button>
              <button
                onClick={handleRestore}
                disabled={restoreConfirm !== domain || restoreBackup.isPending}
                className="px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 transition-colors">
                {restoreBackup.isPending ? 'Restoring...' : 'Restore Backup'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
