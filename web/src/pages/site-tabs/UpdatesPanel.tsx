import { useState } from 'react'
import { Loader2, RefreshCw, AlertCircle, CheckCircle2, XCircle, ChevronLeft, ChevronRight } from 'lucide-react'
import { useAutoUpdateConfig, useUpdateAutoUpdateConfig, useCheckUpdates, useUpdateLogs, useOptimizeDatabase } from '@/hooks/useWordPress'
import type { CheckUpdatesResult } from '@/hooks/useWordPress'
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

export function UpdatesPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const [logPage, setLogPage] = useState(1)
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)
  const { data: configData } = useAutoUpdateConfig(siteId)
  const updateConfig = useUpdateAutoUpdateConfig()
  const checkUpdates = useCheckUpdates()
  const { data: logsData } = useUpdateLogs(siteId, logPage)
  const optimizeDb = useOptimizeDatabase()

  const config = configData?.data
  const logs: UpdateLog[] = logsData?.data || []
  const meta = logsData?.meta
  const checkResult = checkUpdates.data?.data as CheckUpdatesResult | undefined

  const toggleConfig = (field: string, value: boolean) => {
    updateConfig.mutate({ siteId, [field]: value } as any)
  }

  const handleCheckUpdates = () => {
    checkUpdates.mutate(siteId, {
      onSuccess: () => setCheckedAt(new Date()),
    })
  }

  // Count available updates from result
  const availableCount = checkResult
    ? (checkResult.core ? 1 : 0) + (checkResult.plugins?.length ?? 0) + (checkResult.themes?.length ?? 0)
    : 0

  return (
    <div className="space-y-6">
      {/* Auto-Update Config */}
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold">Auto-Update Settings</h2>
            <p className="text-sm text-gray-500">Configure automatic updates for WordPress components</p>
          </div>
          {canManage && (
            <button
              onClick={handleCheckUpdates}
              disabled={checkUpdates.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
            >
              {checkUpdates.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              {checkUpdates.isPending ? 'Scanning...' : 'Check for Updates'}
            </button>
          )}
        </div>

        {/* Check result banner */}
        {!checkUpdates.isPending && checkedAt && (
          <div className={`mb-4 p-3 rounded-lg border text-sm flex items-start gap-2 ${
            availableCount > 0 ? 'bg-yellow-50 border-yellow-200 text-yellow-800' : 'bg-green-50 border-green-200 text-green-800'
          }`}>
            {availableCount > 0 ? <AlertCircle size={16} className="mt-0.5 shrink-0" /> : <CheckCircle2 size={16} className="mt-0.5 shrink-0" />}
            <div className="flex-1">
              {availableCount === 0 ? (
                <span>Everything is up to date.</span>
              ) : (
                <div className="space-y-1">
                  <span className="font-medium">{availableCount} update{availableCount > 1 ? 's' : ''} available:</span>
                  {checkResult?.core && (
                    <div>• WordPress Core: {checkResult.core.from_version} → {checkResult.core.to_version}</div>
                  )}
                  {(checkResult?.plugins?.length ?? 0) > 0 && (
                    <div>• {checkResult!.plugins!.length} plugin(s) need updating</div>
                  )}
                  {(checkResult?.themes?.length ?? 0) > 0 && (
                    <div>• {checkResult!.themes!.length} theme(s) need updating</div>
                  )}
                </div>
              )}
              <div className="mt-1 text-xs opacity-70">
                Checked at {checkedAt.toLocaleTimeString('vi-VN')}
              </div>
            </div>
          </div>
        )}

        {checkUpdates.isPending && (
          <div className="mb-4 p-3 rounded-lg border bg-blue-50 border-blue-200 text-blue-700 text-sm flex items-center gap-2">
            <Loader2 size={14} className="animate-spin shrink-0" />
            Checking WordPress core, plugins, and themes — this can take 1–2 minutes...
          </div>
        )}

        {config && (
          <div className="grid grid-cols-2 gap-4">
            <ToggleRow label="WordPress Core Updates" description="Automatically update WordPress core"
              enabled={config.core_updates} canManage={canManage}
              onToggle={() => toggleConfig('core_updates', !config.core_updates)} />
            <ToggleRow label="Plugin Updates" description="Automatically update all plugins"
              enabled={config.plugin_updates} canManage={canManage}
              onToggle={() => toggleConfig('plugin_updates', !config.plugin_updates)} />
            <ToggleRow label="Theme Updates" description="Automatically update all themes"
              enabled={config.theme_updates} canManage={canManage}
              onToggle={() => toggleConfig('theme_updates', !config.theme_updates)} />
            <ToggleRow label="Backup Before Update" description="Create backup before any auto-update"
              enabled={config.backup_before} canManage={canManage}
              onToggle={() => toggleConfig('backup_before', !config.backup_before)} />
          </div>
        )}
      </div>

      {/* Update Logs */}
      <div className="bg-white rounded-lg border p-6">
        <h2 className="text-lg font-semibold mb-4">Update History</h2>
        {logs.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-4">No update history yet. Enable Auto-Update or click "Check for Updates" to scan.</p>
        ) : (
          <>
            <table className="w-full">
              <thead className="border-b">
                <tr>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Name</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Version</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Date</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {logs.map((log) => (
                  <tr key={log.id} className="hover:bg-gray-50">
                    <td className="px-3 py-2 text-sm capitalize">{log.update_type}</td>
                    <td className="px-3 py-2 text-sm font-medium">{log.item_name}</td>
                    <td className="px-3 py-2 text-sm font-mono text-gray-600">
                      {log.from_version && log.to_version ? `${log.from_version} → ${log.to_version}` : '-'}
                    </td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
                        log.status === 'available' ? 'bg-blue-100 text-blue-800' :
                        log.status === 'updated' || log.status === 'success' || log.status === 'completed' ? 'bg-green-100 text-green-800' :
                        log.status === 'failed' ? 'bg-red-100 text-red-800' : 'bg-gray-100 text-gray-800'
                      }`}>
                        {log.status === 'completed' || log.status === 'success' || log.status === 'updated' ? <CheckCircle2 size={10} /> :
                         log.status === 'failed' ? <XCircle size={10} /> : null}
                        {log.status}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-sm text-gray-500">{new Date(log.created_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {meta && meta.total_pages > 1 && (
              <div className="flex items-center justify-between mt-4">
                <p className="text-sm text-gray-500">Page {meta.page} of {meta.total_pages}</p>
                <div className="flex gap-2">
                  <button onClick={() => setLogPage(p => Math.max(1, p - 1))} disabled={logPage <= 1} className="px-2 py-1 text-sm border rounded disabled:opacity-50"><ChevronLeft size={14} /></button>
                  <button onClick={() => setLogPage(p => p + 1)} disabled={logPage >= meta.total_pages} className="px-2 py-1 text-sm border rounded disabled:opacity-50"><ChevronRight size={14} /></button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
