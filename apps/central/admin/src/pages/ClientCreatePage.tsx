import React, { useState } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import {
  ArrowLeftIcon,
  ExclamationTriangleIcon,
  ClipboardDocumentIcon,
  CheckIcon,
} from '@heroicons/react/24/outline'
import { clientsApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Card, Modal } from '@/components/ui'
import { ClientForm, ClientFormData } from '@/components/clients'

const ClientCreatePage: React.FC = () => {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { selectedTenant } = useTenantStore()

  const [showCredentialsModal, setShowCredentialsModal] = useState(false)
  const [credentials, setCredentials] = useState<{ client_id: string; client_secret: string } | null>(null)
  const [copiedField, setCopiedField] = useState<string | null>(null)

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: ClientFormData) => {
      if (!selectedTenant?.id) {
        throw new Error('No tenant selected')
      }

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

      const allowedOrigins = data.allowed_origins
        ? data.allowed_origins
            .split('\n')
            .map((origin) => origin.trim())
            .filter((origin) => origin.length > 0)
        : []

      return clientsApi.create({
        tenant_id: selectedTenant.id,
        name: data.name,
        description: data.description,
        website: data.website || undefined,
        redirect_uris: redirectUris,
        post_logout_redirect_uris:
          postLogoutRedirectUris.length > 0 ? postLogoutRedirectUris : undefined,
        allowed_origins: allowedOrigins.length > 0 ? allowedOrigins : undefined,
        logout_redirect_policy: data.logout_redirect_policy || 'strict',
        default_logout_uri: data.default_logout_uri || undefined,
        allow_wildcard_logout: data.allow_wildcard_logout || false,
        grant_types: data.grant_types,
        scopes: data.scopes,
        public: data.public,
        skip_consent: data.skip_consent || false,
        skip_logout_consent: data.skip_logout_consent || false,
        // '' means "inherit the server setting" — send nothing.
        access_token_strategy: data.access_token_strategy || undefined,
      })
    },
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      setCredentials(response.data.credentials)
      setShowCredentialsModal(true)
      toast.success('Client created successfully')
    },
    onError: (error: any) => {
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to create client'
      toast.error(errorMessage)
    },
  })

  const handleCopy = async (text: string, field: string) => {
    await navigator.clipboard.writeText(text)
    setCopiedField(field)
    setTimeout(() => setCopiedField(null), 2000)
  }

  const handleCredentialsClosed = () => {
    setShowCredentialsModal(false)
    setCredentials(null)
    navigate('/clients')
  }

  return (
    <div className="space-y-6">
      {/* Header */}
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
          <h1 className="text-2xl font-bold text-gray-900">Create New Client</h1>
          <p className="text-sm text-gray-500 mt-1">
            Register a new OAuth2 application for {selectedTenant?.name}
          </p>
        </div>
      </div>

      {/* Form */}
      <Card>
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-lg font-medium text-gray-900">Client Configuration</h2>
          <p className="text-sm text-gray-500 mt-1">
            Configure the OAuth2 client settings for your application.
          </p>
        </div>
        <div className="p-6">
          <ClientForm
            onSubmit={(data) => createMutation.mutate(data)}
            onCancel={() => navigate('/clients')}
            isSubmitting={createMutation.isPending}
            submitLabel="Create Client"
          />
        </div>
      </Card>

      {/* Credentials Modal */}
      <Modal
        isOpen={showCredentialsModal}
        onClose={handleCredentialsClosed}
        title="Client Created Successfully"
        size="md"
      >
        <div className="space-y-4">
          <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
            <div className="flex">
              <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500 mt-0.5" />
              <div className="ml-3">
                <p className="text-sm text-yellow-700">
                  <strong>Important:</strong> Make sure to copy your client credentials now.
                  The client secret will not be shown again!
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
                    onClick={() => handleCopy(credentials.client_id, 'client_id')}
                  >
                    {copiedField === 'client_id' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
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
                    onClick={() => handleCopy(credentials.client_secret, 'client_secret')}
                  >
                    {copiedField === 'client_secret' ? <CheckIcon className="h-4 w-4" /> : <ClipboardDocumentIcon className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
            </div>
          )}

          <div className="flex justify-end pt-4">
            <Button onClick={handleCredentialsClosed}>
              Done
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default ClientCreatePage
