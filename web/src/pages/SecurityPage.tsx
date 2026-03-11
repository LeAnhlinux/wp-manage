import { useState } from 'react'
import { Shield, Search, Loader2, CheckCircle2, AlertTriangle, XCircle, Bug, ChevronLeft, ChevronRight } from 'lucide-react'
import { useMalwareScans, useStartMalwareScan } from '@/hooks/useSecurity'
import { useSites } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import type { MalwareScan, ThreatDetail } from '@/types'

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    running: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
  }
  const icons: Record<string, React.ReactNode> = {
    running: <Loader2 size={12} className="animate-spin" />,
    completed: <CheckCircle2 size={12} />,
    failed: <XCircle size={12} />,
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${styles[status] || 'bg-gray-100 text-gray-800'}`}>
      {icons[status]}
      {status}
    </span>
  )
}

function SeverityBadge({ severity }: { severity: string }) {
  const styles: Record<string, string> = {
    critical: 'bg-red-100 text-red-800',
    high: 'bg-orange-100 text-orange-800',
    medium: 'bg-yellow-100 text-yellow-800',
    low: 'bg-blue-100 text-blue-800',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${styles[severity] || 'bg-gray-100 text-gray-800'}`}>
      {severity}
    </span>
  )
}

function ScanDetailModal({ scan, onClose }: { scan: MalwareScan; onClose: () => void }) {
  let threats: ThreatDetail[] = []
  try {
    if (scan.threats_detail) {
      threats = JSON.parse(scan.threats_detail)
    }
  } catch { /* empty */ }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] overflow-hidden">
        <div className="p-6 border-b flex items-center justify-between">
          <div>
            <h3 className="text-lg font-semibold">Scan #{scan.id} — {scan.site?.domain || `Site #${scan.site_id}`}</h3>
            <p className="text-sm text-gray-500">{scan.scan_type} scan</p>
          </div>
          <StatusBadge status={scan.status} />
        </div>

        <div className="p-6 space-y-4 overflow-y-auto max-h-[55vh]">
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center p-3 bg-gray-50 rounded-lg">
              <p className="text-2xl font-bold">{scan.files_scanned}</p>
              <p className="text-xs text-gray-500">Files Scanned</p>
            </div>
            <div className={`text-center p-3 rounded-lg ${scan.threats_found > 0 ? 'bg-red-50' : 'bg-green-50'}`}>
              <p className={`text-2xl font-bold ${scan.threats_found > 0 ? 'text-red-600' : 'text-green-600'}`}>{scan.threats_found}</p>
              <p className="text-xs text-gray-500">Threats Found</p>
            </div>
            <div className="text-center p-3 bg-gray-50 rounded-lg">
              <p className="text-sm font-medium">{new Date(scan.started_at).toLocaleString()}</p>
              <p className="text-xs text-gray-500">Started</p>
            </div>
          </div>

          {scan.error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{scan.error}</div>
          )}

          {threats.length > 0 && (
            <div>
              <h4 className="text-sm font-medium mb-2 flex items-center gap-2">
                <AlertTriangle size={14} className="text-orange-500" />
                Threats Detected
              </h4>
              <div className="space-y-2">
                {threats.map((threat, idx) => (
                  <div key={idx} className="p-3 bg-red-50 border border-red-100 rounded-lg">
                    <div className="flex items-center justify-between mb-1">
                      <code className="text-sm font-mono text-red-800">{threat.file}</code>
                      <SeverityBadge severity={threat.severity} />
                    </div>
                    <p className="text-xs text-gray-600">{threat.description}</p>
                    <span className="text-xs text-red-600 font-medium">{threat.threat_type}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {scan.status === 'completed' && threats.length === 0 && (
            <div className="text-center py-4">
              <CheckCircle2 size={32} className="mx-auto text-green-500 mb-2" />
              <p className="text-green-700 font-medium">No threats detected</p>
              <p className="text-sm text-gray-500">Your site appears to be clean</p>
            </div>
          )}
        </div>

        <div className="p-4 border-t flex justify-end">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">Close</button>
        </div>
      </div>
    </div>
  )
}

export default function SecurityPage() {
  const [page, setPage] = useState(1)
  const [showStartScan, setShowStartScan] = useState(false)
  const [viewScan, setViewScan] = useState<MalwareScan | null>(null)
  const { data, isLoading } = useMalwareScans(page)
  const { can } = usePermissions()

  const scans: MalwareScan[] = data?.data || []
  const meta = data?.meta

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Shield size={24} />
            Security
          </h1>
          <p className="text-gray-500 text-sm mt-1">WAF configuration and malware scanning</p>
        </div>
        {can('sites', 'manage') && (
          <button
            onClick={() => setShowStartScan(true)}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm"
          >
            <Search size={16} />
            New Scan
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading...</div>
      ) : scans.length === 0 ? (
        <div className="text-center py-12">
          <Bug size={48} className="mx-auto text-gray-300 mb-4" />
          <p className="text-gray-500">No malware scans yet</p>
          <p className="text-gray-400 text-sm">Run a scan to check your sites for threats</p>
        </div>
      ) : (
        <>
          <div className="bg-white rounded-lg border overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">ID</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Site</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Files</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Threats</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Started</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {scans.map((scan: MalwareScan) => (
                  <tr key={scan.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-mono">#{scan.id}</td>
                    <td className="px-4 py-3 text-sm font-medium">{scan.site?.domain || `Site #${scan.site_id}`}</td>
                    <td className="px-4 py-3 text-sm capitalize">{scan.scan_type}</td>
                    <td className="px-4 py-3"><StatusBadge status={scan.status} /></td>
                    <td className="px-4 py-3 text-sm">{scan.files_scanned.toLocaleString()}</td>
                    <td className="px-4 py-3">
                      <span className={`text-sm font-medium ${scan.threats_found > 0 ? 'text-red-600' : 'text-green-600'}`}>
                        {scan.threats_found}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(scan.started_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => setViewScan(scan)}
                        className="text-sm text-blue-600 hover:underline"
                      >
                        View
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <p className="text-sm text-gray-500">Page {meta.page} of {meta.total_pages} ({meta.total} total)</p>
              <div className="flex gap-2">
                <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1} className="px-3 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-50"><ChevronLeft size={16} /></button>
                <button onClick={() => setPage(p => p + 1)} disabled={page >= meta.total_pages} className="px-3 py-1 text-sm border rounded hover:bg-gray-50 disabled:opacity-50"><ChevronRight size={16} /></button>
              </div>
            </div>
          )}
        </>
      )}

      {showStartScan && <StartScanModal onClose={() => setShowStartScan(false)} />}
      {viewScan && <ScanDetailModal scan={viewScan} onClose={() => setViewScan(null)} />}
    </div>
  )
}

function StartScanModal({ onClose }: { onClose: () => void }) {
  const [siteId, setSiteId] = useState(0)
  const [scanType, setScanType] = useState<'full' | 'quick'>('full')
  const { data: sitesData } = useSites()
  const startScan = useStartMalwareScan()

  const sites = sitesData?.data || []

  const handleSubmit = () => {
    if (!siteId) return
    startScan.mutate(
      { siteId, scan_type: scanType },
      { onSuccess: () => onClose() }
    )
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md">
        <div className="p-6 border-b">
          <h3 className="text-lg font-semibold">Start Malware Scan</h3>
          <p className="text-sm text-gray-500">Scan a WordPress site for malware and suspicious files</p>
        </div>

        <div className="p-6 space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Site</label>
            <select value={siteId} onChange={e => setSiteId(Number(e.target.value))} className="w-full border rounded-lg px-3 py-2 text-sm">
              <option value={0}>Select a site...</option>
              {sites.filter(s => s.status === 'active').map(s => (
                <option key={s.id} value={s.id}>{s.domain}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Scan Type</label>
            <div className="grid grid-cols-2 gap-2">
              <button
                onClick={() => setScanType('full')}
                className={`p-3 border rounded-lg text-sm text-left ${scanType === 'full' ? 'border-blue-600 bg-blue-50' : 'hover:bg-gray-50'}`}
              >
                <p className="font-medium">Full Scan</p>
                <p className="text-xs text-gray-500">Scans all files (PHP, JS, HTML)</p>
              </button>
              <button
                onClick={() => setScanType('quick')}
                className={`p-3 border rounded-lg text-sm text-left ${scanType === 'quick' ? 'border-blue-600 bg-blue-50' : 'hover:bg-gray-50'}`}
              >
                <p className="font-medium">Quick Scan</p>
                <p className="text-xs text-gray-500">Uploads & plugins only</p>
              </button>
            </div>
          </div>

          {startScan.isError && (
            <p className="text-sm text-red-600">{(startScan.error as Error).message || 'Failed to start scan'}</p>
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-lg">Cancel</button>
          <button
            onClick={handleSubmit}
            disabled={!siteId || startScan.isPending}
            className="px-4 py-2 text-sm bg-blue-600 text-white hover:bg-blue-700 rounded-lg disabled:opacity-50 flex items-center gap-2"
          >
            {startScan.isPending ? <Loader2 size={14} className="animate-spin" /> : <Search size={14} />}
            Start Scan
          </button>
        </div>
      </div>
    </div>
  )
}
