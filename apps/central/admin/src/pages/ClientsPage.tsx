import React, { useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Trans, useTranslation } from 'react-i18next'
import {
  PlusIcon,
  KeyIcon,
  BuildingOfficeIcon,
  ChevronRightIcon,
} from '@heroicons/react/24/outline'
import { clientsApi, Client } from '@/lib/api'
import { Button, Loading, EmptyState, Card, Badge, Pagination } from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'

const ClientsPage: React.FC = () => {
  const { t } = useTranslation('clients')
  const navigate = useNavigate()
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''
  const pageSize = 20
  const [currentPage, setCurrentPage] = useState(1)

  // A page at a time, newest first.
  const { data: clientsData, isLoading, error, refetch } = useQuery({
    queryKey: ['clients', selectedTenantId, currentPage],
    queryFn: () => clientsApi.list({ tenant_id: selectedTenantId, limit: pageSize, offset: (currentPage - 1) * pageSize }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  const clients = clientsData?.data.clients || []
  const total = clientsData?.data.total ?? clients.length
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('list.title')}</h1>
          <p className="mt-2 text-sm text-gray-700">
            {t('list.subtitle')}
          </p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title={t('list.noTenant')}
            description={t('list.noTenantHelp')}
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message={t('list.loading')} fullScreen={false} />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">{t('list.loadFailed')}</p>
        <Button className="mt-4" onClick={() => refetch()}>
          {t('list.retry')}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('list.title')}</h1>
          <p className="mt-2 text-sm text-gray-700">
            <Trans
              t={t}
              i18nKey="list.registered"
              values={{ count: total, name: selectedTenant?.name }}
              components={{ tenant: <span className="font-medium" /> }}
            />
          </p>
        </div>
        <div className="mt-4 sm:mt-0">
          <Button
            leftIcon={<PlusIcon className="h-5 w-5" />}
            onClick={() => navigate('/clients/new')}
          >
            {t('list.create')}
          </Button>
        </div>
      </div>

      {/* Client List */}
      <Card>
        {clients.length === 0 ? (
          <EmptyState
            icon={<KeyIcon className="h-12 w-12" />}
            title={t('list.emptyTitle')}
            description={t('list.emptyHelp')}
            action={
              <Button
                leftIcon={<PlusIcon className="h-5 w-5" />}
                onClick={() => navigate('/clients/new')}
              >
                {t('list.create')}
              </Button>
            }
          />
        ) : (
          <div className="divide-y divide-gray-200">
            {clients.map((client: Client) => (
              <button
                key={client.id}
                onClick={() => navigate(`/clients/${client.id}`)}
                className="w-full px-6 py-4 flex items-center justify-between hover:bg-gray-50 transition-colors text-left"
              >
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-indigo-100 rounded-lg flex items-center justify-center">
                    <KeyIcon className="h-5 w-5 text-indigo-600" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <h3 className="text-sm font-medium text-gray-900">{client.name}</h3>
                      <Badge variant={client.active ? 'success' : 'default'} size="sm">
                        {client.active ? t('badge.active') : t('badge.inactive')}
                      </Badge>
                      <Badge variant={client.public ? 'info' : 'purple'} size="sm">
                        {client.public ? t('badge.public') : t('badge.confidential')}
                      </Badge>
                      {client.system && (
                        <Badge variant="default" size="sm">
                          {t('badge.system')}
                        </Badge>
                      )}
                    </div>
                    <p className="text-sm text-gray-500 font-mono">{client.client_id}</p>
                    {client.description && (
                      <p className="text-sm text-gray-400 mt-1 line-clamp-1">{client.description}</p>
                    )}
                  </div>
                </div>
                <ChevronRightIcon className="h-5 w-5 text-gray-400" />
              </button>
            ))}
          </div>
        )}
        {totalPages > 1 && (
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            totalItems={total}
            itemsPerPage={pageSize}
            onPageChange={setCurrentPage}
          />
        )}
      </Card>
    </div>
  )
}

export default ClientsPage
