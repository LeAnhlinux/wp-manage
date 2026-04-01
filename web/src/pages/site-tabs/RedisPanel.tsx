import { Loader2, Database } from 'lucide-react'
import { useRedisConfig, useToggleRedis } from '@/hooks/useRedis'

export function RedisPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: config, isLoading } = useRedisConfig(siteId)
  const toggleRedis = useToggleRedis()

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
        <Database size={20} className="text-red-500" /> Redis Object Cache
      </h2>
      <p className="text-sm text-gray-500 mb-5">
        Enable or disable the Redis object cache plugin for this site. For stats, flush, and server-wide config go to the <strong>Server → Redis</strong> tab.
      </p>

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : config ? (
        <div className="space-y-3">
          <div className="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
            <div>
              <p className="text-sm font-medium text-gray-900">Status</p>
              <p className={`text-sm font-medium ${config.enabled ? 'text-green-600' : 'text-gray-400'}`}>
                {config.enabled ? '● Enabled' : '○ Disabled'}
              </p>
            </div>
            {canManage && (
              <button
                onClick={() => toggleRedis.mutate({ siteId, enabled: !config.enabled })}
                disabled={toggleRedis.isPending}
                className={`flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium disabled:opacity-50 transition-colors ${
                  config.enabled
                    ? 'bg-red-100 text-red-700 hover:bg-red-200'
                    : 'bg-green-100 text-green-700 hover:bg-green-200'
                }`}>
                {toggleRedis.isPending ? <Loader2 size={14} className="animate-spin" /> : null}
                {toggleRedis.isPending ? 'Saving…' : config.enabled ? 'Disable' : 'Enable'}
              </button>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="p-3 bg-gray-50 rounded-lg">
              <p className="text-xs text-gray-500 uppercase font-medium mb-0.5">Max Memory</p>
              <p className="text-base font-semibold text-gray-900">{config.max_memory_mb} MB</p>
            </div>
            <div className="p-3 bg-gray-50 rounded-lg">
              <p className="text-xs text-gray-500 uppercase font-medium mb-0.5">Key Prefix</p>
              <p className="text-base font-semibold text-gray-900 font-mono">{config.prefix || 'wp_'}</p>
            </div>
          </div>
        </div>
      ) : (
        <p className="text-sm text-gray-400 text-center py-8">No Redis configuration found.</p>
      )}
    </div>
  )
}
