import { useState } from 'react'
import { Loader2, HardDrive, Database, RefreshCw, Clock, CheckCircle2, XCircle } from 'lucide-react'
import { useOptimizeDatabase, useUpdateLogs } from '@/hooks/useWordPress'
import type { DBOptimizeResult } from '@/hooks/useWordPress'
import type { UpdateLog } from '@/types'

function ToggleRow({ label, description, enabled, canManage, onToggle }: {
  label: string; description: string; enabled: boolean; canManage: boolean; onToggle: () => void
}) {
  return (
    <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
      <div>
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-gray-500">{description}</p>
      </div>
      <button
        onClick={onToggle}
        disabled={!canManage}
        className={`w-10 h-6 rounded-full transition-colors relative disabled:opacity-50 ${enabled ? 'bg-blue-600' : 'bg-gray-300'}`}
      >
        <span className={`block w-4 h-4 rounded-full bg-white shadow absolute top-1 transition-transform ${enabled ? 'translate-x-5' : 'translate-x-1'}`} />
      </button>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export function parseDbLog(log: UpdateLog): DBOptimizeResult | null {
  try {
    const p = JSON.parse(log.item_name)
    return {
      status: log.status,
      tables_optimized: p.tables ?? 0,
      space_saved: p.space ?? 0,
      revisions_cleaned: p.revisions ?? 0,
      transients_cleared: p.transients ?? 0,
      error: log.error,
      created_at: log.created_at,
    }
  } catch {
    return null
  }
}

export function DBOptimizeResultCard({ result }: { result: DBOptimizeResult }) {
  const isOk = result.status === 'completed'
  return (
    <div className={`p-4 rounded-lg border ${isOk ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
      <div className="flex items-center gap-2 mb-3">
        {isOk
          ? <CheckCircle2 size={16} className="text-green-600" />
          : <XCircle size={16} className="text-red-600" />}
        <span className={`text-sm font-semibold ${isOk ? 'text-green-800' : 'text-red-800'}`}>
          {isOk ? 'Optimization completed' : 'Optimization failed'}
        </span>
        <span className="ml-auto text-xs text-gray-400">
          {new Date(result.created_at).toLocaleString('vi-VN')}
        </span>
      </div>
      {isOk ? (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div className="bg-white rounded p-2 text-center border border-green-100">
            <p className="text-lg font-bold text-blue-600">{result.tables_optimized}</p>
            <p className="text-xs text-gray-500">Tables optimized</p>
          </div>
          <div className="bg-white rounded p-2 text-center border border-green-100">
            <p className="text-lg font-bold text-green-600">{formatBytes(result.space_saved)}</p>
            <p className="text-xs text-gray-500">Space reclaimed</p>
          </div>
          <div className="bg-white rounded p-2 text-center border border-green-100">
            <p className="text-lg font-bold text-yellow-600">{result.revisions_cleaned}</p>
            <p className="text-xs text-gray-500">Revisions deleted</p>
          </div>
          <div className="bg-white rounded p-2 text-center border border-green-100">
            <p className="text-lg font-bold text-purple-600">{result.transients_cleared}</p>
            <p className="text-xs text-gray-500">Transients cleared</p>
          </div>
        </div>
      ) : (
        <p className="text-sm text-red-700">{result.error || 'Unknown error'}</p>
      )}
    </div>
  )
}

export function DatabasePanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const optimizeDb = useOptimizeDatabase()
  const [logPage, setLogPage] = useState(1)
  const { data: logsData } = useUpdateLogs(siteId, logPage)

  const dbLogs = (logsData?.data || []).filter((l: UpdateLog) => l.update_type === 'db_optimize')
  const latestResult = optimizeDb.data?.data as DBOptimizeResult | undefined

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold">Database Optimization</h2>
            <p className="text-sm text-gray-500">Optimize tables, clean up revisions, and clear expired transients</p>
          </div>
          {canManage && (
            <button
              onClick={() => optimizeDb.mutate(siteId)}
              disabled={optimizeDb.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
            >
              {optimizeDb.isPending ? <Loader2 size={14} className="animate-spin" /> : <HardDrive size={14} />}
              {optimizeDb.isPending ? 'Optimizing...' : 'Optimize Now'}
            </button>
          )}
        </div>

        {/* Pending indicator */}
        {optimizeDb.isPending && (
          <div className="mb-4 p-3 rounded-lg border bg-blue-50 border-blue-200 text-blue-700 text-sm flex items-center gap-2">
            <Loader2 size={14} className="animate-spin shrink-0" />
            Running OPTIMIZE TABLE, cleaning up revisions and transients...
          </div>
        )}

        {/* Latest result (from current session) */}
        {!optimizeDb.isPending && latestResult && (
          <div className="mb-4">
            <DBOptimizeResultCard result={latestResult} />
          </div>
        )}

        {/* What it does — info cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <div className="p-3 bg-blue-50 rounded-lg text-center">
            <Database size={20} className="mx-auto text-blue-500 mb-1" />
            <p className="text-xs font-medium">Table Optimization</p>
            <p className="text-xs text-gray-400">OPTIMIZE TABLE (InnoDB)</p>
          </div>
          <div className="p-3 bg-green-50 rounded-lg text-center">
            <HardDrive size={20} className="mx-auto text-green-500 mb-1" />
            <p className="text-xs font-medium">Space Recovery</p>
            <p className="text-xs text-gray-400">Reclaim fragmented space</p>
          </div>
          <div className="p-3 bg-yellow-50 rounded-lg text-center">
            <RefreshCw size={20} className="mx-auto text-yellow-500 mb-1" />
            <p className="text-xs font-medium">Revision Cleanup</p>
            <p className="text-xs text-gray-400">Keeps the 5 most recent per post</p>
          </div>
          <div className="p-3 bg-purple-50 rounded-lg text-center">
            <Clock size={20} className="mx-auto text-purple-500 mb-1" />
            <p className="text-xs font-medium">Transient Cleanup</p>
            <p className="text-xs text-gray-400">Clear expired WP transients</p>
          </div>
        </div>
      </div>

      {/* Optimization History */}
      {dbLogs.length > 0 && (
        <div className="bg-white rounded-lg border p-6">
          <h3 className="text-sm font-semibold mb-3">Optimization History</h3>
          <div className="space-y-3">
            {dbLogs.map((log: UpdateLog) => {
              const parsed = parseDbLog(log)
              return parsed
                ? <DBOptimizeResultCard key={log.id} result={parsed} />
                : (
                  <div key={log.id} className="flex items-center justify-between p-3 bg-gray-50 rounded border">
                    <p className="text-xs text-gray-500">{new Date(log.created_at).toLocaleString('vi-VN')}</p>
                    <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                      log.status === 'completed' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                    }`}>{log.status}</span>
                  </div>
                )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
