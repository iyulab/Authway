import React, { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  KeyIcon,
  UsersIcon,
  BuildingOfficeIcon,
} from '@heroicons/react/24/outline'
import { clientsApi, usersApi, healthApi, Client, User } from '@/lib/api'
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

  // The newest few of each, with the totals (the API lists newest first).
  // Keys of their own: the clients page caches its pages under ['clients', …].
  const { data: clientsData, isLoading: clientsLoading } = useQuery({
    queryKey: ['dashboard-clients', selectedTenantId],
    queryFn: () => clientsApi.list({ limit: 5, tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  // Fetch users for current tenant
  const { data: usersData, isLoading: usersLoading } = useQuery({
    queryKey: ['dashboard-users', selectedTenantId],
    queryFn: () => usersApi.list({ limit: 5, tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  const { data: health, isError: healthFailed } = useQuery({
    queryKey: ['health'],
    queryFn: () => healthApi.get().then((res) => res.data),
  })

  const stats = {
    totalClients: clientsData?.data?.total ?? 0,
    totalUsers: usersData?.data?.total ?? 0,
  }

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
    }))
  }, [usersData])

  // What the API says about itself (GET /health) — nothing else is known here.
  const systemInfoItems: SystemInfoItem[] = healthFailed
    ? [{ label: 'API', value: 'Not answering', status: 'unhealthy' }]
    : [
        { label: 'API', value: health?.status ?? 'Checking…', status: health?.status === 'ok' ? 'healthy' : 'unknown' },
        { label: 'Deployed version', value: health?.version ?? '-' },
        {
          label: 'Database',
          value: health?.migrations?.length ? `migration ${health.migrations[health.migrations.length - 1]}` : '-',
        },
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
      <StatCardGrid columns={2}>
        <StatCard
          name="Apps (Clients)"
          value={stats.totalClients}
          icon={KeyIcon}
          variant="purple"
        />
        <StatCard
          name="Users"
          value={stats.totalUsers}
          icon={UsersIcon}
          variant="blue"
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
        description="What the API answers at /health"
        items={systemInfoItems}
      />
    </div>
  )
}

export default DashboardPage
