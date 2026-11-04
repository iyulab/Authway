import React, { useState } from 'react'
import { useParams, useNavigate } from 'react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  TrashIcon,
  KeyIcon,
  ClipboardDocumentIcon,
  CheckIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline'
import { apiErrorMessage, clientsApi, serverRefusal } from '@/lib/api'
import { Button, Card, Loading, Badge, ConfirmDialog, Modal } from '@/components/ui'
import { ClientForm, ClientFormData, lines } from '@/components/clients'

const ClientDetailPage: React.FC = () => {
  const { t } = useTranslation('clients')
  const { clientId } = useParams<{ clientId: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showRegenerateConfirm, setShowRegenerateConfirm] = useState(false)
  const [showTurnOffConfirm, setShowTurnOffConfirm] = useState(false)
  const [showCredentialsModal, setShowCredentialsModal] = useState(false)
  const [credentials, setCredentials] = useState<{ client_id: string; client_secret: string } | null>(null)
  const [copiedField, setCopiedField] = useState<string | null>(null)

  // Fetch client
  const { data: clientData, isLoading, error } = useQuery({
    queryKey: ['client', clientId],
    queryFn: () => clientsApi.get(clientId!),
    enabled: !!clientId,
  })

  const client = clientData?.data?.client

  // Update mutation
  const updateMutation = useMutation({
    mutationFn: (data: ClientFormData) => {
      const redirectUris = lines(data.redirect_uris)
      const postLogoutRedirectUris = lines(data.post_logout_redirect_uris)
      const allowedOrigins = lines(data.allowed_origins)
      const audience = lines(data.audience)
      const backChannelLogoutUri = (data.backchannel_logout_uri ?? '').trim()

      return clientsApi.update(clientId!, {
        name: data.name,
        description: data.description ?? '',
        // Sent as they stand: '' clears what was there.
        client_uri: (data.client_uri ?? '').trim(),
        logo: (data.logo ?? '').trim(),
        initiate_login_uri: (data.initiate_login_uri ?? '').trim(),
        redirect_uris: redirectUris,
        post_logout_redirect_uris: postLogoutRedirectUris,
        allowed_origins: allowedOrigins,
        // Sent as they stand: an empty list or '' clears what was there.
        audience,
        backchannel_logout_uri: backChannelLogoutUri,
        grant_types: data.grant_types,
        scopes: data.scopes,
        public: data.public,
        enabled_auth_providers: data.enabled_auth_providers || ['email', 'google'],
        skip_consent: data.skip_consent ?? false,
        skip_logout_consent: data.skip_logout_consent ?? false,
        // '' is meaningful on update: it clears the pin and restores inheritance.
        access_token_strategy: data.access_token_strategy ?? '',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['client', clientId] })
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success(t('detail.toast.updated'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'clients:detail.toast.updateFailed'))
    },
  })

  // Turning the client off or on — only `active` is sent.
  const activeMutation = useMutation({
    mutationFn: (active: boolean) => clientsApi.update(clientId!, { active }),
    onSuccess: (_, active) => {
      setShowTurnOffConfirm(false)
      queryClient.invalidateQueries({ queryKey: ['client', clientId] })
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success(active ? t('detail.toast.turnedOn') : t('detail.toast.turnedOff'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'clients:detail.toast.changeFailed'))
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: () => clientsApi.delete(clientId!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success(t('detail.toast.deleted'))
      navigate('/clients')
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'clients:detail.toast.deleteFailed'))
    },
  })

  // Regenerate secret mutation
  const regenerateMutation = useMutation({
    mutationFn: () => clientsApi.regenerateSecret(clientId!),
    onSuccess: (response) => {
      setCredentials(response.data.credentials)
      setShowRegenerateConfirm(false)
      setShowCredentialsModal(true)
      toast.success(t('detail.toast.regenerated'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'clients:detail.toast.regenerateFailed'))
    },
  })

  const handleCopy = async (text: string, field: string) => {
    await navigator.clipboard.writeText(text)
    setCopiedField(field)
    setTimeout(() => setCopiedField(null), 2000)
  }

  if (isLoading) {
    return <Loading message={t('detail.loading')} />
  }

  if (error || !client) {
    return (
      <div className="space-y-6">
        <Button
          variant="secondary"
          leftIcon={<ArrowLeftIcon className="h-4 w-4" />}
          onClick={() => navigate('/clients')}
        >
          {t('detail.backToList')}
        </Button>
        <Card className="p-8 text-center">
          <p className="text-red-600">{t('detail.notFound')}</p>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Button
            variant="secondary"
            size="sm"
            leftIcon={<ArrowLeftIcon className="h-4 w-4" />}
            onClick={() => navigate('/clients')}
          >
            {t('detail.back')}
          </Button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold text-gray-900">{client.name}</h1>
              <Badge variant={client.active ? 'success' : 'default'}>
                {client.active ? t('badge.active') : t('badge.inactive')}
              </Badge>
              <Badge variant={client.public ? 'info' : 'purple'}>
                {client.public ? t('badge.public') : t('badge.confidential')}
              </Badge>
              {client.system && <Badge variant="default">{t('badge.system')}</Badge>}
            </div>
            <p className="text-sm text-gray-500 font-mono mt-1">{client.client_id}</p>
          </div>
        </div>
      </div>

      {/* Client ID Info */}
      <Card className="p-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-gray-700">{t('credentials.clientId')}</h3>
            <p className="text-sm font-mono text-gray-900 mt-1">{client.client_id}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              leftIcon={copiedField === 'client_id' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
              onClick={() => handleCopy(client.client_id, 'client_id')}
            >
              {copiedField === 'client_id' ? t('detail.copiedBang') : t('detail.copy')}
            </Button>
            {!client.public && (
              <Button
                variant="secondary"
                size="sm"
                leftIcon={<KeyIcon className="h-4 w-4" />}
                onClick={() => setShowRegenerateConfirm(true)}
              >
                {t('detail.regenerate')}
              </Button>
            )}
          </div>
        </div>
      </Card>

      {client.system ? (
        <Card>
          <div className="p-6">
            <h2 className="text-lg font-medium text-gray-900">{t('detail.managed.title')}</h2>
            <p className="text-sm text-gray-500 mt-1">
              {t('detail.managed.body')}
            </p>
          </div>
        </Card>
      ) : (
        <>
      {/* Edit Form */}
      <Card>
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-lg font-medium text-gray-900">{t('detail.settings.title')}</h2>
          <p className="text-sm text-gray-500 mt-1">
            {t('detail.settings.help')}
          </p>
        </div>
        <div className="p-6">
          <ClientForm
            initialData={client}
            serverError={serverRefusal(updateMutation.error)}
            onSubmit={(data) => updateMutation.mutate(data)}
            onCancel={() => navigate('/clients')}
            isSubmitting={updateMutation.isPending}
            submitLabel={t('detail.settings.save')}
          />
        </div>
      </Card>

      {/* Danger Zone */}
      <Card className="border-2 border-red-200">
        <div className="px-6 py-4 border-b border-red-200 bg-red-50">
          <div className="flex items-center gap-2">
            <ExclamationTriangleIcon className="h-5 w-5 text-red-500" />
            <h2 className="text-lg font-medium text-red-900">{t('detail.danger.title')}</h2>
          </div>
        </div>
        <div className="p-6 space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-gray-900">
                {client.active ? t('detail.danger.turnOffTitle') : t('detail.danger.turnOnTitle')}
              </h3>
              <p className="text-sm text-gray-500 mt-1">
                {client.active ? t('detail.danger.turnOffHelp') : t('detail.danger.turnOnHelp')}
              </p>
            </div>
            {client.active ? (
              <Button variant="danger" onClick={() => setShowTurnOffConfirm(true)}>
                {t('detail.danger.turnOff')}
              </Button>
            ) : (
              <Button variant="secondary" onClick={() => activeMutation.mutate(true)} isLoading={activeMutation.isPending}>
                {t('detail.danger.turnOn')}
              </Button>
            )}
          </div>
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-gray-900">{t('detail.danger.deleteTitle')}</h3>
              <p className="text-sm text-gray-500 mt-1">
                {t('detail.danger.deleteHelp')}
              </p>
            </div>
            <Button
              variant="danger"
              leftIcon={<TrashIcon className="h-4 w-4" />}
              onClick={() => setShowDeleteConfirm(true)}
            >
              {t('detail.danger.delete')}
            </Button>
          </div>
        </div>
      </Card>
        </>
      )}

      {/* Turn Off Confirmation */}
      <ConfirmDialog
        isOpen={showTurnOffConfirm}
        onClose={() => setShowTurnOffConfirm(false)}
        onConfirm={() => activeMutation.mutate(false)}
        title={t('detail.turnOffConfirm.title')}
        message={t('detail.turnOffConfirm.message', { name: client.name })}
        confirmText={t('detail.turnOffConfirm.confirm')}
        variant="danger"
        isLoading={activeMutation.isPending}
      />

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={showDeleteConfirm}
        onClose={() => setShowDeleteConfirm(false)}
        onConfirm={() => deleteMutation.mutate()}
        title={t('detail.deleteConfirm.title')}
        message={t('detail.deleteConfirm.message', { name: client.name })}
        confirmText={t('detail.deleteConfirm.confirm')}
        variant="danger"
        isLoading={deleteMutation.isPending}
      />

      {/* Regenerate Secret Confirmation */}
      <ConfirmDialog
        isOpen={showRegenerateConfirm}
        onClose={() => setShowRegenerateConfirm(false)}
        onConfirm={() => regenerateMutation.mutate()}
        title={t('detail.regenerateConfirm.title')}
        message={t('detail.regenerateConfirm.message', { name: client.name })}
        confirmText={t('detail.regenerateConfirm.confirm')}
        variant="warning"
        isLoading={regenerateMutation.isPending}
      />

      {/* Credentials Modal */}
      <Modal
        isOpen={showCredentialsModal}
        onClose={() => {
          setShowCredentialsModal(false)
          setCredentials(null)
        }}
        title={t('detail.secretModal.title')}
        size="md"
      >
        <div className="space-y-4">
          <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
            <div className="flex">
              <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500 mt-0.5" />
              <div className="ml-3">
                <p className="text-sm text-yellow-700">
                  {t('detail.secretModal.warning')}
                </p>
              </div>
            </div>
          </div>

          {credentials && (
            <div className="space-y-3">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('credentials.clientId')}</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 bg-gray-100 px-3 py-2 rounded text-sm font-mono break-all">
                    {credentials.client_id}
                  </code>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleCopy(credentials.client_id, 'modal_client_id')}
                  >
                    {copiedField === 'modal_client_id' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">{t('credentials.clientSecret')}</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 bg-gray-100 px-3 py-2 rounded text-sm font-mono break-all">
                    {credentials.client_secret}
                  </code>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleCopy(credentials.client_secret, 'modal_client_secret')}
                  >
                    {copiedField === 'modal_client_secret' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
            </div>
          )}

          <div className="flex justify-end pt-4">
            <Button onClick={() => {
              setShowCredentialsModal(false)
              setCredentials(null)
            }}>
              {t('credentials.done')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default ClientDetailPage
