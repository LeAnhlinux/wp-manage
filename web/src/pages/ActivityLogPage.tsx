import { useState } from 'react'
import { Activity, Filter, ChevronLeft, ChevronRight, Trash2, Settings2, RefreshCw, Clock } from 'lucide-react'
import {
  useActivityLogs,
  useActivityActions,
  useDeleteActivityLog,
  useCleanupActivityLogs,
} from '@/hooks/useActivityLog'

export default function ActivityLogPage() {
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState<{
    action?: string
    from?: string
    to?: string
  }>({})

  // Cleanup section state
  const [retentionDays, setRetentionDays] = useState(90)
  const [lastCleanupResult, setLastCleanupResult] = useState<{ deleted: number; days: number } | null>(null)
  const [deletingId, setDeletingId] = useState<number | null>(null)

  const { data, isLoading } = useActivityLogs(page, 25, filters)
  const { data: actions } = useActivityActions()
  const deleteLog = useDeleteActivityLog()
  const cleanup = useCleanupActivityLogs()

  const logs = data?.data || []
  const meta = data?.meta

  const handleDelete = async (id: number) => {
    setDeletingId(id)
    try {
      await deleteLog.mutateAsync(id)
    } finally {
      setDeletingId(null)
    }
  }

  const handleCleanup = async () => {
    const result = await cleanup.mutateAsync(retentionDays)
    if (result) {
      setLastCleanupResult({ deleted: result.deleted, days: retentionDays })
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Activity className="text-blue-600" size={28} />
          <h1 className="text-2xl font-bold text-gray-900">Activity Log</h1>
        </div>
      </div>

      {/* Filters */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-4 mb-4">
        <div className="flex items-center gap-2 mb-3">
          <Filter size={16} className="text-gray-500" />
          <span className="text-sm font-medium text-gray-700">Filters</span>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Action</label>
            <select
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm"
              value={filters.action || ''}
              onChange={(e) => {
                setFilters(prev => ({ ...prev, action: e.target.value || undefined }))
                setPage(1)
              }}
            >
              <option value="">All actions</option>
              {actions?.map((a) => (
                <option key={a} value={a}>{a}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">From</label>
            <input
              type="date"
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm"
              value={filters.from || ''}
              onChange={(e) => {
                setFilters(prev => ({ ...prev, from: e.target.value || undefined }))
                setPage(1)
              }}
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">To</label>
            <input
              type="date"
              className="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm"
              value={filters.to || ''}
              onChange={(e) => {
                setFilters(prev => ({ ...prev, to: e.target.value || undefined }))
                setPage(1)
              }}
            />
          </div>
        </div>
      </div>

      {/* Cleanup Section */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-4 mb-6">
        <div className="flex items-center gap-2 mb-3">
          <Settings2 size={16} className="text-gray-500" />
          <span className="text-sm font-medium text-gray-700">Log Cleanup</span>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Delete logs older than</label>
            <div className="flex items-center gap-2">
              <input
                type="number"
                min={1}
                max={3650}
                value={retentionDays}
                onChange={(e) => setRetentionDays(Math.max(1, parseInt(e.target.value) || 90))}
                className="w-24 border border-gray-200 rounded-lg px-3 py-2 text-sm"
              />
              <span className="text-sm text-gray-500">days</span>
            </div>
          </div>
          <button
            onClick={handleCleanup}
            disabled={cleanup.isPending}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 text-white text-sm font-medium rounded-lg hover:bg-red-700 disabled:opacity-50 transition-colors"
          >
            <RefreshCw size={14} className={cleanup.isPending ? 'animate-spin' : ''} />
            {cleanup.isPending ? 'Cleaning...' : 'Clean Now'}
          </button>
          <div className="flex items-center gap-2 text-xs text-gray-400">
            <Clock size={13} />
            <span>Auto-cleanup runs daily at midnight (default: 90 days)</span>
          </div>
        </div>

        {/* Cleanup result feedback */}
        {lastCleanupResult !== null && (
          <div className={`mt-3 text-sm px-3 py-2 rounded-lg ${lastCleanupResult.deleted > 0 ? 'bg-green-50 text-green-700' : 'bg-gray-50 text-gray-600'}`}>
            {lastCleanupResult.deleted > 0
              ? `✓ Deleted ${lastCleanupResult.deleted} log${lastCleanupResult.deleted > 1 ? 's' : ''} older than ${lastCleanupResult.days} days`
              : `No logs older than ${lastCleanupResult.days} days found`
            }
          </div>
        )}
      </div>

      {/* Activity Log Table */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden">
        {isLoading ? (
          <div className="p-8 text-center text-gray-400">Loading activity logs...</div>
        ) : logs.length === 0 ? (
          <div className="p-8 text-center text-gray-400">No activity logs found</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-100 bg-gray-50">
                <th className="text-left text-xs font-medium text-gray-500 px-4 py-3">Time</th>
                <th className="text-left text-xs font-medium text-gray-500 px-4 py-3">Action</th>
                <th className="text-left text-xs font-medium text-gray-500 px-4 py-3">Details</th>
                <th className="text-left text-xs font-medium text-gray-500 px-4 py-3">IP Address</th>
                <th className="w-10 px-4 py-3"></th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} className="border-b border-gray-50 hover:bg-gray-50 group">
                  <td className="px-4 py-3 text-sm text-gray-600 whitespace-nowrap">
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-medium ${
                      log.action.startsWith('DELETE') ? 'bg-red-100 text-red-700' :
                      log.action.startsWith('POST') ? 'bg-green-100 text-green-700' :
                      log.action.startsWith('PUT') ? 'bg-yellow-100 text-yellow-700' :
                      'bg-gray-100 text-gray-700'
                    }`}>
                      {log.action}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600 max-w-sm truncate">
                    {log.details || '—'}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-500 font-mono">
                    {log.ip_address}
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => handleDelete(log.id)}
                      disabled={deletingId === log.id}
                      title="Delete this log"
                      className="opacity-0 group-hover:opacity-100 p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 disabled:opacity-50 transition-all"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {/* Pagination */}
        {meta && meta.total_pages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-100">
            <span className="text-sm text-gray-500">
              Page {meta.page} of {meta.total_pages} ({meta.total} total)
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="p-2 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
              >
                <ChevronLeft size={16} />
              </button>
              <button
                onClick={() => setPage(p => Math.min(meta.total_pages, p + 1))}
                disabled={page >= meta.total_pages}
                className="p-2 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
              >
                <ChevronRight size={16} />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
