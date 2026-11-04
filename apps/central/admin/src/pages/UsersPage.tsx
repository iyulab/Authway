import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Trans, useTranslation } from 'react-i18next'
import {
  UserIcon,
  PencilIcon,
  TrashIcon,
  MagnifyingGlassIcon,
  CheckCircleIcon,
  XCircleIcon,
  BuildingOfficeIcon,
  NoSymbolIcon,
  ArrowPathIcon,
} from '@heroicons/react/24/outline'
import { apiErrorMessage, usersApi, User } from '@/lib/api'
import { formatDay } from '@/lib/utils'
import {
  Button,
  Card,
  Loading,
  EmptyState,
  ConfirmDialog,
  StatusBadge,
  Badge,
  Input,
  Pagination,
} from '@/components/ui'
import { UserFormModal, UserFormData, UserAvatar } from '@/components/users'
import { useTenantStore } from '@/stores/tenant'

const UsersPage: React.FC = () => {
  const { t } = useTranslation('users')
  const queryClient = useQueryClient()
  const pageSize = 10

  // Tenant context
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''

  // State
  const [searchTerm, setSearchTerm] = useState('')
  const [currentPage, setCurrentPage] = useState(1)
  const [showEditModal, setShowEditModal] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showSuspendConfirm, setShowSuspendConfirm] = useState(false)
  const [selectedUser, setSelectedUser] = useState<User | null>(null)

  // Fetch users for selected tenant
  const { data: usersData, isLoading, error, refetch } = useQuery({
    queryKey: ['users', selectedTenantId, currentPage],
    queryFn: () =>
      usersApi.list({
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
        tenant_id: selectedTenantId,
      }),
    enabled: !!selectedTenantId,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  const users = usersData?.data.users || []
  const totalUsers = usersData?.data.total || 0
  const totalPages = Math.ceil(totalUsers / pageSize)

  // Filter users by search term (client-side filtering for current page)
  const filteredUsers = users.filter(
    (user: User) =>
      user.email.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (user.name && user.name.toLowerCase().includes(searchTerm.toLowerCase()))
  )

  // Update user mutation
  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: { name?: string; avatar_url?: string } }) =>
      usersApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowEditModal(false)
      setSelectedUser(null)
      toast.success(t('toast.updated'))
    },
    onError: (error: unknown) => {
      toast.error(t('toast.updateFailed', { reason: apiErrorMessage(error, 'users:toast.updateFallback') }))
    },
  })

  // Suspending ends the user's sessions and refuses their sign-ins; lifting
  // it lets them sign in again. The account, its links and second factor stay.
  const activeMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) => usersApi.update(id, { active }),
    onSuccess: (_res, { active }) => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowSuspendConfirm(false)
      setSelectedUser(null)
      toast.success(active ? t('toast.liftedSuspension') : t('toast.suspended'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'users:toast.suspensionFailed'))
    },
  })

  // Delete user mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => usersApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowDeleteConfirm(false)
      setSelectedUser(null)
      toast.success(t('toast.deleted'))
    },
    onError: (error: unknown) => {
      toast.error(t('toast.deleteFailed', { reason: apiErrorMessage(error, 'users:toast.deleteFallback') }))
    },
  })

  // Handlers
  const handleEdit = (user: User) => {
    setSelectedUser(user)
    setShowEditModal(true)
  }

  const handleDelete = (user: User) => {
    setSelectedUser(user)
    setShowDeleteConfirm(true)
  }

  const handleEditSubmit = (data: UserFormData) => {
    if (selectedUser) {
      updateMutation.mutate({
        id: selectedUser.id,
        data: {
          name: data.name,
          // Empty clears the picture; the API keeps a field that is left out.
          avatar_url: data.avatar_url ?? '',
        },
      })
    }
  }

  const handleDeleteConfirm = () => {
    if (selectedUser) {
      deleteMutation.mutate(selectedUser.id)
    }
  }

  // The provider accounts that sign in to this one; none means password or emailed link.
  const PROVIDER_NAMES = ['google', 'github', 'microsoft', 'apple']
  const getProviderBadge = (connections: string[] = []) =>
    connections.length === 0 ? (
      <Badge variant="default" size="sm">{t('provider.passwordOrLink')}</Badge>
    ) : (
      <span className="inline-flex gap-1">
        {connections.map((c) => (
          <Badge key={c} variant="info" size="sm">
            {PROVIDER_NAMES.includes(c) ? t(`provider.${c}`) : c}
          </Badge>
        ))}
      </span>
    )

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
          <p className="mt-2 text-sm text-gray-700">
            {t('subtitle')}
          </p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title={t('noTenant')}
            description={t('noTenantHelp')}
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message={t('loading')} />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">{t('loadFailed')}</p>
        <Button className="mt-4" onClick={() => refetch()}>
          {t('retry')}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
          <p className="mt-2 text-sm text-gray-700">
            <Trans
              t={t}
              i18nKey="count"
              values={{ count: totalUsers, name: selectedTenant?.name }}
              components={{ tenant: <span className="font-medium" /> }}
            />
          </p>
        </div>
      </div>

      {/* Search */}
      <Card className="p-4">
        <div className="relative">
          <MagnifyingGlassIcon className="absolute left-3 top-1/2 transform -translate-y-1/2 h-5 w-5 text-gray-400" />
          <Input
            type="text"
            placeholder={t('search')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="pl-10"
          />
        </div>
      </Card>

      {/* Users Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.user')}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.provider')}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.status')}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.emailVerified')}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.lastLogin')}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {t('columns.created')}
                </th>
                <th className="relative px-6 py-3">
                  <span className="sr-only">{t('columns.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12">
                    <EmptyState
                      icon={<UserIcon className="h-12 w-12" />}
                      title={searchTerm ? t('empty.noResults') : t('empty.noUsers')}
                      description={searchTerm ? t('empty.tryDifferent') : t('empty.willAppear')}
                    />
                  </td>
                </tr>
              ) : (
                filteredUsers.map((user: User) => (
                  <tr key={user.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center">
                        <UserAvatar
                          name={user.name}
                          email={user.email}
                          avatarUrl={user.avatar_url}
                        />
                        <div className="ml-4">
                          <div className="text-sm font-medium text-gray-900">
                            {user.name || t('unnamed')}
                          </div>
                          <div className="text-sm text-gray-500">{user.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getProviderBadge(user.connections)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <StatusBadge
                        active={user.active}
                        activeText={t('status.active')}
                        inactiveText={t('status.suspended')}
                      />
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {user.email_verified ? (
                        <span className="flex items-center text-sm text-green-600">
                          <CheckCircleIcon className="h-5 w-5 mr-1" />
                          {t('verified')}
                        </span>
                      ) : (
                        <span className="flex items-center text-sm text-red-600">
                          <XCircleIcon className="h-5 w-5 mr-1" />
                          {t('notVerified')}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {user.last_login_at ? formatDay(user.last_login_at) : t('never')}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDay(user.created_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      <div className="flex items-center justify-end space-x-2">
                        <button
                          onClick={() => handleEdit(user)}
                          className="text-indigo-600 hover:text-indigo-900"
                          title={t('action.edit')}
                        >
                          <PencilIcon className="h-5 w-5" />
                        </button>
                        {user.active ? (
                          <button
                            onClick={() => {
                              setSelectedUser(user)
                              setShowSuspendConfirm(true)
                            }}
                            className="text-amber-600 hover:text-amber-900"
                            title={t('action.suspend')}
                            aria-label={t('action.suspendLabel', { email: user.email })}
                          >
                            <NoSymbolIcon className="h-5 w-5" />
                          </button>
                        ) : (
                          <button
                            onClick={() => activeMutation.mutate({ id: user.id, active: true })}
                            className="text-green-600 hover:text-green-900"
                            title={t('action.lift')}
                            aria-label={t('action.liftLabel', { email: user.email })}
                          >
                            <ArrowPathIcon className="h-5 w-5" />
                          </button>
                        )}
                        <button
                          onClick={() => handleDelete(user)}
                          className="text-red-600 hover:text-red-900"
                          title={t('action.delete')}
                        >
                          <TrashIcon className="h-5 w-5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            totalItems={totalUsers}
            itemsPerPage={pageSize}
            onPageChange={setCurrentPage}
          />
        )}
      </Card>

      {/* Edit Modal */}
      {selectedUser && (
        <UserFormModal
          isOpen={showEditModal}
          onClose={() => {
            setShowEditModal(false)
            setSelectedUser(null)
          }}
          onSubmit={handleEditSubmit}
          user={selectedUser}
          isSubmitting={updateMutation.isPending}
        />
      )}

      {/* Suspend Confirmation */}
      <ConfirmDialog
        isOpen={showSuspendConfirm}
        onClose={() => {
          setShowSuspendConfirm(false)
          setSelectedUser(null)
        }}
        onConfirm={() => selectedUser && activeMutation.mutate({ id: selectedUser.id, active: false })}
        title={t('suspendConfirm.title')}
        message={t('suspendConfirm.message', { email: selectedUser?.email })}
        confirmText={t('suspendConfirm.confirm')}
        variant="danger"
        isLoading={activeMutation.isPending}
      />

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={showDeleteConfirm}
        onClose={() => {
          setShowDeleteConfirm(false)
          setSelectedUser(null)
        }}
        onConfirm={handleDeleteConfirm}
        title={t('deleteConfirm.title')}
        message={t('deleteConfirm.message', { email: selectedUser?.email })}
        confirmText={t('deleteConfirm.confirm')}
        variant="danger"
        isLoading={deleteMutation.isPending}
      />
    </div>
  )
}

export default UsersPage
