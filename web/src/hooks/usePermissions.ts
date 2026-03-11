import { useAuth } from '@/stores/auth'

export function usePermissions() {
  const { user } = useAuth()
  const role = user?.role || 'viewer'

  const isAdmin = role === 'admin'
  const isManager = role === 'manager'
  const isViewer = role === 'viewer'

  const can = (resource: string, action: string): boolean => {
    if (isAdmin) return true

    switch (resource) {
      case 'servers':
        switch (action) {
          case 'read': return true
          case 'create': case 'delete': case 'deploy': return false
          default: return false
        }
      case 'sites':
        switch (action) {
          case 'read': return true
          case 'create': case 'delete': case 'manage': return isManager
          default: return false
        }
      case 'files':
        switch (action) {
          case 'read': return isManager || isViewer
          case 'write': return isManager
          default: return false
        }
      case 'backups':
        switch (action) {
          case 'read': return isManager || isViewer
          case 'create': case 'restore': return isManager
          default: return false
        }
      case 'infrastructure':
        switch (action) {
          case 'read': return isManager || isViewer
          case 'manage': return isManager
          default: return false
        }
      case 'teams':
        switch (action) {
          case 'read': return true
          case 'create': return isManager
          case 'manage': case 'delete': return false
          default: return false
        }
      case 'users':
      case 'settings':
        return false
      default:
        return false
    }
  }

  return { can, isAdmin, isManager, isViewer, role }
}
