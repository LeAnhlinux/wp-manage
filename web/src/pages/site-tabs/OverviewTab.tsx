import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Globe, Lock, LockOpen, RefreshCw, ExternalLink, Trash2, Pause, Play, Shield,
  Database, AlertTriangle, Eye, EyeOff, Key, CheckCircle, CheckCircle2, XCircle,
  X, Upload
} from 'lucide-react'
import {
  useDeleteSite, useUpdatePHP, useRestartSite, useSuspendSite, useResumeSite,
  useIssueSSL, useSiteCredentials, useUploadSource, useImportDB, useToggleOpenBasedir
} from '@/hooks/useSites'
import { useServerInstalledPHP } from '@/hooks/useServers'
import { usePermissions } from '@/hooks/usePermissions'
import { formatDate } from '@/lib/utils'
import type { Site } from '@/types'
import { DetailRow, CredRow, ActionBtn } from './shared'

type DeployPhase = 'idle' | 'uploading' | 'processing' | 'done' | 'error'
type DeployResult = { success: boolean; message: string; detail?: string }

interface OverviewTabProps {
  site: Site
  onDeleted: () => void
  onMessage: (type: 'success' | 'error', text: string) => void
  canManage: boolean
}

export function OverviewTab({ site, onDeleted, onMessage, canManage }: OverviewTabProps) {
  const deleteSite = useDeleteSite()
  const updatePHP = useUpdatePHP()
  const restartSite = useRestartSite()
  const suspendSite = useSuspendSite()
  const resumeSite = useResumeSite()
  const issueSSL = useIssueSSL()
  const uploadSource = useUploadSource()
  const importDB = useImportDB()
  const toggleOpenBasedir = useToggleOpenBasedir()
  const { can } = usePermissions()

  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmStep, setDeleteConfirmStep] = useState<1 | 2>(1)
  const [deleteConfirmText, setDeleteConfirmText] = useState('')
  const [uploadSourceFile, setUploadSourceFile] = useState<File | null>(null)
  const [importDBFile, setImportDBFile] = useState<File | null>(null)
  const [importDBOldDomain, setImportDBOldDomain] = useState('')

  const [sourcePhase, setSourcePhase] = useState<DeployPhase>('idle')
  const [sourceUploadPct, setSourceUploadPct] = useState(0)
  const [sourceResult, setSourceResult] = useState<DeployResult | null>(null)
  const [dbPhase, setDbPhase] = useState<DeployPhase>('idle')
  const [dbUploadPct, setDbUploadPct] = useState(0)
  const [dbResult, setDbResult] = useState<DeployResult | null>(null)

  const [showPHPModal, setShowPHPModal] = useState(false)
  const [selectedPHP, setSelectedPHP] = useState('')
  const [showCreds, setShowCreds] = useState(false)
  const [copiedField, setCopiedField] = useState('')

  const { data: creds } = useSiteCredentials(Number(site.id), showCreds)
  const { data: installedPHP = [], isLoading: phpLoading } = useServerInstalledPHP(site.server_id)

  const siteBaseUrl = `${site.ssl_enabled ? 'https' : 'http'}://${site.domain}`

  const copyToClipboard = (text: string, field: string) => {
    navigator.clipboard.writeText(text)
    setCopiedField(field)
    setTimeout(() => setCopiedField(''), 2000)
  }

  const handleDelete = () => {
    setShowDeleteModal(true)
    setDeleteConfirmStep(1)
    setDeleteConfirmText('')
  }

  const handleConfirmDelete = async () => {
    if (deleteConfirmText !== site.domain) return
    try {
      await deleteSite.mutateAsync(site.id)
      onDeleted()
    } catch {
      onMessage('error', 'Failed to delete site')
      setShowDeleteModal(false)
    }
  }

  const handleUploadSource = async () => {
    if (!uploadSourceFile) return
    setSourceResult(null)
    setSourceUploadPct(0)
    setSourcePhase('uploading')
    try {
      const res = await uploadSource.mutateAsync({
        siteId: site.id,
        file: uploadSourceFile,
        onProgress: (pct) => setSourceUploadPct(pct),
      })
      setSourcePhase('processing')
      setSourcePhase('done')
      const mb = res.data?.file_size ? (res.data.file_size / 1048576).toFixed(1) : '?'
      setSourceResult({ success: true, message: 'Source deployed successfully!', detail: `Uploaded ${mb} MB and extracted to /var/www/${site.domain}/public/` })
      setUploadSourceFile(null)
    } catch (err: unknown) {
      setSourcePhase('error')
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
        || (err instanceof Error ? err.message : null)
        || 'Upload source thất bại'
      setSourceResult({ success: false, message: 'Deploy thất bại', detail: msg ?? undefined })
    }
  }

  const handleImportDB = async () => {
    if (!importDBFile) return
    setDbResult(null)
    setDbUploadPct(0)
    setDbPhase('uploading')
    try {
      await importDB.mutateAsync({
        siteId: site.id,
        file: importDBFile,
        oldDomain: importDBOldDomain,
        onProgress: (pct) => setDbUploadPct(pct),
      })
      setDbPhase('processing')
      setDbPhase('done')
      const detail = importDBOldDomain.trim()
        ? `Imported + ran wp search-replace: ${importDBOldDomain} → ${site.domain}. siteurl/home updated.`
        : `Database imported into ${site.db_name}. siteurl/home updated to https://${site.domain}.`
      setDbResult({ success: true, message: 'Database imported successfully!', detail })
      setImportDBFile(null)
      setImportDBOldDomain('')
    } catch (err: unknown) {
      setDbPhase('error')
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
        || (err instanceof Error ? err.message : null)
        || 'Import database thất bại'
      setDbResult({ success: false, message: 'Import thất bại', detail: msg ?? undefined })
    }
  }

  const handleRestart = async () => {
    try {
      await restartSite.mutateAsync(site.id)
      onMessage('success', 'Restart command sent for PHP-FPM & Nginx')
    } catch {
      onMessage('error', 'Restart failed')
    }
  }

  const handleSuspend = async () => {
    if (!confirm(`Suspend site ${site.domain}?`)) return
    try {
      await suspendSite.mutateAsync(site.id)
      onMessage('success', 'Site has been suspended')
    } catch {
      onMessage('error', 'Failed to suspend site')
    }
  }

  const handleResume = async () => {
    try {
      await resumeSite.mutateAsync(site.id)
      onMessage('success', 'Site has been resumed')
    } catch {
      onMessage('error', 'Failed to resume site')
    }
  }

  const handleUpdatePHP = async () => {
    if (!selectedPHP || selectedPHP === site.php_version) return
    try {
      await updatePHP.mutateAsync({ id: site.id, version: selectedPHP })
      setShowPHPModal(false)
      onMessage('success', `Switched to PHP ${selectedPHP}`)
    } catch {
      onMessage('error', 'Failed to update PHP version')
    }
  }

  const handleIssueSSL = async () => {
    try {
      await issueSSL.mutateAsync(site.id)
      onMessage('success', 'SSL certificate issued successfully')
    } catch {
      onMessage('error', 'Failed to issue SSL certificate')
    }
  }

  const isActive = site.status === 'active'
  const isSuspended = site.status === 'suspended'

  return (
    <>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* ── Left: Site Info + Credentials ── */}
        <div className="lg:col-span-2 space-y-5">

          {/* Site Info — merged, no duplicates */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-4">Site Info</h2>
            <dl className="space-y-3">
              {/* Domain + WP Admin */}
              <DetailRow label="Domain" value={
                <a href={siteBaseUrl} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
                  {site.domain}
                </a>
              } />
              <DetailRow label="WP Admin" value={
                <a href={`${siteBaseUrl}/wp-admin`} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
                  {site.domain}/wp-admin
                </a>
              } />
              <DetailRow label="Admin User" value={site.wp_admin_user || '-'} />
              <DetailRow label="Admin Email" value={site.wp_admin_email || '-'} />

              <div className="border-t border-gray-100 my-1" />

              {/* PHP — with Change button */}
              <DetailRow label="PHP Version" value={
                <div className="flex items-center gap-2">
                  <span>PHP {site.php_version}</span>
                  {isActive && (
                    <button onClick={() => { setSelectedPHP(site.php_version); setShowPHPModal(true) }}
                      className="text-xs px-2 py-0.5 bg-blue-50 text-blue-600 rounded hover:bg-blue-100">
                      Change
                    </button>
                  )}
                </div>
              } />

              {/* SSL — with Issue button if disabled */}
              <DetailRow label="SSL" value={
                <div className="flex items-center gap-2">
                  {site.ssl_enabled ? (
                    <span className="flex items-center gap-1 text-green-600">
                      <Lock size={13} /> Active
                      {site.ssl_expires_at && <span className="text-xs text-gray-400 ml-1">· hết hạn {formatDate(site.ssl_expires_at)}</span>}
                    </span>
                  ) : (
                    <>
                      <span className="flex items-center gap-1 text-gray-400"><LockOpen size={13} /> Disabled</span>
                      {isActive && (
                        <button onClick={handleIssueSSL} disabled={issueSSL.isPending}
                          className="text-xs px-2 py-0.5 bg-green-50 text-green-600 rounded hover:bg-green-100 disabled:opacity-50">
                          {issueSSL.isPending ? 'Issuing...' : 'Issue SSL'}
                        </button>
                      )}
                    </>
                  )}
                </div>
              } />

              {/* open_basedir toggle */}
              <DetailRow label="open_basedir" value={
                <div className="flex items-center gap-3">
                  <button
                    onClick={() => toggleOpenBasedir.mutate(site.id)}
                    disabled={toggleOpenBasedir.isPending || !can('sites', 'manage')}
                    className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none disabled:opacity-50 ${
                      (site.open_basedir_enabled ?? true) ? 'bg-emerald-500' : 'bg-gray-300'
                    }`}
                  >
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                      (site.open_basedir_enabled ?? true) ? 'translate-x-4' : 'translate-x-0.5'
                    }`} />
                  </button>
                  <span className="text-sm">
                    {toggleOpenBasedir.isPending ? (
                      <span className="text-blue-500">Updating…</span>
                    ) : (site.open_basedir_enabled ?? true) ? (
                      <span className="text-emerald-600 font-medium">Enabled</span>
                    ) : (
                      <span className="text-amber-600">Disabled <span className="text-xs text-amber-500">— PHP can access all paths</span></span>
                    )}
                  </span>
                </div>
              } />

              <div className="border-t border-gray-100 my-1" />

              {/* System */}
              {site.linux_user && (
                <DetailRow label="Linux User" value={
                  <code className="text-xs bg-gray-100 px-2 py-0.5 rounded font-mono text-gray-700">{site.linux_user}</code>
                } />
              )}
              <DetailRow label="Server" value={
                <Link to={`/servers/${site.server_id}`} className="text-blue-600 hover:underline">
                  {site.server?.name || `Server #${site.server_id}`}
                </Link>
              } />
              <DetailRow label="Created" value={formatDate(site.created_at)} />
            </dl>
          </div>

          {/* Credentials */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <div className="flex items-center justify-between mb-1">
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wide flex items-center gap-2">
                <Key size={14} className="text-yellow-500" /> Login Credentials
              </h2>
              <button onClick={() => setShowCreds(!showCreds)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-600 bg-gray-100 rounded-lg hover:bg-gray-200">
                {showCreds ? <EyeOff size={14} /> : <Eye size={14} />}
                {showCreds ? 'Hide' : 'Show'}
              </button>
            </div>
            {showCreds && creds ? (
              <div className="space-y-3 mt-4">
                <CredRow label="WP Admin URL" value={creds.wp_admin_url} field="wp_url" copiedField={copiedField} onCopy={copyToClipboard} isLink />
                <CredRow label="WP Admin User" value={creds.wp_admin_user} field="wp_user" copiedField={copiedField} onCopy={copyToClipboard} />
                <CredRow label="WP Admin Pass" value={creds.wp_admin_pass} field="wp_pass" copiedField={copiedField} onCopy={copyToClipboard} />
                <div className="border-t border-gray-100 pt-3 mt-1">
                  <p className="text-xs font-medium text-gray-400 uppercase mb-2">Database</p>
                </div>
                <CredRow label="DB Name" value={creds.db_name} field="db_name" copiedField={copiedField} onCopy={copyToClipboard} />
                <CredRow label="DB User" value={creds.db_user} field="db_user" copiedField={copiedField} onCopy={copyToClipboard} />
                <CredRow label="DB Password" value={creds.db_password} field="db_pass" copiedField={copiedField} onCopy={copyToClipboard} />
              </div>
            ) : showCreds ? (
              <div className="flex items-center justify-center py-4">
                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
              </div>
            ) : (
              <p className="text-sm text-gray-400 mt-2">Click "Show" to reveal WP Admin and Database credentials.</p>
            )}
          </div>
        </div>

        {/* ── Right: Actions + Deploy + Danger ── */}
        <div className="space-y-5">
          {/* Quick Actions */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Actions</h2>
            <div className="space-y-2">
              {isActive && (
                <>
                  <ActionBtn icon={<RefreshCw size={16} />} label="Restart PHP-FPM & Nginx"
                    onClick={handleRestart} loading={restartSite.isPending} color="blue" />
                  <ActionBtn icon={<Pause size={16} />} label="Suspend Site"
                    onClick={handleSuspend} loading={suspendSite.isPending} color="yellow" />
                </>
              )}
              {isSuspended && (
                <ActionBtn icon={<Play size={16} />} label="Resume Site"
                  onClick={handleResume} loading={resumeSite.isPending} color="green" />
              )}
              {site.status === 'creating' && (
                <div className="flex items-center gap-2 p-3 bg-blue-50 rounded-lg text-sm text-blue-700">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />
                  Creating site... Please wait.
                </div>
              )}
              {site.status === 'error' && (
                <div className="flex items-center gap-2 p-3 bg-red-50 rounded-lg text-sm text-red-700">
                  <AlertTriangle size={16} />
                  Site creation failed. Try deleting and recreating.
                </div>
              )}
            </div>
          </div>

          {/* Manual Install Upload Section */}
          {site.install_type === 'manual' && (
            <div className="bg-white rounded-xl shadow-sm border border-blue-100 p-6 space-y-6">
              <div>
                <h2 className="text-lg font-semibold text-blue-700">Deploy Source & Database</h2>
                <p className="text-xs text-gray-500 mt-0.5">Upload source zip và import SQL database cho site manual.</p>
              </div>

              {/* ── Upload Source ── */}
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <div className="w-6 h-6 rounded-full bg-blue-100 flex items-center justify-center text-xs font-bold text-blue-600">1</div>
                  <span className="text-sm font-semibold text-gray-800">Source Code (.zip)</span>
                </div>

                {/* File picker + button */}
                {sourcePhase === 'idle' || sourcePhase === 'done' || sourcePhase === 'error' ? (
                  <div className="space-y-2">
                    <div className="flex items-center gap-2">
                      <label className="flex items-center gap-2 flex-1 min-w-0 cursor-pointer border border-gray-200 rounded-lg px-3 py-1.5 bg-white hover:bg-gray-50 transition-colors">
                        <span className="text-xs bg-blue-50 text-blue-700 px-2 py-0.5 rounded shrink-0">Chọn file</span>
                        <span className="text-sm text-gray-500 truncate min-w-0">
                          {uploadSourceFile ? uploadSourceFile.name : 'Chưa chọn file (.zip)'}
                        </span>
                        <input type="file" accept=".zip" className="hidden"
                          onChange={(e) => { setUploadSourceFile(e.target.files?.[0] || null); setSourceResult(null); setSourcePhase('idle') }} />
                      </label>
                      <button onClick={handleUploadSource}
                        disabled={!uploadSourceFile}
                        className="px-4 py-1.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-40 whitespace-nowrap flex items-center gap-1.5 shrink-0">
                        <Upload size={14} /> Deploy
                      </button>
                    </div>
                    <p className="text-xs text-gray-400">Max 3 GB · Extract vào <code className="bg-gray-100 px-1 rounded">/var/www/{site.domain}/public/</code></p>
                  </div>
                ) : null}

                {/* Progress panel */}
                {(sourcePhase === 'uploading' || sourcePhase === 'processing') && (
                  <div className="rounded-xl border border-blue-100 bg-blue-50 p-4 space-y-3">
                    {/* Step 1: Upload */}
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2 font-medium text-blue-700">
                          {sourcePhase === 'uploading'
                            ? <div className="w-3.5 h-3.5 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" />
                            : <CheckCircle size={14} className="text-emerald-500" />}
                          Uploading file lên server
                        </div>
                        <span className="text-blue-600 font-semibold">{sourceUploadPct}%</span>
                      </div>
                      <div className="h-1.5 bg-blue-200 rounded-full overflow-hidden">
                        <div className="h-full bg-blue-500 rounded-full transition-all duration-300"
                          style={{ width: `${sourceUploadPct}%` }} />
                      </div>
                    </div>
                    {/* Step 2: Extract */}
                    <div className="flex items-center gap-2 text-xs">
                      {sourcePhase === 'processing'
                        ? <div className="w-3.5 h-3.5 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" />
                        : sourceUploadPct === 100
                          ? <div className="w-3.5 h-3.5 rounded-full border-2 border-blue-300 border-t-transparent animate-spin" />
                          : <div className="w-3.5 h-3.5 rounded-full border border-gray-300" />}
                      <span className={sourcePhase === 'processing' ? 'text-blue-700 font-medium' : 'text-gray-400'}>
                        Agent đang extract files vào webroot...
                      </span>
                    </div>
                  </div>
                )}

                {/* Result */}
                {sourceResult && (
                  <div className={`rounded-xl border p-4 ${sourceResult.success ? 'border-emerald-200 bg-emerald-50' : 'border-red-200 bg-red-50'}`}>
                    <div className="flex items-start gap-2.5">
                      {sourceResult.success
                        ? <CheckCircle size={18} className="text-emerald-500 mt-0.5 shrink-0" />
                        : <XCircle size={18} className="text-red-500 mt-0.5 shrink-0" />}
                      <div className="flex-1 min-w-0">
                        <p className={`text-sm font-semibold ${sourceResult.success ? 'text-emerald-700' : 'text-red-700'}`}>
                          {sourceResult.message}
                        </p>
                        {sourceResult.detail && (
                          <p className={`text-xs mt-1 ${sourceResult.success ? 'text-emerald-600' : 'text-red-600'} break-words`}>
                            {sourceResult.detail}
                          </p>
                        )}
                      </div>
                      <button onClick={() => { setSourceResult(null); setSourcePhase('idle') }}
                        className="text-gray-400 hover:text-gray-600 shrink-0">
                        <X size={14} />
                      </button>
                    </div>
                  </div>
                )}
              </div>

              <div className="border-t border-gray-100" />

              {/* ── Import DB ── */}
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <div className="w-6 h-6 rounded-full bg-emerald-100 flex items-center justify-center text-xs font-bold text-emerald-600">2</div>
                  <span className="text-sm font-semibold text-gray-800">Database (.sql / .gz / .zip)</span>
                </div>

                {dbPhase === 'idle' || dbPhase === 'done' || dbPhase === 'error' ? (
                  <div className="space-y-2">
                    <div className="flex items-center gap-2">
                      <label className="flex items-center gap-2 flex-1 min-w-0 cursor-pointer border border-gray-200 rounded-lg px-3 py-1.5 bg-white hover:bg-gray-50 transition-colors">
                        <span className="text-xs bg-emerald-50 text-emerald-700 px-2 py-0.5 rounded shrink-0">Chọn file</span>
                        <span className="text-sm text-gray-500 truncate min-w-0">
                          {importDBFile ? importDBFile.name : 'Chưa chọn file (.sql / .gz / .zip)'}
                        </span>
                        <input type="file" accept=".sql,.gz,.sql.gz,.zip" className="hidden"
                          onChange={(e) => { setImportDBFile(e.target.files?.[0] || null); setDbResult(null); setDbPhase('idle') }} />
                      </label>
                      <button onClick={handleImportDB}
                        disabled={!importDBFile}
                        className="px-4 py-1.5 bg-emerald-600 text-white text-sm font-medium rounded-lg hover:bg-emerald-700 disabled:opacity-40 whitespace-nowrap flex items-center gap-1.5 shrink-0">
                        <Database size={14} /> Import
                      </button>
                    </div>
                    {/* Old domain */}
                    <div className="flex gap-2 items-center">
                      <input type="text" value={importDBOldDomain}
                        onChange={(e) => setImportDBOldDomain(e.target.value)}
                        placeholder="Old domain (e.g. old-site.com) — leave blank to skip URL replace"
                        className="flex-1 text-sm border border-gray-200 rounded-lg px-3 py-1.5 bg-gray-50 focus:bg-white focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 focus:outline-none" />
                    </div>
                    <p className="text-xs text-gray-400">
                      Import into <code className="bg-gray-100 px-1 rounded">{site.db_name}</code>
                      {importDBOldDomain.trim() && <> · Will run <code className="bg-gray-100 px-1 rounded">wp search-replace</code> {importDBOldDomain.trim()} → {site.domain}</>}
                    </p>
                  </div>
                ) : null}

                {/* Progress panel */}
                {(dbPhase === 'uploading' || dbPhase === 'processing') && (
                  <div className="rounded-xl border border-emerald-100 bg-emerald-50 p-4 space-y-3">
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2 font-medium text-emerald-700">
                          {dbPhase === 'uploading'
                            ? <div className="w-3.5 h-3.5 rounded-full border-2 border-emerald-500 border-t-transparent animate-spin" />
                            : <CheckCircle size={14} className="text-emerald-500" />}
                          Uploading file lên server
                        </div>
                        <span className="text-emerald-600 font-semibold">{dbUploadPct}%</span>
                      </div>
                      <div className="h-1.5 bg-emerald-200 rounded-full overflow-hidden">
                        <div className="h-full bg-emerald-500 rounded-full transition-all duration-300"
                          style={{ width: `${dbUploadPct}%` }} />
                      </div>
                    </div>
                    <div className="flex items-center gap-2 text-xs">
                      {dbPhase === 'processing'
                        ? <div className="w-3.5 h-3.5 rounded-full border-2 border-emerald-500 border-t-transparent animate-spin" />
                        : dbUploadPct === 100
                          ? <div className="w-3.5 h-3.5 rounded-full border-2 border-emerald-300 border-t-transparent animate-spin" />
                          : <div className="w-3.5 h-3.5 rounded-full border border-gray-300" />}
                      <span className={dbPhase === 'processing' ? 'text-emerald-700 font-medium' : 'text-gray-400'}>
                        Agent đang import SQL vào database...
                      </span>
                    </div>
                    {importDBOldDomain.trim() && (
                      <div className="flex items-center gap-2 text-xs text-gray-400">
                        <div className="w-3.5 h-3.5 rounded-full border border-gray-300" />
                        Running wp search-replace: {importDBOldDomain} → {site.domain}
                      </div>
                    )}
                    <p className="text-xs text-amber-600 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">
                      ⏳ File lớn có thể mất 1–3 phút. Vui lòng giữ nguyên tab này.
                    </p>
                  </div>
                )}

                {/* Result */}
                {dbResult && (
                  <div className={`rounded-xl border p-4 ${dbResult.success ? 'border-emerald-200 bg-emerald-50' : 'border-red-200 bg-red-50'}`}>
                    <div className="flex items-start gap-2.5">
                      {dbResult.success
                        ? <CheckCircle size={18} className="text-emerald-500 mt-0.5 shrink-0" />
                        : <XCircle size={18} className="text-red-500 mt-0.5 shrink-0" />}
                      <div className="flex-1 min-w-0">
                        <p className={`text-sm font-semibold ${dbResult.success ? 'text-emerald-700' : 'text-red-700'}`}>
                          {dbResult.message}
                        </p>
                        {dbResult.detail && (
                          <p className={`text-xs mt-1 ${dbResult.success ? 'text-emerald-600' : 'text-red-600'} break-words whitespace-pre-wrap`}>
                            {dbResult.detail}
                          </p>
                        )}
                        {dbResult.success && (
                          <a href={siteBaseUrl} target="_blank" rel="noopener"
                            className="inline-flex items-center gap-1 mt-2 text-xs text-emerald-600 hover:text-emerald-700 font-medium">
                            <ExternalLink size={12} /> Kiểm tra site →
                          </a>
                        )}
                      </div>
                      <button onClick={() => { setDbResult(null); setDbPhase('idle') }}
                        className="text-gray-400 hover:text-gray-600 shrink-0">
                        <X size={14} />
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Danger Zone */}
          <div className="bg-white rounded-xl shadow-sm border border-red-100 p-6">
            <h2 className="text-lg font-semibold text-red-600 mb-4">Danger Zone</h2>
            <p className="text-sm text-gray-500 mb-3">Permanently delete site files, database, Nginx vhost and SSL cert. This cannot be undone.</p>
            <button onClick={handleDelete} disabled={deleteSite.isPending}
              className="w-full flex items-center justify-center gap-2 px-4 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
              <Trash2 size={16} />
              {deleteSite.isPending ? 'Deleting...' : 'Delete Site'}
            </button>
          </div>
        </div>
      </div>

      {/* ===== Delete Confirmation Modal ===== */}
      {showDeleteModal && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl shadow-2xl w-full max-w-md" onClick={(e) => e.stopPropagation()}>
            <div className="p-6">
              <div className="flex items-center gap-3 mb-4">
                <div className="w-10 h-10 rounded-full bg-red-100 flex items-center justify-center">
                  <Trash2 size={20} className="text-red-600" />
                </div>
                <div>
                  <h3 className="text-lg font-bold text-gray-900">Delete WordPress Site</h3>
                  <p className="text-sm text-red-600">This action cannot be undone!</p>
                </div>
              </div>

              {deleteConfirmStep === 1 ? (
                <>
                  <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-5 text-sm text-red-700 space-y-1">
                    <p className="font-semibold">The following will be permanently deleted:</p>
                    <ul className="list-disc list-inside space-y-0.5 ml-1">
                      <li>All files at <code className="bg-red-100 px-1 rounded">/var/www/{site.domain}/</code></li>
                      <li>Database <code className="bg-red-100 px-1 rounded">{site.db_name}</code> and DB user</li>
                      <li>Nginx vhost <code className="bg-red-100 px-1 rounded">{site.domain}.conf</code></li>
                      <li>SSL certificate (certbot delete)</li>
                    </ul>
                  </div>
                  <div className="flex gap-3">
                    <button onClick={() => setShowDeleteModal(false)}
                      className="flex-1 px-4 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50">
                      Cancel
                    </button>
                    <button onClick={() => setDeleteConfirmStep(2)}
                      className="flex-1 px-4 py-2 bg-red-600 text-white rounded-lg text-sm hover:bg-red-700 font-medium">
                      I understand, continue →
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <p className="text-sm text-gray-600 mb-3">
                    To confirm, type the domain <span className="font-mono font-bold text-red-600">{site.domain}</span> below:
                  </p>
                  <input
                    type="text"
                    value={deleteConfirmText}
                    onChange={(e) => setDeleteConfirmText(e.target.value)}
                    placeholder={site.domain}
                    className="w-full px-4 py-2 border-2 border-gray-300 rounded-lg mb-4 text-sm font-mono focus:outline-none focus:border-red-400"
                    autoFocus
                  />
                  <div className="flex gap-3">
                    <button onClick={() => setDeleteConfirmStep(1)}
                      className="flex-1 px-4 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50">
                      ← Back
                    </button>
                    <button
                      onClick={handleConfirmDelete}
                      disabled={deleteConfirmText !== site.domain || deleteSite.isPending}
                      className="flex-1 px-4 py-2 bg-red-600 text-white rounded-lg text-sm font-medium hover:bg-red-700 disabled:opacity-40 disabled:cursor-not-allowed">
                      {deleteSite.isPending ? 'Deleting...' : 'Delete permanently'}
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      )}

      {/* PHP Version Modal */}
      {showPHPModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowPHPModal(false)}>
          <div className="bg-white rounded-xl p-6 w-full max-w-sm shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-semibold mb-1">Change PHP Version</h3>
            <p className="text-sm text-gray-500 mb-5">
              Select a PHP version installed on this server.
            </p>

            {phpLoading ? (
              <div className="flex items-center justify-center py-6">
                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
              </div>
            ) : installedPHP.filter(p => p.installed).length === 0 ? (
              <p className="text-sm text-gray-400 text-center py-4">No PHP versions installed on this server.</p>
            ) : (
              <div className="grid grid-cols-3 gap-2 mb-5">
                {installedPHP.filter(p => p.installed).map(p => (
                  <button key={p.version} onClick={() => setSelectedPHP(p.version)}
                    className={`relative px-3 py-2.5 rounded-lg text-sm font-medium border transition-colors text-center ${
                      selectedPHP === p.version
                        ? 'bg-blue-600 text-white border-blue-600'
                        : p.version === site.php_version
                          ? 'bg-gray-100 text-gray-500 border-gray-200 cursor-default'
                          : 'bg-white text-gray-700 border-gray-200 hover:border-blue-300 hover:bg-blue-50'
                    }`}>
                    PHP {p.version}
                    {p.version === site.php_version && (
                      <span className="block text-xs font-normal opacity-70 mt-0.5">current</span>
                    )}
                  </button>
                ))}
              </div>
            )}

            <div className="flex gap-3">
              <button onClick={() => setShowPHPModal(false)}
                className="flex-1 px-4 py-2 text-sm text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200">
                Cancel
              </button>
              <button onClick={handleUpdatePHP}
                disabled={updatePHP.isPending || selectedPHP === site.php_version || !selectedPHP}
                className="flex-1 px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {updatePHP.isPending ? 'Updating...' : 'Confirm'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
