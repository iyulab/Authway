import React, { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Trans, useTranslation } from 'react-i18next'
import {
  CogIcon,
  ServerIcon,
  ShieldCheckIcon,
  InformationCircleIcon,
  BuildingOfficeIcon,
  ArrowPathIcon,
  TrashIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline'
import { apiErrorMessage, tenantsApi, consoleApi, healthApi, capabilitiesApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Modal, Input, ConfirmDialog } from '@/components/ui'
import { getConfig } from '../config'
import { useTenantActive } from '@/lib/tenantActive'
import TenantSettingsForm from '@/components/settings/TenantSettingsForm'

const SettingsPage: React.FC = () => {
  const { t } = useTranslation(['settings', 'tenants'])
  const queryClient = useQueryClient()
  const { selectedTenant, setSelectedTenant, clearSelectedTenant } = useTenantStore()

  const [showTurnOffConfirm, setShowTurnOffConfirm] = useState(false)
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmText, setDeleteConfirmText] = useState('')

  // Delete tenant mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => tenantsApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      clearSelectedTenant()
      toast.success(t('toast.deleted'))
      // Navigate will happen automatically when tenant is cleared
    },
    onError: (error: unknown) => {
      toast.error(apiErrorMessage(error, 'settings:toast.deleteFailed', 'settings:deleteRefusals'))
    },
  })

  // Turning the tenant off or on (see useTenantActive).
  const activeMutation = useTenantActive((tenant) => {
    setShowTurnOffConfirm(false)
    if (tenant) setSelectedTenant(tenant)
  })

  const handleSwitchTenant = () => {
    clearSelectedTenant()
    // App will automatically show TenantSelectionPage when no tenant is selected
  }

  const handleDeleteTenant = () => {
    if (selectedTenant && deleteConfirmText === selectedTenant.slug) {
      deleteMutation.mutate(selectedTenant.id)
      setShowDeleteModal(false)
      setDeleteConfirmText('')
    }
  }

  // Who is signed in, whichever way: a console account, the shared admin
  // password, or the admin key. /admin/validate answers the password's
  // session only, and a console account's token there is refused.
  const { data: me } = useQuery({
    queryKey: ['console-me'],
    queryFn: () => consoleApi.me().then((res) => res.data),
  })
  const signedInAs = !me
    ? t('session.checking')
    : me.actor_type === 'console_user'
      ? t('session.consoleAccount')
      : me.actor_type === 'admin_session'
        ? t('session.adminPassword')
        : t('session.adminKey')

  // Everything below is read from the deployment itself — the console's own
  // configuration, the API's /health, and what it says it offers. Nothing is
  // shown that the console cannot read.
  const { data: health, isError: healthFailed } = useQuery({
    queryKey: ['health'],
    queryFn: () => healthApi.get().then((res) => res.data),
  })
  const { data: caps } = useQuery({
    queryKey: ['capabilities'],
    queryFn: () => capabilitiesApi.get().then((res) => res.data),
  })

  const config = getConfig()
  const yesNo = (on?: boolean) => (on ? t('sections.on') : t('sections.off'))
  const listOrNone = (items: string[]) => (items.length ? items.join(', ') : t('sections.none'))
  const settingSections: { id: string; title: string; description: string; icon: typeof CogIcon; settings: { name: string; value: string; description: string; status?: 'success' | 'error' }[] }[] = [
    {
      id: 'addresses',
      title: t('sections.addresses.title'),
      description: t('sections.addresses.description'),
      icon: CogIcon,
      settings: [
        { name: 'API', value: config.apiUrl, description: t('sections.addresses.api') },
        { name: t('sections.addresses.issuer'), value: config.issuerUrl || '-', description: t('sections.addresses.issuerHelp') },
      ],
    },
    {
      id: 'server',
      title: t('sections.server.title'),
      description: t('sections.server.description'),
      icon: ServerIcon,
      settings: healthFailed
        ? [{ name: t('sections.server.status'), value: t('sections.server.notAnswering'), description: t('sections.server.notAnsweringHelp'), status: 'error' }]
        : [
            { name: t('sections.server.status'), value: health?.status ?? t('sections.server.checking'), description: t('sections.server.healthHelp'), status: health?.status === 'ok' ? 'success' : undefined },
            { name: t('sections.server.version'), value: health?.version ?? '-', description: t('sections.server.versionHelp') },
            {
              name: t('sections.server.database'),
              value: health?.migrations?.length ? t('sections.server.migration', { id: health.migrations[health.migrations.length - 1] }) : '-',
              description: t('sections.server.databaseHelp'),
            },
          ],
    },
    {
      id: 'capabilities',
      title: t('sections.capabilities.title'),
      description: t('sections.capabilities.description'),
      icon: ShieldCheckIcon,
      settings: caps
        ? [
            { name: t('sections.capabilities.social'), value: listOrNone(caps.providers), description: t('sections.capabilities.socialHelp') },
            { name: t('sections.capabilities.magicLink'), value: yesNo(caps.magic_link), description: t('sections.capabilities.magicLinkHelp') },
            { name: t('sections.capabilities.mfa'), value: listOrNone(caps.mfa), description: t('sections.capabilities.mfaHelp') },
            { name: t('sections.capabilities.signupModes'), value: caps.signup_modes.join(', '), description: t('sections.capabilities.signupModesHelp') },
            { name: t('sections.capabilities.device'), value: yesNo(caps.device_authorization), description: t('sections.capabilities.deviceHelp') },
            { name: t('sections.capabilities.exchange'), value: `${yesNo(caps.token_exchange)} · ${yesNo(caps.ciba)}`, description: t('sections.capabilities.exchangeHelp') },
          ]
        : [],
    },
  ]

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900">{t('title')}</h1>
        <p className="mt-2 text-sm text-gray-700">
          {t('subtitle')}
        </p>
      </div>

      {/* 현재 테넌트 정보 */}
      {selectedTenant && (
        <div className="bg-white shadow-sm rounded-lg">
          <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
            <div className="flex items-center">
              <BuildingOfficeIcon className="h-6 w-6 text-gray-400 mr-3" />
              <div>
                <h3 className="text-lg leading-6 font-medium text-gray-900">
                  {t('tenant.title')}
                </h3>
                <p className="mt-1 max-w-2xl text-sm text-gray-500">
                  {t('tenant.help')}
                </p>
              </div>
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-4">
                <div className="w-16 h-16 bg-linear-to-br from-indigo-500 to-purple-600 rounded-xl flex items-center justify-center shrink-0">
                  <span className="text-2xl font-bold text-white">
                    {selectedTenant.name.charAt(0).toUpperCase()}
                  </span>
                </div>
                <div>
                  <h4 className="text-xl font-semibold text-gray-900">
                    {selectedTenant.name}
                  </h4>
                  <p className="text-sm text-gray-500 font-mono">
                    {selectedTenant.slug}
                  </p>
                  {selectedTenant.description && (
                    <p className="mt-1 text-sm text-gray-600">
                      {selectedTenant.description}
                    </p>
                  )}
                  <div className="mt-2">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                      selectedTenant.active
                        ? 'bg-green-100 text-green-800'
                        : 'bg-gray-100 text-gray-600'
                    }`}>
                      {selectedTenant.active ? t('tenants:card.active') : t('tenants:card.inactive')}
                    </span>
                  </div>
                </div>
              </div>
              <div className="flex flex-col gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  leftIcon={<ArrowPathIcon className="h-4 w-4" />}
                  onClick={handleSwitchTenant}
                >
                  {t('tenant.switch')}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 테넌트 설정 */}
      {selectedTenant && (
        <div className="bg-white shadow-sm rounded-lg">
          <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
            <h3 className="text-lg leading-6 font-medium text-gray-900">{t('tenantSettings.title')}</h3>
            <p className="mt-1 max-w-2xl text-sm text-gray-500">
              {t('tenantSettings.help')}
            </p>
          </div>
          <div className="px-4 py-5 sm:px-6">
            <TenantSettingsForm />
          </div>
        </div>
      )}

      {/* 관리자 세션 정보 */}
      <div className="bg-white shadow-sm rounded-lg">
        <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
          <h3 className="text-lg leading-6 font-medium text-gray-900">
            {t('session.title')}
          </h3>
          <p className="mt-1 max-w-2xl text-sm text-gray-500">
            {t('session.help')}
          </p>
        </div>
        <div className="px-4 py-5 sm:px-6">
          <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-gray-500">{t('session.signedInAs')}</dt>
              <dd className="mt-1 text-sm text-gray-900">{signedInAs}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">{t('session.scope')}</dt>
              <dd className="mt-1 text-sm text-gray-900">
                {!me ? '-' : me.operator ? (me.actor_type === 'console_user' ? t('session.scopeOperatorSupport') : t('session.scopeOperator')) : t('session.scopeCount', { count: me.memberships.length })}
              </dd>
            </div>
          </dl>
        </div>
      </div>

      {/* 설정 섹션들 */}
      {settingSections.map((section) => {
        const Icon = section.icon
        return (
          <div key={section.id} className="bg-white shadow-sm rounded-lg">
            <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
              <div className="flex items-center">
                <Icon className="h-6 w-6 text-gray-400 mr-3" />
                <div>
                  <h3 className="text-lg leading-6 font-medium text-gray-900">
                    {section.title}
                  </h3>
                  <p className="mt-1 max-w-2xl text-sm text-gray-500">
                    {section.description}
                  </p>
                </div>
              </div>
            </div>
            <div className="px-4 py-5 sm:px-6">
              <dl className="space-y-6">
                {section.settings.map((setting, index) => (
                  <div key={index} className="sm:grid sm:grid-cols-3 sm:gap-4">
                    <dt className="text-sm font-medium text-gray-500">
                      {setting.name}
                    </dt>
                    <dd className="mt-1 text-sm text-gray-900 sm:mt-0 sm:col-span-2">
                      <div className="flex items-center justify-between">
                        <div>
                          <div className="flex items-center">
                            <span className="font-mono bg-gray-100 px-2 py-1 rounded text-sm">
                              {setting.value}
                            </span>
                            {setting.status === 'success' && (
                              <span className="ml-2 inline-flex px-2 py-1 text-xs font-semibold rounded-full bg-green-100 text-green-800">
                                {t('sections.healthy')}
                              </span>
                            )}
                            {setting.status === 'error' && (
                              <span className="ml-2 inline-flex px-2 py-1 text-xs font-semibold rounded-full bg-red-100 text-red-800">
                                {t('sections.needsAttention')}
                              </span>
                            )}
                          </div>
                          <p className="mt-1 text-xs text-gray-500">
                            {setting.description}
                          </p>
                        </div>
                      </div>
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          </div>
        )
      })}

      {/* 도움말 */}
      <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
        <div className="flex">
          <InformationCircleIcon className="h-5 w-5 text-blue-400 mt-0.5" />
          <div className="ml-3">
            <h3 className="text-sm font-medium text-blue-800">
              {t('help.title')}
            </h3>
            <div className="mt-2 text-sm text-blue-700">
              <p>
                <Trans
                  t={t}
                  i18nKey="help.body"
                  components={{
                    docs: (
                      <a
                        href="https://github.com/iyulab/authway"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="font-medium underline"
                      />
                    ),
                  }}
                />
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Danger Zone - 테넌트 끄기·삭제 */}
      {selectedTenant && (
        <div className="bg-white shadow-sm rounded-lg border-2 border-red-200">
          <div className="px-4 py-5 sm:px-6 border-b border-red-200 bg-red-50">
            <div className="flex items-center">
              <ExclamationTriangleIcon className="h-6 w-6 text-red-500 mr-3" />
              <div>
                <h3 className="text-lg leading-6 font-medium text-red-900">
                  {t('danger.title')}
                </h3>
                <p className="mt-1 max-w-2xl text-sm text-red-600">
                  {t('danger.help')}
                </p>
              </div>
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6 border-b border-red-100">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-900">
                  {selectedTenant.active ? t('danger.turnOffTitle') : t('danger.turnOnTitle')}
                </h4>
                <p className="mt-1 text-sm text-gray-500">
                  {selectedTenant.active ? t('danger.turnOffHelp') : t('danger.turnOnHelp')}
                </p>
              </div>
              {selectedTenant.active ? (
                <Button variant="danger" size="sm" onClick={() => setShowTurnOffConfirm(true)}>
                  {t('danger.turnOff')}
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => activeMutation.mutate({ tenant: selectedTenant, active: true })}
                  isLoading={activeMutation.isPending}
                >
                  {t('danger.turnOn')}
                </Button>
              )}
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-900">
                  {t('danger.deleteTitle')}
                </h4>
                <p className="mt-1 text-sm text-gray-500">
                  {t('danger.deleteHelp')}
                </p>
              </div>
              <Button
                variant="danger"
                size="sm"
                leftIcon={<TrashIcon className="h-4 w-4" />}
                onClick={() => setShowDeleteModal(true)}
              >
                {t('danger.delete')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        isOpen={showTurnOffConfirm}
        onClose={() => setShowTurnOffConfirm(false)}
        onConfirm={() => selectedTenant && activeMutation.mutate({ tenant: selectedTenant, active: false })}
        title={t('tenants:turnOff.title')}
        message={t('tenants:turnOff.message', { name: selectedTenant?.name })}
        confirmText={t('tenants:turnOff.confirm')}
        variant="danger"
        isLoading={activeMutation.isPending}
      />

      {/* Delete Tenant Modal */}
      <Modal
        isOpen={showDeleteModal}
        onClose={() => {
          setShowDeleteModal(false)
          setDeleteConfirmText('')
        }}
        title={t('deleteModal.title')}
        size="md"
      >
        <div className="space-y-4">
          <div className="bg-red-50 border border-red-200 rounded-lg p-4">
            <div className="flex">
              <ExclamationTriangleIcon className="h-5 w-5 text-red-500 mt-0.5" />
              <div className="ml-3">
                <h3 className="text-sm font-medium text-red-800">
                  {t('deleteModal.warning')}
                </h3>
                <div className="mt-2 text-sm text-red-700">
                  <p>
                    <Trans
                      t={t}
                      i18nKey="deleteModal.body"
                      values={{ name: selectedTenant?.name }}
                      components={{ name: <strong className="font-mono" /> }}
                    />
                  </p>
                  <ul className="list-disc list-inside mt-2 space-y-1">
                    <li>{t('deleteModal.rule1')}</li>
                    <li>{t('deleteModal.rule2')}</li>
                    <li>{t('deleteModal.rule3')}</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              <Trans
                t={t}
                i18nKey="deleteModal.confirm"
                values={{ slug: selectedTenant?.slug }}
                components={{ slug: <span className="font-mono text-red-600" /> }}
              />
            </label>
            <Input
              type="text"
              placeholder={selectedTenant?.slug}
              value={deleteConfirmText}
              onChange={(e) => setDeleteConfirmText(e.target.value)}
            />
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <Button
              variant="secondary"
              onClick={() => {
                setShowDeleteModal(false)
                setDeleteConfirmText('')
              }}
            >
              {t('deleteModal.cancel')}
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteTenant}
              disabled={deleteConfirmText !== selectedTenant?.slug}
              isLoading={deleteMutation.isPending}
            >
              {t('deleteModal.delete')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default SettingsPage
