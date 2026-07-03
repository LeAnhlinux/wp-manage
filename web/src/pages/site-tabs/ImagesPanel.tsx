import { useState } from 'react'
import { Loader2, ImageIcon, Zap, CheckCircle2, X, RotateCcw, Trash2, HardDrive } from 'lucide-react'
import { useImageOptConfig, useUpdateImageOptConfig, useToggleImageOpt, useRunImageOpt, useImageOptResults, useRunningImageOpt, useCancelImageOpt, useToggleNginxWebP, useOriginalsStats, useRevertImages, useDeleteOriginals } from '@/hooks/useImageOpt'
import { formatDate } from '@/lib/utils'
import type { ImageOptResult } from '@/types'

export function ImagesPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: config, isLoading: configLoading } = useImageOptConfig(siteId)
  const toggleOpt = useToggleImageOpt(siteId)
  const updateConfig = useUpdateImageOptConfig(siteId)
  const runOpt = useRunImageOpt(siteId)
  const cancelOpt = useCancelImageOpt(siteId)
  const toggleNginxWebP = useToggleNginxWebP(siteId)
  const { data: resultsData } = useImageOptResults(siteId)
  const runningResult = useRunningImageOpt(siteId)
  const { data: originalsStats, isLoading: statsLoading } = useOriginalsStats(siteId)
  const revertImages = useRevertImages(siteId)
  const deleteOriginals = useDeleteOriginals(siteId)
  const [quality, setQuality] = useState<number | null>(null)

  const results = ((resultsData?.data as unknown) ?? []) as ImageOptResult[]

  // Compute cumulative stats from all completed runs
  const totalImages   = results.filter(r => r.status === 'completed').reduce((s, r) => s + r.total_images, 0)
  const totalOptimized = results.filter(r => r.status === 'completed').reduce((s, r) => s + r.optimized_count, 0)
  const totalSaved    = results.filter(r => r.status === 'completed').reduce((s, r) => s + r.bytes_saved, 0)
  const hasRunning    = results.some(r => r.status === 'running')

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  }

  if (configLoading) return <div className="flex items-center justify-center py-12"><Loader2 size={24} className="animate-spin text-gray-400" /></div>

  const q = quality ?? (config?.quality ?? 80)

  return (
    <div className="space-y-4">
      {/* Header card — enable toggle + stats */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className={`p-2.5 rounded-xl ${config?.enabled ? 'bg-emerald-100' : 'bg-gray-100'}`}>
              <ImageIcon size={20} className={config?.enabled ? 'text-emerald-600' : 'text-gray-400'} />
            </div>
            <div>
              <h3 className="text-base font-semibold text-gray-900">Image Optimization</h3>
              <p className="text-sm text-gray-500">
                {config?.enabled
                  ? 'Auto-compress enabled — images will be compressed on WordPress upload'
                  : 'Disabled — uploaded images will not be auto-compressed'}
              </p>
            </div>
          </div>
          {canManage && config && (
            <button
              onClick={() => toggleOpt.mutate(!config.enabled)}
              disabled={toggleOpt.isPending}
              className={`relative inline-flex h-7 w-12 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${config.enabled ? 'bg-emerald-500' : 'bg-gray-300'}`}>
              {toggleOpt.isPending
                ? <Loader2 size={12} className="animate-spin absolute inset-0 m-auto text-white" />
                : <span className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${config.enabled ? 'translate-x-6' : 'translate-x-1'}`} />
              }
            </button>
          )}
        </div>

        {/* Stats row */}
        {totalImages > 0 && (
          <div className="mt-5 grid grid-cols-3 gap-3">
            {[
              { label: 'Images processed', value: totalImages.toLocaleString(), color: 'text-gray-900' },
              { label: 'Optimized', value: totalOptimized.toLocaleString(), color: 'text-emerald-600' },
              { label: 'Space saved', value: formatBytes(totalSaved), color: 'text-blue-600' },
            ].map(stat => (
              <div key={stat.label} className="bg-gray-50 rounded-xl p-3 text-center">
                <p className={`text-xl font-bold ${stat.color}`}>{stat.value}</p>
                <p className="text-xs text-gray-500 mt-0.5">{stat.label}</p>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Settings card */}
      {canManage && config && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
          <h4 className="text-sm font-semibold text-gray-700 mb-4">Compression Settings</h4>
          <div className="space-y-4">
            {/* WebP toggle */}
            <div className="flex items-center justify-between py-3 border-b border-gray-100">
              <div>
                <p className="text-sm font-medium text-gray-800">Generate WebP files</p>
                <p className="text-xs text-gray-500 mt-0.5">Automatically create a .webp version alongside the original image during optimization</p>
              </div>
              <button onClick={() => updateConfig.mutate({ webp_enabled: !config.webp_enabled })}
                disabled={updateConfig.isPending}
                className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${config.webp_enabled ? 'bg-blue-600' : 'bg-gray-300'}`}>
                <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${config.webp_enabled ? 'translate-x-6' : 'translate-x-1'}`} />
              </button>
            </div>

            {/* Nginx WebP serving toggle */}
            <div className="flex items-center justify-between py-3 border-b border-gray-100">
              <div>
                <p className="text-sm font-medium text-gray-800">Nginx WebP serving</p>
                <p className="text-xs text-gray-500 mt-0.5">
                  WebP-capable browsers automatically receive <code className="bg-gray-100 px-1 rounded text-xs">.webp</code> instead of <code className="bg-gray-100 px-1 rounded text-xs">.jpg/.png</code> — ~40% smaller
                </p>
                {config.nginx_webp && (
                  <span className="inline-flex items-center gap-1 mt-1 text-xs text-emerald-600 font-medium">
                    <CheckCircle2 size={11} /> Active
                  </span>
                )}
              </div>
              <button
                onClick={() => toggleNginxWebP.mutate(!config.nginx_webp)}
                disabled={toggleNginxWebP.isPending}
                className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${config.nginx_webp ? 'bg-emerald-500' : 'bg-gray-300'}`}
              >
                {toggleNginxWebP.isPending
                  ? <Loader2 size={10} className="animate-spin absolute inset-0 m-auto text-white" />
                  : <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${config.nginx_webp ? 'translate-x-6' : 'translate-x-1'}`} />
                }
              </button>
            </div>

            {/* Quality slider */}
            <div className="py-3 border-b border-gray-100">
              <div className="flex items-center justify-between mb-2">
                <div>
                  <p className="text-sm font-medium text-gray-800">Compression Quality</p>
                  <p className="text-xs text-gray-500 mt-0.5">Lower = smaller file, slightly less sharp</p>
                </div>
                <span className="text-sm font-bold text-blue-600 bg-blue-50 px-2.5 py-1 rounded-lg">{q}%</span>
              </div>
              <input type="range" min="30" max="100" value={q}
                onChange={(e) => setQuality(parseInt(e.target.value))}
                onMouseUp={() => { if (quality !== null) { updateConfig.mutate({ quality }); setQuality(null) } }}
                onTouchEnd={() => { if (quality !== null) { updateConfig.mutate({ quality }); setQuality(null) } }}
                className="w-full accent-blue-600" />
              <div className="flex justify-between text-xs text-gray-400 mt-1">
                <span>30% (smallest)</span><span>80% (recommended)</span><span>100% (lossless)</span>
              </div>
            </div>

            {/* Max width */}
            <div className="flex items-center justify-between py-3">
              <div>
                <p className="text-sm font-medium text-gray-800">Max Width</p>
                <p className="text-xs text-gray-500 mt-0.5">Resize image if wider than this value</p>
              </div>
              <select value={config.max_width}
                onChange={(e) => updateConfig.mutate({ max_width: parseInt(e.target.value) })}
                className="px-3 py-1.5 border border-gray-200 rounded-lg text-sm bg-gray-50 focus:bg-white focus:border-blue-500 focus:ring-1 focus:ring-blue-500/20">
                <option value={1280}>1280px</option>
                <option value={1920}>1920px (FHD)</option>
                <option value={2560}>2560px (QHD)</option>
                <option value={3840}>3840px (4K)</option>
              </select>
            </div>
          </div>
        </div>
      )}

      {/* Bulk optimize + history */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-gray-100 flex items-center justify-between">
          <div>
            <h4 className="text-sm font-semibold text-gray-700">Optimization History</h4>
            <p className="text-xs text-gray-500 mt-0.5">Manually optimize all uploaded images</p>
          </div>
          {canManage && (
            <button onClick={() => runOpt.mutate()} disabled={runOpt.isPending || hasRunning}
              className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white text-sm font-semibold rounded-xl hover:bg-blue-700 disabled:opacity-50 transition-colors">
              {(runOpt.isPending || hasRunning) ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
              {hasRunning ? 'Running...' : 'Optimize All Images'}
            </button>
          )}
        </div>

        {/* Live progress banner */}
        {runningResult && (
          <div className="mx-5 mt-3 px-4 py-3 bg-blue-50 border border-blue-200 rounded-xl">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2 text-blue-700 text-sm font-semibold">
                <Loader2 size={14} className="animate-spin" />
                Optimizing images...
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-blue-500">
                  {runningResult.optimized_count + runningResult.skipped_count} / {runningResult.total_images > 0 ? runningResult.total_images : '?'}
                </span>
                {canManage && (
                  <button
                    onClick={() => cancelOpt.mutate()}
                    disabled={cancelOpt.isPending}
                    className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-red-600 bg-red-50 border border-red-200 rounded-lg hover:bg-red-100 disabled:opacity-50 transition-colors"
                  >
                    {cancelOpt.isPending ? <Loader2 size={10} className="animate-spin" /> : <X size={10} />}
                    Cancel
                  </button>
                )}
              </div>
            </div>
            {/* Progress bar */}
            {runningResult.total_images > 0 && (
              <div className="w-full bg-blue-100 rounded-full h-1.5 mb-2">
                <div
                  className="bg-blue-500 h-1.5 rounded-full transition-all duration-500"
                  style={{ width: `${Math.min(100, Math.round(((runningResult.optimized_count + runningResult.skipped_count) / runningResult.total_images) * 100))}%` }}
                />
              </div>
            )}
            <div className="flex gap-4 text-xs text-blue-600">
              <span>✓ Optimized: <strong>{runningResult.optimized_count}</strong></span>
              <span>⏭ Skipped: <strong>{runningResult.skipped_count}</strong></span>
              <span>💾 Saved: <strong>{formatBytes(runningResult.bytes_saved)}</strong></span>
            </div>
          </div>
        )}

        {results.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <ImageIcon size={32} className="mx-auto mb-2 text-gray-200" />
            <p className="text-sm font-medium">No optimization runs yet</p>
            <p className="text-xs text-gray-400 mt-1">Click "Optimize All Images" to get started</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-gray-50/80">
                    <th className="text-left px-5 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Time</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Total</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Optimized</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Skipped</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Saved</th>
                    <th className="text-center px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {results.map((r: ImageOptResult) => (
                    <tr key={r.id} className="hover:bg-gray-50 transition-colors">
                      <td className="px-5 py-3 text-gray-600 text-xs">{formatDate(r.started_at)}</td>
                      <td className="px-4 py-3 text-right font-medium">{r.total_images}</td>
                      <td className="px-4 py-3 text-right text-emerald-600 font-medium">{r.optimized_count}</td>
                      <td className="px-4 py-3 text-right text-gray-400">{r.skipped_count}</td>
                      <td className="px-4 py-3 text-right font-semibold text-blue-600">{formatBytes(r.bytes_saved)}</td>
                      <td className="px-4 py-3 text-center">
                        <span className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full ${
                          r.status === 'completed'  ? 'bg-emerald-100 text-emerald-700' :
                          r.status === 'running'    ? 'bg-blue-100 text-blue-700' :
                          r.status === 'cancelled'  ? 'bg-amber-100 text-amber-700' :
                          'bg-red-100 text-red-700'
                        }`}>
                          {r.status === 'running' && <Loader2 size={10} className="animate-spin" />}
                          {r.status === 'completed' ? 'Completed' : r.status === 'running' ? 'Running' : r.status === 'cancelled' ? 'Cancelled' : 'Failed'}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Mobile cards */}
            <div className="md:hidden divide-y divide-gray-100">
              {results.map((r: ImageOptResult) => (
                <div key={r.id} className="px-4 py-3">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs text-gray-500">{formatDate(r.started_at)}</span>
                    <span className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full ${
                      r.status === 'completed'  ? 'bg-emerald-100 text-emerald-700' :
                      r.status === 'running'    ? 'bg-blue-100 text-blue-700' :
                      r.status === 'cancelled'  ? 'bg-amber-100 text-amber-700' :
                      'bg-red-100 text-red-700'
                    }`}>
                      {r.status === 'running' && <Loader2 size={10} className="animate-spin" />}
                      {r.status === 'completed' ? 'Completed' : r.status === 'running' ? 'Running' : r.status === 'cancelled' ? 'Cancelled' : 'Error'}
                    </span>
                  </div>
                  <div className="flex gap-4 text-xs text-gray-600">
                    <span>{r.total_images} images</span>
                    <span className="text-emerald-600">✓ {r.optimized_count} optimized</span>
                    <span className="text-blue-600 font-medium">{formatBytes(r.bytes_saved)} saved</span>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>

      {/* Originals Backup */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2.5 rounded-xl bg-amber-50">
            <HardDrive size={20} className="text-amber-600" />
          </div>
          <div>
            <h3 className="text-base font-semibold text-gray-900">Original Backups</h3>
            <p className="text-sm text-gray-500">Files backed up before optimization — stored alongside optimized images</p>
          </div>
        </div>

        {statsLoading ? (
          <div className="flex items-center gap-2 text-sm text-gray-400 py-2">
            <Loader2 size={14} className="animate-spin" /> Scanning originals...
          </div>
        ) : originalsStats && originalsStats.count > 0 ? (
          <div className="space-y-4">
            {/* Stats */}
            <div className="flex items-center gap-6 p-4 bg-amber-50 border border-amber-100 rounded-xl">
              <div className="text-center">
                <p className="text-2xl font-bold text-amber-700">{originalsStats.count}</p>
                <p className="text-xs text-amber-600 mt-0.5">Original files</p>
              </div>
              <div className="text-center">
                <p className="text-2xl font-bold text-amber-700">{formatBytes(originalsStats.size_bytes)}</p>
                <p className="text-xs text-amber-600 mt-0.5">Disk space used</p>
              </div>
              <p className="text-xs text-amber-600 ml-auto max-w-xs">
                Originals stored as <code className="font-mono bg-amber-100 px-1 rounded">.wpmanage-orig</code> next to each image
              </p>
            </div>

            {canManage && (
              <div className="flex flex-col sm:flex-row gap-3">
                {/* Revert */}
                <button
                  onClick={() => {
                    if (confirm(`Revert all ${originalsStats.count} images back to originals? Optimized versions and WebP files will be removed.`))
                      revertImages.mutate()
                  }}
                  disabled={revertImages.isPending || deleteOriginals.isPending}
                  className="flex items-center gap-2 px-4 py-2.5 border border-amber-300 text-amber-700 bg-amber-50 hover:bg-amber-100 rounded-xl text-sm font-medium transition-colors disabled:opacity-50"
                >
                  {revertImages.isPending
                    ? <><Loader2 size={14} className="animate-spin" /> Reverting...</>
                    : <><RotateCcw size={14} /> Revert to Originals</>}
                </button>

                {/* Delete originals */}
                <button
                  onClick={() => {
                    if (confirm(`Delete all ${originalsStats.count} backup files? This frees ${formatBytes(originalsStats.size_bytes)} but you won't be able to revert.`))
                      deleteOriginals.mutate()
                  }}
                  disabled={revertImages.isPending || deleteOriginals.isPending}
                  className="flex items-center gap-2 px-4 py-2.5 border border-red-200 text-red-600 bg-red-50 hover:bg-red-100 rounded-xl text-sm font-medium transition-colors disabled:opacity-50"
                >
                  {deleteOriginals.isPending
                    ? <><Loader2 size={14} className="animate-spin" /> Deleting...</>
                    : <><Trash2 size={14} /> Delete Originals (free {formatBytes(originalsStats.size_bytes)})</>}
                </button>
              </div>
            )}

            {revertImages.isSuccess && (
              <p className="text-sm text-emerald-600 flex items-center gap-1.5">
                <CheckCircle2 size={14} /> Reverted {(revertImages.data as any)?.count ?? 0} images to originals
              </p>
            )}
            {deleteOriginals.isSuccess && (
              <p className="text-sm text-emerald-600 flex items-center gap-1.5">
                <CheckCircle2 size={14} /> Deleted {(deleteOriginals.data as any)?.count ?? 0} backup files
              </p>
            )}
          </div>
        ) : (
          <div className="text-center py-6 text-gray-400">
            <HardDrive size={32} className="mx-auto mb-2 text-gray-200" />
            <p className="text-sm">No backup files found</p>
            <p className="text-xs mt-1">Run "Bulk Optimize" to compress existing images — originals will be saved automatically</p>
          </div>
        )}
      </div>
    </div>
  )
}
