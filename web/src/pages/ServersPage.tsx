import { useState, useEffect, useRef, useCallback } from 'react'
import { Link } from 'react-router-dom'
import {
  Plus, Trash2, ExternalLink, Copy, Upload, Terminal, Key, Lock,
  CheckCircle, XCircle, Loader2, Server, Cpu, HardDrive, X, ChevronRight,
  Search, Filter,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useServers, useCreateServer, useDeleteServer, useAutoDeploy } from '@/hooks/useServers'
import { StatusBadge } from '@/components/ui/Badge'
import type { DeployProgress } from '@/types'

type DeployMethod = 'auto' | 'manual'
type AuthMethod = 'password' | 'key'

export default function ServersPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [ip, setIP] = useState('')
  const [installCmd, setInstallCmd] = useState('')

  const [deployMethod, setDeployMethod] = useState<DeployMethod>('auto')
  const [authMethod, setAuthMethod] = useState<AuthMethod>('password')
  const [sshUser, setSSHUser] = useState('root')
  const [sshPort, setSSHPort] = useState(22)
  const [sshPassword, setSSHPassword] = useState('')
  const [sshKey, setSSHKey] = useState('')
  const [deployLogs, setDeployLogs] = useState<DeployProgress[]>([])
  const [isDeploying, setIsDeploying] = useState(false)
  const [cmdCopied, setCmdCopied] = useState(false)
  const [createError, setCreateError] = useState('')

  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')

  const wsRef = useRef<WebSocket | null>(null)
  const logsEndRef = useRef<HTMLDivElement>(null)

  const qc = useQueryClient()
  const { data, isLoading } = useServers()
  const createServer = useCreateServer()
  const deleteServer = useDeleteServer()
  const autoDeploy = useAutoDeploy()

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [deployLogs])

  useEffect(() => {
    return () => { wsRef.current?.close() }
  }, [])

  const connectWebSocket = useCallback((channel: string) => {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${wsProtocol}//${window.location.host}/ws`)
    wsRef.current = ws
    ws.onopen = () => { ws.send(JSON.stringify({ action: 'subscribe', channel })) }
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      if (msg.type === 'deploy_progress') {
        const progress = msg.data as DeployProgress
        setDeployLogs(prev => {
          const updated = prev.map((p, i) =>
            i === prev.length - 1 && p.status === 'running' &&
              (progress.status === 'running' || progress.status === 'success')
              ? { ...p, status: 'success' as const }
              : p
          )
          return [...updated, progress]
        })
        if (progress.status === 'success' || progress.status === 'error') {
          setIsDeploying(false)
          ws.close()
          qc.invalidateQueries({ queryKey: ['servers'] })
        }
      }
    }
    ws.onerror = () => { setIsDeploying(false) }
  }, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError('')
    try {
      const result = await createServer.mutateAsync({ name, ip_address: ip })
      const serverId = result.data?.server?.id
      if (deployMethod === 'manual') {
        setInstallCmd(result.data?.install_command || '')
        setName('')
        setIP('')
      } else if (serverId) {
        setDeployLogs([])
        setIsDeploying(true)
        setInstallCmd('')
        const deployResult = await autoDeploy.mutateAsync({
          server_id: serverId,
          ssh_user: sshUser,
          ssh_port: sshPort,
          auth_method: authMethod,
          password: authMethod === 'password' ? sshPassword : undefined,
          private_key: authMethod === 'key' ? sshKey : undefined,
        })
        setSSHPassword('')
        setSSHKey('')
        const wsChannel = deployResult.data?.ws_channel
        if (wsChannel) connectWebSocket(wsChannel)
        setName('')
        setIP('')
      }
    } catch (err: any) {
      setIsDeploying(false)
      const msg = err?.response?.data?.error || err?.message || 'Something went wrong. Please try again.'
      setCreateError(msg)
    }
  }

  const getStepIcon = (progress: DeployProgress) => {
    if (progress.status === 'success') return <CheckCircle size={15} className="text-emerald-400 flex-shrink-0" />
    if (progress.status === 'error') return <XCircle size={15} className="text-red-400 flex-shrink-0" />
    return <Loader2 size={15} className="text-blue-400 animate-spin flex-shrink-0" />
  }

  const handleCopyCmd = () => {
    navigator.clipboard.writeText(installCmd)
    setCmdCopied(true)
    setTimeout(() => setCmdCopied(false), 2000)
  }

  const servers = data?.data || []
  const lastLog = deployLogs[deployLogs.length - 1]
  const deployProgress = lastLog ? (lastLog.step / lastLog.total) * 100 : 0

  const filteredServers = servers.filter(server => {
    const matchesSearch = searchTerm === '' ||
      server.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      server.ip_address.includes(searchTerm)
    const matchesStatus = statusFilter === 'all' || server.status === statusFilter
    return matchesSearch && matchesStatus
  })

  const inputCls = 'w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-gray-50 focus:bg-white transition-colors'

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-7">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 leading-none">Servers</h1>
          <p className="text-gray-500 text-sm mt-1">{servers.length} server{servers.length !== 1 ? 's' : ''} managed</p>
        </div>
        <button
          onClick={() => {
            setShowCreate(!showCreate)
            setInstallCmd('')
            setDeployLogs([])
            setCreateError('')
          }}
          className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 transition-colors text-sm font-semibold shadow-sm shadow-blue-200"
        >
          <Plus size={16} /> Add Server
        </button>
      </div>

      {/* Create Form */}
      {showCreate && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-6 mb-6">
          <div className="flex items-center justify-between mb-5">
            <h2 className="text-base font-semibold text-gray-900">Add New Server</h2>
            <button onClick={() => { setShowCreate(false); setCreateError('') }} className="text-gray-400 hover:text-gray-600">
              <X size={18} />
            </button>
          </div>

          {/* Deploy Method Cards */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-5">
            {([
              { value: 'auto' as const, icon: Upload, label: '⚡ Auto Deploy', desc: 'Connect via SSH and deploy automatically' },
              { value: 'manual' as const, icon: Terminal, label: '📋 Manual Install', desc: 'Get a one-line install command to run on your VPS' },
            ]).map(opt => (
              <button key={opt.value} type="button"
                onClick={() => setDeployMethod(opt.value)}
                className={`p-4 rounded-xl border-2 text-left transition-all ${
                  deployMethod === opt.value
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-100 hover:border-gray-200 bg-gray-50'
                }`}>
                <div className="font-semibold text-sm text-gray-800">{opt.label}</div>
                <div className="text-xs text-gray-500 mt-1">{opt.desc}</div>
              </button>
            ))}
          </div>

          <form onSubmit={handleCreate} className="space-y-5">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">Server Name</label>
                <input
                  value={name} onChange={(e) => setName(e.target.value)}
                  className={inputCls}
                  placeholder="Production VPS 1" required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">IP Address</label>
                <input
                  value={ip} onChange={(e) => setIP(e.target.value)}
                  className={inputCls + ' font-mono'}
                  placeholder="103.xxx.xxx.xxx" required
                />
              </div>
            </div>

            {deployMethod === 'auto' && (
              <div className="border border-gray-100 rounded-xl p-5 space-y-4 bg-gray-50">
                <h3 className="text-sm font-semibold text-gray-700 flex items-center gap-2">
                  <Lock size={14} className="text-gray-400" />
                  SSH Credentials
                  <span className="text-xs font-normal text-gray-400 ml-1">(Used once — not stored in DB)</span>
                </h3>

                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5">SSH User</label>
                    <input
                      value={sshUser} onChange={(e) => setSSHUser(e.target.value)}
                      className="w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-white transition-colors"
                      placeholder="root" required
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5">SSH Port</label>
                    <input
                      type="number" value={sshPort}
                      onChange={(e) => setSSHPort(Number(e.target.value))}
                      className="w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-white transition-colors"
                      min={1} max={65535}
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5">Auth Method</label>
                    <select
                      value={authMethod} onChange={(e) => setAuthMethod(e.target.value as AuthMethod)}
                      className="w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-white"
                    >
                      <option value="password">Password</option>
                      <option value="key">SSH Key</option>
                    </select>
                  </div>
                </div>

                {authMethod === 'password' ? (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5 flex items-center gap-1">
                      <Key size={13} className="text-gray-400" /> Password
                    </label>
                    <input
                      type="password"
                      value={sshPassword} onChange={(e) => setSSHPassword(e.target.value)}
                      className="w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm bg-white transition-colors"
                      placeholder="SSH password" required
                    />
                  </div>
                ) : (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5 flex items-center gap-1">
                      <Key size={13} className="text-gray-400" /> Private Key
                    </label>
                    <textarea
                      value={sshKey} onChange={(e) => setSSHKey(e.target.value)}
                      className="w-full px-4 py-2.5 border border-gray-200 rounded-xl focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 outline-none text-sm font-mono bg-white transition-colors"
                      rows={5}
                      placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
                      required
                    />
                  </div>
                )}
              </div>
            )}

            {createError && (
              <div className="p-4 bg-red-50 border border-red-200 rounded-xl text-sm text-red-700">
                {createError}
              </div>
            )}

            <div className="flex items-center gap-3">
              <button
                type="submit"
                disabled={createServer.isPending || autoDeploy.isPending || isDeploying}
                className="flex items-center gap-2 px-5 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 disabled:opacity-50 text-sm font-semibold transition-colors"
              >
                {(createServer.isPending || autoDeploy.isPending) ? (
                  <><Loader2 size={15} className="animate-spin" /> Processing...</>
                ) : deployMethod === 'auto' ? (
                  <><Upload size={15} /> Create & Deploy</>
                ) : (
                  'Create Server'
                )}
              </button>
              <button
                type="button" onClick={() => { setShowCreate(false); setCreateError('') }}
                className="px-5 py-2.5 text-gray-500 rounded-xl hover:bg-gray-100 text-sm transition-colors"
              >
                Cancel
              </button>
            </div>
          </form>

          {/* Manual: Install Command */}
          {installCmd && deployMethod === 'manual' && (
            <div className="mt-5 rounded-xl bg-[#0F172A] p-4">
              <p className="text-sm text-emerald-400 mb-2.5 font-medium">Run this command on your VPS:</p>
              <div className="flex items-start gap-3">
                <code className="text-sm text-white flex-1 break-all leading-relaxed">{installCmd}</code>
                <button
                  onClick={handleCopyCmd}
                  className={`p-1.5 rounded-lg transition-colors flex-shrink-0 ${cmdCopied ? 'text-emerald-400' : 'text-slate-400 hover:text-white'}`}
                  title="Copy"
                >
                  {cmdCopied ? <CheckCircle size={16} /> : <Copy size={16} />}
                </button>
              </div>
            </div>
          )}

          {/* Auto Deploy: Progress */}
          {(isDeploying || deployLogs.length > 0) && deployMethod === 'auto' && (
            <div className="mt-5 rounded-xl bg-[#0F172A] p-4">
              <p className="text-sm text-blue-400 mb-3 font-semibold">Deploy Progress</p>

              {deployLogs.length > 0 && (
                <div className="mb-4">
                  <div className="flex items-center justify-between text-xs text-slate-400 mb-1.5">
                    <span>Step {lastLog?.step} / {lastLog?.total}</span>
                    <span>{Math.round(deployProgress)}%</span>
                  </div>
                  <div className="w-full bg-slate-700 rounded-full h-1.5">
                    <div
                      className={`h-1.5 rounded-full transition-all duration-500 ${
                        lastLog?.status === 'error' ? 'bg-red-500' :
                        lastLog?.status === 'success' ? 'bg-emerald-500' : 'bg-blue-500'
                      }`}
                      style={{ width: `${deployProgress}%` }}
                    />
                  </div>
                </div>
              )}

              <div className="space-y-2 max-h-52 overflow-y-auto">
                {deployLogs.map((log, i) => (
                  <div key={i} className="flex items-start gap-2 text-sm">
                    {getStepIcon(log)}
                    <span className={
                      log.status === 'error' ? 'text-red-400' :
                      log.status === 'success' ? 'text-emerald-400' : 'text-slate-300'
                    }>
                      {log.message}
                    </span>
                  </div>
                ))}
                {lastLog?.error && (
                  <div className="mt-2 p-3 bg-red-900/30 border border-red-800 rounded-lg text-xs text-red-300">
                    {lastLog.error}
                  </div>
                )}
                {isDeploying && (
                  <div className="flex items-center gap-2 text-sm text-slate-400 pt-1">
                    <Loader2 size={14} className="animate-spin" /> Processing...
                  </div>
                )}
                <div ref={logsEndRef} />
              </div>
            </div>
          )}
        </div>
      )}

      {/* Search + Filter */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 mb-5">
        <div className="flex-1 relative">
          <Search size={15} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            placeholder="Search by name or IP address..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-9 pr-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 transition-colors"
          />
        </div>
        <div className="relative flex-shrink-0">
          <Filter size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="pl-8 pr-4 py-2.5 border border-gray-200 rounded-xl text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 cursor-pointer appearance-none"
          >
            <option value="all">All Status</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
            <option value="deploying">Deploying</option>
            <option value="error">Error</option>
          </select>
        </div>
      </div>

      {/* Desktop Table */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden hidden md:block">
        <table className="w-full">
          <thead>
            <tr className="border-b border-gray-50">
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Server</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">IP Address</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Specs</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Status</th>
              <th className="text-left px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Sites</th>
              <th className="text-right px-6 py-3.5 text-xs font-semibold text-gray-400 uppercase tracking-wide">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {isLoading && (
              <tr><td colSpan={6} className="px-6 py-10 text-center">
                <Loader2 size={22} className="animate-spin text-gray-300 mx-auto" />
              </td></tr>
            )}
            {filteredServers.map((server) => (
              <tr key={server.id} className="hover:bg-gray-50/60 transition-colors group">
                <td className="px-6 py-4">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-lg bg-[#0F172A] flex items-center justify-center flex-shrink-0">
                      <Server size={14} className="text-blue-400" />
                    </div>
                    <div>
                      <Link to={`/servers/${server.id}`} className="font-semibold text-sm text-gray-900 hover:text-blue-600 transition-colors">
                        {server.name}
                      </Link>
                      <p className="text-xs text-gray-400">{server.os || 'Ubuntu'}</p>
                    </div>
                  </div>
                </td>
                <td className="px-6 py-4 text-sm text-gray-600 font-mono">{server.ip_address}</td>
                <td className="px-6 py-4 text-sm text-gray-500">
                  <div className="flex items-center gap-3">
                    {server.ram_mb && (
                      <span className="flex items-center gap-1">
                        <HardDrive size={12} className="text-gray-400" />
                        {Math.round(server.ram_mb / 1024)}GB
                      </span>
                    )}
                    {server.cpu_cores && (
                      <span className="flex items-center gap-1">
                        <Cpu size={12} className="text-gray-400" />
                        {server.cpu_cores}c
                      </span>
                    )}
                    {server.disk_gb && (
                      <span className="flex items-center gap-1">
                        <HardDrive size={12} className="text-gray-400" />
                        {server.disk_gb}GB
                      </span>
                    )}
                    {!server.ram_mb && !server.cpu_cores && !server.disk_gb && (
                      <span className="text-gray-300">—</span>
                    )}
                  </div>
                </td>
                <td className="px-6 py-4"><StatusBadge status={server.status} /></td>
                <td className="px-6 py-4">
                  <span className="text-sm font-medium text-gray-700">{server.sites?.length || 0}</span>
                  <span className="text-xs text-gray-400 ml-1">site{(server.sites?.length || 0) !== 1 ? 's' : ''}</span>
                </td>
                <td className="px-6 py-4 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <Link
                      to={`/servers/${server.id}`}
                      className="p-1.5 text-gray-300 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors"
                      title="View"
                    >
                      <ExternalLink size={15} />
                    </Link>
                    <button
                      onClick={() => { if (confirm(`Delete server "${server.name}"? This cannot be undone.`)) deleteServer.mutate(server.id) }}
                      className="p-1.5 text-gray-300 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors"
                      title="Delete"
                    >
                      <Trash2 size={15} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {!isLoading && filteredServers.length === 0 && (
              <tr>
                <td colSpan={6} className="px-6 py-14 text-center">
                  <Server size={32} className="text-gray-200 mx-auto mb-3" />
                  <p className="text-gray-400 text-sm">
                    {servers.length === 0 ? 'No servers yet.' : 'No servers match your search.'}
                  </p>
                  {servers.length === 0 && (
                    <p className="text-gray-300 text-xs mt-1">Click "Add Server" to get started.</p>
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Mobile Cards */}
      <div className="space-y-3 md:hidden">
        {isLoading && (
          <div className="bg-white rounded-2xl border border-gray-100 p-6 text-center">
            <Loader2 size={22} className="animate-spin text-gray-300 mx-auto" />
          </div>
        )}
        {filteredServers.map((server) => (
          <div key={server.id} className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
            <div className="flex items-center gap-3 p-4">
              <div className="w-10 h-10 rounded-xl bg-[#0F172A] flex items-center justify-center flex-shrink-0">
                <Server size={18} className="text-blue-400" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="font-semibold text-gray-900 truncate">{server.name}</p>
                  <StatusBadge status={server.status} />
                </div>
                <p className="text-xs text-gray-400 font-mono mt-0.5">{server.ip_address}</p>
              </div>
              <Link to={`/servers/${server.id}`} className="text-gray-300 hover:text-blue-600 flex-shrink-0">
                <ChevronRight size={18} />
              </Link>
            </div>
            <div className="flex items-center gap-4 px-4 py-3 border-t border-gray-50 text-xs text-gray-500">
              {server.ram_mb && <span>{Math.round(server.ram_mb / 1024)}GB RAM</span>}
              {server.cpu_cores && <span>{server.cpu_cores} CPU</span>}
              {server.disk_gb && <span>{server.disk_gb}GB Disk</span>}
              <span className="ml-auto">{server.sites?.length || 0} sites</span>
              <button
                onClick={() => { if (confirm(`Delete server "${server.name}"?`)) deleteServer.mutate(server.id) }}
                className="p-1 text-gray-300 hover:text-red-600 transition-colors"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        ))}
        {!isLoading && filteredServers.length === 0 && (
          <div className="bg-white rounded-2xl border border-gray-100 p-10 text-center">
            <Server size={32} className="text-gray-200 mx-auto mb-3" />
            <p className="text-gray-400 text-sm">
              {servers.length === 0 ? 'No servers yet.' : 'No servers match your search.'}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
