import { useEffect, lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAuth } from '@/stores/auth'
import Layout from '@/components/layout/Layout'

// Login is the first paint for a signed-out user, so it stays in the entry chunk.
import LoginPage from '@/pages/LoginPage'

// Every other page is code-split. Statically importing all of them put the whole
// app — ~13k lines of routed pages plus ~4k lines of site tab panels — into a
// single 828 kB (186 kB gzip) chunk that had to download before the login form
// could render. Each page now loads on navigation instead.
const DashboardPage = lazy(() => import('@/pages/DashboardPage'))
const ServersPage = lazy(() => import('@/pages/ServersPage'))
const ServerDetailPage = lazy(() => import('@/pages/ServerDetailPage'))
const SitesPage = lazy(() => import('@/pages/SitesPage'))
const SiteDetailPage = lazy(() => import('@/pages/SiteDetailPage'))
const BackupsPage = lazy(() => import('@/pages/BackupsPage'))
const MigrationsPage = lazy(() => import('@/pages/MigrationsPage'))
const SecurityPage = lazy(() => import('@/pages/SecurityPage'))
const UsersPage = lazy(() => import('@/pages/UsersPage'))
const SettingsPage = lazy(() => import('@/pages/SettingsPage'))
const TeamsPage = lazy(() => import('@/pages/TeamsPage'))
const ActivityLogPage = lazy(() => import('@/pages/ActivityLogPage'))
const UptimeMonitoringPage = lazy(() => import('@/pages/UptimeMonitoringPage'))
const DNSProvidersPage = lazy(() => import('@/pages/DNSProvidersPage'))
const TransferPage = lazy(() => import('@/pages/TransferPage'))
const TransferWizardPage = lazy(() => import('@/pages/TransferWizardPage'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      // Without a staleTime, every query is stale the instant it resolves, so
      // every mount refetches. Servers and sites are mounted by most pages, so a
      // normal Dashboard → Sites → detail → back loop refired them on each visit.
      // 30s is well under how often this data actually changes, and the hooks that
      // need live data set their own refetchInterval.
      staleTime: 30_000,
    },
  },
})

// Shown while a route chunk is fetched. Matches ProtectedRoute's loading state so
// navigation does not visibly jump.
function RouteFallback() {
  return <div className="min-h-screen flex items-center justify-center text-[var(--t3)]">Loading…</div>
}

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
    <Suspense fallback={<RouteFallback />}>
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
        <Route path="transfer" element={<TransferPage />} />
        <Route path="transfer/new" element={<TransferWizardPage />} />
        <Route path="transfer/:id" element={<TransferWizardPage />} />
      </Route>
    </Routes>
    </Suspense>
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
