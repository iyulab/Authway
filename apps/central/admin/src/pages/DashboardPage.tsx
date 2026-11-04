import React, { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Trans, useTranslation } from 'react-i18next'
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
  const { t } = useTranslation('dashboard')
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
    ? [{ label: t('system.api'), value: t('system.notAnswering'), status: 'unhealthy' }]
    : [
        { label: t('system.api'), value: health?.status ?? t('system.checking'), status: health?.status === 'ok' ? 'healthy' : 'unknown' },
        { label: t('system.version'), value: health?.version ?? '-' },
        {
          label: t('system.database'),
          value: health?.migrations?.length ? t('system.migration', { id: health.migrations[health.migrations.length - 1] }) : '-',
        },
      ]

  // Show message if no tenant selected (should not happen in normal flow)
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">{t('title')}</h2>
          <p className="mt-2 text-sm text-gray-600">
            {t('overview')}
          </p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title={t('noTenant.title')}
            description={t('noTenant.description')}
          />
        </Card>
      </div>
    )
  }

  const isLoading = clientsLoading || usersLoading

  if (isLoading) {
    return <Loading message={t('loading')} />
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900">{t('title')}</h2>
        <p className="mt-2 text-sm text-gray-600">
          <Trans
            t={t}
            i18nKey="welcome"
            values={{ name: selectedTenant?.name }}
            components={{ tenant: <span className="font-medium" /> }}
          />
        </p>
      </div>

      {/* Stats for current tenant */}
      <StatCardGrid columns={2}>
        <StatCard
          name={t('stats.clients')}
          value={stats.totalClients}
          icon={KeyIcon}
          variant="purple"
        />
        <StatCard
          name={t('stats.users')}
          value={stats.totalUsers}
          icon={UsersIcon}
          variant="blue"
        />
      </StatCardGrid>

      {/* Recent Activity */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <RecentActivityList
          title={t('recent.clients.title')}
          description={t('recent.clients.description')}
          items={clientActivityItems}
          emptyMessage={t('recent.clients.empty')}
        />
        <RecentActivityList
          title={t('recent.users.title')}
          description={t('recent.users.description')}
          items={userActivityItems}
          emptyMessage={t('recent.users.empty')}
        />
      </div>

      {/* System Info */}
      <SystemInfoCard
        title={t('system.title')}
        description={t('system.description')}
        items={systemInfoItems}
      />
    </div>
  )
}

export default DashboardPage
