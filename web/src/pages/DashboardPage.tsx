import { useMemo, useState } from 'react'
import { Server, Globe, HardDrive, Activity, Cpu, MemoryStick, Database, Wifi, WifiOff, TrendingUp, ArrowRight, LineChart } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useServers, useMetricsHistory, type MetricHistoryPoint } from '@/hooks/useServers'
import { useSites } from '@/hooks/useSites'
import { useDashboardMetrics } from '@/hooks/useWebSocket'

// Area chart for CPU/RAM history. Pure SVG: normalized 0..100 coords stretched
// to the container (vector-effect keeps strokes crisp); labels live in HTML so
// they never distort. Colors come from theme tokens (--chart-*) so both the
// dark console and light theme render correctly.
function HistoryChart({ points }: { points: MetricHistoryPoint[] }) {
  // Downsample to ≤144 points so the path stays light even for 7-day ranges.
  const data = useMemo(() => {
    if (points.length <= 144) return points
    const step = Math.ceil(points.length / 144)
    return points.filter((_, i) => i % step === 0)
  }, [points])

  if (data.length < 2) {
    return (
      <p className="text-sm text-gray-400 text-center py-10">
        Not enough metric history yet — data appears after a few heartbeats.
      </p>
    )
  }

  const toPath = (key: 'cpu_usage' | 'ram_usage') =>
    data.map((p, i) => {
      const x = (i / (data.length - 1)) * 100
      const y = 100 - Math.min(p[key], 100)
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`
    }).join(' ')

  const cpuPath = toPath('cpu_usage')
  const ramPath = toPath('ram_usage')
  const last = data[data.length - 1]
  // recorded_at is stored in server-local time but the history endpoint suffixes
  // a literal 'Z'; strip it so the browser doesn't shift the time a second time.
  const fmtTime = (ts: string) =>
    new Date(ts.replace('Z', '')).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })

  return (
    <div>
      <div className="relative">
        {/* Grid % labels (HTML so they don't stretch) */}
        <div className="absolute inset-y-0 left-0 flex flex-col justify-between text-[10px] font-mono py-0.5" style={{ color: 'var(--chart-label)' }}>
          <span>100</span><span>50</span><span>0</span>
        </div>
        <div className="ml-8">
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="w-full h-36 block">
            {[25, 50, 75].map(g => (
              <line key={g} x1="0" y1={g} x2="100" y2={g}
                stroke="var(--chart-grid)" strokeWidth="1" strokeDasharray="2 3"
                vectorEffect="non-scaling-stroke" />
            ))}
            <path d={`${cpuPath} L100,100 L0,100 Z`} fill="var(--chart-cpu)" opacity="0.10" />
            <path d={cpuPath} fill="none" stroke="var(--chart-cpu)" strokeWidth="1.6"
              strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
            <path d={ramPath} fill="none" stroke="var(--chart-ram)" strokeWidth="1.4"
              strokeDasharray="4 3" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          </svg>
        </div>
      </div>
      <div className="ml-8 flex items-center justify-between mt-2">
        <span className="text-[10px] font-mono" style={{ color: 'var(--chart-label)' }}>{fmtTime(data[0].ts)}</span>
        <div className="flex items-center gap-4 text-[11px] font-mono">
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-0.5 rounded" style={{ background: 'var(--chart-cpu)' }} />
            <span className="text-gray-500">CPU</span>
            <b className="text-gray-900 font-semibold">{last.cpu_usage.toFixed(0)}%</b>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-2.5 h-0.5 rounded" style={{ background: 'var(--chart-ram)' }} />
            <span className="text-gray-500">RAM</span>
            <b className="text-gray-900 font-semibold">{last.ram_usage.toFixed(0)}%</b>
          </span>
        </div>
        <span className="text-[10px] font-mono" style={{ color: 'var(--chart-label)' }}>{fmtTime(last.ts)}</span>
      </div>
    </div>
  )
}

function MetricBar({ label, value, color, trackColor }: {
  label: string; value: number; color: string; trackColor: string
}) {
  return (
    <div>
      <div className="flex justify-between text-xs mb-1.5">
        <span className="text-slate-500">{label}</span>
        <span className={`font-semibold ${
          value >= 90 ? 'text-red-600' : value >= 70 ? 'text-amber-600' : 'text-slate-700'
        }`}>{value.toFixed(1)}%</span>
      </div>
      <div className={`h-1.5 ${trackColor} rounded-full overflow-hidden`}>
        <div
          className={`h-full rounded-full transition-all duration-700 ${
            value >= 90 ? 'bg-red-500' : value >= 70 ? 'bg-amber-500' : color
          }`}
          style={{ width: `${Math.min(value, 100)}%` }}
        />
      </div>
    </div>
  )
}

function StatCard({ label, value, icon: Icon, gradient, sub }: {
  label: string; value: number; icon: React.ElementType;
  gradient: string; sub?: string
}) {
  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 flex items-center gap-5">
      <div className={`w-14 h-14 rounded-2xl flex items-center justify-center flex-shrink-0 ${gradient}`}>
        <Icon size={24} className="text-white" strokeWidth={1.8} />
      </div>
      <div className="min-w-0">
        <p className="text-sm text-gray-500 leading-none mb-1.5">{label}</p>
        <p className="text-3xl font-bold text-gray-900 leading-none font-mono tracking-tight tabular-nums">{value}</p>
        {sub && <p className="text-xs text-gray-400 mt-1.5">{sub}</p>}
      </div>
    </div>
  )
}

export default function DashboardPage() {
  const { data: serversData } = useServers()
  const { data: sitesData } = useSites()
  const servers = serversData?.data || []

  // Pass REST server data as initial metrics so Dashboard renders immediately
  // without waiting up to 30s for the first WebSocket heartbeat.
  const { metrics: liveMetrics, connected: wsConnected } = useDashboardMetrics(servers)
  const sites = sitesData?.data || []

  // History chart: default to the first active server, switchable when there are several.
  const activeServerList = servers.filter((s) => s.status === 'active')
  const [chartServerId, setChartServerId] = useState<number | null>(null)
  const effectiveChartServer = chartServerId ?? activeServerList[0]?.id ?? 0
  const { data: history = [], isLoading: historyLoading } = useMetricsHistory(effectiveChartServer, 24)
  const activeServers = servers.filter((s) => s.status === 'active').length
  const activeSites = sites.filter((s) => s.status === 'active').length
  const totalServers = serversData?.meta?.total || 0
  const totalSites = sitesData?.meta?.total || 0

  const stats = [
    {
      label: 'Total Servers',
      value: totalServers,
      icon: Server,
      gradient: 'bg-gradient-to-br from-blue-500 to-blue-700',
      sub: `${activeServers} active`,
    },
    {
      label: 'Total Sites',
      value: totalSites,
      icon: Globe,
      gradient: 'bg-gradient-to-br from-violet-500 to-violet-700',
      sub: `${activeSites} active`,
    },
    {
      label: 'Active Sites',
      value: activeSites,
      icon: Activity,
      gradient: 'bg-gradient-to-br from-emerald-500 to-emerald-700',
      sub: totalSites > 0 ? `${Math.round(activeSites / totalSites * 100)}% online` : undefined,
    },
    {
      label: 'Storage Tracked',
      value: servers.reduce((acc, s) => acc + (s.disk_gb || 0), 0),
      icon: HardDrive,
      gradient: 'bg-gradient-to-br from-orange-500 to-orange-700',
      sub: 'GB total disk',
    },
  ]

  type MetricEntry = { server_id: number; server_name: string; ip_address: string; cpu_usage: number; ram_usage: number; disk_usage: number; uptime: number; status: string }
  const liveMetricsList = Object.values(liveMetrics) as MetricEntry[]

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 leading-none">Dashboard</h1>
          <p className="text-gray-500 text-sm mt-1">System overview — servers, sites, and live metrics</p>
        </div>
        <div className={`flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-medium border ${
          wsConnected
            ? 'bg-emerald-50 border-emerald-200 text-emerald-700'
            : 'bg-gray-50 border-gray-200 text-gray-500'
        }`}>
          {wsConnected ? (
            <><div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" /><Wifi size={12} /> Live</>
          ) : (
            <><div className="w-1.5 h-1.5 rounded-full bg-gray-400" /><WifiOff size={12} /> Offline</>
          )}
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-5 mb-8">
        {stats.map((stat) => (
          <StatCard key={stat.label} {...stat} />
        ))}
      </div>

      {/* Metrics history chart */}
      {effectiveChartServer > 0 && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5 mb-8">
          <div className="flex items-center justify-between mb-4 gap-3 flex-wrap">
            <h2 className="text-sm font-semibold text-gray-900 flex items-center gap-2">
              <LineChart size={16} className="text-blue-600" />
              CPU / RAM — last 24h
            </h2>
            {activeServerList.length > 1 && (
              <select
                value={effectiveChartServer}
                onChange={(e) => setChartServerId(Number(e.target.value))}
                className="text-xs font-mono px-2.5 py-1.5 border border-gray-200 rounded-lg bg-gray-50 focus:bg-white focus:border-blue-500 outline-none"
              >
                {activeServerList.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            )}
          </div>
          {historyLoading
            ? <p className="text-sm text-gray-400 text-center py-10">Loading metric history…</p>
            : <HistoryChart points={history} />}
        </div>
      )}

      {/* Real-time metrics */}
      {liveMetricsList.length > 0 && (
        <div className="mb-8">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-semibold text-gray-900 flex items-center gap-2">
              <TrendingUp size={18} className="text-blue-600" />
              Real-time Server Metrics
            </h2>
            <span className="text-xs text-gray-400">{liveMetricsList.length} server{liveMetricsList.length !== 1 ? 's' : ''} reporting</span>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {liveMetricsList.map((m) => (
              <div key={m.server_id} className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
                <div className="flex items-center justify-between mb-5">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-xl bg-[#0F172A] flex items-center justify-center">
                      <Server size={16} className="text-blue-400" />
                    </div>
                    <div>
                      <p className="font-semibold text-sm text-gray-900 leading-none">{m.server_name}</p>
                      <p className="text-xs text-gray-400 mt-0.5 font-mono">{m.ip_address}</p>
                    </div>
                  </div>
                  <div className={`w-2 h-2 rounded-full ${m.status === 'active' ? 'bg-emerald-500' : 'bg-gray-300'}`} />
                </div>
                <div className="space-y-3">
                  <MetricBar label="CPU" value={m.cpu_usage} color="bg-blue-500" trackColor="bg-blue-50" />
                  <MetricBar label="RAM" value={m.ram_usage} color="bg-violet-500" trackColor="bg-violet-50" />
                  <MetricBar label="Disk" value={m.disk_usage} color="bg-orange-500" trackColor="bg-orange-50" />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Recent lists */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Recent Servers */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-50">
            <h2 className="text-sm font-semibold text-gray-900 flex items-center gap-2">
              <Server size={16} className="text-gray-400" /> Servers
            </h2>
            <Link to="/servers" className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1 font-medium">
              View all <ArrowRight size={12} />
            </Link>
          </div>
          <div className="divide-y divide-gray-50">
            {servers.slice(0, 5).map((server) => {
              const live = liveMetrics[server.id]
              return (
                <div key={server.id} className="flex items-center gap-3 px-6 py-3.5">
                  <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                    server.status === 'active' ? 'bg-emerald-500' : 'bg-gray-300'
                  }`} />
                  <div className="flex-1 min-w-0">
                    <Link to={`/servers/${server.id}`} className="font-medium text-sm text-gray-900 hover:text-blue-600 transition-colors block truncate">
                      {server.name}
                    </Link>
                    <p className="text-xs text-gray-400 font-mono">{server.ip_address}</p>
                  </div>
                  {live && (
                    <div className="flex gap-3 text-xs text-gray-400 flex-shrink-0">
                      <span className="flex items-center gap-1">
                        <Cpu size={11} className="text-blue-400" /> {live.cpu_usage.toFixed(0)}%
                      </span>
                      <span className="flex items-center gap-1">
                        <MemoryStick size={11} className="text-violet-400" /> {live.ram_usage.toFixed(0)}%
                      </span>
                      <span className="flex items-center gap-1">
                        <Database size={11} className="text-orange-400" /> {live.disk_usage.toFixed(0)}%
                      </span>
                    </div>
                  )}
                  <span className={`flex-shrink-0 px-2 py-0.5 rounded-full text-[11px] font-medium ${
                    server.status === 'active'
                      ? 'bg-emerald-50 text-emerald-700'
                      : 'bg-gray-100 text-gray-500'
                  }`}>
                    {server.status}
                  </span>
                </div>
              )
            })}
            {servers.length === 0 && (
              <div className="px-6 py-10 text-center">
                <Server size={28} className="text-gray-200 mx-auto mb-3" />
                <p className="text-sm text-gray-400">No servers yet</p>
                <Link to="/servers" className="text-xs text-blue-600 hover:underline mt-1 inline-block">Add a server →</Link>
              </div>
            )}
          </div>
        </div>

        {/* Recent Sites */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-50">
            <h2 className="text-sm font-semibold text-gray-900 flex items-center gap-2">
              <Globe size={16} className="text-gray-400" /> Sites
            </h2>
            <Link to="/sites" className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1 font-medium">
              View all <ArrowRight size={12} />
            </Link>
          </div>
          <div className="divide-y divide-gray-50">
            {sites.slice(0, 5).map((site) => (
              <div key={site.id} className="flex items-center gap-3 px-6 py-3.5">
                <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                  site.status === 'active' ? 'bg-emerald-500' :
                  site.status === 'error' ? 'bg-red-500' :
                  site.status === 'creating' ? 'bg-blue-500 animate-pulse' :
                  'bg-gray-300'
                }`} />
                <div className="flex-1 min-w-0">
                  <Link to={`/sites/${site.id}`} className="font-medium text-sm text-gray-900 hover:text-blue-600 transition-colors block truncate">
                    {site.domain}
                  </Link>
                  <p className="text-xs text-gray-400">PHP {site.php_version} · {site.server?.name || `Server #${site.server_id}`}</p>
                </div>
                <span className={`flex-shrink-0 px-2 py-0.5 rounded-full text-[11px] font-medium ${
                  site.status === 'active' ? 'bg-emerald-50 text-emerald-700' :
                  site.status === 'error' ? 'bg-red-50 text-red-700' :
                  site.status === 'creating' ? 'bg-blue-50 text-blue-700' :
                  'bg-gray-100 text-gray-500'
                }`}>
                  {site.status}
                </span>
              </div>
            ))}
            {sites.length === 0 && (
              <div className="px-6 py-10 text-center">
                <Globe size={28} className="text-gray-200 mx-auto mb-3" />
                <p className="text-sm text-gray-400">No sites yet</p>
                <Link to="/sites" className="text-xs text-blue-600 hover:underline mt-1 inline-block">Create a site →</Link>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
