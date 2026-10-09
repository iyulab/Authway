import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  BellAlertIcon,
  PlusIcon,
  PencilIcon,
  TrashIcon,
  PlayIcon,
  BuildingOfficeIcon,
  KeyIcon,
} from '@heroicons/react/24/outline'
import { webhooksApi, Webhook } from '@/lib/features-api'
import {
  Button,
  Card,
  Loading,
  EmptyState,
  ConfirmDialog,
  Badge,
  Input,
  Modal,
} from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'

// Available webhook event types
const WEBHOOK_EVENTS = [
  { value: 'user.created', label: 'User Created' },
  { value: 'user.updated', label: 'User Updated' },
  { value: 'user.deleted', label: 'User Deleted' },
  { value: 'user.login', label: 'User Login' },
  { value: 'user.logout', label: 'User Logout' },
  { value: 'user.password_changed', label: 'Password Changed' },
  { value: 'user.mfa_enabled', label: 'MFA Enabled' },
  { value: 'user.mfa_disabled', label: 'MFA Disabled' },
  { value: 'session.created', label: 'Session Created' },
  { value: 'session.revoked', label: 'Session Revoked' },
  { value: 'client.created', label: 'Client Created' },
  { value: 'client.updated', label: 'Client Updated' },
  { value: 'client.deleted', label: 'Client Deleted' },
]

interface WebhookFormData {
  name: string
  url: string
  events: string[]
  enabled: boolean
  retry_count: number
  timeout_secs: number
}

const WebhooksPage: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant } = useTenantStore()
  const selectedTenantId = selectedTenant?.id || ''

  // State
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showEditModal, setShowEditModal] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [selectedWebhook, setSelectedWebhook] = useState<Webhook | null>(null)
  // The signing secret is shown once, right after creation or rotation.
  const [revealed, setRevealed] = useState<{ name: string; secret: string } | null>(null)
  const [rotateTarget, setRotateTarget] = useState<Webhook | null>(null)
  const [formData, setFormData] = useState<WebhookFormData>({
    name: '',
    url: '',
    events: [],
    enabled: true,
    retry_count: 3,
    timeout_secs: 30,
  })

  // Fetch webhooks
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['webhooks', selectedTenantId],
    queryFn: () => webhooksApi.list({ tenant_id: selectedTenantId }),
    enabled: !!selectedTenantId,
  })

  const webhooks = data?.data.webhooks || []

  // Create mutation — the API client sends the selected tenant as X-Tenant-ID
  const createMutation = useMutation({
    mutationFn: (data: WebhookFormData) => webhooksApi.create(data),
    onSuccess: ({ data }) => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setShowCreateModal(false)
      resetForm()
      setRevealed({ name: data.webhook.name, secret: data.secret })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to create webhook')
    },
  })

  // Update mutation
  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Webhook> }) =>
      webhooksApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setShowEditModal(false)
      setSelectedWebhook(null)
      resetForm()
      toast.success('Webhook updated successfully')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to update webhook')
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => webhooksApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setShowDeleteConfirm(false)
      setSelectedWebhook(null)
      toast.success('Webhook deleted successfully')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to delete webhook')
    },
  })

  const rotateMutation = useMutation({
    mutationFn: (webhook: Webhook) => webhooksApi.rotateSecret(webhook.id),
    onSuccess: ({ data }, webhook) => {
      setRotateTarget(null)
      setRevealed({ name: webhook.name, secret: data.secret })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to rotate secret')
    },
  })

  // Test mutation
  const testMutation = useMutation({
    mutationFn: (id: string) => webhooksApi.test(id),
    onSuccess: ({ data: { delivery } }) => {
      if (delivery.success) {
        toast.success(`Test delivered (HTTP ${delivery.status_code})`)
      } else {
        toast.error(`Test not delivered: ${delivery.error_message || `HTTP ${delivery.status_code}`}`)
      }
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to test webhook')
    },
  })

  const resetForm = () => {
    setFormData({
      name: '',
      url: '',
      events: [],
      enabled: true,
      retry_count: 3,
      timeout_secs: 30,
    })
  }

  const handleEdit = (webhook: Webhook) => {
    setSelectedWebhook(webhook)
    setFormData({
      name: webhook.name,
      url: webhook.url,
      events: webhook.events,
      enabled: webhook.enabled,
      retry_count: webhook.retry_count,
      timeout_secs: webhook.timeout_secs,
    })
    setShowEditModal(true)
  }

  const handleDelete = (webhook: Webhook) => {
    setSelectedWebhook(webhook)
    setShowDeleteConfirm(true)
  }

  const handleEventToggle = (event: string) => {
    setFormData((prev) => ({
      ...prev,
      events: prev.events.includes(event)
        ? prev.events.filter((e) => e !== event)
        : [...prev.events, event],
    }))
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (selectedWebhook) {
      updateMutation.mutate({ id: selectedWebhook.id, data: formData })
    } else {
      createMutation.mutate(formData)
    }
  }

  // Show message if no tenant selected
  if (!selectedTenantId) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Webhook Management</h1>
          <p className="mt-2 text-sm text-gray-700">Configure webhooks to receive event notifications.</p>
        </div>
        <Card className="p-8">
          <EmptyState
            icon={<BuildingOfficeIcon className="h-12 w-12" />}
            title="No tenant selected"
            description="Please select a tenant from the header to manage webhooks."
          />
        </Card>
      </div>
    )
  }

  if (isLoading) {
    return <Loading message="Loading webhooks..." />
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">Failed to load webhooks.</p>
        <Button className="mt-4" onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Webhook Management</h1>
          <p className="mt-2 text-sm text-gray-700">
            {webhooks.length} webhook(s) configured for <span className="font-medium">{selectedTenant?.name}</span>.
          </p>
        </div>
        <Button onClick={() => setShowCreateModal(true)}>
          <PlusIcon className="h-5 w-5 mr-2" />
          Add Webhook
        </Button>
      </div>

      {/* Webhooks Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">URL</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Events</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                <th className="relative px-6 py-3"><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {webhooks.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-6 py-12">
                    <EmptyState
                      icon={<BellAlertIcon className="h-12 w-12" />}
                      title="No webhooks"
                      description="Create a webhook to receive event notifications"
                    />
                  </td>
                </tr>
              ) : (
                webhooks.map((webhook) => (
                  <tr key={webhook.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{webhook.name}</div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="text-sm text-gray-500 truncate max-w-xs">{webhook.url}</div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap gap-1">
                        {webhook.events.slice(0, 3).map((event) => (
                          <Badge key={event} variant="default" size="sm">{event}</Badge>
                        ))}
                        {webhook.events.length > 3 && (
                          <Badge variant="default" size="sm">+{webhook.events.length - 3}</Badge>
                        )}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <Badge variant={webhook.enabled ? 'success' : 'default'}>
                        {webhook.enabled ? 'Active' : 'Disabled'}
                      </Badge>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(webhook.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      <div className="flex items-center justify-end space-x-2">
                        <button
                          onClick={() => testMutation.mutate(webhook.id)}
                          className="text-green-600 hover:text-green-900"
                          title="Test"
                          disabled={testMutation.isPending}
                        >
                          <PlayIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => setRotateTarget(webhook)}
                          className="text-amber-600 hover:text-amber-900"
                          title="Rotate signing secret"
                        >
                          <KeyIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => handleEdit(webhook)}
                          className="text-indigo-600 hover:text-indigo-900"
                          title="Edit"
                        >
                          <PencilIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => handleDelete(webhook)}
                          className="text-red-600 hover:text-red-900"
                          title="Delete"
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
      </Card>

      {/* Create/Edit Modal */}
      <Modal
        isOpen={showCreateModal || showEditModal}
        onClose={() => {
          setShowCreateModal(false)
          setShowEditModal(false)
          setSelectedWebhook(null)
          resetForm()
        }}
        title={selectedWebhook ? 'Edit Webhook' : 'Create Webhook'}
      >
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">Name</label>
            <Input
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="My Webhook"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">URL</label>
            <Input
              type="url"
              value={formData.url}
              onChange={(e) => setFormData({ ...formData, url: e.target.value })}
              placeholder="https://example.com/webhook"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">Events</label>
            <div className="grid grid-cols-2 gap-2 max-h-48 overflow-y-auto">
              {WEBHOOK_EVENTS.map((event) => (
                <label key={event.value} className="flex items-center space-x-2">
                  <input
                    type="checkbox"
                    checked={formData.events.includes(event.value)}
                    onChange={() => handleEventToggle(event.value)}
                    className="rounded border-gray-300"
                  />
                  <span className="text-sm text-gray-700">{event.label}</span>
                </label>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">Retry Count</label>
              <Input
                type="number"
                min={0}
                max={10}
                value={formData.retry_count}
                onChange={(e) => setFormData({ ...formData, retry_count: parseInt(e.target.value) })}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Timeout (seconds)</label>
              <Input
                type="number"
                min={5}
                max={60}
                value={formData.timeout_secs}
                onChange={(e) => setFormData({ ...formData, timeout_secs: parseInt(e.target.value) })}
              />
            </div>
          </div>

          <div className="flex items-center">
            <input
              type="checkbox"
              checked={formData.enabled}
              onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
              className="rounded border-gray-300"
            />
            <label className="ml-2 text-sm text-gray-700">Enabled</label>
          </div>

          <div className="flex justify-end space-x-3 pt-4">
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setShowCreateModal(false)
                setShowEditModal(false)
                setSelectedWebhook(null)
                resetForm()
              }}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              isLoading={createMutation.isPending || updateMutation.isPending}
            >
              {selectedWebhook ? 'Update' : 'Create'}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Signing secret, shown once */}
      <Modal isOpen={revealed !== null} onClose={() => setRevealed(null)} title="Signing secret">
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
            Deliveries to <strong>{revealed?.name}</strong> are signed with this secret. Copy it now —
            it is not shown again. Rotate it to get a new one.
          </p>
          <code className="block break-all rounded bg-gray-100 p-3 font-mono text-sm" data-testid="webhook-secret">
            {revealed?.secret}
          </code>
          <div className="flex justify-end gap-3">
            <Button
              variant="outline"
              onClick={() => {
                if (revealed) {
                  navigator.clipboard?.writeText(revealed.secret).then(
                    () => toast.success('Copied'),
                    () => toast.error('Copy failed — select and copy it by hand'),
                  )
                }
              }}
            >
              Copy
            </Button>
            <Button onClick={() => setRevealed(null)}>Done</Button>
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        isOpen={rotateTarget !== null}
        onClose={() => setRotateTarget(null)}
        onConfirm={() => rotateTarget && rotateMutation.mutate(rotateTarget)}
        title="Rotate signing secret"
        message={`Deliveries to "${rotateTarget?.name}" will be signed with a new secret, and the receiver will reject them until it has it. Continue?`}
        confirmText="Rotate"
        variant="danger"
        isLoading={rotateMutation.isPending}
      />

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={showDeleteConfirm}
        onClose={() => {
          setShowDeleteConfirm(false)
          setSelectedWebhook(null)
        }}
        onConfirm={() => selectedWebhook && deleteMutation.mutate(selectedWebhook.id)}
        title="Delete Webhook"
        message={`Are you sure you want to delete "${selectedWebhook?.name}"? This action cannot be undone.`}
        confirmText="Delete"
        variant="danger"
        isLoading={deleteMutation.isPending}
      />
    </div>
  )
}

export default WebhooksPage
