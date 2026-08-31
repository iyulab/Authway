import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  UserGroupIcon,
  PlayIcon,
  StopIcon,
  BuildingOfficeIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline'
import { impersonationApi, ImpersonationSession } from '@/lib/features-api'
import { usersApi, User } from '@/lib/api'
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

const ImpersonationPage: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''
  const pageSize = 10

  // State
  const [currentPage, setCurrentPage] = useState(1)
  const [showStartModal, setShowStartModal] = useState(false)
  const [showEndConfirm, setShowEndConfirm] = useState(false)
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [reason, setReason] = useState('')
  const [userSearch, setUserSearch] = useState('')

  // Fetch active impersonation sessions
  const { data: activeData } = useQuery({
    queryKey: ['impersonation-active', selectedTenantId],
    enabled: !!selectedTenantId,
    queryFn: () => impersonationApi.activeSessions({ tenant_id: selectedTenantId }),
  })

  const activeSessions = activeData?.data.sessions || []
  const currentSession = activeSessions.length > 0 ? activeSessions[0] : null

  // Fetch impersonation sessions history
  const { data: sessionsData, isLoading, error, refetch } = useQuery({
    queryKey: ['impersonation-history', selectedTenantId, currentPage],
    enabled: !!selectedTenantId,
    queryFn: () =>
      impersonationApi.history({
        tenant_id: selectedTenantId,
        limit: pageSize,
      }),
  })

  const sessions = sessionsData?.data.sessions || []
  const totalSessions = sessionsData?.data.count || 0
  const totalPages = Math.ceil(totalSessions / pageSize)

  // Fetch users for selection
  const { data: usersData, isLoading: usersLoading } = useQuery({
    queryKey: ['users', selectedTenantId, userSearch],
    queryFn: () =>
      usersApi.list({
        tenant_id: selectedTenantId,
        limit: 20,
      }),
    enabled: !!selectedTenantId && showStartModal,
  })

  const users = usersData?.data.users || []
  const filteredUsers = users.filter(
    (user) =>
      user.email.toLowerCase().includes(userSearch.toLowerCase()) ||
      (user.name && user.name.toLowerCase().includes(userSearch.toLowerCase()))
  )

  // Start impersonation mutation
  const startMutation = useMutation({
    mutationFn: (data: { target_user_id: string; reason: string }) =>
      impersonationApi.start(data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['impersonation-active'] })
      queryClient.invalidateQueries({ queryKey: ['impersonation-history'] })
      setShowStartModal(false)
      setSelectedUser(null)
      setReason('')
      // Store the impersonation token and redirect
      toast.success('Impersonation started. Opening new tab...')
      // Open a new window/tab with the impersonation token
      const token = response.data.token
      window.open(`/?impersonation_token=${token}`, '_blank')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to start impersonation')
    },
  })

  // End impersonation mutation
  const endMutation = useMutation({
    mutationFn: (sessionId: string) => impersonationApi.end(sessionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['impersonation-active'] })
      queryClient.invalidateQueries({ queryKey: ['impersonation-history'] })
      setShowEndConfirm(false)
      toast.success('Impersonation ended')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to end impersonation')
    },
  })

  const handleStartImpersonation = () => {
    if (!selectedUser || !reason.trim()) {
      toast.error('Please select a user and provide a reason')
      return
    }
    startMutation.mutate({
      target_user_id: selectedUser.id,
      reason: reason.trim(),
    })
  }

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">User Impersonation</h1>
          <p className="mt-2 text-sm text-gray-700">Impersonate users for support and debugging.</p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant from the header to use impersonation."
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message="Loading impersonation data..." />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">Failed to load impersonation data.</p>
        <Button className="mt-4" onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">User Impersonation</h1>
          <p className="mt-2 text-sm text-gray-700">
            Impersonate users for support and debugging purposes.
          </p>
        </div>
        <Button onClick={() => setShowStartModal(true)}>
          <PlayIcon className="h-5 w-5 mr-2" />
          Start Impersonation
        </Button>
      </div>

      {/* Warning Banner */}
      <Card className="bg-yellow-50 border-yellow-200 p-4">
        <div className="flex items-start">
          <ExclamationTriangleIcon className="h-6 w-6 text-yellow-600 mr-3 shrink-0" />
          <div>
            <h3 className="text-sm font-medium text-yellow-800">Important Security Notice</h3>
            <p className="mt-1 text-sm text-yellow-700">
              User impersonation is a sensitive operation. All impersonation sessions are logged in the audit trail.
              Only use this feature for legitimate support purposes and always document the reason.
            </p>
          </div>
        </div>
      </Card>

      {/* Current Session */}
      {currentSession && (
        <Card className="bg-red-50 border-red-200 p-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center">
              <div className="h-3 w-3 bg-red-500 rounded-full animate-pulse mr-3" />
              <div>
                <p className="text-sm font-medium text-red-800">Active Impersonation Session</p>
                <p className="text-sm text-red-700">
                  Impersonating: {currentSession.target_user_email}
                </p>
                <p className="text-xs text-red-600">
                  Started: {new Date(currentSession.started_at).toLocaleString()}
                </p>
              </div>
            </div>
            <Button
              variant="danger"
              onClick={() => setShowEndConfirm(true)}
            >
              <StopIcon className="h-5 w-5 mr-2" />
              End Session
            </Button>
          </div>
        </Card>
      )}

      {/* Sessions History */}
      <Card>
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-lg font-medium text-gray-900">Session History</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Admin</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Target User</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Reason</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Started</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Ended</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {sessions.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-6 py-12">
                    <EmptyState
                      icon={<UserGroupIcon className="h-12 w-12" />}
                      title="No impersonation sessions"
                      description="Impersonation session history will appear here"
                    />
                  </td>
                </tr>
              ) : (
                sessions.map((session: ImpersonationSession) => (
                  <tr key={session.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                      {session.admin_email}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                      {session.target_user_email}
                    </td>
                    <td className="px-6 py-4">
                      <div className="text-sm text-gray-500 max-w-xs truncate">{session.reason}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {session.active ? (
                        <Badge variant="danger">Active</Badge>
                      ) : (
                        <Badge variant="default">Ended</Badge>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(session.started_at).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {session.ended_at ? new Date(session.ended_at).toLocaleString() : '-'}
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
            totalItems={totalSessions}
            itemsPerPage={pageSize}
            onPageChange={setCurrentPage}
          />
        )}
      </Card>

      {/* Start Impersonation Modal */}
      <Modal
        isOpen={showStartModal}
        onClose={() => {
          setShowStartModal(false)
          setSelectedUser(null)
          setReason('')
          setUserSearch('')
        }}
        title="Start Impersonation"
      >
        <div className="space-y-4">
          <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-3">
            <p className="text-sm text-yellow-800">
              This action will be logged in the audit trail. Ensure you have a valid reason.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">Search User</label>
            <Input
              value={userSearch}
              onChange={(e) => setUserSearch(e.target.value)}
              placeholder="Search by email or name..."
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Select User</label>
            <div className="border rounded-md max-h-48 overflow-y-auto">
              {usersLoading ? (
                <div className="p-4 text-center text-gray-500">Loading users...</div>
              ) : filteredUsers.length === 0 ? (
                <div className="p-4 text-center text-gray-500">No users found</div>
              ) : (
                filteredUsers.map((user) => (
                  <div
                    key={user.id}
                    onClick={() => setSelectedUser(user)}
                    className={`p-3 cursor-pointer hover:bg-gray-50 border-b last:border-b-0 ${
                      selectedUser?.id === user.id ? 'bg-indigo-50' : ''
                    }`}
                  >
                    <div className="text-sm font-medium text-gray-900">{user.name || 'Unnamed'}</div>
                    <div className="text-sm text-gray-500">{user.email}</div>
                  </div>
                ))
              )}
            </div>
          </div>

          {selectedUser && (
            <div className="bg-gray-50 rounded-lg p-3">
              <p className="text-sm text-gray-700">
                Selected: <span className="font-medium">{selectedUser.email}</span>
              </p>
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-gray-700">Reason (required)</label>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Explain why you need to impersonate this user..."
              rows={3}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
              required
            />
          </div>

          <div className="flex justify-end space-x-3 pt-4">
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setShowStartModal(false)
                setSelectedUser(null)
                setReason('')
                setUserSearch('')
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={handleStartImpersonation}
              isLoading={startMutation.isPending}
              disabled={!selectedUser || !reason.trim()}
            >
              Start Impersonation
            </Button>
          </div>
        </div>
      </Modal>

      {/* End Impersonation Confirmation */}
      <ConfirmDialog
        isOpen={showEndConfirm}
        onClose={() => setShowEndConfirm(false)}
        onConfirm={() => currentSession && endMutation.mutate(currentSession.id)}
        title="End Impersonation"
        message="Are you sure you want to end the current impersonation session?"
        confirmText="End Session"
        variant="danger"
        isLoading={endMutation.isPending}
      />
    </div>
  )
}

export default ImpersonationPage
