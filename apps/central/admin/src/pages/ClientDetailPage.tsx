import React, { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  ArrowLeftIcon,
  TrashIcon,
  KeyIcon,
  ClipboardDocumentIcon,
  CheckIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline'
import { clientsApi } from '@/lib/api'
import { Button, Card, Loading, Badge, ConfirmDialog, Modal } from '@/components/ui'
import { ClientForm, ClientFormData } from '@/components/clients'

const ClientDetailPage: React.FC = () => {
  const { clientId } = useParams<{ clientId: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showRegenerateConfirm, setShowRegenerateConfirm] = useState(false)
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
      const redirectUris = data.redirect_uris
        .split('\n')
        .map((uri) => uri.trim())
        .filter((uri) => uri.length > 0)

      const postLogoutRedirectUris = data.post_logout_redirect_uris
        ? data.post_logout_redirect_uris
            .split('\n')
            .map((uri) => uri.trim())
            .filter((uri) => uri.length > 0)
        : []

      return clientsApi.update(clientId!, {
        name: data.name,
        description: data.description ?? '',
        website: data.website ?? '',
        redirect_uris: redirectUris,
        post_logout_redirect_uris: postLogoutRedirectUris,
        logout_redirect_policy: data.logout_redirect_policy || 'strict',
        default_logout_uri: data.default_logout_uri ?? '',
        allow_wildcard_logout: data.allow_wildcard_logout ?? false,
        grant_types: data.grant_types,
        scopes: data.scopes,
        public: data.public,
        enabled_auth_providers: data.enabled_auth_providers || ['email', 'google'],
        allow_email_signup: data.allow_email_signup ?? true,
        allow_email_login: data.allow_email_login ?? true,
        skip_consent: data.skip_consent ?? false,
        skip_logout_consent: data.skip_logout_consent ?? false,
        // '' is meaningful on update: it clears the pin and restores inheritance.
        access_token_strategy: data.access_token_strategy ?? '',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['client', clientId] })
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success('Client updated successfully')
    },
    onError: (error: any) => {
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to update client'
      toast.error(errorMessage)
    },
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: () => clientsApi.delete(clientId!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success('Client deleted successfully')
      navigate('/clients')
    },
    onError: (error: any) => {
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to delete client'
      toast.error(errorMessage)
    },
  })

  // Regenerate secret mutation
  const regenerateMutation = useMutation({
    mutationFn: () => clientsApi.regenerateSecret(clientId!),
    onSuccess: (response) => {
      setCredentials(response.data.credentials)
      setShowRegenerateConfirm(false)
      setShowCredentialsModal(true)
      toast.success('Client secret regenerated successfully')
    },
    onError: (error: any) => {
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to regenerate secret'
      toast.error(errorMessage)
    },
  })

  const handleCopy = async (text: string, field: string) => {
    await navigator.clipboard.writeText(text)
    setCopiedField(field)
    setTimeout(() => setCopiedField(null), 2000)
  }

  if (isLoading) {
    return <Loading message="Loading client..." />
  }

  if (error || !client) {
    return (
      <div className="space-y-6">
        <Button
          variant="secondary"
          leftIcon={<ArrowLeftIcon className="h-4 w-4" />}
          onClick={() => navigate('/clients')}
        >
          Back to Clients
        </Button>
        <Card className="p-8 text-center">
          <p className="text-red-600">Client not found or failed to load.</p>
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
            Back
          </Button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold text-gray-900">{client.name}</h1>
              <Badge variant={client.active ? 'success' : 'default'}>
                {client.active ? 'Active' : 'Inactive'}
              </Badge>
              <Badge variant={client.public ? 'info' : 'purple'}>
                {client.public ? 'Public' : 'Confidential'}
              </Badge>
            </div>
            <p className="text-sm text-gray-500 font-mono mt-1">{client.client_id}</p>
          </div>
        </div>
      </div>

      {/* Client ID Info */}
      <Card className="p-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-gray-700">Client ID</h3>
            <p className="text-sm font-mono text-gray-900 mt-1">{client.client_id}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              leftIcon={copiedField === 'client_id' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
              onClick={() => handleCopy(client.client_id, 'client_id')}
            >
              {copiedField === 'client_id' ? 'Copied!' : 'Copy'}
            </Button>
            {!client.public && (
              <Button
                variant="secondary"
                size="sm"
                leftIcon={<KeyIcon className="h-4 w-4" />}
                onClick={() => setShowRegenerateConfirm(true)}
              >
                Regenerate Secret
              </Button>
            )}
          </div>
        </div>
      </Card>

      {/* Edit Form */}
      <Card>
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-lg font-medium text-gray-900">Client Settings</h2>
          <p className="text-sm text-gray-500 mt-1">
            Update the OAuth2 client configuration.
          </p>
        </div>
        <div className="p-6">
          <ClientForm
            initialData={client}
            onSubmit={(data) => updateMutation.mutate(data)}
            onCancel={() => navigate('/clients')}
            isSubmitting={updateMutation.isPending}
            submitLabel="Save Changes"
          />
        </div>
      </Card>

      {/* Danger Zone */}
      <Card className="border-2 border-red-200">
        <div className="px-6 py-4 border-b border-red-200 bg-red-50">
          <div className="flex items-center gap-2">
            <ExclamationTriangleIcon className="h-5 w-5 text-red-500" />
            <h2 className="text-lg font-medium text-red-900">Danger Zone</h2>
          </div>
        </div>
        <div className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-gray-900">Delete this client</h3>
              <p className="text-sm text-gray-500 mt-1">
                Once deleted, this client cannot be recovered. All associated tokens will be invalidated.
              </p>
            </div>
            <Button
              variant="danger"
              leftIcon={<TrashIcon className="h-4 w-4" />}
              onClick={() => setShowDeleteConfirm(true)}
            >
              Delete Client
            </Button>
          </div>
        </div>
      </Card>

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={showDeleteConfirm}
        onClose={() => setShowDeleteConfirm(false)}
        onConfirm={() => deleteMutation.mutate()}
        title="Delete Client"
        message={`Are you sure you want to delete "${client.name}"? This action cannot be undone.`}
        confirmText="Delete"
        variant="danger"
        isLoading={deleteMutation.isPending}
      />

      {/* Regenerate Secret Confirmation */}
      <ConfirmDialog
        isOpen={showRegenerateConfirm}
        onClose={() => setShowRegenerateConfirm(false)}
        onConfirm={() => regenerateMutation.mutate()}
        title="Regenerate Client Secret"
        message={`Are you sure you want to regenerate the secret for "${client.name}"? The old secret will immediately stop working.`}
        confirmText="Regenerate"
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
        title="New Client Secret"
        size="md"
      >
        <div className="space-y-4">
          <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
            <div className="flex">
              <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500 mt-0.5" />
              <div className="ml-3">
                <p className="text-sm text-yellow-700">
                  Make sure to copy your new client secret now. You won't be able to see it again!
                </p>
              </div>
            </div>
          </div>

          {credentials && (
            <div className="space-y-3">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Client ID</label>
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
                <label className="block text-sm font-medium text-gray-700 mb-1">Client Secret</label>
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
              Done
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default ClientDetailPage
