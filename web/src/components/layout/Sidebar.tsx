import { NavLink } from 'react-router-dom'
import {
  Server, Globe, Shield, LayoutDashboard, Settings, LogOut,
  HardDrive, Users, ArrowRightLeft, UsersRound, Activity,
  ChevronRight, MonitorDot, Network, Download
} from 'lucide-react'
import { useAuth } from '@/stores/auth'
import { usePermissions } from '@/hooks/usePermissions'
import { useUpdateAvailable } from '@/hooks/usePanelUpdate'

const RoleColors: Record<string, string> = {
  admin: 'bg-red-500',
  manager: 'bg-blue-500',
  viewer: 'bg-slate-500',
}

const RoleLabels: Record<string, string> = {
  admin: 'Admin',
  manager: 'Manager',
  viewer: 'Viewer',
}

interface SidebarProps {
  mobile?: boolean
}

export default function Sidebar({ mobile }: SidebarProps) {
  const { user, logout } = useAuth()
  const { isAdmin } = usePermissions()
  const { data: updateInfo } = useUpdateAvailable()

  const navGroups = [
    {
      label: 'Overview',
      items: [
        { to: '/', icon: LayoutDashboard, label: 'Dashboard', exact: true },
      ],
    },
    {
      label: 'Infrastructure',
      items: [
        { to: '/servers', icon: Server, label: 'Servers' },
        { to: '/sites', icon: Globe, label: 'Sites' },
        { to: '/backups', icon: HardDrive, label: 'Backups' },
        { to: '/migrations', icon: ArrowRightLeft, label: 'Migrations' },
        { to: '/transfer', icon: Download, label: 'Transfer Sites' },
      ],
    },
    {
      label: 'Management',
      items: [
        { to: '/security', icon: Shield, label: 'Security' },
        { to: '/teams', icon: UsersRound, label: 'Teams' },
        { to: '/activity', icon: Activity, label: 'Activity Log' },
        { to: '/monitoring', icon: MonitorDot, label: 'Monitoring' },
        { to: '/dns', icon: Network, label: 'DNS' },
        ...(isAdmin ? [
          { to: '/users', icon: Users, label: 'Users' },
        ] : []),
        { to: '/settings', icon: Settings, label: 'Settings' },
      ],
    },
  ]

  return (
    <aside className="w-64 bg-[#0F172A] text-white flex flex-col h-full select-none">
      {/* Logo */}
      <div className={`px-5 ${mobile ? 'pt-10 pb-5' : 'py-5'} border-b border-white/8`}>
        <div className="flex items-center gap-3">
          <div className="relative w-9 h-9 flex-shrink-0">
            <div className="w-9 h-9 rounded-xl bg-gradient-to-br from-blue-500 to-blue-700 flex items-center justify-center shadow-lg shadow-blue-900/40">
              <span className="text-white font-black text-base tracking-tight">W</span>
            </div>
            {updateInfo?.hasUpdate && (
              <span className="absolute -top-1 -right-1 w-3 h-3 rounded-full bg-amber-400 border-2 border-[#0F172A] animate-pulse" />
            )}
          </div>
          <div className="min-w-0">
            <h1 className="text-[15px] font-bold text-white tracking-tight leading-none">WP Manage</h1>
            {updateInfo?.hasUpdate ? (
              <p className="text-[10px] text-amber-400 mt-0.5 leading-none font-medium truncate">
                ↑ {updateInfo.latestVersion} available
              </p>
            ) : (
              <p className="text-[11px] text-slate-400 mt-0.5 leading-none">Server Management</p>
            )}
          </div>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-3 px-3 space-y-4 scrollbar-thin">
        {navGroups.map((group) => (
          <div key={group.label}>
            <p className="px-3 mb-1 text-[10px] font-semibold tracking-widest uppercase text-slate-500">
              {group.label}
            </p>
            <div className="space-y-0.5">
              {group.items.map(({ to, icon: Icon, label }) => {
                const isSettings = to === '/settings'
                const showUpdateDot = isSettings && updateInfo?.hasUpdate
                return (
                  <NavLink
                    key={to}
                    to={to}
                    end={to === '/'}
                    title={showUpdateDot ? `Có bản cập nhật mới: ${updateInfo?.latestVersion}` : undefined}
                    className={({ isActive }) =>
                      `group flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-all duration-150 ${
                        isActive
                          ? 'bg-blue-600/20 text-blue-400 font-medium'
                          : 'text-slate-400 hover:bg-white/5 hover:text-slate-200'
                      }`
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <span className={`relative flex-shrink-0 transition-colors ${isActive ? 'text-blue-400' : 'text-slate-500 group-hover:text-slate-300'}`}>
                          <Icon size={16} />
                          {showUpdateDot && (
                            <span className="absolute -top-1.5 -right-1.5 w-2 h-2 rounded-full bg-amber-400 animate-pulse" />
                          )}
                        </span>
                        <span className="flex-1 truncate">{label}</span>
                        {showUpdateDot && !isActive && (
                          <span className="text-[9px] font-semibold px-1.5 py-0.5 rounded-full bg-amber-400/15 text-amber-400 flex-shrink-0">
                            NEW
                          </span>
                        )}
                        {isActive && (
                          <span className="w-1.5 h-1.5 rounded-full bg-blue-400 flex-shrink-0" />
                        )}
                      </>
                    )}
                  </NavLink>
                )
              })}
            </div>
          </div>
        ))}
      </nav>

      {/* User section */}
      <div className="p-3 border-t border-white/8">
        <div className="flex items-center gap-2.5 px-2 py-2 rounded-xl bg-white/5 mb-2">
          <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center text-sm font-bold text-white flex-shrink-0 shadow-sm">
            {user?.name?.[0]?.toUpperCase() || 'U'}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-[13px] font-medium text-slate-200 truncate leading-tight">{user?.name}</p>
            <div className="flex items-center gap-1.5 mt-0.5">
              <span className={`inline-block w-1.5 h-1.5 rounded-full ${RoleColors[user?.role || 'viewer']}`} />
              <span className="text-[11px] text-slate-500 leading-none">
                {RoleLabels[user?.role || 'viewer']}
              </span>
            </div>
          </div>
          <ChevronRight size={14} className="text-slate-600 flex-shrink-0" />
        </div>
        <button
          onClick={logout}
          className="flex items-center gap-2.5 w-full px-3 py-2 text-sm text-slate-500 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors group"
        >
          <LogOut size={15} className="group-hover:text-red-400 transition-colors" />
          <span>Sign Out</span>
        </button>
      </div>
    </aside>
  )
}
