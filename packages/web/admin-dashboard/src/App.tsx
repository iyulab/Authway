import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import Layout from '@/components/Layout'
import LoginPage from '@/pages/LoginPage'
import DashboardPage from '@/pages/DashboardPage'
import UsersPage from '@/pages/UsersPage'
import ClientsPage from '@/pages/ClientsPage'
import SettingsPage from '@/pages/SettingsPage'

function App() {
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated)

  if (!isAuthenticated) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <Layout>
      <Routes>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/clients" element={<ClientsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/login" element={<Navigate to="/dashboard" replace />} />
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Routes>
    </Layout>
  )
}

export default App