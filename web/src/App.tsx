import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAuth } from '@/stores/auth'
import Layout from '@/components/layout/Layout'
import LoginPage from '@/pages/LoginPage'
import DashboardPage from '@/pages/DashboardPage'
import ServersPage from '@/pages/ServersPage'
import ServerDetailPage from '@/pages/ServerDetailPage'
import SitesPage from '@/pages/SitesPage'
import SiteDetailPage from '@/pages/SiteDetailPage'
import BackupsPage from '@/pages/BackupsPage'
import MigrationsPage from '@/pages/MigrationsPage'
import SecurityPage from '@/pages/SecurityPage'
import UsersPage from '@/pages/UsersPage'
import SettingsPage from '@/pages/SettingsPage'
import TeamsPage from '@/pages/TeamsPage'
import ActivityLogPage from '@/pages/ActivityLogPage'
import UptimeMonitoringPage from '@/pages/UptimeMonitoringPage'
import DNSProvidersPage from '@/pages/DNSProvidersPage'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
})

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { token, loading } = useAuth()
  if (loading) return <div className="min-h-screen flex items-center justify-center">Loading...</div>
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  const { token, loadUser } = useAuth()

  useEffect(() => {
    if (token) loadUser()
  }, [token])

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<ProtectedRoute><Layout /></ProtectedRoute>}>
        <Route index element={<DashboardPage />} />
        <Route path="servers" element={<ServersPage />} />
        <Route path="servers/:id" element={<ServerDetailPage />} />
        <Route path="sites" element={<SitesPage />} />
        <Route path="sites/:id" element={<SiteDetailPage />} />
        <Route path="backups" element={<BackupsPage />} />
        <Route path="migrations" element={<MigrationsPage />} />

        <Route path="security" element={<SecurityPage />} />
        <Route path="teams" element={<TeamsPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="activity" element={<ActivityLogPage />} />
        <Route path="monitoring" element={<UptimeMonitoringPage />} />
        <Route path="dns" element={<DNSProvidersPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </QueryClientProvider>
  )
}
