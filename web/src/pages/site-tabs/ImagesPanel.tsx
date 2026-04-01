import { useState } from 'react'
import { Loader2, ImageIcon, Zap, CheckCircle2, X } from 'lucide-react'
import { useImageOptConfig, useUpdateImageOptConfig, useToggleImageOpt, useRunImageOpt, useImageOptResults, useRunningImageOpt, useCancelImageOpt, useToggleNginxWebP } from '@/hooks/useImageOpt'
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
                  ? 'Auto-compress bật — ảnh sẽ được nén khi upload lên WordPress'
                  : 'Tắt — ảnh upload sẽ không được tự động nén'}
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
              { label: 'Tổng ảnh đã xử lý', value: totalImages.toLocaleString(), color: 'text-gray-900' },
              { label: 'Đã tối ưu', value: totalOptimized.toLocaleString(), color: 'text-emerald-600' },
              { label: 'Dung lượng tiết kiệm', value: formatBytes(totalSaved), color: 'text-blue-600' },
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
          <h4 className="text-sm font-semibold text-gray-700 mb-4">Cài đặt nén ảnh</h4>
          <div className="space-y-4">
            {/* WebP toggle */}
            <div className="flex items-center justify-between py-3 border-b border-gray-100">
              <div>
                <p className="text-sm font-medium text-gray-800">Tạo file WebP</p>
                <p className="text-xs text-gray-500 mt-0.5">Tự động tạo phiên bản .webp bên cạnh ảnh gốc khi optimize</p>
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
                <p className="text-sm font-medium text-gray-800">Nginx tự serve WebP</p>
                <p className="text-xs text-gray-500 mt-0.5">
                  Browser hỗ trợ WebP → tự nhận <code className="bg-gray-100 px-1 rounded text-xs">.webp</code> thay vì <code className="bg-gray-100 px-1 rounded text-xs">.jpg/.png</code> — giảm ~40% dung lượng
                </p>
                {config.nginx_webp && (
                  <span className="inline-flex items-center gap-1 mt-1 text-xs text-emerald-600 font-medium">
                    <CheckCircle2 size={11} /> Đang hoạt động
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
                  <p className="text-sm font-medium text-gray-800">Chất lượng nén</p>
                  <p className="text-xs text-gray-500 mt-0.5">Thấp hơn = file nhỏ hơn, ảnh mờ hơn</p>
                </div>
                <span className="text-sm font-bold text-blue-600 bg-blue-50 px-2.5 py-1 rounded-lg">{q}%</span>
              </div>
              <input type="range" min="30" max="100" value={q}
                onChange={(e) => setQuality(parseInt(e.target.value))}
                onMouseUp={() => { if (quality !== null) { updateConfig.mutate({ quality }); setQuality(null) } }}
                onTouchEnd={() => { if (quality !== null) { updateConfig.mutate({ quality }); setQuality(null) } }}
                className="w-full accent-blue-600" />
              <div className="flex justify-between text-xs text-gray-400 mt-1">
                <span>30% (nhỏ nhất)</span><span>80% (khuyến nghị)</span><span>100% (lossless)</span>
              </div>
            </div>

            {/* Max width */}
            <div className="flex items-center justify-between py-3">
              <div>
                <p className="text-sm font-medium text-gray-800">Chiều rộng tối đa</p>
                <p className="text-xs text-gray-500 mt-0.5">Resize ảnh nếu lớn hơn giá trị này</p>
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
            <h4 className="text-sm font-semibold text-gray-700">Lịch sử tối ưu</h4>
            <p className="text-xs text-gray-500 mt-0.5">Optimize thủ công tất cả ảnh đã upload</p>
          </div>
          {canManage && (
            <button onClick={() => runOpt.mutate()} disabled={runOpt.isPending || hasRunning}
              className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white text-sm font-semibold rounded-xl hover:bg-blue-700 disabled:opacity-50 transition-colors">
              {(runOpt.isPending || hasRunning) ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
              {hasRunning ? 'Đang chạy...' : 'Optimize tất cả ảnh'}
            </button>
          )}
        </div>

        {/* Live progress banner */}
        {runningResult && (
          <div className="mx-5 mt-3 px-4 py-3 bg-blue-50 border border-blue-200 rounded-xl">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2 text-blue-700 text-sm font-semibold">
                <Loader2 size={14} className="animate-spin" />
                Đang tối ưu ảnh...
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
                    Hủy
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
              <span>✓ Tối ưu: <strong>{runningResult.optimized_count}</strong></span>
              <span>⏭ Bỏ qua: <strong>{runningResult.skipped_count}</strong></span>
              <span>💾 Tiết kiệm: <strong>{formatBytes(runningResult.bytes_saved)}</strong></span>
            </div>
          </div>
        )}

        {results.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <ImageIcon size={32} className="mx-auto mb-2 text-gray-200" />
            <p className="text-sm font-medium">Chưa có lần optimize nào</p>
            <p className="text-xs text-gray-400 mt-1">Bấm "Optimize tất cả ảnh" để bắt đầu</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-gray-50/80">
                    <th className="text-left px-5 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Thời gian</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Tổng</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Tối ưu</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Bỏ qua</th>
                    <th className="text-right px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Tiết kiệm</th>
                    <th className="text-center px-4 py-3 font-medium text-gray-500 text-xs uppercase tracking-wide">Trạng thái</th>
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
                          {r.status === 'completed' ? 'Hoàn tất' : r.status === 'running' ? 'Đang chạy' : r.status === 'cancelled' ? 'Đã hủy' : 'Lỗi'}
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
                      {r.status === 'completed' ? 'Hoàn tất' : r.status === 'running' ? 'Đang chạy' : r.status === 'cancelled' ? 'Đã hủy' : 'Lỗi'}
                    </span>
                  </div>
                  <div className="flex gap-4 text-xs text-gray-600">
                    <span>{r.total_images} ảnh</span>
                    <span className="text-emerald-600">✓ {r.optimized_count}</span>
                    <span className="text-blue-600 font-medium">{formatBytes(r.bytes_saved)} tiết kiệm</span>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
