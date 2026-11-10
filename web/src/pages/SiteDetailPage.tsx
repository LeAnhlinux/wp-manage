import React, { useState, lazy, Suspense } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  ArrowLeft, Globe, Lock, LockOpen, ExternalLink,
  Code, Clock, Database, Settings,
  FolderOpen, LayoutDashboard, Layers, Download, HardDrive,
  Network, ImageIcon, Zap, Archive, Shield, Key, Gauge, ServerCrash, SlidersHorizontal
} from 'lucide-react'
import { useSite, useAutologin } from '@/hooks/useSites'
import { usePermissions } from '@/hooks/usePermissions'
import { StatusBadge } from '@/components/ui/Badge'
import { formatDate } from '@/lib/utils'
import FileBrowser from '@/components/files/FileBrowser'

import { StatCard } from './site-tabs/shared'

// Tab panels are code-split: 18 panels live on this page but only one is ever
// mounted, and together they were ~4.3k lines sitting in the entry chunk. They are
// named exports, so each import is mapped onto `default` for React.lazy.
const OverviewTab = lazy(() => import('./site-tabs/OverviewTab').then(m => ({ default: m.OverviewTab })))
const StagingPanel = lazy(() => import('./site-tabs/StagingPanel').then(m => ({ default: m.StagingPanel })))
const UpdatesPanel = lazy(() => import('./site-tabs/UpdatesPanel').then(m => ({ default: m.UpdatesPanel })))
const DatabasePanel = lazy(() => import('./site-tabs/DatabasePanel').then(m => ({ default: m.DatabasePanel })))
const PerformancePanel = lazy(() => import('./site-tabs/PerformancePanel').then(m => ({ default: m.PerformancePanel })))
const ImagesPanel = lazy(() => import('./site-tabs/ImagesPanel').then(m => ({ default: m.ImagesPanel })))
const MultisitePanel = lazy(() => import('./site-tabs/MultisitePanel').then(m => ({ default: m.MultisitePanel })))
const DomainsPanel = lazy(() => import('./site-tabs/DomainsPanel').then(m => ({ default: m.DomainsPanel })))
const RedisPanel = lazy(() => import('./site-tabs/RedisPanel').then(m => ({ default: m.RedisPanel })))
const CronPanel = lazy(() => import('./site-tabs/CronPanel').then(m => ({ default: m.CronPanel })))
const SFTPPanel = lazy(() => import('./site-tabs/SFTPPanel').then(m => ({ default: m.SFTPPanel })))
const WAFPanel = lazy(() => import('./site-tabs/WAFPanel').then(m => ({ default: m.WAFPanel })))
const BotControlPanel = lazy(() => import('./site-tabs/BotControlPanel').then(m => ({ default: m.BotControlPanel })))
const GeoAccessPanel = lazy(() => import('./site-tabs/GeoAccessPanel').then(m => ({ default: m.GeoAccessPanel })))
const SecurityPanel = lazy(() => import('./site-tabs/SecurityPanel').then(m => ({ default: m.SecurityPanel })))
const WPCLIPanel = lazy(() => import('./site-tabs/WPCLIPanel').then(m => ({ default: m.WPCLIPanel })))
const CachePanel = lazy(() => import('./site-tabs/CachePanel').then(m => ({ default: m.CachePanel })))
const SSLPanel = lazy(() => import('./site-tabs/SSLPanel').then(m => ({ default: m.SSLPanel })))
const BackupPanel = lazy(() => import('./site-tabs/BackupPanel').then(m => ({ default: m.BackupPanel })))
const ResourceLimitsPanel = lazy(() => import('./site-tabs/ResourceLimitsPanel').then(m => ({ default: m.ResourceLimitsPanel })))

type TabId = 'overview' | 'files' | 'staging' | 'updates' | 'database' | 'performance' |
  'multisite' | 'images' | 'domains' | 'redis' | 'cron' | 'sftp' | 'security' |
  'ssl' | 'wpcli' | 'cache' | 'backup' | 'limits'

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
        <StatCard
          icon={<Clock size={20} />}
          label="WP Version"
          value={site.wp_version || ''}
          sub={!site.wp_version ? 'Detecting...' : undefined}
        />
      </div>

      {/* Sidebar + Content layout */}
      <div className="flex gap-6 items-start">

        {/* Left sidebar nav */}
        <nav className="w-48 flex-shrink-0 bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden sticky top-4">
          {([
            { group: 'Site', items: [
              { id: 'overview', icon: <LayoutDashboard size={15} />, label: 'Overview' },
              { id: 'files',    icon: <FolderOpen size={15} />,      label: 'Files' },
            ]},
            { group: 'Deploy', items: [
              { id: 'staging', icon: <Layers size={15} />,  label: 'Staging' },
              { id: 'updates', icon: <Download size={15} />, label: 'Updates' },
              { id: 'backup',  icon: <Archive size={15} />,  label: 'Backups' },
            ]},
            { group: 'Config', items: [
              { id: 'database', icon: <HardDrive size={15} />, label: 'Database' },
              { id: 'domains',  icon: <Globe size={15} />,     label: 'Domains' },
              { id: 'ssl',      icon: <Lock size={15} />,      label: 'SSL' },
              { id: 'sftp',     icon: <Key size={15} />,       label: 'SFTP' },
              { id: 'cron',     icon: <Clock size={15} />,     label: 'Cron Jobs' },
              { id: 'limits',   icon: <SlidersHorizontal size={15} />, label: 'Resource Limits' },
            ]},
            { group: 'Performance', items: [
              { id: 'performance', icon: <Gauge size={15} />,      label: 'Performance' },
              { id: 'cache',       icon: <Zap size={15} />,        label: 'Cache' },
              { id: 'redis',       icon: <ServerCrash size={15} />, label: 'Redis' },
              { id: 'images',      icon: <ImageIcon size={15} />,  label: 'Images' },
            ]},
            { group: 'Advanced', items: [
              { id: 'multisite', icon: <Network size={15} />, label: 'Multisite' },
              { id: 'wpcli',     icon: <Code size={15} />,    label: 'WP-CLI' },
              { id: 'security',  icon: <Shield size={15} />,  label: 'Security' },
            ]},
          ] as { group: string; items: { id: TabId; icon: React.ReactNode; label: string }[] }[]).map(({ group, items }, gi) => (
            <div key={group} className={gi > 0 ? 'border-t border-gray-100' : ''}>
              <p className="px-3 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-gray-400">
                {group}
              </p>
              {items.map(({ id, icon, label }) => (
                <button
                  key={id}
                  onClick={() => setActiveTab(id)}
                  className={`w-full flex items-center gap-2.5 px-3 py-2 text-sm transition-colors ${
                    activeTab === id
                      ? 'bg-blue-50 text-blue-700 font-medium'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
                  }`}
                >
                  <span className={activeTab === id ? 'text-blue-600' : 'text-gray-400'}>{icon}</span>
                  {label}
                  {activeTab === id && (
                    <span className="ml-auto w-1 h-4 rounded-full bg-blue-600" />
                  )}
                </button>
              ))}
              {gi === 4 && <div className="pb-2" />}
            </div>
          ))}
        </nav>

        {/* Right content */}
        <div className="flex-1 min-w-0">
          {/* Panels are lazy-loaded; this keeps the page frame visible while a tab's
              chunk is fetched instead of blanking the whole route. */}
          <Suspense fallback={<div className="py-12 text-center text-sm text-gray-500">Loading…</div>}>
          {activeTab === 'overview' && (
            <OverviewTab site={site} onDeleted={() => navigate('/sites')} onMessage={showMsg} canManage={can('sites', 'manage')} />
          )}
          {activeTab === 'files' && <FileBrowser siteId={site.id} />}
          {activeTab === 'staging' && <StagingPanel siteId={site.id} canManage={can('sites', 'manage')} siteDomain={site.domain} />}
          {activeTab === 'updates' && <UpdatesPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'database' && <DatabasePanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'performance' && <PerformancePanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'images' && <ImagesPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'multisite' && <MultisitePanel siteId={site.id} isMultisite={site.is_multisite} canManage={can('sites', 'manage')} />}
          {activeTab === 'domains' && <DomainsPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'redis' && <RedisPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'cron' && <CronPanel siteId={site.id} />}
          {activeTab === 'ssl' && <SSLPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />}
          {activeTab === 'sftp' && <SFTPPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'limits' && <ResourceLimitsPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'backup' && <BackupPanel siteId={site.id} domain={site.domain} canManage={can('sites', 'manage')} />}
          {activeTab === 'security' && (
            <div className="space-y-6">
              <WAFPanel siteId={site.id} canManage={can('sites', 'manage')} />
              <BotControlPanel siteId={site.id} canManage={can('sites', 'manage')} />
              <GeoAccessPanel siteId={site.id} canManage={can('sites', 'manage')} />
              <SecurityPanel siteId={site.id} canManage={can('sites', 'manage')} />
            </div>
          )}
          {activeTab === 'wpcli' && <WPCLIPanel siteId={site.id} canManage={can('sites', 'manage')} />}
          {activeTab === 'cache' && <CachePanel siteId={site.id} canManage={can('sites', 'manage')} domain={site.domain} />}
          </Suspense>
        </div>

      </div>
    </div>
  )
}
