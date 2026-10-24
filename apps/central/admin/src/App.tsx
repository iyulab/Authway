import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router'
import { Toaster } from 'react-hot-toast'
import { useAuthStore, selectIsAuthenticated } from '@/stores/auth'
import { useTenantStore } from '@/stores/tenant'
import Layout from '@/components/Layout'
import LoginPage from '@/pages/LoginPage'
import CallbackPage from '@/pages/CallbackPage'
import TenantSelectionPage from '@/pages/TenantSelectionPage'
import DashboardPage from '@/pages/DashboardPage'
import ClientsPage from '@/pages/ClientsPage'
import ClientCreatePage from '@/pages/ClientCreatePage'
import ClientDetailPage from '@/pages/ClientDetailPage'
import UsersPage from '@/pages/UsersPage'
import SettingsPage from '@/pages/SettingsPage'
import WebhooksPage from '@/pages/WebhooksPage'
import AuditLogsPage from '@/pages/AuditLogsPage'
import InvitationsPage from '@/pages/InvitationsPage'

function App() {
  const isAuthenticated = useAuthStore(selectIsAuthenticated)
  const selectedTenant = useTenantStore((state) => state.selectedTenant)
  const signingOut = useAuthStore((state) => state.signingOut)

  // Coming back to this page from the browser's history (back from Authway's
  // sign-out) restores it as it was left, still signing out.
  useEffect(() => {
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted && useAuthStore.getState().signingOut) useAuthStore.getState().logout()
    }
    window.addEventListener('pageshow', onPageShow)
    return () => window.removeEventListener('pageshow', onPageShow)
  }, [])

  // Leaving for Authway's sign-out: nothing to click until the page goes.
  if (signingOut) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50" role="status">
        <p className="text-gray-600">로그아웃 중…</p>
      </div>
    )
  }

  // Step 1: Not authenticated -> Login page
  if (!isAuthenticated) {
    return (
      <>
        <Toaster position="top-right" />
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          {/* Where Authway's sign-in of a console account comes back to. */}
          <Route path="/callback" element={<CallbackPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </>
    )
  }

  // Step 2: Authenticated but no tenant selected -> Tenant selection page
  if (!selectedTenant) {
    return (
      <>
        <Toaster position="top-right" />
        <TenantSelectionPage />
      </>
    )
  }

  // Step 3: Authenticated with tenant selected -> Main app
  return (
    <>
      <Toaster position="top-right" />
      <Layout>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/clients" element={<ClientsPage />} />
          <Route path="/clients/new" element={<ClientCreatePage />} />
          <Route path="/clients/:clientId" element={<ClientDetailPage />} />
          <Route path="/users" element={<UsersPage />} />
          <Route path="/webhooks" element={<WebhooksPage />} />
          <Route path="/audit-logs" element={<AuditLogsPage />} />
          <Route path="/invitations" element={<InvitationsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/login" element={<Navigate to="/dashboard" replace />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Layout>
    </>
  )
}

export default App
