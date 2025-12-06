import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  BuildingOfficeIcon,
  PlusIcon,
  CheckIcon,
  ArrowRightOnRectangleIcon,
} from '@heroicons/react/24/outline'
import { tenantsApi, Tenant } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { useAuthStore } from '@/stores/auth'
import { Button, Modal, Input } from '@/components/ui'

const TenantSelectionPage: React.FC = () => {
  const queryClient = useQueryClient()
  const { setSelectedTenant } = useTenantStore()
  const logout = useAuthStore((state) => state.logout)

  const [showCreateModal, setShowCreateModal] = useState(false)
  const [newTenantName, setNewTenantName] = useState('')
  const [newTenantSlug, setNewTenantSlug] = useState('')

  // Fetch tenants
  const { data: tenants = [], isLoading } = useQuery({
    queryKey: ['tenants'],
    queryFn: async () => {
      const response = await tenantsApi.list()
      return Array.isArray(response.data) ? response.data : []
    },
  })

  // Create tenant mutation
  const createMutation = useMutation({
    mutationFn: (data: { name: string; slug: string }) => tenantsApi.create(data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      setShowCreateModal(false)
      setNewTenantName('')
      setNewTenantSlug('')
      // Auto-select the newly created tenant
      setSelectedTenant(response.data)
      toast.success('Tenant created successfully')
    },
    onError: (error: any) => {
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to create tenant'
      toast.error(errorMessage)
    },
  })

  const handleSelectTenant = (tenant: Tenant) => {
    setSelectedTenant(tenant)
    toast.success(`Selected tenant: ${tenant.name}`)
  }

  const handleCreateTenant = () => {
    if (!newTenantName.trim() || !newTenantSlug.trim()) {
      toast.error('Please fill in all fields')
      return
    }
    createMutation.mutate({ name: newTenantName, slug: newTenantSlug })
  }

  const handleLogout = () => {
    logout()
    window.location.href = '/login'
  }

  // Auto-generate slug from name
  const handleNameChange = (name: string) => {
    setNewTenantName(name)
    const slug = name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
    setNewTenantSlug(slug)
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-indigo-50 via-white to-purple-50">
      {/* Header */}
      <header className="bg-white/80 backdrop-blur-sm border-b border-gray-200">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-indigo-600 rounded-lg flex items-center justify-center">
                <BuildingOfficeIcon className="h-6 w-6 text-white" />
              </div>
              <div>
                <h1 className="text-xl font-bold text-gray-900">Authway Admin</h1>
                <p className="text-xs text-gray-500">Multi-tenant Identity Platform</p>
              </div>
            </div>
            <button
              onClick={handleLogout}
              className="flex items-center gap-2 px-4 py-2 text-sm text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
            >
              <ArrowRightOnRectangleIcon className="h-5 w-5" />
              Logout
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="text-center mb-10">
          <h2 className="text-3xl font-bold text-gray-900 mb-3">
            Select a Tenant
          </h2>
          <p className="text-gray-600 max-w-md mx-auto">
            Choose a tenant to manage or create a new one to get started.
          </p>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
          </div>
        ) : tenants.length === 0 ? (
          // Empty state
          <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-12 text-center">
            <div className="w-16 h-16 bg-indigo-100 rounded-full flex items-center justify-center mx-auto mb-6">
              <BuildingOfficeIcon className="h-8 w-8 text-indigo-600" />
            </div>
            <h3 className="text-xl font-semibold text-gray-900 mb-2">
              No tenants yet
            </h3>
            <p className="text-gray-500 mb-6 max-w-sm mx-auto">
              Create your first tenant to start managing users and applications.
            </p>
            <Button
              onClick={() => setShowCreateModal(true)}
              leftIcon={<PlusIcon className="h-5 w-5" />}
              size="lg"
            >
              Create First Tenant
            </Button>
          </div>
        ) : (
          // Tenant grid
          <div className="space-y-6">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {tenants.map((tenant: Tenant) => (
                <button
                  key={tenant.id}
                  onClick={() => handleSelectTenant(tenant)}
                  className="group relative bg-white rounded-xl border border-gray-200 p-6 text-left hover:border-indigo-300 hover:shadow-lg hover:shadow-indigo-100 transition-all duration-200"
                >
                  <div className="flex items-start gap-4">
                    <div className="w-12 h-12 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-lg flex items-center justify-center flex-shrink-0">
                      <span className="text-lg font-bold text-white">
                        {tenant.name.charAt(0).toUpperCase()}
                      </span>
                    </div>
                    <div className="flex-1 min-w-0">
                      <h3 className="text-lg font-semibold text-gray-900 truncate group-hover:text-indigo-600 transition-colors">
                        {tenant.name}
                      </h3>
                      <p className="text-sm text-gray-500 truncate">
                        {tenant.slug}
                      </p>
                      {tenant.description && (
                        <p className="text-sm text-gray-400 mt-1 line-clamp-2">
                          {tenant.description}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="absolute top-4 right-4 opacity-0 group-hover:opacity-100 transition-opacity">
                    <div className="w-8 h-8 bg-indigo-100 rounded-full flex items-center justify-center">
                      <CheckIcon className="h-5 w-5 text-indigo-600" />
                    </div>
                  </div>
                  <div className="mt-4 pt-4 border-t border-gray-100">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                      tenant.active
                        ? 'bg-green-100 text-green-800'
                        : 'bg-gray-100 text-gray-600'
                    }`}>
                      {tenant.active ? 'Active' : 'Inactive'}
                    </span>
                  </div>
                </button>
              ))}

              {/* Create new tenant card */}
              <button
                onClick={() => setShowCreateModal(true)}
                className="group bg-gray-50 rounded-xl border-2 border-dashed border-gray-300 p-6 text-center hover:border-indigo-400 hover:bg-indigo-50 transition-all duration-200 flex flex-col items-center justify-center min-h-[180px]"
              >
                <div className="w-12 h-12 bg-white rounded-lg flex items-center justify-center shadow-sm group-hover:shadow-md transition-shadow mb-4">
                  <PlusIcon className="h-6 w-6 text-gray-400 group-hover:text-indigo-600 transition-colors" />
                </div>
                <h3 className="text-sm font-medium text-gray-600 group-hover:text-indigo-600 transition-colors">
                  Create New Tenant
                </h3>
              </button>
            </div>
          </div>
        )}
      </main>

      {/* Create Tenant Modal */}
      <Modal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title="Create New Tenant"
        size="md"
      >
        <div className="space-y-4">
          <Input
            label="Tenant Name"
            placeholder="My Organization"
            value={newTenantName}
            onChange={(e) => handleNameChange(e.target.value)}
          />
          <Input
            label="Tenant Slug"
            placeholder="my-organization"
            value={newTenantSlug}
            onChange={(e) => setNewTenantSlug(e.target.value)}
            helperText="URL-friendly identifier (auto-generated from name)"
          />
          <div className="flex justify-end gap-3 pt-4">
            <Button
              variant="secondary"
              onClick={() => setShowCreateModal(false)}
            >
              Cancel
            </Button>
            <Button
              onClick={handleCreateTenant}
              isLoading={createMutation.isPending}
            >
              Create Tenant
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default TenantSelectionPage
