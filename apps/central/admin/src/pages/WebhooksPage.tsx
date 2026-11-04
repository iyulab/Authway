import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Trans, useTranslation } from 'react-i18next'
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
import { apiErrorMessage, refusalAt } from '@/lib/api'
import { formatDay } from '@/lib/utils'
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
// `key` names the event's words in the `webhooks` namespace (`events.<key>`).
const WEBHOOK_EVENTS = [
  { value: '*', key: 'all' },
  { value: 'user.created', key: 'userCreated' },
  { value: 'user.updated', key: 'userUpdated' },
  { value: 'user.deleted', key: 'userDeleted' },
  { value: 'user.login', key: 'userLogin' },
  { value: 'user.logout', key: 'userLogout' },
  { value: 'user.password_changed', key: 'userPasswordChanged' },
  { value: 'user.mfa_enabled', key: 'userMfaEnabled' },
  { value: 'user.mfa_disabled', key: 'userMfaDisabled' },
  { value: 'client.created', key: 'clientCreated' },
  { value: 'client.updated', key: 'clientUpdated' },
  { value: 'client.deleted', key: 'clientDeleted' },
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
  const { t } = useTranslation('webhooks')
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
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'webhooks:toast.createFailed'))
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
      toast.success(t('toast.updated'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'webhooks:toast.updateFailed'))
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => webhooksApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setShowDeleteConfirm(false)
      setSelectedWebhook(null)
      toast.success(t('toast.deleted'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'webhooks:toast.deleteFailed'))
    },
  })

  const rotateMutation = useMutation({
    mutationFn: (webhook: Webhook) => webhooksApi.rotateSecret(webhook.id),
    onSuccess: ({ data }, webhook) => {
      setRotateTarget(null)
      setRevealed({ name: webhook.name, secret: data.secret })
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'webhooks:toast.rotateFailed'))
    },
  })

  // Test mutation
  const testMutation = useMutation({
    mutationFn: (id: string) => webhooksApi.test(id),
    onSuccess: ({ data: { delivery } }) => {
      if (delivery.success) {
        toast.success(t('toast.testDelivered', { status: delivery.status_code }))
      } else {
        toast.error(
          t('toast.testNotDelivered', {
            reason: delivery.error_message || t('toast.httpStatus', { status: delivery.status_code }),
          }),
        )
      }
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'webhooks:toast.testFailed'))
    },
  })

  // The refusal of the form's last submission, shown at the member it names.
  // Any edit, or closing the form, makes it stale.
  const formError = selectedWebhook ? updateMutation.error : createMutation.error
  const refused = (field: string) => refusalAt(formError, field)
  const edit = (patch: Partial<WebhookFormData>) => {
    if (createMutation.error) createMutation.reset()
    if (updateMutation.error) updateMutation.reset()
    setFormData((prev) => ({ ...prev, ...patch }))
  }

  const resetForm = () => {
    createMutation.reset()
    updateMutation.reset()
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
    edit({
      events: formData.events.includes(event)
        ? formData.events.filter((e) => e !== event)
        : [...formData.events, event],
    })
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    // A blank number field would be sent as nothing and quietly mean "the default".
    if (!Number.isFinite(formData.retry_count) || formData.retry_count < 0 || formData.retry_count > 10) {
      toast.error(t('form.retryInvalid'))
      return
    }
    if (!Number.isFinite(formData.timeout_secs) || formData.timeout_secs < 1 || formData.timeout_secs > 60) {
      toast.error(t('form.timeoutInvalid'))
      return
    }
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
          <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
          <p className="mt-2 text-sm text-gray-700">{t('subtitle')}</p>
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
        <Button className="mt-4" onClick={() => refetch()}>{t('retry')}</Button>
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
              values={{ count: webhooks.length, name: selectedTenant?.name }}
              components={{ tenant: <span className="font-medium" /> }}
            />
          </p>
        </div>
        <Button onClick={() => setShowCreateModal(true)}>
          <PlusIcon className="h-5 w-5 mr-2" />
          {t('add')}
        </Button>
      </div>

      {/* Webhooks Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.name')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.url')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.events')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.status')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.created')}</th>
                <th className="relative px-6 py-3"><span className="sr-only">{t('columns.actions')}</span></th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {webhooks.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-6 py-12">
                    <EmptyState
                      icon={<BellAlertIcon className="h-12 w-12" />}
                      title={t('empty.title')}
                      description={t('empty.description')}
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
                        {webhook.enabled ? t('status.active') : t('status.disabled')}
                      </Badge>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDay(webhook.created_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      <div className="flex items-center justify-end space-x-2">
                        <button
                          onClick={() => testMutation.mutate(webhook.id)}
                          className="text-green-600 hover:text-green-900"
                          title={t('action.test')}
                          disabled={testMutation.isPending}
                        >
                          <PlayIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => setRotateTarget(webhook)}
                          className="text-amber-600 hover:text-amber-900"
                          title={t('action.rotate')}
                        >
                          <KeyIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => handleEdit(webhook)}
                          className="text-indigo-600 hover:text-indigo-900"
                          title={t('action.edit')}
                        >
                          <PencilIcon className="h-5 w-5" />
                        </button>
                        <button
                          onClick={() => handleDelete(webhook)}
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
        title={selectedWebhook ? t('form.editTitle') : t('form.createTitle')}
      >
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">{t('form.name')}</label>
            <Input
              value={formData.name}
              onChange={(e) => edit({ name: e.target.value })}
              error={refused('name')}
              placeholder={t('form.namePlaceholder')}
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700">{t('form.url')}</label>
            <Input
              type="url"
              value={formData.url}
              onChange={(e) => edit({ url: e.target.value })}
              error={refused('url')}
              placeholder={t('form.urlPlaceholder')}
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">{t('form.events')}</label>
            <div className="grid grid-cols-2 gap-2 max-h-48 overflow-y-auto">
              {WEBHOOK_EVENTS.map((event) => (
                <label key={event.value} className="flex items-center space-x-2">
                  <input
                    type="checkbox"
                    checked={formData.events.includes(event.value)}
                    onChange={() => handleEventToggle(event.value)}
                    className="rounded border-gray-300"
                  />
                  <span className="text-sm text-gray-700">{t(`events.${event.key}`)}</span>
                </label>
              ))}
            </div>
            {refused('events') && <p className="mt-1 text-sm text-red-600">{refused('events')}</p>}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('form.retryCount')}</label>
              <Input
                type="number"
                min={0}
                max={10}
                value={formData.retry_count}
                onChange={(e) => edit({ retry_count: parseInt(e.target.value) })}
                error={refused('retry_count')}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('form.timeout')}</label>
              <Input
                type="number"
                min={1}
                max={60}
                value={formData.timeout_secs}
                onChange={(e) => edit({ timeout_secs: parseInt(e.target.value) })}
                error={refused('timeout_secs')}
              />
            </div>
          </div>

          <div className="flex items-center">
            <input
              type="checkbox"
              checked={formData.enabled}
              onChange={(e) => edit({ enabled: e.target.checked })}
              className="rounded border-gray-300"
            />
            <label className="ml-2 text-sm text-gray-700">{t('form.enabled')}</label>
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
              {t('form.cancel')}
            </Button>
            <Button
              type="submit"
              isLoading={createMutation.isPending || updateMutation.isPending}
            >
              {selectedWebhook ? t('form.update') : t('form.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Signing secret, shown once */}
      <Modal isOpen={revealed !== null} onClose={() => setRevealed(null)} title={t('secret.title')}>
        <div className="space-y-4">
          <p className="text-sm text-gray-600">
            <Trans t={t} i18nKey="secret.body" values={{ name: revealed?.name }} components={{ name: <strong /> }} />
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
                    () => toast.success(t('secret.copied')),
                    () => toast.error(t('secret.copyFailed')),
                  )
                }
              }}
            >
              {t('secret.copy')}
            </Button>
            <Button onClick={() => setRevealed(null)}>{t('secret.done')}</Button>
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        isOpen={rotateTarget !== null}
        onClose={() => setRotateTarget(null)}
        onConfirm={() => rotateTarget && rotateMutation.mutate(rotateTarget)}
        title={t('rotateConfirm.title')}
        message={t('rotateConfirm.message', { name: rotateTarget?.name })}
        confirmText={t('rotateConfirm.confirm')}
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
        title={t('deleteConfirm.title')}
        message={t('deleteConfirm.message', { name: selectedWebhook?.name })}
        confirmText={t('deleteConfirm.confirm')}
        variant="danger"
        isLoading={deleteMutation.isPending}
      />
    </div>
  )
}

export default WebhooksPage
