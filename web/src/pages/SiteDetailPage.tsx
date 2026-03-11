import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft, Globe, Lock, LockOpen, RefreshCw,
  ExternalLink, Trash2, Pause, Play, Shield, Database,
  Code, Clock, Settings, AlertTriangle, Eye, EyeOff, Key, Copy, Check,
  FolderOpen, LayoutDashboard, Layers, Download, Loader2, HardDrive,
  CheckCircle2, XCircle, ChevronLeft, ChevronRight, Network, Plus, ImageIcon, Activity, Zap, Archive
} from 'lucide-react'
import { useSite, useDeleteSite, useUpdatePHP, useRestartSite, useSuspendSite, useResumeSite, useIssueSSL, useSiteCredentials, useAutologin, useUploadSource, useImportDB } from '@/hooks/useSites'
import { useStagingSites, useCreateStaging, usePushStaging, useDeleteStaging, useAutoUpdateConfig, useUpdateAutoUpdateConfig, useCheckUpdates, useUpdateLogs, useOptimizeDatabase } from '@/hooks/useWordPress'
import { useLatestPerformance, usePerformanceMetrics, useRunPageSpeed } from '@/hooks/usePerformance'
import { useSubsites, useEnableMultisite, useCreateSubsite, useDeleteSubsite, useSyncSubsites } from '@/hooks/useMultisite'
import { useImageOptConfig, useUpdateImageOptConfig, useRunImageOpt, useImageOptResults } from '@/hooks/useImageOpt'
import { useDomains, useCreateDomain, useDeleteDomain } from '@/hooks/useDomains'
import { useRedisConfig, useToggleRedis } from '@/hooks/useRedis'
import { useCronJobsBySite } from '@/hooks/useCronJobs'
import { useSFTPAccounts, useCreateSFTP, useDeleteSFTP } from '@/hooks/useSFTP'
import { useHardenWordPress } from '@/hooks/useHardening'
import { useUptimeChecks, useCreateUptimeCheck, useDeleteUptimeCheck, useUptimeHistory } from '@/hooks/useUptime'
import { useWPCLIHistory, useExecuteWPCLI } from '@/hooks/useWPCLI'
import { useCacheConfig, useToggleCache, usePurgeCache } from '@/hooks/useCache'
import { useImports, useStartImport } from '@/hooks/useImport'
import { useSSLStatus, useEnableSSL, useDisableSSL, useRenewSSL, useUploadCustomSSL, useForceHTTPS } from '@/hooks/useSSL'
import { useSiteBackups, useCreateBackup, useDeleteBackup, useRestoreBackup, useSiteBackupSchedules, useCreateBackupSchedule, useUpdateBackupSchedule, useDeleteBackupSchedule } from '@/hooks/useBackups'
import { usePermissions } from '@/hooks/usePermissions'
import { StatusBadge } from '@/components/ui/Badge'
import { formatDate } from '@/lib/utils'
import api from '@/lib/api'
import FileBrowser from '@/components/files/FileBrowser'
import type { StagingSite, UpdateLog, PerformanceMetric, Subsite, ImageOptResult, APIResponse, Backup } from '@/types'

export default function SiteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: site, isLoading, error } = useSite(Number(id))
  const deleteSite = useDeleteSite()
  const updatePHP = useUpdatePHP()
  const restartSite = useRestartSite()
  const suspendSite = useSuspendSite()
  const resumeSite = useResumeSite()
  const issueSSL = useIssueSSL()
  const autologin = useAutologin()
  const uploadSource = useUploadSource()
  const importDB = useImportDB()

  const [activeTab, setActiveTab] = useState<'overview' | 'files' | 'staging' | 'updates' | 'database' | 'performance' | 'multisite' | 'images' | 'domains' | 'redis' | 'cron' | 'sftp' | 'security' | 'monitoring' | 'ssl' | 'wpcli' | 'cache' | 'import' | 'backup'>('overview')
  // Delete confirmation modal state
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmStep, setDeleteConfirmStep] = useState<1 | 2>(1)
  const [deleteConfirmText, setDeleteConfirmText] = useState('')
  const [uploadSourceFile, setUploadSourceFile] = useState<File | null>(null)
  const [importDBFile, setImportDBFile] = useState<File | null>(null)
  const { can } = usePermissions()
  const [showPHPModal, setShowPHPModal] = useState(false)
  const [selectedPHP, setSelectedPHP] = useState('')
  const [actionMsg, setActionMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [showCreds, setShowCreds] = useState(false)
  const [copiedField, setCopiedField] = useState('')

  const { data: creds } = useSiteCredentials(Number(id), showCreds)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
      </div>
    )
  }

  if (error || !site) {
    return (
      <div className="text-center py-20">
        <p className="text-gray-500 mb-4">Site not found</p>
        <Link to="/sites" className="text-blue-600 hover:underline">Back to list</Link>
      </div>
    )
  }

  const showMsg = (type: 'success' | 'error', text: string) => {
    setActionMsg({ type, text })
    setTimeout(() => setActionMsg(null), 4000)
  }

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
      navigate('/sites')
    } catch {
      showMsg('error', 'Failed to delete site')
      setShowDeleteModal(false)
    }
  }

  const handleUploadSource = async () => {
    if (!uploadSourceFile) return
    try {
      await uploadSource.mutateAsync({ siteId: site.id, file: uploadSourceFile })
      showMsg('success', 'Source uploaded! Agent is extracting the files...')
      setUploadSourceFile(null)
    } catch {
      showMsg('error', 'Failed to upload source file')
    }
  }

  const handleImportDB = async () => {
    if (!importDBFile) return
    try {
      await importDB.mutateAsync({ siteId: site.id, file: importDBFile })
      showMsg('success', 'Database file uploaded! Agent is importing...')
      setImportDBFile(null)
    } catch {
      showMsg('error', 'Failed to upload database file')
    }
  }

  const handleRestart = async () => {
    try {
      await restartSite.mutateAsync(site.id)
      showMsg('success', 'Restart command sent for PHP-FPM & Nginx')
    } catch {
      showMsg('error', 'Restart failed')
    }
  }

  const handleSuspend = async () => {
    if (!confirm(`Suspend site ${site.domain}?`)) return
    try {
      await suspendSite.mutateAsync(site.id)
      showMsg('success', 'Site has been suspended')
    } catch {
      showMsg('error', 'Failed to suspend site')
    }
  }

  const handleResume = async () => {
    try {
      await resumeSite.mutateAsync(site.id)
      showMsg('success', 'Site has been resumed')
    } catch {
      showMsg('error', 'Failed to resume site')
    }
  }

  const handleUpdatePHP = async () => {
    if (!selectedPHP || selectedPHP === site.php_version) return
    try {
      await updatePHP.mutateAsync({ id: site.id, version: selectedPHP })
      setShowPHPModal(false)
      showMsg('success', `Switched to PHP ${selectedPHP}`)
    } catch {
      showMsg('error', 'Failed to update PHP version')
    }
  }

  const handleIssueSSL = async () => {
    try {
      await issueSSL.mutateAsync(site.id)
      showMsg('success', 'SSL certificate issued successfully')
    } catch {
      showMsg('error', 'Failed to issue SSL certificate')
    }
  }

  const handleAutologin = async () => {
    try {
      const result = await autologin.mutateAsync(site.id)
      if (result.success && result.data?.url) {
        window.open(result.data.url, '_blank')
      } else {
        showMsg('error', result.error || 'Configuring autologin, please try again in 30 seconds')
      }
    } catch {
      showMsg('error', 'Auto-login failed. Please try again later.')
    }
  }

  const isActive = site.status === 'active'
  const isSuspended = site.status === 'suspended'

  return (
    <div>
      {/* Alert message */}
      {actionMsg && (
        <div className={`mb-4 px-4 py-3 rounded-lg text-sm font-medium ${
          actionMsg.type === 'success' ? 'bg-green-50 text-green-800 border border-green-200' : 'bg-red-50 text-red-800 border border-red-200'
        }`}>
          {actionMsg.text}
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Link to="/sites" className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-lg">
            <ArrowLeft size={20} />
          </Link>
          <div>
            <div className="flex items-center gap-3">
              <Globe size={22} className="text-blue-500" />
              <h1 className="text-2xl font-bold text-gray-900">{site.domain}</h1>
              <StatusBadge status={site.status} />
            </div>
            <p className="text-sm text-gray-500 mt-0.5 ml-[34px]">
              Server: {site.server?.name || `#${site.server_id}`} ({site.server?.ip_address})
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <a href={`https://${site.domain}`} target="_blank" rel="noopener"
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50">
            <ExternalLink size={14} /> Visit
          </a>
          <button onClick={handleAutologin} disabled={autologin.isPending}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
            {autologin.isPending ? (
              <div className="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-white" />
            ) : (
              <Settings size={14} />
            )}
            WP Admin
          </button>
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatCard icon={<Code size={20} />} label="PHP Version" value={`PHP ${site.php_version}`} />
        <StatCard icon={site.ssl_enabled ? <Lock size={20} /> : <LockOpen size={20} />} label="SSL"
          value={site.ssl_enabled ? 'Active' : 'Disabled'}
          sub={site.ssl_expires_at ? `Expires: ${formatDate(site.ssl_expires_at)}` : undefined} />
        <StatCard icon={<Database size={20} />} label="Database" value={site.db_name || '-'} />
        <StatCard icon={<Clock size={20} />} label="WP Version" value={site.wp_version || 'Updating...'} />
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-1 mb-6 border-b border-gray-200 overflow-x-auto">
        <TabButton active={activeTab === 'overview'} onClick={() => setActiveTab('overview')}
          icon={<LayoutDashboard size={16} />} label="Overview" />
        <TabButton active={activeTab === 'files'} onClick={() => setActiveTab('files')}
          icon={<FolderOpen size={16} />} label="Files" />
        <TabButton active={activeTab === 'staging'} onClick={() => setActiveTab('staging')}
          icon={<Layers size={16} />} label="Staging" />
        <TabButton active={activeTab === 'updates'} onClick={() => setActiveTab('updates')}
          icon={<Download size={16} />} label="Updates" />
        <TabButton active={activeTab === 'database'} onClick={() => setActiveTab('database')}
          icon={<HardDrive size={16} />} label="Database" />
        <TabButton active={activeTab === 'performance'} onClick={() => setActiveTab('performance')}
          icon={<Globe size={16} />} label="Performance" />
        <TabButton active={activeTab === 'images'} onClick={() => setActiveTab('images')}
          icon={<ImageIcon size={16} />} label="Images" />
        <TabButton active={activeTab === 'multisite'} onClick={() => setActiveTab('multisite')}
          icon={<Network size={16} />} label="Multisite" />
        <TabButton active={activeTab === 'domains'} onClick={() => setActiveTab('domains')}
          icon={<Globe size={16} />} label="Domains" />
        <TabButton active={activeTab === 'redis'} onClick={() => setActiveTab('redis')}
          icon={<Database size={16} />} label="Redis" />
        <TabButton active={activeTab === 'cron'} onClick={() => setActiveTab('cron')}
          icon={<Clock size={16} />} label="Cron Jobs" />
        <TabButton active={activeTab === 'monitoring'} onClick={() => setActiveTab('monitoring')}
          icon={<Activity size={16} />} label="Monitoring" />
        <TabButton active={activeTab === 'ssl'} onClick={() => setActiveTab('ssl')}
          icon={<Lock size={16} />} label="SSL" />
        <TabButton active={activeTab === 'wpcli'} onClick={() => setActiveTab('wpcli')}
          icon={<Code size={16} />} label="WP-CLI" />
        <TabButton active={activeTab === 'cache'} onClick={() => setActiveTab('cache')}
          icon={<Zap size={16} />} label="Cache" />
        <TabButton active={activeTab === 'sftp'} onClick={() => setActiveTab('sftp')}
          icon={<Key size={16} />} label="SFTP" />
        <TabButton active={activeTab === 'backup'} onClick={() => setActiveTab('backup')}
          icon={<Archive size={16} />} label="Backups" />
        <TabButton active={activeTab === 'security'} onClick={() => setActiveTab('security')}
          icon={<Shield size={16} />} label="Security" />
      </div>

      {/* File Manager Tab */}
      {activeTab === 'files' && (
        <FileBrowser siteId={site.id} />
      )}

      {/* Staging Tab */}
      {activeTab === 'staging' && (
        <StagingPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Updates Tab */}
      {activeTab === 'updates' && (
        <UpdatesPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Database Tab */}
      {activeTab === 'database' && (
        <DatabasePanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Performance Tab */}
      {activeTab === 'performance' && (
        <PerformancePanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Images Tab */}
      {activeTab === 'images' && (
        <ImagesPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Multisite Tab */}
      {activeTab === 'multisite' && (
        <MultisitePanel siteId={site.id} isMultisite={site.is_multisite} canManage={can('sites', 'manage')} />
      )}

      {/* Domain Tab */}
      {activeTab === 'domains' && (
        <DomainsPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Redis Tab */}
      {activeTab === 'redis' && (
        <RedisPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Cron Tab */}
      {activeTab === 'cron' && (
        <CronPanel siteId={site.id} />
      )}

      {/* Monitoring Tab */}
      {activeTab === 'monitoring' && (
        <MonitoringPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* SSL Tab */}
      {activeTab === 'ssl' && (
        <SSLPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />
      )}

      {/* SFTP Tab */}
      {activeTab === 'sftp' && (
        <SFTPPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Backup Tab */}
      {activeTab === 'backup' && (
        <BackupPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />
      )}

      {/* Security Tab */}
      {activeTab === 'security' && (
        <SecurityPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* WP-CLI Tab */}
      {activeTab === 'wpcli' && (
        <WPCLIPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {/* Cache Tab */}
      {activeTab === 'cache' && (
        <CachePanel siteId={site.id} canManage={can('sites', 'manage')} domain={site.domain} />
      )}

      {/* Overview Tab */}
      {activeTab === 'overview' && (
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Site Details */}
        <div className="lg:col-span-2 space-y-6">
          {/* WordPress Info */}
          <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <h2 className="text-lg font-semibold text-gray-900 mb-4">WordPress Info</h2>
            <dl className="space-y-3">
              <DetailRow label="Domain" value={
                <a href={`https://${site.domain}`} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
                  {site.domain}
                </a>
              } />
              <DetailRow label="WP Version" value={site.wp_version || 'Updating...'} />
              <DetailRow label="Admin User" value={site.wp_admin_user} />
              <DetailRow label="Admin Email" value={site.wp_admin_email} />
              <DetailRow label="WP Admin URL" value={
                <a href={`https://${site.domain}/wp-admin`} target="_blank" rel="noopener" className="text-blue-600 hover:underline">
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
            <div className="bg-white rounded-xl shadow-sm border border-blue-100 p-6">
              <h2 className="text-lg font-semibold text-blue-700 mb-1">Deploy Source & Database</h2>
              <p className="text-xs text-gray-500 mb-4">Site này được tạo theo chế độ manual. Upload source zip và import SQL database.</p>
              {/* Upload Source */}
              <div className="mb-4">
                <label className="block text-sm font-medium text-gray-700 mb-1">Source Code (.zip)</label>
                <div className="flex gap-2">
                  <input type="file" accept=".zip"
                    onChange={(e) => setUploadSourceFile(e.target.files?.[0] || null)}
                    className="flex-1 text-sm border border-gray-300 rounded-lg px-3 py-1.5 file:mr-3 file:text-xs file:bg-blue-50 file:border-0 file:rounded file:px-2 file:py-1" />
                  <button onClick={handleUploadSource}
                    disabled={!uploadSourceFile || uploadSource.isPending}
                    className="px-4 py-1.5 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700 disabled:opacity-50 whitespace-nowrap">
                    {uploadSource.isPending ? 'Uploading...' : 'Upload & Deploy'}
                  </button>
                </div>
                <p className="text-xs text-gray-400 mt-1">Max 512 MB. Sẽ extract vào /var/www/{site.domain}/public/</p>
              </div>
              {/* Import DB */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Database (.sql / .gz / .zip)</label>
                <div className="flex gap-2">
                  <input type="file" accept=".sql,.gz,.sql.gz,.zip"
                    onChange={(e) => setImportDBFile(e.target.files?.[0] || null)}
                    className="flex-1 text-sm border border-gray-300 rounded-lg px-3 py-1.5 file:mr-3 file:text-xs file:bg-green-50 file:border-0 file:rounded file:px-2 file:py-1" />
                  <button onClick={handleImportDB}
                    disabled={!importDBFile || importDB.isPending}
                    className="px-4 py-1.5 bg-green-600 text-white text-sm rounded-lg hover:bg-green-700 disabled:opacity-50 whitespace-nowrap">
                    {importDB.isPending ? 'Importing...' : 'Import DB'}
                  </button>
                </div>
                <p className="text-xs text-gray-400 mt-1">Sẽ import vào database: {site.db_name}</p>
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
      )}

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
            <div className="grid grid-cols-5 gap-2 mb-6">
              {['7.4', '8.0', '8.1', '8.2', '8.3'].map(v => (
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
    </div>
  )
}

// --- Staging Panel ---
function StagingPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: stagingData, isLoading } = useStagingSites(siteId)
  const createStaging = useCreateStaging()
  const pushStaging = usePushStaging()
  const deleteStaging = useDeleteStaging()

  const stagings: StagingSite[] = stagingData?.data || []
  const activeStagings = stagings.filter(s => s.status !== 'deleted')

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Staging Environments</h2>
          <p className="text-sm text-gray-500">Clone your production site for testing</p>
        </div>
        {canManage && activeStagings.length === 0 && (
          <button
            onClick={() => createStaging.mutate(siteId)}
            disabled={createStaging.isPending}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
          >
            {createStaging.isPending ? <Loader2 size={14} className="animate-spin" /> : <Layers size={14} />}
            Create Staging
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="text-center py-8 text-gray-500">Loading...</div>
      ) : activeStagings.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-lg border">
          <Layers size={40} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500">No staging environment</p>
          <p className="text-sm text-gray-400">Create a staging copy to test changes safely</p>
        </div>
      ) : (
        <div className="space-y-3">
          {activeStagings.map((staging) => (
            <div key={staging.id} className="bg-white rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">{staging.staging_site?.domain || `Staging #${staging.staging_site_id}`}</p>
                  <p className="text-sm text-gray-500">Created {formatDate(staging.created_at)}</p>
                </div>
                <div className="flex items-center gap-2">
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                    staging.status === 'active' ? 'bg-green-100 text-green-800' :
                    staging.status === 'creating' ? 'bg-blue-100 text-blue-800' :
                    staging.status === 'pushing' ? 'bg-yellow-100 text-yellow-800' :
                    'bg-gray-100 text-gray-800'
                  }`}>{staging.status}</span>
                  {canManage && staging.status === 'active' && (
                    <>
                      <button
                        onClick={() => { if (confirm('Push staging to production? This will overwrite production files.')) pushStaging.mutate(staging.id) }}
                        disabled={pushStaging.isPending}
                        className="px-3 py-1 text-xs bg-yellow-50 text-yellow-700 rounded hover:bg-yellow-100"
                      >Push to Production</button>
                      <button
                        onClick={() => { if (confirm('Delete this staging environment?')) deleteStaging.mutate(staging.id) }}
                        disabled={deleteStaging.isPending}
                        className="px-3 py-1 text-xs bg-red-50 text-red-700 rounded hover:bg-red-100"
                      >Delete</button>
                    </>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// --- Updates Panel ---
function UpdatesPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const [logPage, setLogPage] = useState(1)
  const { data: configData } = useAutoUpdateConfig(siteId)
  const updateConfig = useUpdateAutoUpdateConfig()
  const checkUpdates = useCheckUpdates()
  const { data: logsData } = useUpdateLogs(siteId, logPage)
  const optimizeDb = useOptimizeDatabase()

  const config = configData?.data
  const logs: UpdateLog[] = logsData?.data || []
  const meta = logsData?.meta

  const toggleConfig = (field: string, value: boolean) => {
    updateConfig.mutate({ siteId, [field]: value } as any)
  }

  return (
    <div className="space-y-6">
      {/* Auto-Update Config */}
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold">Auto-Update Settings</h2>
            <p className="text-sm text-gray-500">Configure automatic updates for WordPress components</p>
          </div>
          {canManage && (
            <button
              onClick={() => checkUpdates.mutate(siteId)}
              disabled={checkUpdates.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
            >
              {checkUpdates.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              Check for Updates
            </button>
          )}
        </div>

        {config && (
          <div className="grid grid-cols-2 gap-4">
            <ToggleRow label="WordPress Core Updates" description="Automatically update WordPress core"
              enabled={config.core_updates} canManage={canManage}
              onToggle={() => toggleConfig('core_updates', !config.core_updates)} />
            <ToggleRow label="Plugin Updates" description="Automatically update all plugins"
              enabled={config.plugin_updates} canManage={canManage}
              onToggle={() => toggleConfig('plugin_updates', !config.plugin_updates)} />
            <ToggleRow label="Theme Updates" description="Automatically update all themes"
              enabled={config.theme_updates} canManage={canManage}
              onToggle={() => toggleConfig('theme_updates', !config.theme_updates)} />
            <ToggleRow label="Backup Before Update" description="Create backup before any auto-update"
              enabled={config.backup_before} canManage={canManage}
              onToggle={() => toggleConfig('backup_before', !config.backup_before)} />
          </div>
        )}
      </div>

      {/* Update Logs */}
      <div className="bg-white rounded-lg border p-6">
        <h2 className="text-lg font-semibold mb-4">Update History</h2>
        {logs.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-4">No update logs yet. Click "Check for Updates" to scan.</p>
        ) : (
          <>
            <table className="w-full">
              <thead className="border-b">
                <tr>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Name</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Version</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-gray-500 uppercase">Date</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {logs.map((log) => (
                  <tr key={log.id} className="hover:bg-gray-50">
                    <td className="px-3 py-2 text-sm capitalize">{log.update_type}</td>
                    <td className="px-3 py-2 text-sm font-medium">{log.item_name}</td>
                    <td className="px-3 py-2 text-sm font-mono text-gray-600">
                      {log.from_version && log.to_version ? `${log.from_version} → ${log.to_version}` : '-'}
                    </td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
                        log.status === 'available' ? 'bg-blue-100 text-blue-800' :
                        log.status === 'updated' || log.status === 'success' || log.status === 'completed' ? 'bg-green-100 text-green-800' :
                        log.status === 'failed' ? 'bg-red-100 text-red-800' : 'bg-gray-100 text-gray-800'
                      }`}>
                        {log.status === 'completed' || log.status === 'success' || log.status === 'updated' ? <CheckCircle2 size={10} /> :
                         log.status === 'failed' ? <XCircle size={10} /> : null}
                        {log.status}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-sm text-gray-500">{new Date(log.created_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {meta && meta.total_pages > 1 && (
              <div className="flex items-center justify-between mt-4">
                <p className="text-sm text-gray-500">Page {meta.page} of {meta.total_pages}</p>
                <div className="flex gap-2">
                  <button onClick={() => setLogPage(p => Math.max(1, p - 1))} disabled={logPage <= 1} className="px-2 py-1 text-sm border rounded disabled:opacity-50"><ChevronLeft size={14} /></button>
                  <button onClick={() => setLogPage(p => p + 1)} disabled={logPage >= meta.total_pages} className="px-2 py-1 text-sm border rounded disabled:opacity-50"><ChevronRight size={14} /></button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function ToggleRow({ label, description, enabled, canManage, onToggle }: {
  label: string; description: string; enabled: boolean; canManage: boolean; onToggle: () => void
}) {
  return (
    <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
      <div>
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-gray-500">{description}</p>
      </div>
      <button
        onClick={onToggle}
        disabled={!canManage}
        className={`w-10 h-6 rounded-full transition-colors relative disabled:opacity-50 ${enabled ? 'bg-blue-600' : 'bg-gray-300'}`}
      >
        <span className={`block w-4 h-4 rounded-full bg-white shadow absolute top-1 transition-transform ${enabled ? 'translate-x-5' : 'translate-x-1'}`} />
      </button>
    </div>
  )
}

// --- Database Panel ---
function DatabasePanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const optimizeDb = useOptimizeDatabase()
  const [logPage, setLogPage] = useState(1)
  const { data: logsData } = useUpdateLogs(siteId, logPage)

  const dbLogs = (logsData?.data || []).filter((l: UpdateLog) => l.update_type === 'db_optimize')

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold">Database Optimization</h2>
            <p className="text-sm text-gray-500">Optimize tables, clean revisions, and clear expired transients</p>
          </div>
          {canManage && (
            <button
              onClick={() => optimizeDb.mutate(siteId)}
              disabled={optimizeDb.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm disabled:opacity-50"
            >
              {optimizeDb.isPending ? <Loader2 size={14} className="animate-spin" /> : <HardDrive size={14} />}
              Optimize Now
            </button>
          )}
        </div>

        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="p-4 bg-blue-50 rounded-lg text-center">
            <Database size={24} className="mx-auto text-blue-500 mb-2" />
            <p className="text-sm font-medium">Table Optimization</p>
            <p className="text-xs text-gray-500">OPTIMIZE TABLE on all InnoDB tables</p>
          </div>
          <div className="p-4 bg-green-50 rounded-lg text-center">
            <HardDrive size={24} className="mx-auto text-green-500 mb-2" />
            <p className="text-sm font-medium">Space Recovery</p>
            <p className="text-xs text-gray-500">Reclaim fragmented disk space</p>
          </div>
          <div className="p-4 bg-yellow-50 rounded-lg text-center">
            <RefreshCw size={24} className="mx-auto text-yellow-500 mb-2" />
            <p className="text-sm font-medium">Revision Cleanup</p>
            <p className="text-xs text-gray-500">Remove old post revisions</p>
          </div>
          <div className="p-4 bg-purple-50 rounded-lg text-center">
            <Clock size={24} className="mx-auto text-purple-500 mb-2" />
            <p className="text-sm font-medium">Transient Cleanup</p>
            <p className="text-xs text-gray-500">Clear expired WP transients</p>
          </div>
        </div>
      </div>

      {/* Optimization History */}
      {dbLogs.length > 0 && (
        <div className="bg-white rounded-lg border p-6">
          <h3 className="text-sm font-semibold mb-3">Optimization History</h3>
          <div className="space-y-2">
            {dbLogs.map((log: UpdateLog) => (
              <div key={log.id} className="flex items-center justify-between p-3 bg-gray-50 rounded">
                <div>
                  <p className="text-sm font-medium">{log.item_name}</p>
                  <p className="text-xs text-gray-500">{new Date(log.created_at).toLocaleString()}</p>
                </div>
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                  log.status === 'completed' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                }`}>{log.status}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {optimizeDb.isSuccess && (
        <div className="p-4 bg-green-50 border border-green-200 rounded-lg text-sm text-green-700">
          Database optimization task queued. Results will appear shortly after the agent processes it.
        </div>
      )}
    </div>
  )
}

// --- Performance Panel ---
function PerformancePanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
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

function StatCard({ icon, label, value, sub }: { icon: React.ReactNode; label: string; value: string; sub?: string }) {
  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-4">
      <div className="flex items-center gap-2 text-gray-400 mb-2">
        {icon}
        <span className="text-xs font-medium uppercase">{label}</span>
      </div>
      <p className="text-lg font-semibold text-gray-900">{value}</p>
      {sub && <p className="text-xs text-gray-400 mt-0.5">{sub}</p>}
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between py-2 border-b border-gray-50 last:border-0">
      <dt className="text-sm text-gray-500">{label}</dt>
      <dd className="text-sm font-medium text-gray-900">{value}</dd>
    </div>
  )
}

function CredRow({ label, value, field, copiedField, onCopy, isLink }: {
  label: string; value: string; field: string; copiedField: string
  onCopy: (text: string, field: string) => void; isLink?: boolean
}) {
  const isCopied = copiedField === field
  return (
    <div className="flex items-center justify-between py-2">
      <span className="text-sm text-gray-500">{label}</span>
      <div className="flex items-center gap-2">
        {isLink ? (
          <a href={value} target="_blank" rel="noopener" className="text-sm font-mono text-blue-600 hover:underline">
            {value}
          </a>
        ) : (
          <code className="text-sm font-mono bg-gray-50 px-2 py-0.5 rounded text-gray-800">{value}</code>
        )}
        <button onClick={() => onCopy(value, field)}
          className="p-1 text-gray-400 hover:text-blue-600 rounded">
          {isCopied ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
        </button>
      </div>
    </div>
  )
}

function TabButton({ active, onClick, icon, label }: {
  active: boolean; onClick: () => void; icon: React.ReactNode; label: string
}) {
  return (
    <button onClick={onClick}
      className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors ${
        active
          ? 'text-blue-600 border-blue-600'
          : 'text-gray-500 border-transparent hover:text-gray-700 hover:border-gray-300'
      }`}>
      {icon} {label}
    </button>
  )
}

function ImagesPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: config, isLoading: configLoading } = useImageOptConfig(siteId)
  const updateConfig = useUpdateImageOptConfig(siteId)
  const runOpt = useRunImageOpt(siteId)
  const { data: resultsData } = useImageOptResults(siteId)

  const results = ((resultsData?.data as unknown) ?? []) as ImageOptResult[]

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }

  if (configLoading) return <div className="flex items-center justify-center py-12"><Loader2 size={24} className="animate-spin text-gray-400" /></div>

  return (
    <div className="space-y-4">
      {/* Config Panel */}
      <div className="bg-white rounded-xl border border-gray-200 p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">Image Optimization Settings</h3>
          {canManage && (
            <button onClick={() => runOpt.mutate()} disabled={runOpt.isPending}
              className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {runOpt.isPending ? <Loader2 size={16} className="animate-spin" /> : <ImageIcon size={16} />}
              Run Optimization
            </button>
          )}
        </div>
        {runOpt.isSuccess && (
          <p className="text-green-600 text-sm mb-4">Optimization task queued! Results will appear below when complete.</p>
        )}

        {canManage && config && (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
              <span className="text-sm text-gray-600">WebP Generation</span>
              <button onClick={() => updateConfig.mutate({ webp_enabled: !config.webp_enabled })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${config.webp_enabled ? 'bg-blue-600' : 'bg-gray-300'}`}>
                <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${config.webp_enabled ? 'translate-x-6' : 'translate-x-1'}`} />
              </button>
            </div>
            <div className="p-3 bg-gray-50 rounded-lg">
              <label className="block text-sm text-gray-600 mb-1">Quality ({config.quality}%)</label>
              <input type="range" min="30" max="100" value={config.quality}
                onChange={(e) => updateConfig.mutate({ quality: parseInt(e.target.value) })}
                className="w-full" />
            </div>
            <div className="p-3 bg-gray-50 rounded-lg">
              <label className="block text-sm text-gray-600 mb-1">Max Width</label>
              <select value={config.max_width}
                onChange={(e) => updateConfig.mutate({ max_width: parseInt(e.target.value) })}
                className="w-full px-2 py-1 border rounded text-sm">
                <option value={1920}>1920px</option>
                <option value={2560}>2560px</option>
                <option value={3840}>3840px</option>
              </select>
            </div>
          </div>
        )}
      </div>

      {/* Results History */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b bg-gray-50">
          <h4 className="text-sm font-semibold text-gray-700">Optimization History</h4>
        </div>
        {results.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <ImageIcon size={32} className="mx-auto mb-2 text-gray-300" />
            <p className="text-sm">No optimization runs yet. Click "Run Optimization" to start.</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-gray-50/50">
                <th className="text-left px-4 py-2 font-medium text-gray-600">Date</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Total</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Optimized</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Skipped</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Saved</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {results.map((r: ImageOptResult) => (
                <tr key={r.id} className="hover:bg-gray-50">
                  <td className="px-4 py-2 text-gray-600">{formatDate(r.started_at)}</td>
                  <td className="px-4 py-2">{r.total_images}</td>
                  <td className="px-4 py-2 text-green-600">{r.optimized_count}</td>
                  <td className="px-4 py-2 text-gray-500">{r.skipped_count}</td>
                  <td className="px-4 py-2 font-medium text-blue-600">{formatBytes(r.bytes_saved)}</td>
                  <td className="px-4 py-2">
                    <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
                      r.status === 'completed' ? 'bg-green-100 text-green-700' :
                      r.status === 'running' ? 'bg-blue-100 text-blue-700' :
                      'bg-red-100 text-red-700'
                    }`}>{r.status}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function MultisitePanel({ siteId, isMultisite, canManage }: { siteId: number; isMultisite: boolean; canManage: boolean }) {
  const { data: subsites, isLoading } = useSubsites(siteId, isMultisite)
  const enableMultisite = useEnableMultisite(siteId)
  const createSubsite = useCreateSubsite(siteId)
  const deleteSubsite = useDeleteSubsite(siteId)
  const syncSubsites = useSyncSubsites(siteId)

  const [msType, setMsType] = useState<'subdomain' | 'subfolder'>('subdomain')
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [newSlug, setNewSlug] = useState('')
  const [newTitle, setNewTitle] = useState('')

  const handleEnable = async () => {
    if (!confirm(`Enable WordPress Multisite (${msType} mode)? This cannot be undone.`)) return
    try {
      await enableMultisite.mutateAsync({ type: msType })
    } catch {}
  }

  const handleCreate = async () => {
    if (!newSlug || !newTitle) return
    try {
      await createSubsite.mutateAsync({ slug: newSlug, title: newTitle })
      setNewSlug('')
      setNewTitle('')
      setShowCreateForm(false)
    } catch {}
  }

  const handleDelete = async (subsiteId: number, domain: string) => {
    if (!confirm(`Delete subsite "${domain}"? This cannot be undone.`)) return
    try {
      await deleteSubsite.mutateAsync(subsiteId)
    } catch {}
  }

  // Not multisite yet — show enable form
  if (!isMultisite) {
    return (
      <div className="bg-white rounded-xl border border-gray-200 p-8">
        <div className="max-w-lg mx-auto text-center">
          <Network size={48} className="mx-auto text-gray-400 mb-4" />
          <h3 className="text-lg font-semibold mb-2">Enable WordPress Multisite</h3>
          <p className="text-sm text-gray-500 mb-6">
            Transform this WordPress installation into a multisite network. This allows you to manage multiple sites from a single WordPress dashboard.
          </p>
          {canManage && (
            <div className="space-y-4">
              <div className="flex items-center justify-center gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="radio" name="msType" value="subdomain" checked={msType === 'subdomain'}
                    onChange={() => setMsType('subdomain')} className="text-blue-600" />
                  <span className="text-sm font-medium">Subdomain</span>
                  <span className="text-xs text-gray-400">(sub.domain.com)</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="radio" name="msType" value="subfolder" checked={msType === 'subfolder'}
                    onChange={() => setMsType('subfolder')} className="text-blue-600" />
                  <span className="text-sm font-medium">Subfolder</span>
                  <span className="text-xs text-gray-400">(domain.com/sub/)</span>
                </label>
              </div>
              <button onClick={handleEnable} disabled={enableMultisite.isPending}
                className="inline-flex items-center gap-2 px-6 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {enableMultisite.isPending ? <Loader2 size={16} className="animate-spin" /> : <Network size={16} />}
                Enable Multisite
              </button>
              {enableMultisite.isSuccess && (
                <p className="text-green-600 text-sm">Multisite enable task queued! Refresh after the agent completes the task.</p>
              )}
            </div>
          )}
        </div>
      </div>
    )
  }

  // Multisite enabled — show subsites management
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">Subsites</h3>
        <div className="flex gap-2">
          {canManage && (
            <>
              <button onClick={() => syncSubsites.mutate()} disabled={syncSubsites.isPending}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm border border-gray-300 rounded-lg hover:bg-gray-50 disabled:opacity-50">
                {syncSubsites.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                Sync
              </button>
              <button onClick={() => setShowCreateForm(!showCreateForm)}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700">
                <Plus size={14} /> Add Subsite
              </button>
            </>
          )}
        </div>
      </div>

      {/* Create form */}
      {showCreateForm && (
        <div className="bg-white rounded-xl border border-gray-200 p-4">
          <h4 className="text-sm font-semibold mb-3">Create New Subsite</h4>
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="block text-xs text-gray-500 mb-1">Slug</label>
              <input type="text" value={newSlug} onChange={(e) => setNewSlug(e.target.value)}
                placeholder="blog" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <div className="flex-1">
              <label className="block text-xs text-gray-500 mb-1">Title</label>
              <input type="text" value={newTitle} onChange={(e) => setNewTitle(e.target.value)}
                placeholder="My Blog" className="w-full px-3 py-2 border rounded-lg text-sm" />
            </div>
            <button onClick={handleCreate} disabled={createSubsite.isPending || !newSlug || !newTitle}
              className="px-4 py-2 bg-green-600 text-white text-sm rounded-lg hover:bg-green-700 disabled:opacity-50">
              {createSubsite.isPending ? <Loader2 size={14} className="animate-spin" /> : 'Create'}
            </button>
            <button onClick={() => setShowCreateForm(false)}
              className="px-4 py-2 text-sm border border-gray-300 rounded-lg hover:bg-gray-50">Cancel</button>
          </div>
        </div>
      )}

      {/* Subsites table */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 size={24} className="animate-spin text-gray-400" />
          </div>
        ) : !subsites || subsites.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            <Network size={32} className="mx-auto mb-2 text-gray-300" />
            <p className="text-sm">No subsites yet. Click "Add Subsite" to create one.</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-gray-50">
                <th className="text-left px-4 py-3 font-medium text-gray-600">Blog ID</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Domain / Path</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Title</th>
                <th className="text-left px-4 py-3 font-medium text-gray-600">Status</th>
                {canManage && <th className="text-right px-4 py-3 font-medium text-gray-600">Actions</th>}
              </tr>
            </thead>
            <tbody className="divide-y">
              {subsites.map((sub: Subsite) => (
                <tr key={sub.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-gray-700">{sub.wp_blog_id}</td>
                  <td className="px-4 py-3">
                    <span className="font-mono text-xs">{sub.domain}{sub.path}</span>
                  </td>
                  <td className="px-4 py-3 text-gray-700">{sub.title || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
                      sub.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'
                    }`}>{sub.status}</span>
                  </td>
                  {canManage && (
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => handleDelete(sub.id, sub.domain + sub.path)}
                        disabled={deleteSubsite.isPending}
                        className="text-red-600 hover:text-red-800 text-xs font-medium">
                        Delete
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

// ==================== Domains Panel ====================

function DomainsPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: domains = [], isLoading } = useDomains(siteId)
  const createDomain = useCreateDomain()
  const deleteDomain = useDeleteDomain()
  const [showForm, setShowForm] = useState(false)
  const [newDomain, setNewDomain] = useState('')
  const [newType, setNewType] = useState<'alias' | 'subdomain'>('alias')

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    await createDomain.mutateAsync({ siteId, domain: newDomain, type: newType })
    setNewDomain('')
    setShowForm(false)
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Globe size={20} className="text-blue-500" /> Domain Management
        </h2>
        {canManage && (
          <button onClick={() => setShowForm(!showForm)}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Add Domain
          </button>
        )}
      </div>

      {showForm && (
        <form onSubmit={handleCreate} className="flex gap-3 mb-4 p-4 bg-gray-50 rounded-lg">
          <input type="text" placeholder="example.com" value={newDomain}
            onChange={e => setNewDomain(e.target.value)}
            className="flex-1 px-3 py-2 border rounded-lg text-sm" required />
          <select value={newType} onChange={e => setNewType(e.target.value as 'alias' | 'subdomain')}
            className="px-3 py-2 border rounded-lg text-sm">
            <option value="alias">Alias</option>
            <option value="subdomain">Subdomain</option>
          </select>
          <button type="submit" disabled={createDomain.isPending}
            className="px-4 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
            {createDomain.isPending ? '...' : 'Add'}
          </button>
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : domains.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No domains configured</p>
      ) : (
        <div className="space-y-2">
          {domains.map(d => (
            <div key={d.id} className="flex items-center justify-between p-3 border border-gray-200 rounded-lg">
              <div className="flex items-center gap-3">
                <Globe size={16} className="text-blue-500" />
                <span className="text-sm font-medium text-gray-900">{d.domain}</span>
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                  d.type === 'primary' ? 'bg-blue-100 text-blue-700' : 'bg-gray-100 text-gray-600'
                }`}>{d.type}</span>
              </div>
              {canManage && d.type !== 'primary' && (
                <button onClick={() => deleteDomain.mutate({ siteId, domainId: d.id })}
                  className="p-1 text-gray-400 hover:text-red-600">
                  <Trash2 size={14} />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ==================== Redis Panel ====================

function RedisPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
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

// ==================== Cron Panel ====================

function CronPanel({ siteId }: { siteId: number }) {
  const { data: jobs = [], isLoading } = useCronJobsBySite(siteId)

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
        <Clock size={20} className="text-green-600" /> Cron Jobs
      </h2>
      <p className="text-sm text-gray-500 mb-4">
        Cron jobs associated with this site. Manage all cron jobs from the server detail page.
      </p>

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : jobs.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No cron jobs for this site</p>
      ) : (
        <div className="space-y-3">
          {jobs.map(job => (
            <div key={job.id} className="p-3 border border-gray-200 rounded-lg">
              <div className="flex items-center gap-2 mb-1">
                <code className="text-xs bg-gray-100 px-2 py-0.5 rounded font-mono">{job.schedule}</code>
                <span className={`px-2 py-0.5 rounded text-xs ${job.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                  {job.enabled ? 'Active' : 'Disabled'}
                </span>
              </div>
              <p className="text-sm font-mono text-gray-700">{job.command}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ==================== Monitoring Panel ====================

function MonitoringPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: checks = [], isLoading } = useUptimeChecks(siteId)
  const createCheck = useCreateUptimeCheck(siteId)
  const deleteCheck = useDeleteUptimeCheck(siteId)
  const [showForm, setShowForm] = useState(false)
  const [url, setUrl] = useState('')
  const [selectedCheck, setSelectedCheck] = useState<number | null>(null)
  const { data: history = [] } = useUptimeHistory(siteId, selectedCheck ?? 0)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createCheck.mutateAsync({ url, interval: 60 })
      setUrl('')
      setShowForm(false)
    } catch {}
  }

  const getStatusColor = (code: number) => {
    if (code === 0) return 'bg-gray-100 text-gray-600'
    if (code >= 200 && code < 300) return 'bg-green-100 text-green-700'
    if (code >= 400) return 'bg-red-100 text-red-700'
    return 'bg-yellow-100 text-yellow-700'
  }

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Activity size={20} className="text-blue-500" /> Uptime Monitoring
          </h2>
          {canManage && (
            <button onClick={() => setShowForm(!showForm)}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
              <Plus size={14} /> Add Check
            </button>
          )}
        </div>

        <p className="text-sm text-gray-500 mb-4">
          Monitor your site&apos;s availability with periodic HTTP checks. Get notified when your site goes down.
        </p>

        {showForm && (
          <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 flex gap-3">
            <input value={url} onChange={e => setUrl(e.target.value)}
              className="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm"
              placeholder="https://example.com" required />
            <button type="submit" disabled={createCheck.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createCheck.isPending ? 'Adding...' : 'Add'}
            </button>
            <button type="button" onClick={() => setShowForm(false)}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </form>
        )}

        {isLoading ? (
          <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
        ) : checks.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">No uptime checks configured</p>
        ) : (
          <div className="space-y-3">
            {checks.map(check => (
              <div key={check.id}
                className={`p-4 border rounded-lg cursor-pointer transition-colors ${selectedCheck === check.id ? 'border-blue-300 bg-blue-50' : 'border-gray-100 hover:bg-gray-50'}`}
                onClick={() => setSelectedCheck(selectedCheck === check.id ? null : check.id)}>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`w-3 h-3 rounded-full ${check.last_status >= 200 && check.last_status < 300 ? 'bg-green-500' : check.last_status === 0 && !check.last_check ? 'bg-gray-300' : 'bg-red-500'}`} />
                    <div>
                      <p className="text-sm font-medium text-gray-900">{check.url}</p>
                      <p className="text-xs text-gray-500">
                        Check every {check.interval}s
                        {check.last_check && ` · Last: ${new Date(check.last_check).toLocaleString()}`}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`px-2 py-0.5 rounded text-xs font-medium ${check.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                      {check.enabled ? 'Active' : 'Paused'}
                    </span>
                    {canManage && (
                      <button onClick={(e) => { e.stopPropagation(); if (confirm('Delete this check?')) deleteCheck.mutate(check.id) }}
                        className="p-1 text-gray-400 hover:text-red-600">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Uptime History */}
      {selectedCheck && history.length > 0 && (
        <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
          <h3 className="text-sm font-semibold text-gray-900 mb-3">Recent Check History</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b">
                  <th className="pb-2 font-medium">Time</th>
                  <th className="pb-2 font-medium">Status</th>
                  <th className="pb-2 font-medium">Response</th>
                  <th className="pb-2 font-medium">Error</th>
                </tr>
              </thead>
              <tbody>
                {history.slice(0, 20).map(log => (
                  <tr key={log.id} className="border-b border-gray-50">
                    <td className="py-2 text-gray-600">{new Date(log.created_at).toLocaleString()}</td>
                    <td className="py-2">
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${getStatusColor(log.status_code)}`}>
                        {log.status_code || 'ERR'}
                      </span>
                    </td>
                    <td className="py-2 text-gray-600">{log.response_ms}ms</td>
                    <td className="py-2 text-red-500 text-xs truncate max-w-[200px]">{log.error || '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

// ==================== SFTP Panel ====================

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button onClick={handleCopy} title="Copy"
      className="p-1 text-gray-400 hover:text-blue-600 flex-shrink-0">
      {copied ? <Check size={13} className="text-green-500" /> : <Copy size={13} />}
    </button>
  )
}

function SFTPPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: accounts = [], isLoading } = useSFTPAccounts(siteId)
  const createSFTP = useCreateSFTP(siteId)
  const deleteSFTP = useDeleteSFTP(siteId)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ username: '', password: '' })
  const [showPassword, setShowPassword] = useState(false)
  const [showCredPassword, setShowCredPassword] = useState(false)
  const [newCreds, setNewCreds] = useState<{ host: string; port: number; username: string; password: string } | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError(null)
    try {
      const result = await createSFTP.mutateAsync(form)
      setNewCreds({
        host: result.host || '—',
        port: result.port || 22,
        username: result.username,
        password: form.password,
      })
      setForm({ username: '', password: '' })
      setShowForm(false)
      setShowCredPassword(false)
    } catch (err: any) {
      setCreateError(err?.response?.data?.error || 'Failed to create SFTP account')
    }
  }

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
          <Key size={20} className="text-purple-500" /> SFTP Accounts
        </h2>
        {canManage && (
          <button onClick={() => { setShowForm(!showForm); setCreateError(null) }}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700">
            <Plus size={14} /> Create Account
          </button>
        )}
      </div>

      <p className="text-sm text-gray-500 mb-4">
        SFTP accounts provide chrooted file access to this site. Users are restricted to the site directory. Port: <span className="font-mono font-medium">22</span>.
      </p>

      {/* Credential Card — shown once after creation */}
      {newCreds && (
        <div className="mb-4 p-4 bg-green-50 border border-green-200 rounded-xl space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-green-800 flex items-center gap-2">
              <Check size={15} className="text-green-600" /> SFTP Account Created — Save these credentials!
            </h3>
            <button onClick={() => setNewCreds(null)} className="text-xs text-green-600 hover:text-green-800 underline">Dismiss</button>
          </div>
          <p className="text-xs text-green-600">⚠️ The password will not be shown again. Copy it now.</p>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-sm">
            {[
              { label: 'Host', value: newCreds.host },
              { label: 'Port', value: String(newCreds.port) },
              { label: 'Username', value: newCreds.username },
            ].map(({ label, value }) => (
              <div key={label} className="flex items-center gap-2 bg-white border border-green-200 rounded-lg px-3 py-2">
                <span className="text-xs text-gray-500 w-20 flex-shrink-0">{label}</span>
                <span className="font-mono text-gray-900 flex-1 truncate">{value}</span>
                <CopyButton value={value} />
              </div>
            ))}
            <div className="flex items-center gap-2 bg-white border border-green-200 rounded-lg px-3 py-2">
              <span className="text-xs text-gray-500 w-20 flex-shrink-0">Password</span>
              <span className="font-mono text-gray-900 flex-1 truncate">
                {showCredPassword ? newCreds.password : '••••••••••••'}
              </span>
              <button onClick={() => setShowCredPassword(v => !v)} className="p-1 text-gray-400 hover:text-gray-600 flex-shrink-0">
                {showCredPassword ? <EyeOff size={13} /> : <Eye size={13} />}
              </button>
              <CopyButton value={newCreds.password} />
            </div>
          </div>
          <div className="text-xs text-green-700 font-mono bg-green-100 rounded px-3 py-2">
            sftp {newCreds.username}@{newCreds.host}
          </div>
        </div>
      )}

      {showForm && (
        <form onSubmit={handleCreate} className="bg-gray-50 rounded-lg p-4 mb-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
              <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm"
                placeholder="sftp_mysite" required />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
              <div className="relative">
                <input type={showPassword ? 'text' : 'password'}
                  value={form.password} onChange={e => setForm({ ...form, password: e.target.value })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm pr-10"
                  placeholder="Strong password" required />
                <button type="button" onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-gray-400 hover:text-gray-600">
                  {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
            </div>
          </div>
          {createError && (
            <p className="text-xs text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2">{createError}</p>
          )}
          <div className="flex gap-2">
            <button type="submit" disabled={createSFTP.isPending}
              className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
              {createSFTP.isPending ? 'Creating...' : 'Create'}
            </button>
            <button type="button" onClick={() => { setShowForm(false); setCreateError(null) }}
              className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
              Cancel
            </button>
          </div>
        </form>
      )}

      {isLoading ? (
        <div className="flex justify-center py-8"><div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" /></div>
      ) : accounts.length === 0 ? (
        <p className="text-sm text-gray-400 text-center py-8">No SFTP accounts configured</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500 border-b">
                <th className="pb-2 font-medium">Username</th>
                <th className="pb-2 font-medium">Home Directory</th>
                <th className="pb-2 font-medium">Created</th>
                <th className="pb-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {accounts.map(acc => (
                <tr key={acc.id} className="border-b border-gray-50">
                  <td className="py-3 font-mono font-medium">{acc.username}</td>
                  <td className="py-3 font-mono text-gray-500 text-xs">{acc.home_dir}</td>
                  <td className="py-3 text-gray-500">{new Date(acc.created_at).toLocaleDateString()}</td>
                  <td className="py-3">
                    {canManage && (
                      <button onClick={() => { if (confirm(`Delete SFTP account "${acc.username}"?`)) deleteSFTP.mutate(acc.id) }}
                        className="p-1 text-gray-400 hover:text-red-600">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ==================== Security Panel ====================

function SecurityPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const harden = useHardenWordPress(siteId)
  const [options, setOptions] = useState({
    disable_xmlrpc: true,
    hide_wp_version: true,
    secure_file_perms: true,
    security_headers: true,
    disable_file_editor: true,
  })

  const handleHarden = async () => {
    if (!confirm('Apply security hardening? This will modify server configurations.')) return
    try {
      await harden.mutateAsync(options)
    } catch {}
  }

  const toggleOption = (key: keyof typeof options) => {
    setOptions(prev => ({ ...prev, [key]: !prev[key] }))
  }

  const hardeningOptions = [
    { key: 'disable_xmlrpc' as const, label: 'Disable XML-RPC', desc: 'Block xmlrpc.php to prevent brute-force and DDoS attacks' },
    { key: 'hide_wp_version' as const, label: 'Hide WordPress Version', desc: 'Remove WP version meta tag from page source' },
    { key: 'secure_file_perms' as const, label: 'Secure File Permissions', desc: 'Set directories to 755, files to 644, wp-config.php to 600' },
    { key: 'security_headers' as const, label: 'Add Security Headers', desc: 'Add X-Content-Type-Options, X-Frame-Options, XSS-Protection headers' },
    { key: 'disable_file_editor' as const, label: 'Disable File Editor', desc: 'Disable the built-in WordPress theme/plugin editor' },
  ]

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
      <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-4">
        <Shield size={20} className="text-green-600" /> WordPress Security Hardening
      </h2>

      <p className="text-sm text-gray-500 mb-6">
        Apply security hardening measures to protect your WordPress site. Select the options below and click apply.
      </p>

      <div className="space-y-3 mb-6">
        {hardeningOptions.map(opt => (
          <label key={opt.key}
            className={`flex items-start gap-3 p-4 border rounded-lg cursor-pointer transition-colors ${
              options[opt.key] ? 'border-green-200 bg-green-50' : 'border-gray-200 hover:bg-gray-50'
            }`}>
            <input type="checkbox" checked={options[opt.key]} onChange={() => toggleOption(opt.key)}
              className="mt-0.5 h-4 w-4 text-green-600 rounded border-gray-300 focus:ring-green-500" />
            <div>
              <p className="text-sm font-medium text-gray-900">{opt.label}</p>
              <p className="text-xs text-gray-500 mt-0.5">{opt.desc}</p>
            </div>
          </label>
        ))}
      </div>

      {canManage && (
        <button onClick={handleHarden} disabled={harden.isPending || !Object.values(options).some(v => v)}
          className="flex items-center gap-2 px-4 py-2.5 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
          <Shield size={16} />
          {harden.isPending ? 'Applying...' : 'Apply Hardening'}
        </button>
      )}

      {harden.isSuccess && (
        <div className="mt-4 p-3 bg-green-50 border border-green-200 rounded-lg">
          <p className="text-sm text-green-700">Hardening task queued successfully. Changes will be applied shortly.</p>
        </div>
      )}

      {harden.isError && (
        <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded-lg">
          <p className="text-sm text-red-700">Failed to apply hardening. Please try again.</p>
        </div>
      )}
    </div>
  )
}

// ==================== WP-CLI Panel ====================

function WPCLIPanel({ siteId, canManage }: { siteId: number; canManage: boolean }) {
  const { data: history = [], isLoading } = useWPCLIHistory(siteId)
  const executeCmd = useExecuteWPCLI(siteId)
  const [command, setCommand] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!command.trim()) return
    await executeCmd.mutateAsync({ command: command.trim() })
    setCommand('')
  }

  return (
    <div className="space-y-4">
      <div className="bg-gray-900 rounded-xl p-6">
        <div className="flex items-center gap-2 mb-4">
          <Code size={18} className="text-green-400" />
          <h2 className="text-lg font-semibold text-white">WP-CLI Terminal</h2>
        </div>

        {/* Command output area */}
        <div className="bg-black rounded-lg p-4 mb-4 max-h-96 overflow-y-auto font-mono text-sm space-y-3">
          {isLoading ? (
            <p className="text-gray-500">Loading history...</p>
          ) : history.length === 0 ? (
            <p className="text-gray-500">No commands executed yet. Type a WP-CLI command below.</p>
          ) : (
            [...history].reverse().map((entry) => (
              <div key={entry.id} className="border-b border-gray-800 pb-2">
                <div className="flex items-center gap-2">
                  <span className="text-green-400">$</span>
                  <span className="text-blue-300">wp {entry.command}</span>
                  <span className="ml-auto text-xs text-gray-600">{new Date(entry.created_at).toLocaleString()}</span>
                </div>
                {entry.status === 'running' ? (
                  <div className="flex items-center gap-2 mt-1 text-yellow-400 text-xs">
                    <Loader2 size={12} className="animate-spin" /> Running...
                  </div>
                ) : (
                  <pre className={`mt-1 whitespace-pre-wrap text-xs ${entry.exit_code === 0 ? 'text-gray-300' : 'text-red-400'}`}>
                    {entry.output || '(no output)'}
                  </pre>
                )}
              </div>
            ))
          )}
        </div>

        {/* Command input */}
        {canManage && (
          <form onSubmit={handleSubmit} className="flex items-center gap-2">
            <span className="text-green-400 font-mono text-sm">wp</span>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="plugin list, cache flush, option get siteurl..."
              className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white font-mono placeholder-gray-500 focus:outline-none focus:border-green-500"
              disabled={executeCmd.isPending}
            />
            <button
              type="submit"
              disabled={executeCmd.isPending || !command.trim()}
              className="px-4 py-2 bg-green-600 text-white rounded-lg text-sm hover:bg-green-700 disabled:opacity-50 flex items-center gap-1.5"
            >
              {executeCmd.isPending ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              Run
            </button>
          </form>
        )}
      </div>

      <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
        <div className="flex items-center gap-2 text-yellow-800 mb-2">
          <AlertTriangle size={16} />
          <p className="text-sm font-medium">Security Notice</p>
        </div>
        <p className="text-xs text-yellow-700">
          Commands like <code className="bg-yellow-100 px-1 rounded">eval</code>, <code className="bg-yellow-100 px-1 rounded">shell</code>, <code className="bg-yellow-100 px-1 rounded">db export</code>, and <code className="bg-yellow-100 px-1 rounded">db import</code> are blocked for security reasons.
          The <code className="bg-yellow-100 px-1 rounded">wp</code> prefix is added automatically.
        </p>
      </div>
    </div>
  )
}

// ==================== Cache Panel ====================

function CachePanel({ siteId, canManage, domain }: { siteId: number; canManage: boolean; domain?: string }) {
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

// ==================== SSL Panel ====================

function SSLPanel({ siteId, domain, canManage }: { siteId: number; domain: string; canManage: boolean }) {
  const { data: status, isLoading, refetch: refetchStatus } = useSSLStatus(siteId)
  const enableSSL = useEnableSSL(siteId)
  const disableSSL = useDisableSSL(siteId)
  const renewSSL = useRenewSSL(siteId)
  const uploadSSL = useUploadCustomSSL(siteId)
  const forceHTTPS = useForceHTTPS(siteId)
  const [showUpload, setShowUpload] = useState(false)
  const [certForm, setCertForm] = useState({ cert_pem: '', key_pem: '', ca_pem: '' })
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const showMsg = (type: 'success' | 'error', text: string, ms = 6000) => {
    setMsg({ type, text })
    setTimeout(() => setMsg(null), ms)
  }

  const handleEnable = async () => {
    try {
      const res = await enableSSL.mutateAsync()
      showMsg('success', res.message || 'SSL certificate issued successfully! Site now serves HTTPS.')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to enable SSL. Check certbot logs.')
    }
  }

  const handleDisable = async () => {
    if (!confirm(`Remove SSL from ${domain}? The site will revert to HTTP only.`)) return
    try {
      const res = await disableSSL.mutateAsync()
      showMsg('success', res.message || 'SSL disabled. Site now serves HTTP only.')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to disable SSL.')
    }
  }

  const handleRenew = async () => {
    try {
      const res = await renewSSL.mutateAsync()
      showMsg('success', res.message || 'SSL certificate renewed successfully!')
      refetchStatus()
    } catch (err: any) {
      showMsg('error', err?.response?.data?.error || 'Failed to renew SSL.')
    }
  }

  const handleForceHTTPS = async (enable: boolean) => {
    try {
      await forceHTTPS.mutateAsync(enable)
      showMsg('success', enable ? 'Force HTTPS enabled — HTTP traffic will redirect to HTTPS.' : 'Force HTTPS disabled.')
      refetchStatus()
    } catch {
      showMsg('error', 'Failed to update Force HTTPS setting')
    }
  }

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await uploadSSL.mutateAsync(certForm)
      showMsg('success', 'Custom SSL certificate uploaded successfully')
      setShowUpload(false)
      setCertForm({ cert_pem: '', key_pem: '', ca_pem: '' })
    } catch {
      showMsg('error', 'Failed to upload SSL certificate')
    }
  }

  const anyPending = enableSSL.isPending || disableSSL.isPending || renewSSL.isPending

  if (isLoading) {
    return <div className="flex items-center justify-center py-12"><Loader2 size={24} className="animate-spin text-gray-400" /></div>
  }

  const isEnabled = status?.ssl_enabled ?? false
  const expiresAt = status?.ssl_expires_at

  // Calculate days until expiry
  let daysLeft = 0
  let expiryColor = 'text-green-600'
  if (expiresAt) {
    daysLeft = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
    if (daysLeft <= 7) expiryColor = 'text-red-600'
    else if (daysLeft <= 30) expiryColor = 'text-yellow-600'
  }

  return (
    <div className="space-y-6">
      {msg && (
        <div className={`p-3 rounded-lg text-sm ${msg.type === 'success' ? 'bg-green-50 text-green-700 border border-green-200' : 'bg-red-50 text-red-700 border border-red-200'}`}>
          {msg.text}
        </div>
      )}

      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Lock size={20} className="text-green-500" /> SSL Certificate
          </h2>
          {canManage && (
            <div className="flex gap-2 flex-wrap">
              <button onClick={() => setShowUpload(!showUpload)} disabled={anyPending}
                className="flex items-center gap-1.5 px-3 py-2 text-sm border border-gray-200 rounded-lg hover:bg-gray-50 disabled:opacity-50">
                <Key size={14} /> Custom SSL
              </button>
              {!isEnabled ? (
                <button onClick={handleEnable} disabled={anyPending}
                  className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-green-600 rounded-lg hover:bg-green-700 disabled:opacity-50">
                  {enableSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <Shield size={14} />}
                  {enableSSL.isPending ? 'Issuing SSL…' : 'Enable SSL'}
                </button>
              ) : (
                <>
                  <button onClick={handleRenew} disabled={anyPending}
                    className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                    {renewSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                    {renewSSL.isPending ? 'Renewing…' : 'Renew SSL'}
                  </button>
                  <button onClick={handleDisable} disabled={anyPending}
                    className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
                    {disableSSL.isPending ? <Loader2 size={14} className="animate-spin" /> : <LockOpen size={14} />}
                    {disableSSL.isPending ? 'Disabling…' : 'Disable SSL'}
                  </button>
                </>
              )}
            </div>
          )}
        </div>

        {anyPending && (
          <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-lg text-sm text-blue-700 flex items-center gap-2">
            <Loader2 size={14} className="animate-spin" />
            {enableSSL.isPending ? 'Issuing SSL certificate via Let\'s Encrypt… this may take up to 60 seconds.' :
             disableSSL.isPending ? 'Removing SSL certificate and updating nginx config…' :
             'Renewing SSL certificate… this may take up to 60 seconds.'}
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
          <div className={`p-4 rounded-lg ${isEnabled ? 'bg-green-50 border border-green-200' : 'bg-red-50 border border-red-200'}`}>
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Status</p>
            <div className="flex items-center gap-2">
              {isEnabled ? <CheckCircle2 size={20} className="text-green-500" /> : <XCircle size={20} className="text-red-500" />}
              <p className={`text-lg font-semibold ${isEnabled ? 'text-green-600' : 'text-red-600'}`}>
                {isEnabled ? 'Active' : 'Disabled'}
              </p>
            </div>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Domain</p>
            <p className="text-lg font-semibold text-gray-900">{domain}</p>
          </div>
          <div className="p-4 bg-gray-50 rounded-lg">
            <p className="text-xs text-gray-500 uppercase font-medium mb-1">Expires</p>
            {expiresAt ? (
              <div>
                <p className={`text-lg font-semibold ${expiryColor}`}>{daysLeft} days left</p>
                <p className="text-xs text-gray-500">{new Date(expiresAt).toLocaleDateString()}</p>
              </div>
            ) : (
              <p className="text-lg font-semibold text-gray-400">N/A</p>
            )}
          </div>
        </div>

        {/* Force HTTPS toggle — only shown when SSL is active */}
        {isEnabled && (() => {
          const isForceHTTPS = status?.force_https ?? false
          return (
            <div className="flex items-center justify-between p-4 bg-gray-50 rounded-lg border border-gray-200">
              <div>
                <p className="text-sm font-medium text-gray-900">Force HTTPS</p>
                <p className="text-xs text-gray-500 mt-0.5">Redirect all HTTP traffic to HTTPS via Nginx</p>
              </div>
              {canManage ? (
                <div className="flex items-center gap-3">
                  {forceHTTPS.isPending ? (
                    <Loader2 size={16} className="animate-spin text-gray-400" />
                  ) : (
                    <span className={`text-xs font-medium ${isForceHTTPS ? 'text-green-600' : 'text-gray-400'}`}>
                      {isForceHTTPS ? 'Enabled' : 'Disabled'}
                    </span>
                  )}
                  <button
                    onClick={() => handleForceHTTPS(!isForceHTTPS)}
                    disabled={forceHTTPS.isPending}
                    className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors disabled:opacity-50 focus:outline-none ${
                      isForceHTTPS ? 'bg-green-500' : 'bg-gray-300'
                    }`}
                    role="switch"
                    aria-checked={isForceHTTPS}>
                    <span
                      className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                        isForceHTTPS ? 'translate-x-6' : 'translate-x-1'
                      }`}
                    />
                  </button>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <span className={`text-xs font-medium ${isForceHTTPS ? 'text-green-600' : 'text-gray-400'}`}>
                    {isForceHTTPS ? 'Enabled' : 'Disabled'}
                  </span>
                  <div className={`relative inline-flex h-6 w-11 items-center rounded-full ${isForceHTTPS ? 'bg-green-500' : 'bg-gray-300'} opacity-50`}>
                    <span className={`inline-block h-4 w-4 transform rounded-full bg-white shadow ${isForceHTTPS ? 'translate-x-6' : 'translate-x-1'}`} />
                  </div>
                </div>
              )}
            </div>
          )
        })()}
      </div>

      {/* Custom SSL Upload Form */}
      {showUpload && (
        <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
          <h3 className="text-md font-semibold text-gray-900 mb-4">Upload Custom SSL Certificate</h3>
          <form onSubmit={handleUpload} className="space-y-4">
            <div>
              <label className="block text-sm text-gray-600 mb-1">Certificate (PEM)</label>
              <textarea value={certForm.cert_pem} onChange={e => setCertForm({...certForm, cert_pem: e.target.value})}
                placeholder="-----BEGIN CERTIFICATE-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" required />
            </div>
            <div>
              <label className="block text-sm text-gray-600 mb-1">Private Key (PEM)</label>
              <textarea value={certForm.key_pem} onChange={e => setCertForm({...certForm, key_pem: e.target.value})}
                placeholder="-----BEGIN PRIVATE KEY-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" required />
            </div>
            <div>
              <label className="block text-sm text-gray-600 mb-1">CA Bundle (optional)</label>
              <textarea value={certForm.ca_pem} onChange={e => setCertForm({...certForm, ca_pem: e.target.value})}
                placeholder="-----BEGIN CERTIFICATE-----&#10;..."
                className="w-full h-28 px-3 py-2 border rounded-lg text-sm font-mono resize-none" />
            </div>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowUpload(false)} className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-lg">Cancel</button>
              <button type="submit" disabled={uploadSSL.isPending}
                className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {uploadSSL.isPending ? 'Uploading...' : 'Upload Certificate'}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}

// ==================== Backup Panel ====================

const BACKUP_CRON_PRESETS = [
  { label: 'Daily at 3:00 AM', value: '0 3 * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Weekly (Sunday 3am)', value: '0 3 * * 0' },
  { label: 'Monthly (1st 3am)', value: '0 3 1 * *' },
  { label: 'Custom', value: 'custom' },
]

function formatBytes(bytes: number): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function BackupPanel({ siteId, domain, canManage }: { siteId: number; domain: string; canManage: boolean }) {
  const [page, setPage] = useState(1)
  const { data: backupsData, isLoading: loadingBackups } = useSiteBackups(siteId, page)
  const { data: schedules = [], isLoading: loadingSchedules } = useSiteBackupSchedules(siteId)
  const createBackup = useCreateBackup()
  const deleteBackup = useDeleteBackup()
  const restoreBackup = useRestoreBackup()
  const createSchedule = useCreateBackupSchedule()
  const updateSchedule = useUpdateBackupSchedule()
  const deleteSchedule = useDeleteBackupSchedule()

  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null)
  const [restoreConfirm, setRestoreConfirm] = useState('')
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  // Schedule form
  const [showSchedForm, setShowSchedForm] = useState(false)
  const [schedForm, setSchedForm] = useState({
    type: 'full' as 'full' | 'database_only',
    cron_expr: '0 3 * * *',
    retention: 7,
  })
  const [customCron, setCustomCron] = useState(false)

  const showMsg = (type: 'success' | 'error', text: string) => {
    setMsg({ type, text })
    setTimeout(() => setMsg(null), 5000)
  }

  const handleCreateBackup = async (type: 'full' | 'database_only') => {
    try {
      await createBackup.mutateAsync({ siteId, type })
      showMsg('success', 'Backup started — it will appear in the list once complete.')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to start backup')
    }
  }

  const handleRestore = async () => {
    if (!restoreTarget || restoreConfirm !== domain) return
    try {
      await restoreBackup.mutateAsync({ siteId, backup_id: restoreTarget.id, confirm_domain: domain })
      showMsg('success', 'Restore started successfully')
      setRestoreTarget(null)
      setRestoreConfirm('')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to restore backup')
    }
  }

  const handleCreateSchedule = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createSchedule.mutateAsync({ siteId, ...schedForm })
      setShowSchedForm(false)
      setSchedForm({ type: 'full', cron_expr: '0 3 * * *', retention: 7 })
      setCustomCron(false)
      showMsg('success', 'Backup schedule created')
    } catch (e: any) {
      showMsg('error', e?.response?.data?.error || 'Failed to create schedule')
    }
  }

  const backups = backupsData?.data ?? []
  const meta = backupsData?.meta
  const totalPages = meta ? Math.ceil(meta.total / meta.per_page) : 1

  return (
    <div className="space-y-6">
      {/* Flash message */}
      {msg && (
        <div className={`p-3 rounded-lg text-sm border ${msg.type === 'success'
          ? 'bg-green-50 text-green-700 border-green-200'
          : 'bg-red-50 text-red-700 border-red-200'}`}>
          {msg.text}
        </div>
      )}

      {/* ── Manual Backup ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2 mb-1">
          <Archive size={20} className="text-blue-500" /> Create Backup
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Create an on-demand backup. Files are stored on the server and can be restored at any time.
        </p>
        {canManage ? (
          <div className="flex flex-wrap gap-3">
            <button
              onClick={() => handleCreateBackup('full')}
              disabled={createBackup.isPending}
              className="flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors">
              {createBackup.isPending
                ? <Loader2 size={16} className="animate-spin" />
                : <Archive size={16} />}
              Full Backup (Files + DB)
            </button>
            <button
              onClick={() => handleCreateBackup('database_only')}
              disabled={createBackup.isPending}
              className="flex items-center gap-2 px-4 py-2.5 bg-gray-100 text-gray-700 text-sm font-medium rounded-lg hover:bg-gray-200 disabled:opacity-50 transition-colors">
              {createBackup.isPending
                ? <Loader2 size={16} className="animate-spin" />
                : <Database size={16} />}
              Database Only
            </button>
          </div>
        ) : (
          <p className="text-sm text-gray-400">You don't have permission to create backups.</p>
        )}
      </div>

      {/* ── Backup History ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">Backup History</h2>

        {loadingBackups ? (
          <div className="flex justify-center py-10">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" />
          </div>
        ) : backups.length === 0 ? (
          <div className="text-center py-10">
            <Archive size={36} className="text-gray-300 mx-auto mb-2" />
            <p className="text-sm text-gray-400">No backups yet. Create one above.</p>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-gray-500 border-b">
                    <th className="pb-2 font-medium">Type</th>
                    <th className="pb-2 font-medium">Status</th>
                    <th className="pb-2 font-medium">Size</th>
                    <th className="pb-2 font-medium">Created</th>
                    <th className="pb-2 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {backups.map(backup => (
                    <tr key={backup.id} className="border-b border-gray-50 hover:bg-gray-50/50">
                      <td className="py-3">
                        <span className={`inline-flex px-2 py-0.5 rounded-md text-xs font-medium ${
                          backup.type === 'full'
                            ? 'bg-blue-100 text-blue-700'
                            : backup.type === 'database_only'
                            ? 'bg-purple-100 text-purple-700'
                            : 'bg-gray-100 text-gray-600'
                        }`}>
                          {backup.type === 'full' ? 'Full' : backup.type === 'database_only' ? 'DB Only' : 'Incremental'}
                        </span>
                      </td>
                      <td className="py-3">
                        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium ${
                          backup.status === 'completed'
                            ? 'bg-green-100 text-green-700'
                            : backup.status === 'running'
                            ? 'bg-amber-100 text-amber-700'
                            : 'bg-red-100 text-red-700'
                        }`}>
                          {backup.status === 'running' && <Loader2 size={10} className="animate-spin" />}
                          {backup.status === 'completed' ? 'Completed'
                            : backup.status === 'running' ? 'Running'
                            : 'Failed'}
                        </span>
                        {backup.error && (
                          <p className="text-xs text-red-500 mt-0.5 max-w-xs truncate" title={backup.error}>
                            {backup.error}
                          </p>
                        )}
                      </td>
                      <td className="py-3 text-gray-600 tabular-nums">{formatBytes(backup.size_bytes)}</td>
                      <td className="py-3 text-gray-500 text-xs">
                        {new Date(backup.created_at).toLocaleString()}
                      </td>
                      <td className="py-3">
                        <div className="flex items-center gap-2 justify-end">
                          {backup.status === 'completed' && canManage && (
                            <button
                              onClick={() => { setRestoreTarget(backup); setRestoreConfirm('') }}
                              className="px-2.5 py-1 text-xs font-medium text-blue-600 border border-blue-200 rounded hover:bg-blue-50 transition-colors">
                              Restore
                            </button>
                          )}
                          {canManage && (
                            <button
                              onClick={() => {
                                if (confirm('Delete this backup? This cannot be undone.'))
                                  deleteBackup.mutate(backup.id)
                              }}
                              className="p-1 text-gray-400 hover:text-red-600 transition-colors">
                              <Trash2 size={14} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between mt-4 pt-4 border-t border-gray-100 text-sm text-gray-500">
                <span>Page {page} of {totalPages} · {meta?.total} total backups</span>
                <div className="flex gap-2">
                  <button
                    onClick={() => setPage(p => Math.max(1, p - 1))}
                    disabled={page === 1}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronLeft size={14} />
                  </button>
                  <button
                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                    disabled={page >= totalPages}
                    className="p-1.5 border rounded-lg disabled:opacity-40 hover:bg-gray-50">
                    <ChevronRight size={14} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* ── Scheduled Backups ── */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Clock size={20} className="text-orange-500" /> Backup Schedules
          </h2>
          {canManage && (
            <button
              onClick={() => setShowSchedForm(v => !v)}
              className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors">
              <Plus size={14} /> Add Schedule
            </button>
          )}
        </div>

        {/* Schedule creation form */}
        {showSchedForm && (
          <form onSubmit={handleCreateSchedule} className="bg-gray-50 rounded-lg p-4 mb-5 space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Backup Type</label>
                <select
                  value={schedForm.type}
                  onChange={e => setSchedForm({ ...schedForm, type: e.target.value as 'full' | 'database_only' })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                  <option value="full">Full (Files + DB)</option>
                  <option value="database_only">Database Only</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Schedule Preset</label>
                <select
                  value={customCron ? 'custom' : (BACKUP_CRON_PRESETS.find(p => p.value === schedForm.cron_expr) ? schedForm.cron_expr : 'custom')}
                  onChange={e => {
                    if (e.target.value === 'custom') {
                      setCustomCron(true)
                    } else {
                      setCustomCron(false)
                      setSchedForm({ ...schedForm, cron_expr: e.target.value })
                    }
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm bg-white">
                  {BACKUP_CRON_PRESETS.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-600 mb-1">Keep Backups (days)</label>
                <input
                  type="number" min={1} max={365}
                  value={schedForm.retention}
                  onChange={e => setSchedForm({ ...schedForm, retention: Number(e.target.value) })}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm" />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1">Cron Expression</label>
              <input
                value={schedForm.cron_expr}
                onChange={e => { setCustomCron(true); setSchedForm({ ...schedForm, cron_expr: e.target.value }) }}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono"
                placeholder="0 3 * * *"
                required />
              <p className="text-xs text-gray-400 mt-1">Format: minute hour day-of-month month day-of-week</p>
            </div>
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={createSchedule.isPending}
                className="px-4 py-2 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50">
                {createSchedule.isPending ? 'Creating...' : 'Create Schedule'}
              </button>
              <button
                type="button"
                onClick={() => { setShowSchedForm(false); setCustomCron(false) }}
                className="px-4 py-2 text-sm bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300">
                Cancel
              </button>
            </div>
          </form>
        )}

        {loadingSchedules ? (
          <div className="flex justify-center py-6">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
          </div>
        ) : schedules.length === 0 ? (
          <div className="text-center py-8">
            <Clock size={30} className="text-gray-300 mx-auto mb-2" />
            <p className="text-sm text-gray-400">No backup schedules configured.</p>
            <p className="text-xs text-gray-400 mt-1">Add a schedule to automate backups.</p>
          </div>
        ) : (
          <div className="space-y-2">
            {schedules.map(sched => (
              <div key={sched.id}
                className="flex items-center justify-between p-3 border border-gray-200 rounded-lg hover:bg-gray-50/50">
                <div className="flex items-center gap-3 min-w-0">
                  <span className={`shrink-0 inline-flex px-2 py-0.5 rounded-md text-xs font-medium ${
                    sched.type === 'full' ? 'bg-blue-100 text-blue-700' : 'bg-purple-100 text-purple-700'
                  }`}>
                    {sched.type === 'full' ? 'Full' : 'DB Only'}
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm font-mono font-medium text-gray-800 truncate">{sched.cron_expr}</p>
                    <p className="text-xs text-gray-500">
                      {BACKUP_CRON_PRESETS.find(p => p.value === sched.cron_expr)?.label ?? sched.cron_expr}
                      {' · '}Keep {sched.retention} days
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-3 shrink-0 ml-4">
                  <span className={`text-xs font-medium ${sched.enabled ? 'text-green-600' : 'text-gray-400'}`}>
                    {sched.enabled ? 'Active' : 'Paused'}
                  </span>
                  {canManage && (
                    <button
                      onClick={() => updateSchedule.mutate({ id: sched.id, enabled: !sched.enabled })}
                      className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                        sched.enabled ? 'bg-green-500' : 'bg-gray-300'
                      }`}>
                      <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                        sched.enabled ? 'translate-x-4' : 'translate-x-0.5'
                      }`} />
                    </button>
                  )}
                  {canManage && (
                    <button
                      onClick={() => {
                        if (confirm('Delete this backup schedule?'))
                          deleteSchedule.mutate(sched.id)
                      }}
                      className="p-1 text-gray-400 hover:text-red-600 transition-colors">
                      <Trash2 size={14} />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── Restore Confirmation Modal ── */}
      {restoreTarget && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl max-w-md w-full p-6 shadow-xl">
            <h3 className="text-lg font-semibold text-gray-900 mb-2 flex items-center gap-2">
              <AlertTriangle size={20} className="text-amber-500" /> Confirm Restore
            </h3>
            <div className="bg-amber-50 border border-amber-200 rounded-lg p-3 mb-4 text-sm text-amber-800">
              This will <strong>overwrite your live site</strong> with the backup from{' '}
              <strong>{new Date(restoreTarget.created_at).toLocaleString()}</strong>.{' '}
              All current files and database data will be replaced. This cannot be undone.
            </div>
            <p className="text-sm text-gray-700 mb-2">
              Type <code className="bg-gray-100 px-1.5 py-0.5 rounded text-sm">{domain}</code> to confirm:
            </p>
            <input
              autoFocus
              value={restoreConfirm}
              onChange={e => setRestoreConfirm(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-red-200 focus:border-red-400"
              placeholder={domain} />
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => { setRestoreTarget(null); setRestoreConfirm('') }}
                className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded-lg transition-colors">
                Cancel
              </button>
              <button
                onClick={handleRestore}
                disabled={restoreConfirm !== domain || restoreBackup.isPending}
                className="px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 transition-colors">
                {restoreBackup.isPending ? 'Restoring...' : 'Restore Backup'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ==================== Action Button ====================

function ActionBtn({ icon, label, onClick, loading, color }: {
  icon: React.ReactNode; label: string; onClick: () => void; loading: boolean; color: string
}) {
  const colors: Record<string, string> = {
    blue: 'bg-blue-50 text-blue-700 hover:bg-blue-100',
    yellow: 'bg-yellow-50 text-yellow-700 hover:bg-yellow-100',
    green: 'bg-green-50 text-green-700 hover:bg-green-100',
    red: 'bg-red-50 text-red-700 hover:bg-red-100',
  }
  return (
    <button onClick={onClick} disabled={loading}
      className={`w-full flex items-center gap-2 px-4 py-2.5 text-sm font-medium rounded-lg disabled:opacity-50 transition-colors ${colors[color]}`}>
      {loading ? <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current" /> : icon}
      {label}
    </button>
  )
}
