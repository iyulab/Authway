import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  EnvelopeIcon,
  PlusIcon,
  ArrowPathIcon,
  XMarkIcon,
  BuildingOfficeIcon,
} from '@heroicons/react/24/outline'
import { invitationsApi, Invitation } from '@/lib/features-api'
import {
  Button,
  Card,
  Loading,
  EmptyState,
  ConfirmDialog,
  Badge,
  Input,
  Modal,
  Pagination,
} from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'

interface InvitationFormData {
  email: string
  role: string
  message: string
  expires_in_hours: number
}

const InvitationsPage: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''
  const pageSize = 10

  // State
  const [currentPage, setCurrentPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showRevokeConfirm, setShowRevokeConfirm] = useState(false)
  const [selectedInvitation, setSelectedInvitation] = useState<Invitation | null>(null)
  const [formData, setFormData] = useState<InvitationFormData>({
    email: '',
    role: 'user',
    message: '',
    expires_in_hours: 72,
  })

  // Fetch invitations
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['invitations', selectedTenantId, currentPage, statusFilter],
    queryFn: () =>
      invitationsApi.list({
        tenant_id: selectedTenantId,
        status: statusFilter || undefined,
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
      }),
    enabled: !!selectedTenantId,
  })

  const invitations = data?.data.invitations || []
  const totalInvitations = data?.data.count || 0
  const totalPages = Math.ceil(totalInvitations / pageSize)

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: InvitationFormData) =>
      invitationsApi.create(data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['invitations'] })
      setShowCreateModal(false)
      resetForm()
      toast.success(response.data.message || 'Invitation created successfully')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to create invitation')
    },
  })

  // Revoke mutation
  const revokeMutation = useMutation({
    mutationFn: (id: string) => invitationsApi.revoke(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['invitations'] })
      setShowRevokeConfirm(false)
      setSelectedInvitation(null)
      toast.success('Invitation revoked successfully')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to revoke invitation')
    },
  })

  // Resend mutation
  const resendMutation = useMutation({
    mutationFn: (id: string) => invitationsApi.resend(id),
    onSuccess: (response) => {
      toast.success(response.data.message || 'Invitation resent successfully')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to resend invitation')
    },
  })

  const resetForm = () => {
    setFormData({
      email: '',
      role: 'user',
      message: '',
      expires_in_hours: 72,
    })
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'pending':
        return <Badge variant="warning">Pending</Badge>
      case 'accepted':
        return <Badge variant="success">Accepted</Badge>
      case 'expired':
        return <Badge variant="default">Expired</Badge>
      case 'revoked':
        return <Badge variant="danger">Revoked</Badge>
      default:
        return <Badge variant="default">{status}</Badge>
    }
  }

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Invitations</h1>
          <p className="mt-2 text-sm text-gray-700">Invite users to join your organization.</p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant from the header to manage invitations."
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message="Loading invitations..." />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">Failed to load invitations.</p>
        <Button className="mt-4" onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Invitations</h1>
          <p className="mt-2 text-sm text-gray-700">
            {totalInvitations} invitation(s) for <span className="font-medium">{selectedTenant?.name}</span>.
          </p>
        </div>
        <Button onClick={() => setShowCreateModal(true)}>
          <PlusIcon className="h-5 w-5 mr-2" />
          Invite User
        </Button>
      </div>

      {/* Status Filter */}
      <Card className="p-4">
        <div className="flex space-x-2">
          <Button
            variant={statusFilter === '' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter(''); setCurrentPage(1) }}
          >
            All
          </Button>
          <Button
            variant={statusFilter === 'pending' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('pending'); setCurrentPage(1) }}
          >
            Pending
          </Button>
          <Button
            variant={statusFilter === 'accepted' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('accepted'); setCurrentPage(1) }}
          >
            Accepted
          </Button>
          <Button
            variant={statusFilter === 'expired' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('expired'); setCurrentPage(1) }}
          >
            Expired
          </Button>
        </div>
      </Card>

      {/* Invitations Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Email</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Role</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Invited By</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Expires</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                <th className="relative px-6 py-3"><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {invitations.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12">
                    <EmptyState
                      icon={<EnvelopeIcon className="h-12 w-12" />}
                      title="No invitations"
                      description="Invite users to join your organization"
                    />
                  </td>
                </tr>
              ) : (
                invitations.map((invitation: Invitation) => (
                  <tr key={invitation.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{invitation.email}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <Badge variant="info">{invitation.role}</Badge>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getStatusBadge(invitation.status)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {invitation.inviter_name || '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(invitation.expires_at).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(invitation.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      {invitation.status === 'pending' && (
                        <div className="flex items-center justify-end space-x-2">
                          <button
                            onClick={() => resendMutation.mutate(invitation.id)}
                            className="text-indigo-600 hover:text-indigo-900"
                            title="Resend"
                            disabled={resendMutation.isPending}
                          >
                            <ArrowPathIcon className="h-5 w-5" />
                          </button>
                          <button
                            onClick={() => {
                              setSelectedInvitation(invitation)
                              setShowRevokeConfirm(true)
                            }}
                            className="text-red-600 hover:text-red-900"
                            title="Revoke"
                          >
                            <XMarkIcon className="h-5 w-5" />
                          </button>
                        </div>
                      )}
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
            totalItems={totalInvitations}
            itemsPerPage={pageSize}
            onPageChange={setCurrentPage}
          />
        )}
      </Card>

      {/* Create Modal */}
      <Modal
        isOpen={showCreateModal}
        onClose={() => {
          setShowCreateModal(false)
          resetForm()
        }}
        title="Invite User"
      >
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">Email</label>
            <Input
              type="email"
              value={formData.email}
              onChange={(e) => setFormData({ ...formData, email: e.target.value })}
              placeholder="user@example.com"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">Role</label>
            <select
              value={formData.role}
              onChange={(e) => setFormData({ ...formData, role: e.target.value })}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
            >
              <option value="user">User</option>
              <option value="admin">Admin</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">Message (optional)</label>
            <textarea
              value={formData.message}
              onChange={(e) => setFormData({ ...formData, message: e.target.value })}
              placeholder="Personal message to include in the invitation..."
              rows={3}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">Expires In (hours)</label>
            <Input
              type="number"
              min={1}
              max={168}
              value={formData.expires_in_hours}
              onChange={(e) => setFormData({ ...formData, expires_in_hours: parseInt(e.target.value) })}
            />
          </div>

          <div className="flex justify-end space-x-3 pt-4">
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setShowCreateModal(false)
                resetForm()
              }}
            >
              Cancel
            </Button>
            <Button type="submit" isLoading={createMutation.isPending}>
              Send Invitation
            </Button>
          </div>
        </form>
      </Modal>

      {/* Revoke Confirmation */}
      <ConfirmDialog
        isOpen={showRevokeConfirm}
        onClose={() => {
          setShowRevokeConfirm(false)
          setSelectedInvitation(null)
        }}
        onConfirm={() => selectedInvitation && revokeMutation.mutate(selectedInvitation.id)}
        title="Revoke Invitation"
        message={`Are you sure you want to revoke the invitation for "${selectedInvitation?.email}"?`}
        confirmText="Revoke"
        variant="danger"
        isLoading={revokeMutation.isPending}
      />
    </div>
  )
}

export default InvitationsPage
