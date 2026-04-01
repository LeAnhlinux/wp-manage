import { useState } from 'react'
import { Loader2, RefreshCw, Globe } from 'lucide-react'
import { useLatestPerformance, usePerformanceMetrics, useRunPageSpeed } from '@/hooks/usePerformance'
import type { PerformanceMetric } from '@/types'

export function PerformancePanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const [device, setDevice] = useState<'mobile' | 'desktop'>('mobile')
  const [days, setDays] = useState(30)
  const { data: latest } = useLatestPerformance(siteId)
  const { data: historyData } = usePerformanceMetrics(siteId, device, days)
  const runAudit = useRunPageSpeed(siteId)

  const currentMetric = latest?.[device]
  const history: PerformanceMetric[] = historyData?.data || []

  const getScoreColor = (score: number) => {
    if (score >= 90) return 'text-green-600 bg-green-50 border-green-200'
    if (score >= 50) return 'text-yellow-600 bg-yellow-50 border-yellow-200'
    return 'text-red-600 bg-red-50 border-red-200'
  }

  const getScoreRing = (score: number) => {
    if (score >= 90) return 'border-green-500'
    if (score >= 50) return 'border-yellow-500'
    return 'border-red-500'
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold">Performance Metrics</h2>
            <p className="text-sm text-gray-500">Google PageSpeed Insights — Core Web Vitals</p>
          </div>
          <div className="flex items-center gap-3">
            {/* Device toggle */}
            <div className="flex items-center bg-gray-100 rounded-lg p-0.5">
              <button onClick={() => setDevice('mobile')}
                className={`px-3 py-1.5 text-xs font-medium rounded-md transition ${device === 'mobile' ? 'bg-white shadow text-gray-900' : 'text-gray-500'}`}>
                Mobile
              </button>
              <button onClick={() => setDevice('desktop')}
                className={`px-3 py-1.5 text-xs font-medium rounded-md transition ${device === 'desktop' ? 'bg-white shadow text-gray-900' : 'text-gray-500'}`}>
                Desktop
              </button>
            </div>
            {canManage && (
              <button onClick={() => runAudit.mutate({ device_type: device })}
                disabled={runAudit.isPending}
                className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50">
                {runAudit.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                Run Audit
              </button>
            )}
          </div>
        </div>

        {runAudit.isPending && (
          <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-700 flex items-center gap-2">
            <Loader2 size={14} className="animate-spin" /> Running PageSpeed audit... This may take up to 60 seconds.
          </div>
        )}

        {/* Score Gauges */}
        {currentMetric ? (
          <div className="grid grid-cols-2 lg:grid-cols-6 gap-4">
            <div className={`p-4 rounded-lg border text-center ${getScoreColor(currentMetric.performance_score)}`}>
              <div className={`w-16 h-16 rounded-full border-4 flex items-center justify-center mx-auto mb-2 ${getScoreRing(currentMetric.performance_score)}`}>
                <span className="text-xl font-bold">{Math.round(currentMetric.performance_score)}</span>
              </div>
              <p className="text-xs font-medium">Performance</p>
            </div>
            <div className={`p-4 rounded-lg border text-center ${getScoreColor(currentMetric.accessibility_score)}`}>
              <div className={`w-16 h-16 rounded-full border-4 flex items-center justify-center mx-auto mb-2 ${getScoreRing(currentMetric.accessibility_score)}`}>
                <span className="text-xl font-bold">{Math.round(currentMetric.accessibility_score)}</span>
              </div>
              <p className="text-xs font-medium">Accessibility</p>
            </div>
            <div className="p-4 rounded-lg border bg-gray-50 text-center">
              <p className="text-2xl font-bold text-gray-900">{(currentMetric.lcp_ms / 1000).toFixed(1)}s</p>
              <p className="text-xs font-medium text-gray-500 mt-1">LCP</p>
              <p className="text-[10px] text-gray-400">Largest Contentful Paint</p>
            </div>
            <div className="p-4 rounded-lg border bg-gray-50 text-center">
              <p className="text-2xl font-bold text-gray-900">{currentMetric.cls.toFixed(3)}</p>
              <p className="text-xs font-medium text-gray-500 mt-1">CLS</p>
              <p className="text-[10px] text-gray-400">Cumulative Layout Shift</p>
            </div>
            <div className="p-4 rounded-lg border bg-gray-50 text-center">
              <p className="text-2xl font-bold text-gray-900">{Math.round(currentMetric.fid_ms)}ms</p>
              <p className="text-xs font-medium text-gray-500 mt-1">FID</p>
              <p className="text-[10px] text-gray-400">Max Potential FID</p>
            </div>
            <div className="p-4 rounded-lg border bg-gray-50 text-center">
              <p className="text-2xl font-bold text-gray-900">{Math.round(currentMetric.ttfb_ms)}ms</p>
              <p className="text-xs font-medium text-gray-500 mt-1">TTFB</p>
              <p className="text-[10px] text-gray-400">Time to First Byte</p>
            </div>
          </div>
        ) : (
          <div className="text-center py-8 text-gray-500">
            <Globe size={40} className="mx-auto mb-3 text-gray-300" />
            <p className="font-medium">No performance data yet</p>
            <p className="text-sm">Click "Run Audit" to measure your site's performance</p>
          </div>
        )}
      </div>

      {/* History */}
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-semibold">Measurement History ({device})</h3>
          <div className="flex items-center gap-2">
            {[7, 30, 90].map(d => (
              <button key={d} onClick={() => setDays(d)}
                className={`px-2 py-1 text-xs rounded ${days === d ? 'bg-blue-100 text-blue-700 font-medium' : 'text-gray-500 hover:bg-gray-100'}`}>
                {d}d
              </button>
            ))}
          </div>
        </div>
        {history.length === 0 ? (
          <p className="text-sm text-gray-500 text-center py-4">No measurements in this period</p>
        ) : (
          <table className="w-full">
            <thead className="border-b">
              <tr>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Date</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Perf</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Access</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">LCP</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">CLS</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">FID</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">TTFB</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {history.map((m) => (
                <tr key={m.id} className="hover:bg-gray-50">
                  <td className="px-3 py-2 text-sm text-gray-600">{new Date(m.measured_at).toLocaleString()}</td>
                  <td className="px-3 py-2">
                    <span className={`inline-block px-2 py-0.5 rounded text-xs font-bold ${
                      m.performance_score >= 90 ? 'bg-green-100 text-green-800' :
                      m.performance_score >= 50 ? 'bg-yellow-100 text-yellow-800' : 'bg-red-100 text-red-800'
                    }`}>{Math.round(m.performance_score)}</span>
                  </td>
                  <td className="px-3 py-2">
                    <span className={`inline-block px-2 py-0.5 rounded text-xs font-bold ${
                      m.accessibility_score >= 90 ? 'bg-green-100 text-green-800' :
                      m.accessibility_score >= 50 ? 'bg-yellow-100 text-yellow-800' : 'bg-red-100 text-red-800'
                    }`}>{Math.round(m.accessibility_score)}</span>
                  </td>
                  <td className="px-3 py-2 text-sm font-mono">{(m.lcp_ms / 1000).toFixed(1)}s</td>
                  <td className="px-3 py-2 text-sm font-mono">{m.cls.toFixed(3)}</td>
                  <td className="px-3 py-2 text-sm font-mono">{Math.round(m.fid_ms)}ms</td>
                  <td className="px-3 py-2 text-sm font-mono">{Math.round(m.ttfb_ms)}ms</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
