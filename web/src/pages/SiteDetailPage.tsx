import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft, Globe, Lock, LockOpen, ExternalLink,
  Code, Clock, Database, Settings,
  FolderOpen, LayoutDashboard, Layers, Download, HardDrive,
  Network, ImageIcon, Zap, Archive, Shield, Key
} from 'lucide-react'
import { useSite, useAutologin } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import { StatusBadge } from '@/components/ui/Badge'
import { formatDate } from '@/lib/utils'
import FileBrowser from '@/components/files/FileBrowser'

import { StatCard, TabButton } from './site-tabs/shared'
import { OverviewTab } from './site-tabs/OverviewTab'
import { StagingPanel } from './site-tabs/StagingPanel'
import { UpdatesPanel } from './site-tabs/UpdatesPanel'
import { DatabasePanel } from './site-tabs/DatabasePanel'
import { PerformancePanel } from './site-tabs/PerformancePanel'
import { ImagesPanel } from './site-tabs/ImagesPanel'
import { MultisitePanel } from './site-tabs/MultisitePanel'
import { DomainsPanel } from './site-tabs/DomainsPanel'
import { RedisPanel } from './site-tabs/RedisPanel'
import { CronPanel } from './site-tabs/CronPanel'
import { SFTPPanel } from './site-tabs/SFTPPanel'
import { WAFPanel } from './site-tabs/WAFPanel'
import { SecurityPanel } from './site-tabs/SecurityPanel'
import { WPCLIPanel } from './site-tabs/WPCLIPanel'
import { CachePanel } from './site-tabs/CachePanel'
import { SSLPanel } from './site-tabs/SSLPanel'
import { BackupPanel } from './site-tabs/BackupPanel'

type TabId = 'overview' | 'files' | 'staging' | 'updates' | 'database' | 'performance' |
  'multisite' | 'images' | 'domains' | 'redis' | 'cron' | 'sftp' | 'security' |
  'ssl' | 'wpcli' | 'cache' | 'backup'

export default function SiteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: site, isLoading, error } = useSite(Number(id))
  const autologin = useAutologin()
  const { can } = usePermissions()

  const [activeTab, setActiveTab] = useState<TabId>('overview')
  const [actionMsg, setActionMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

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

  const siteBaseUrl = `${site.ssl_enabled ? 'https' : 'http'}://${site.domain}`

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

  return (
    <div>
      {/* Alert message */}
      {actionMsg && (
        <div className={`mb-4 px-4 py-3 rounded-lg text-sm font-medium ${
          actionMsg.type === 'success'
            ? 'bg-green-50 text-green-800 border border-green-200'
            : 'bg-red-50 text-red-800 border border-red-200'
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
          <a
            href={siteBaseUrl}
            target="_blank"
            rel="noopener"
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-gray-700 bg-white border border-gray-200 rounded-lg hover:bg-gray-50"
          >
            <ExternalLink size={14} /> Visit
          </a>
          <button
            onClick={handleAutologin}
            disabled={autologin.isPending}
            className="flex items-center gap-1.5 px-3 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
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
        <StatCard
          icon={site.ssl_enabled ? <Lock size={20} /> : <LockOpen size={20} />}
          label="SSL"
          value={site.ssl_enabled ? 'Active' : 'Disabled'}
          sub={site.ssl_expires_at ? `Expires: ${formatDate(site.ssl_expires_at)}` : undefined}
        />
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

      {/* Tab Content */}
      {activeTab === 'overview' && (
        <OverviewTab
          site={site}
          onDeleted={() => navigate('/sites')}
          onMessage={showMsg}
          canManage={can('sites', 'manage')}
        />
      )}

      {activeTab === 'files' && (
        <FileBrowser siteId={site.id} />
      )}

      {activeTab === 'staging' && (
        <StagingPanel siteId={site.id} canManage={can('sites', 'manage')} siteDomain={site.domain} />
      )}

      {activeTab === 'updates' && (
        <UpdatesPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'database' && (
        <DatabasePanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'performance' && (
        <PerformancePanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'images' && (
        <ImagesPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'multisite' && (
        <MultisitePanel siteId={site.id} isMultisite={site.is_multisite} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'domains' && (
        <DomainsPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'redis' && (
        <RedisPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'cron' && (
        <CronPanel siteId={site.id} />
      )}

      {activeTab === 'ssl' && (
        <SSLPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'sftp' && (
        <SFTPPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'backup' && (
        <BackupPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'security' && (
        <div className="space-y-6">
          <WAFPanel siteId={site.id} canManage={can('sites', 'manage')} />
          <SecurityPanel siteId={site.id} canManage={can('sites', 'manage')} />
        </div>
      )}

      {activeTab === 'wpcli' && (
        <WPCLIPanel siteId={site.id} canManage={can('sites', 'manage')} />
      )}

      {activeTab === 'cache' && (
        <CachePanel siteId={site.id} canManage={can('sites', 'manage')} domain={site.domain} />
      )}
    </div>
  )
}
