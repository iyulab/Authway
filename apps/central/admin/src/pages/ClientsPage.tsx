import React from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import {
  PlusIcon,
  KeyIcon,
  BuildingOfficeIcon,
  ChevronRightIcon,
} from '@heroicons/react/24/outline'
import { clientsApi, Client } from '@/lib/api'
import { Button, Loading, EmptyState, Card, Badge } from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'

const ClientsPage: React.FC = () => {
  const navigate = useNavigate()
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''

  // Fetch clients
  const { data: clientsData, isLoading, error, refetch } = useQuery({
    queryKey: ['clients', selectedTenantId],
    queryFn: () => clientsApi.list({ tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  const clients = clientsData?.data.clients || []

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">OAuth Client Management</h1>
          <p className="mt-2 text-sm text-gray-700">
            Manage OAuth 2.0 clients for your applications.
          </p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant to view and manage OAuth clients."
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message="Loading clients..." fullScreen={false} />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">Failed to load clients.</p>
        <Button className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">OAuth Client Management</h1>
          <p className="mt-2 text-sm text-gray-700">
            {clients.length} OAuth client(s) registered for <span className="font-medium">{selectedTenant?.name}</span>.
          </p>
        </div>
        <div className="mt-4 sm:mt-0">
          <Button
            leftIcon={<PlusIcon className="h-5 w-5" />}
            onClick={() => navigate('/clients/new')}
          >
            Create New Client
          </Button>
        </div>
      </div>

      {/* Client List */}
      <Card>
        {clients.length === 0 ? (
          <EmptyState
            icon={<KeyIcon className="h-12 w-12" />}
            title="No clients"
            description="Create a new OAuth client to get started."
            action={
              <Button
                leftIcon={<PlusIcon className="h-5 w-5" />}
                onClick={() => navigate('/clients/new')}
              >
                Create New Client
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
                        {client.active ? 'Active' : 'Inactive'}
                      </Badge>
                      <Badge variant={client.public ? 'info' : 'purple'} size="sm">
                        {client.public ? 'Public' : 'Confidential'}
                      </Badge>
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
      </Card>
    </div>
  )
}

export default ClientsPage
