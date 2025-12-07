import { Routes, Route, Navigate } from 'react-router-dom'
import { Toaster } from 'react-hot-toast'
import { useAuthStore } from '@/stores/auth'
import { useTenantStore } from '@/stores/tenant'
import Layout from '@/components/Layout'
import LoginPage from '@/pages/LoginPage'
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
import ImpersonationPage from '@/pages/ImpersonationPage'

function App() {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated)
  const selectedTenant = useTenantStore((state) => state.selectedTenant)

  // Step 1: Not authenticated -> Login page
  if (!isAuthenticated) {
    return (
      <>
        <Toaster position="top-right" />
        <Routes>
          <Route path="/login" element={<LoginPage />} />
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
          <Route path="/impersonation" element={<ImpersonationPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/login" element={<Navigate to="/dashboard" replace />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Layout>
    </>
  )
}

export default App
