import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Trans, useTranslation } from 'react-i18next'
import {
  EnvelopeIcon,
  PlusIcon,
  ArrowPathIcon,
  XMarkIcon,
  BuildingOfficeIcon,
} from '@heroicons/react/24/outline'
import { invitationsApi, Invitation } from '@/lib/features-api'
import { apiErrorMessage, clientsApi, refusalAt } from '@/lib/api'
import { formatDateTime, formatDay } from '@/lib/utils'
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
  Select,
} from '@/components/ui'
import { useTenantStore } from '@/stores/tenant'
import { SUPPORTED_LANGUAGES, currentLanguage, type LanguageCode } from '@/i18n'

interface InvitationFormData {
  email: string
  message: string
}

const emptyForm: InvitationFormData = {
  email: '',
  message: '',
}

const InvitationsPage: React.FC = () => {
  const { t } = useTranslation('invitations')
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
  const [formData, setFormData] = useState<InvitationFormData>(emptyForm)

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

  // The applications an invitation can be for: the tenant's own, not the
  // ones that are part of Authway. Loaded when the invite dialog opens.
  const { data: clientsData } = useQuery({
    queryKey: ['clients', selectedTenantId, 'invitable'],
    queryFn: () => clientsApi.list({ tenant_id: selectedTenantId, limit: 100 }),
    enabled: !!selectedTenantId && showCreateModal,
  })
  const applications = (clientsData?.data.clients || []).filter((c) => !c.system)
  // An invitation without an application ends on Authway with nowhere to go,
  // so when exactly one of the tenant's applications has an address to send
  // the user on to, it is chosen unless the administrator picks otherwise.
  const landing = applications.filter((c) => c.initiate_login_uri || c.client_uri)
  const defaultApplication = landing.length === 1 ? landing[0].client_id : ''
  const [pickedApplication, setPickedApplication] = useState<string | null>(null)
  const applicationId = pickedApplication ?? defaultApplication
  const chosenApplication = applications.find((c) => c.client_id === applicationId)

  // The language of the invitation email. The administrator knows the person
  // they invite; Authway does not. It starts as the console's own language.
  const [pickedLanguage, setPickedLanguage] = useState<LanguageCode | null>(null)
  const mailLanguage = pickedLanguage ?? currentLanguage()

  const invitations = data?.data.invitations || []
  const totalInvitations = data?.data.total || 0
  const totalPages = Math.ceil(totalInvitations / pageSize)

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: Parameters<typeof invitationsApi.create>[0]) =>
      invitationsApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['invitations'] })
      setShowCreateModal(false)
      resetForm()
      toast.success(t('toast.created'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'invitations:toast.createFailed'))
    },
  })

  // Revoke mutation
  const revokeMutation = useMutation({
    mutationFn: (id: string) => invitationsApi.revoke(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['invitations'] })
      setShowRevokeConfirm(false)
      setSelectedInvitation(null)
      toast.success(t('toast.revoked'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'invitations:toast.revokeFailed'))
    },
  })

  // Resend mutation
  const resendMutation = useMutation({
    mutationFn: (id: string) => invitationsApi.resend(id),
    onSuccess: () => {
      toast.success(t('toast.resent'))
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'invitations:toast.resendFailed'))
    },
  })

  // The refusal of the last invitation sent, shown at the member it names.
  // Any edit, or closing the form, makes it stale.
  const refused = (field: string) => refusalAt(createMutation.error, field)
  const edit = (patch: Partial<InvitationFormData>) => {
    if (createMutation.error) createMutation.reset()
    setFormData((prev) => ({ ...prev, ...patch }))
  }

  const resetForm = () => {
    createMutation.reset()
    setFormData(emptyForm)
    setPickedApplication(null)
    setPickedLanguage(null)
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const body = { ...formData, locale: mailLanguage }
    createMutation.mutate(applicationId ? { ...body, client_id: applicationId } : body)
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'pending':
        return <Badge variant="warning">{t('status.pending')}</Badge>
      case 'accepted':
        return <Badge variant="success">{t('status.accepted')}</Badge>
      case 'expired':
        return <Badge variant="default">{t('status.expired')}</Badge>
      case 'revoked':
        return <Badge variant="danger">{t('status.revoked')}</Badge>
      default:
        return <Badge variant="default">{status}</Badge>
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
              values={{ count: totalInvitations, name: selectedTenant?.name }}
              components={{ tenant: <span className="font-medium" /> }}
            />
          </p>
        </div>
        <Button onClick={() => setShowCreateModal(true)}>
          <PlusIcon className="h-5 w-5 mr-2" />
          {t('invite')}
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
            {t('filter.all')}
          </Button>
          <Button
            variant={statusFilter === 'pending' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('pending'); setCurrentPage(1) }}
          >
            {t('filter.pending')}
          </Button>
          <Button
            variant={statusFilter === 'accepted' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('accepted'); setCurrentPage(1) }}
          >
            {t('filter.accepted')}
          </Button>
          <Button
            variant={statusFilter === 'expired' ? 'primary' : 'secondary'}
            size="sm"
            onClick={() => { setStatusFilter('expired'); setCurrentPage(1) }}
          >
            {t('filter.expired')}
          </Button>
        </div>
      </Card>

      {/* Invitations Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.email')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.application')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.status')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.invitedBy')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.expires')}</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{t('columns.created')}</th>
                <th className="relative px-6 py-3"><span className="sr-only">{t('columns.actions')}</span></th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {invitations.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12">
                    <EmptyState
                      icon={<EnvelopeIcon className="h-12 w-12" />}
                      title={t('empty.title')}
                      description={t('empty.description')}
                    />
                  </td>
                </tr>
              ) : (
                invitations.map((invitation: Invitation) => (
                  <tr key={invitation.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{invitation.email}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {invitation.client?.name ?? '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getStatusBadge(invitation.status)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {invitation.inviter_name || '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDateTime(invitation.expires_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDay(invitation.created_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      {invitation.status === 'pending' && (
                        <div className="flex items-center justify-end space-x-2">
                          <button
                            onClick={() => resendMutation.mutate(invitation.id)}
                            className="text-indigo-600 hover:text-indigo-900"
                            title={t('action.resend')}
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
                            title={t('action.revoke')}
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
        title={t('form.title')}
      >
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">{t('form.email')}</label>
            <Input
              type="email"
              value={formData.email}
              onChange={(e) => edit({ email: e.target.value })}
              error={refused('email')}
              placeholder={t('form.emailPlaceholder')}
              required
            />
          </div>

          <Select
            id="invitation-application"
            label={t('form.application')}
            value={applicationId}
            onChange={(e) => {
              if (createMutation.error) createMutation.reset()
              setPickedApplication(e.target.value)
            }}
            error={refused('client_id')}
            options={[
              { value: '', label: t('form.noApplication') },
              ...applications.map((c) => ({ value: c.client_id, label: c.name })),
            ]}
            helperText={
              !chosenApplication
                ? t('form.helpNone')
                : !chosenApplication.initiate_login_uri && !chosenApplication.client_uri
                  ? t('form.helpNoAddress')
                  : t('form.helpChosen')
            }
          />

          <Select
            id="invitation-language"
            label={t('form.language')}
            value={mailLanguage}
            onChange={(e) => {
              if (createMutation.error) createMutation.reset()
              setPickedLanguage(e.target.value as LanguageCode)
            }}
            error={refused('locale')}
            options={SUPPORTED_LANGUAGES.map((l) => ({ value: l.code, label: l.name }))}
            helperText={t('form.languageHelp')}
          />

          <div>
            <label className="block text-sm font-medium text-gray-700">{t('form.message')}</label>
            <textarea
              value={formData.message}
              onChange={(e) => edit({ message: e.target.value })}
              placeholder={t('form.messagePlaceholder')}
              rows={3}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-xs focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm"
            />
          </div>

          <p className="text-xs text-gray-500">{t('form.note')}</p>

          <div className="flex justify-end space-x-3 pt-4">
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setShowCreateModal(false)
                resetForm()
              }}
            >
              {t('form.cancel')}
            </Button>
            <Button type="submit" isLoading={createMutation.isPending}>
              {t('form.send')}
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
        title={t('revokeConfirm.title')}
        message={t('revokeConfirm.message', { email: selectedInvitation?.email })}
        confirmText={t('revokeConfirm.confirm')}
        variant="danger"
        isLoading={revokeMutation.isPending}
      />
    </div>
  )
}

export default InvitationsPage
