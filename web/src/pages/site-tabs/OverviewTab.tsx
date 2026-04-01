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
      setSourceResult({ success: true, message: 'Source deployed thành công!', detail: `Đã upload ${mb} MB và extract vào /var/www/${site.domain}/public/` })
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
        ? `Import xong + đã chạy wp search-replace: ${importDBOldDomain} → ${site.domain}. siteurl/home đã cập nhật.`
        : `Database đã import vào ${site.db_name}. siteurl/home đã cập nhật thành https://${site.domain}.`
      setDbResult({ success: true, message: 'Database imported thành công!', detail })
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
        {/* Site Details */}
        <div className="lg:col-span-2 space-y-6">
          {/* WordPress Info */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-lg font-semibold text-gray-900 mb-4">WordPress Info</h2>
            <dl className="space-y-3">
              <DetailRow label="Domain" value={
                <a href={siteBaseUrl} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
                  {site.domain}
                </a>
              } />
              <DetailRow label="WP Version" value={site.wp_version || 'Updating...'} />
              <DetailRow label="Admin User" value={site.wp_admin_user} />
              <DetailRow label="Admin Email" value={site.wp_admin_email} />
              <DetailRow label="WP Admin URL" value={
                <a href={`${siteBaseUrl}/wp-admin`} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
                  {site.domain}/wp-admin
                </a>
              } />
              <DetailRow label="Created" value={formatDate(site.created_at)} />
            </dl>
          </div>

          {/* Credentials */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                <Key size={20} className="text-yellow-500" /> Login Credentials
              </h2>
              <button onClick={() => setShowCreds(!showCreds)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-600 bg-gray-100 rounded-lg hover:bg-gray-200">
                {showCreds ? <EyeOff size={14} /> : <Eye size={14} />}
                {showCreds ? 'Hide' : 'Show'}
              </button>
            </div>
            {showCreds && creds ? (
              <div className="space-y-3">
                <CredRow label="WP Admin URL" value={creds.wp_admin_url} field="wp_url" copiedField={copiedField} onCopy={copyToClipboard} isLink />
                <CredRow label="WP Admin User" value={creds.wp_admin_user} field="wp_user" copiedField={copiedField} onCopy={copyToClipboard} />
                <CredRow label="WP Admin Pass" value={creds.wp_admin_pass} field="wp_pass" copiedField={copiedField} onCopy={copyToClipboard} />
                <div className="border-t border-gray-100 pt-3 mt-3">
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
              <p className="text-sm text-gray-400">Click "Show" to reveal WP Admin and Database credentials.</p>
            )}
          </div>

          {/* Technical Config */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-lg font-semibold text-gray-900 mb-4">Technical Configuration</h2>
            <dl className="space-y-3">
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
              <DetailRow label="SSL" value={
                <div className="flex items-center gap-2">
                  {site.ssl_enabled ? (
                    <span className="flex items-center gap-1 text-green-600"><Lock size={14} /> Active</span>
                  ) : (
                    <>
                      <span className="flex items-center gap-1 text-gray-400"><LockOpen size={14} /> Disabled</span>
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
              {site.ssl_expires_at && (
                <DetailRow label="SSL Expires" value={formatDate(site.ssl_expires_at)} />
              )}
              <DetailRow label="Database Name" value={site.db_name || '-'} />
              {site.linux_user && (
                <DetailRow label="Linux User" value={
                  <code className="text-xs bg-gray-100 px-2 py-0.5 rounded font-mono text-gray-700">{site.linux_user}</code>
                } />
              )}
              <DetailRow label="open_basedir" value={
                <div className="flex items-center gap-3">
                  <button
                    onClick={() => toggleOpenBasedir.mutate(site.id)}
                    disabled={toggleOpenBasedir.isPending || !can('sites', 'manage')}
                    className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-1 disabled:opacity-50 ${
                      (site.open_basedir_enabled ?? true) ? 'bg-emerald-500' : 'bg-gray-300'
                    }`}
                    title={(site.open_basedir_enabled ?? true) ? 'Click to disable open_basedir' : 'Click to enable open_basedir'}
                  >
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                      (site.open_basedir_enabled ?? true) ? 'translate-x-4' : 'translate-x-0.5'
                    }`} />
                  </button>
                  <span className="text-sm text-gray-600">
                    {toggleOpenBasedir.isPending ? (
                      <span className="text-blue-500">Rebuilding PHP-FPM…</span>
                    ) : (site.open_basedir_enabled ?? true) ? (
                      <span className="text-emerald-600 font-medium">Enabled</span>
                    ) : (
                      <span className="text-gray-400">Disabled</span>
                    )}
                  </span>
                  {!(site.open_basedir_enabled ?? true) && (
                    <span className="text-xs text-amber-600 bg-amber-50 border border-amber-200 rounded px-1.5 py-0.5">⚠ PHP can access all paths</span>
                  )}
                </div>
              } />
              <DetailRow label="Server" value={
                <Link to={`/servers/${site.server_id}`} className="text-blue-600 hover:underline">
                  {site.server?.name || `Server #${site.server_id}`}
                </Link>
              } />
            </dl>
          </div>
        </div>

        {/* Actions Panel */}
        <div className="space-y-6">
          {/* Quick Actions */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-lg font-semibold text-gray-900 mb-4">Actions</h2>
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

          {/* Security */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-lg font-semibold text-gray-900 mb-4">Security</h2>
            <div className="space-y-3 text-sm">
              <div className="flex items-center gap-2">
                <Shield size={16} className={site.ssl_enabled ? 'text-green-500' : 'text-gray-300'} />
                <span className={site.ssl_enabled ? 'text-green-700' : 'text-gray-500'}>
                  SSL/HTTPS {site.ssl_enabled ? 'Enabled' : 'Disabled'}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <Shield size={16} className="text-green-500" />
                <span className="text-green-700">File Editor Disabled</span>
              </div>
              <div className="flex items-center gap-2">
                <Shield size={16} className="text-green-500" />
                <span className="text-green-700">Auto Updates Enabled</span>
              </div>
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
                    <div className="flex gap-2">
                      <input type="file" accept=".zip"
                        onChange={(e) => { setUploadSourceFile(e.target.files?.[0] || null); setSourceResult(null); setSourcePhase('idle') }}
                        className="flex-1 text-sm border border-gray-200 rounded-lg px-3 py-1.5 file:mr-3 file:text-xs file:bg-blue-50 file:border-0 file:rounded file:px-2 file:py-1 file:text-blue-700" />
                      <button onClick={handleUploadSource}
                        disabled={!uploadSourceFile}
                        className="px-4 py-1.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-40 whitespace-nowrap flex items-center gap-1.5">
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
                    <div className="flex gap-2">
                      <input type="file" accept=".sql,.gz,.sql.gz,.zip"
                        onChange={(e) => { setImportDBFile(e.target.files?.[0] || null); setDbResult(null); setDbPhase('idle') }}
                        className="flex-1 text-sm border border-gray-200 rounded-lg px-3 py-1.5 file:mr-3 file:text-xs file:bg-emerald-50 file:border-0 file:rounded file:px-2 file:py-1 file:text-emerald-700" />
                      <button onClick={handleImportDB}
                        disabled={!importDBFile}
                        className="px-4 py-1.5 bg-emerald-600 text-white text-sm font-medium rounded-lg hover:bg-emerald-700 disabled:opacity-40 whitespace-nowrap flex items-center gap-1.5">
                        <Database size={14} /> Import
                      </button>
                    </div>
                    {/* Old domain */}
                    <div className="flex gap-2 items-center">
                      <input type="text" value={importDBOldDomain}
                        onChange={(e) => setImportDBOldDomain(e.target.value)}
                        placeholder="Old domain (vd: old-site.com) — để trống nếu không cần đổi URL"
                        className="flex-1 text-sm border border-gray-200 rounded-lg px-3 py-1.5 bg-gray-50 focus:bg-white focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 focus:outline-none" />
                    </div>
                    <p className="text-xs text-gray-400">
                      Import vào <code className="bg-gray-100 px-1 rounded">{site.db_name}</code>
                      {importDBOldDomain.trim() && <> · Sẽ chạy <code className="bg-gray-100 px-1 rounded">wp search-replace</code> {importDBOldDomain.trim()} → {site.domain}</>}
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
                        Chạy wp search-replace: {importDBOldDomain} → {site.domain}
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
            <p className="text-sm text-gray-500 mb-3">Xóa vĩnh viễn site, database, files, vhost Nginx và SSL cert. Không thể hoàn tác!</p>
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
                  <h3 className="text-lg font-bold text-gray-900">Xóa site WordPress</h3>
                  <p className="text-sm text-red-600">Hành động này không thể hoàn tác!</p>
                </div>
              </div>

              {deleteConfirmStep === 1 ? (
                <>
                  <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-5 text-sm text-red-700 space-y-1">
                    <p className="font-semibold">Thao tác này sẽ xóa hoàn toàn:</p>
                    <ul className="list-disc list-inside space-y-0.5 ml-1">
                      <li>Toàn bộ files tại <code className="bg-red-100 px-1 rounded">/var/www/{site.domain}/</code></li>
                      <li>Database <code className="bg-red-100 px-1 rounded">{site.db_name}</code> và user DB</li>
                      <li>Nginx vhost <code className="bg-red-100 px-1 rounded">{site.domain}.conf</code></li>
                      <li>SSL certificate (certbot delete)</li>
                    </ul>
                  </div>
                  <div className="flex gap-3">
                    <button onClick={() => setShowDeleteModal(false)}
                      className="flex-1 px-4 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50">
                      Hủy
                    </button>
                    <button onClick={() => setDeleteConfirmStep(2)}
                      className="flex-1 px-4 py-2 bg-red-600 text-white rounded-lg text-sm hover:bg-red-700 font-medium">
                      Tôi hiểu, tiếp tục →
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <p className="text-sm text-gray-600 mb-3">
                    Để xác nhận, hãy nhập tên domain <span className="font-mono font-bold text-red-600">{site.domain}</span> vào ô bên dưới:
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
                      ← Quay lại
                    </button>
                    <button
                      onClick={handleConfirmDelete}
                      disabled={deleteConfirmText !== site.domain || deleteSite.isPending}
                      className="flex-1 px-4 py-2 bg-red-600 text-white rounded-lg text-sm font-medium hover:bg-red-700 disabled:opacity-40 disabled:cursor-not-allowed">
                      {deleteSite.isPending ? 'Đang xóa...' : 'Xóa vĩnh viễn'}
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
          <div className="bg-white rounded-xl p-6 w-full max-w-md shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-semibold mb-4">Change PHP Version</h3>
            <p className="text-sm text-gray-500 mb-4">Select a new PHP version for {site.domain}</p>
            <div className="grid grid-cols-7 gap-2 mb-6">
              {['7.4', '8.0', '8.1', '8.2', '8.3', '8.4', '8.5'].map(v => (
                <button key={v} onClick={() => setSelectedPHP(v)}
                  className={`px-3 py-2 rounded-lg text-sm font-medium border transition-colors ${
                    selectedPHP === v
                      ? 'bg-blue-600 text-white border-blue-600'
                      : v === site.php_version
                        ? 'bg-gray-100 text-gray-600 border-gray-200'
                        : 'bg-white text-gray-700 border-gray-200 hover:border-blue-300'
                  }`}>
                  {v}
                </button>
              ))}
            </div>
            {selectedPHP === site.php_version && (
              <p className="text-sm text-gray-400 mb-4">Currently using PHP {site.php_version}</p>
            )}
            <div className="flex gap-3">
              <button onClick={() => setShowPHPModal(false)}
                className="flex-1 px-4 py-2 text-sm text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200">
                Cancel
              </button>
              <button onClick={handleUpdatePHP}
                disabled={updatePHP.isPending || selectedPHP === site.php_version}
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
