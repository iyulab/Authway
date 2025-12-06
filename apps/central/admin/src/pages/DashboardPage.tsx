import React, { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  KeyIcon,
  UsersIcon,
  CheckCircleIcon,
  BuildingOfficeIcon,
} from '@heroicons/react/24/outline'
import { clientsApi, usersApi, Client, User } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Loading, Card, EmptyState } from '@/components/ui'
import {
  StatCard,
  StatCardGrid,
  RecentActivityList,
  SystemInfoCard,
  ActivityItem,
  SystemInfoItem,
} from '@/components/dashboard'

const DashboardPage: React.FC = () => {
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''

  // Fetch clients for current tenant
  const { data: clientsData, isLoading: clientsLoading } = useQuery({
    queryKey: ['clients', selectedTenantId],
    queryFn: () => clientsApi.list({ limit: 100, tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  // Fetch users for current tenant
  const { data: usersData, isLoading: usersLoading } = useQuery({
    queryKey: ['users', selectedTenantId],
    queryFn: () => usersApi.list({ limit: 100, tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  // Calculate stats for current tenant
  const stats = useMemo(() => {
    const clients = clientsData?.data?.clients || []
    const users = usersData?.data?.users || []

    return {
      totalClients: clients.length,
      activeClients: clients.filter((c: Client) => c.active).length,
      totalUsers: users.length,
      activeUsers: users.filter((u: User) => u.active).length,
    }
  }, [clientsData, usersData])

  // Transform clients to activity items
  const clientActivityItems: ActivityItem[] = useMemo(() => {
    const clients = clientsData?.data?.clients || []
    return clients.slice(0, 5).map((client: Client) => ({
      id: client.id,
      name: client.name,
      subtitle: client.client_id,
      icon: KeyIcon,
      iconColor: 'text-purple-500',
      active: client.active,
    }))
  }, [clientsData])

  // Transform users to activity items
  const userActivityItems: ActivityItem[] = useMemo(() => {
    const users = usersData?.data?.users || []
    return users.slice(0, 5).map((user: User) => ({
      id: user.id,
      name: user.name || user.email,
      subtitle: user.email,
      icon: UsersIcon,
      iconColor: 'text-blue-500',
      active: user.active,
    }))
  }, [usersData])

  // System info items
  const systemInfoItems: SystemInfoItem[] = [
    { label: 'Server Version', value: 'v1.0.0' },
    { label: 'Ory Hydra Integration', value: 'Connected', status: 'healthy' },
    { label: 'Database', value: 'PostgreSQL' },
    { label: 'Redis Cache', value: 'Connected', status: 'healthy' },
  ]

  // Show message if no tenant selected (should not happen in normal flow)
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Dashboard</h2>
          <p className="mt-2 text-sm text-gray-600">
            Tenant management overview.
          </p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant to view the dashboard."
          />
        </Card>
      </div>
    )
  }

  const isLoading = clientsLoading || usersLoading

  if (isLoading) {
    return <Loading message="Loading dashboard..." />
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Dashboard</h2>
        <p className="mt-2 text-sm text-gray-600">
          Welcome to <span className="font-medium">{selectedTenant?.name}</span> management console.
        </p>
      </div>

      {/* Stats for current tenant */}
      <StatCardGrid columns={4}>
        <StatCard
          name="Total Apps (Clients)"
          value={stats.totalClients}
          icon={KeyIcon}
          variant="purple"
        />
        <StatCard
          name="Active Apps"
          value={stats.activeClients}
          icon={CheckCircleIcon}
          variant="green"
        />
        <StatCard
          name="Total Users"
          value={stats.totalUsers}
          icon={UsersIcon}
          variant="blue"
        />
        <StatCard
          name="Active Users"
          value={stats.activeUsers}
          icon={CheckCircleIcon}
          variant="green"
        />
      </StatCardGrid>

      {/* Recent Activity */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <RecentActivityList
          title="Recent Apps (Clients)"
          description="Recently created OAuth2 applications"
          items={clientActivityItems}
          emptyMessage="No apps created yet"
        />
        <RecentActivityList
          title="Recent Users"
          description="Recently registered users"
          items={userActivityItems}
          emptyMessage="No users registered yet"
        />
      </div>

      {/* System Info */}
      <SystemInfoCard
        title="System Information"
        description="Authway OAuth 2.0 server system status"
        items={systemInfoItems}
      />
    </div>
  )
}

export default DashboardPage
