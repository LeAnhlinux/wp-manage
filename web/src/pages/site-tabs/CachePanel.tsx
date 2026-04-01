import { useState } from 'react'
import { Loader2, Zap, RefreshCw, Settings, CheckCircle2, XCircle, Code, Check } from 'lucide-react'
import { useCacheConfig, useToggleCache, usePurgeCache } from '@/hooks/useCache'
import api from '@/lib/api'
import type { APIResponse } from '@/types'

export function CachePanel({ siteId, canManage, domain }: { siteId: number; canManage: boolean; domain?: string }) {
  const { data: config, isLoading } = useCacheConfig(siteId)
  const toggleCache = useToggleCache(siteId)
  const purgeCache = usePurgeCache(siteId)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [showVhost, setShowVhost] = useState(false)
  const [vhostContent, setVhostContent] = useState('')
  const [loadingVhost, setLoadingVhost] = useState(false)
  const [savingVhost, setSavingVhost] = useState(false)

  const handleToggle = async (enabled: boolean) => {
    setMsg(null)
    try {
      await toggleCache.mutateAsync(enabled)
      setMsg({ type: 'success', text: `Cache ${enabled ? 'enabled' : 'disabled'} successfully` })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || err?.message || 'Failed to toggle cache' })
    }
  }

  const handlePurge = async () => {
    setMsg(null)
    try {
      await purgeCache.mutateAsync()
      setMsg({ type: 'success', text: 'Cache purged successfully' })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || err?.message || 'Failed to purge cache' })
    }
  }

  const loadVhost = async () => {
    setLoadingVhost(true)
    setMsg(null)
    try {
      const { data } = await api.get<APIResponse<{ content: string; file_path: string }>>(`/sites/${siteId}/nginx/vhost`)
      setVhostContent(data.data?.content || '')
      setShowVhost(true)
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to load vhost config' })
    } finally {
      setLoadingVhost(false)
    }
  }

  const saveVhost = async () => {
    setSavingVhost(true)
    setMsg(null)
    try {
      await api.put(`/sites/${siteId}/nginx/vhost`, { content: vhostContent })
      setMsg({ type: 'success', text: 'Vhost config saved & Nginx reloaded' })
    } catch (err: any) {
      setMsg({ type: 'error', text: err?.response?.data?.error || 'Failed to save vhost config' })
    } finally {
      setSavingVhost(false)
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={24} className="animate-spin text-gray-400" />
      </div>
    )
  }

  const isEnabled = config?.enabled ?? false

  return (
    <div className="space-y-6">
      {/* Status Messages */}
      {msg && (
        <div className={`p-3 rounded-lg border ${msg.type === 'success' ? 'bg-green-50 border-green-200' : 'bg-red-50 border-red-200'}`}>
          <p className={`text-sm flex items-center gap-2 ${msg.type === 'success' ? 'text-green-700' : 'text-red-700'}`}>
            {msg.type === 'success' ? <CheckCircle2 size={16} /> : <XCircle size={16} />}
            {msg.text}
          </p>
        </div>
      )}

      {/* FastCGI Cache */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
              <Zap size={20} className="text-yellow-500" /> Nginx FastCGI Cache
            </h2>
            <p className="text-sm text-gray-500 mt-1">Full-page caching via Nginx FastCGI cache for maximum performance</p>
          </div>
          <div className="flex items-center gap-3">
            {canManage && (
              <button
                onClick={handlePurge}
                disabled={purgeCache.isPending || toggleCache.isPending || !isEnabled}
                className="flex items-center gap-1.5 px-3 py-2 text-sm border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50"
              >
                {purgeCache.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                {purgeCache.isPending ? 'Purging...' : 'Purge Cache'}
              </button>
            )}
            {canManage && (
              <button
                onClick={() => handleToggle(!isEnabled)}
                disabled={toggleCache.isPending || purgeCache.isPending}
                className={`relative inline-flex h-7 w-12 items-center rounded-full transition-colors disabled:opacity-50 ${
                  toggleCache.isPending ? 'bg-yellow-400 animate-pulse' : isEnabled ? 'bg-green-500' : 'bg-gray-300'
                }`}
              >
                {toggleCache.isPending ? (
                  <span className="absolute inset-0 flex items-center justify-center">
                    <Loader2 size={14} className="animate-spin text-white" />
                  </span>
                ) : (
                  <span className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${isEnabled ? 'translate-x-6' : 'translate-x-1'}`} />
                )}
              </button>
            )}
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className={`p-4 rounded-lg ${isEnabled ? 'bg-green-50 border border-green-200' : 'bg-gray-50'}`}>
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Status</p>
            <p className={`text-lg font-semibold ${isEnabled ? 'text-green-600' : 'text-gray-400'}`}>
              {toggleCache.isPending ? 'Processing...' : isEnabled ? 'Enabled' : 'Disabled'}
            </p>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Cache Duration</p>
            <p className="text-lg font-semibold text-gray-900">{config?.cache_time || '60m'}</p>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Excludes</p>
            <p className="text-sm text-gray-600">{config?.excludes || 'wp-admin, wp-login'}</p>
          </div>
        </div>
      </div>

      {/* Nginx Vhost Editor */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
              <Settings size={20} className="text-blue-500" /> Nginx Vhost Config
            </h2>
            <p className="text-xs text-gray-400 font-mono mt-1">
              /etc/nginx/sites-available/{domain || '<domain>'}.conf
            </p>
          </div>
          {canManage && !showVhost && (
            <button
              onClick={loadVhost}
              disabled={loadingVhost}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
            >
              {loadingVhost ? <Loader2 size={14} className="animate-spin" /> : <Code size={14} />}
              {loadingVhost ? 'Loading...' : 'Edit Vhost'}
            </button>
          )}
        </div>

        {showVhost && (
          <div className="space-y-3">
            <textarea
              value={vhostContent}
              onChange={(e) => setVhostContent(e.target.value)}
              className="w-full h-96 px-3 py-2 text-sm font-mono bg-gray-900 text-green-400 rounded-lg border border-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
              spellCheck={false}
            />
            <div className="flex items-center justify-between">
              <p className="text-xs text-gray-400">
                Changes are validated with nginx -t before applying. Auto-rollback on failure.
              </p>
              <div className="flex gap-2">
                <button onClick={() => setShowVhost(false)}
                  className="px-4 py-2 text-sm text-gray-600 bg-gray-100 rounded-lg hover:bg-gray-200">
                  Cancel
                </button>
                <button onClick={loadVhost} disabled={loadingVhost}
                  className="px-4 py-2 text-sm text-gray-600 bg-gray-100 rounded-lg hover:bg-gray-200 disabled:opacity-50">
                  {loadingVhost ? 'Reloading...' : 'Reload'}
                </button>
                <button onClick={saveVhost} disabled={savingVhost}
                  className="flex items-center gap-1.5 px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                  {savingVhost ? <Loader2 size={14} className="animate-spin" /> : <Check size={14} />}
                  {savingVhost ? 'Saving...' : 'Save & Reload Nginx'}
                </button>
              </div>
            </div>
          </div>
        )}

        {!showVhost && (
          <div className="bg-gray-50 rounded-lg p-4">
            <p className="text-sm text-gray-500">
              Edit the Nginx virtual host configuration for this site. Advanced users can customize proxy settings,
              SSL configuration, location blocks, and more.
            </p>
          </div>
        )}
      </div>

      <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
        <p className="text-sm text-blue-800">
          <strong>How it works:</strong> FastCGI cache stores rendered pages as static files in Nginx, bypassing PHP entirely for cached pages.
          Logged-in users and POST requests are automatically excluded.
        </p>
      </div>
    </div>
  )
}
