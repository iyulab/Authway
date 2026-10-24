import React, { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
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
import { tenantsApi, consoleApi, healthApi, capabilitiesApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Modal, Input, ConfirmDialog } from '@/components/ui'
import { getConfig } from '../config'
import TenantSettingsForm from '@/components/settings/TenantSettingsForm'

// Why the API refused to delete the tenant (its 409/403 codes), in the console's words.
const DELETE_REFUSALS: Record<string, string> = {
  tenant_has_users: '사용자가 남아 있어 지울 수 없습니다. 먼저 사용자를 지우세요.',
  tenant_has_clients: '클라이언트가 남아 있어 지울 수 없습니다. 먼저 클라이언트를 지우세요.',
  tenant_has_service_clients: '서비스 자격증명이 남아 있어 지울 수 없습니다. 먼저 폐기하세요.',
  default_tenant_protected: '기본 테넌트는 지울 수 없습니다.',
}

// Why the API refused to turn the tenant off or on, in the console's words.
const ACTIVE_REFUSALS: Record<string, string> = {
  default_tenant_protected: '기본 테넌트는 끌 수 없습니다.',
  upstream_sync_failed: '일부 클라이언트를 인가 서버에 반영하지 못했습니다. 같은 작업을 다시 하면 마저 반영됩니다.',
}

const SettingsPage: React.FC = () => {
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
      toast.success('Tenant deleted successfully')
      // Navigate will happen automatically when tenant is cleared
    },
    onError: (error: any) => {
      const code: string | undefined = error.response?.data?.code
      toast.error(
        (code && DELETE_REFUSALS[code]) || error.response?.data?.detail || '테넌트를 지우지 못했습니다',
      )
    },
  })

  // Turning the tenant off or on — only `active` is sent. Off, none of its
  // clients signs anyone in or gets tokens; on, each is as its own setting says.
  const activeMutation = useMutation({
    mutationFn: (active: boolean) => tenantsApi.update(selectedTenant!.id, { active }),
    onSuccess: (res, active) => {
      setShowTurnOffConfirm(false)
      setSelectedTenant(res.data)
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      toast.success(active ? '테넌트를 켰습니다' : '테넌트를 껐습니다')
    },
    onError: (error: any) => {
      setShowTurnOffConfirm(false)
      const code: string | undefined = error.response?.data?.code
      toast.error((code && ACTIVE_REFUSALS[code]) || error.response?.data?.detail || '테넌트를 바꾸지 못했습니다')
    },
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
    ? '확인 중'
    : me.actor_type === 'console_user'
      ? '콘솔 계정'
      : me.actor_type === 'admin_session'
        ? '관리자 비밀번호'
        : '관리 키'

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
  const yesNo = (on?: boolean) => (on ? '켜짐' : '꺼짐')
  const settingSections: { id: string; title: string; description: string; icon: typeof CogIcon; settings: { name: string; value: string; description: string; status?: 'success' | 'error' }[] }[] = [
    {
      id: 'addresses',
      title: '주소',
      description: '이 콘솔이 쓰는 주소입니다.',
      icon: CogIcon,
      settings: [
        { name: 'API', value: config.apiUrl, description: 'Authway API — 테넌트·클라이언트·사용자 관리' },
        { name: '발급자(issuer)', value: config.issuerUrl || '-', description: '앱이 OIDC 설정에 넣는 주소' },
      ],
    },
    {
      id: 'server',
      title: '서버',
      description: 'API 가 /health 로 답한 것입니다.',
      icon: ServerIcon,
      settings: healthFailed
        ? [{ name: '상태', value: '응답 없음', description: 'API 의 /health 를 읽지 못했습니다', status: 'error' }]
        : [
            { name: '상태', value: health?.status ?? '확인 중', description: 'API 의 /health', status: health?.status === 'ok' ? 'success' : undefined },
            { name: '배포 버전', value: health?.version ?? '-', description: '실행 중인 API 이미지' },
            {
              name: '데이터베이스',
              value: health?.migrations?.length ? `migration ${health.migrations[health.migrations.length - 1]}` : '-',
              description: '이 버전이 가진 마지막 마이그레이션',
            },
          ],
    },
    {
      id: 'capabilities',
      title: '이 배포가 제공하는 것',
      description: 'API 가 GET /api/v1/capabilities 로 답한 것입니다.',
      icon: ShieldCheckIcon,
      settings: caps
        ? [
            { name: '소셜 로그인', value: caps.providers.length ? caps.providers.join(', ') : '없음', description: '자격증명이 들어 있는 공급자' },
            { name: '메일 링크 로그인', value: yesNo(caps.magic_link), description: '클라이언트마다 켠다' },
            { name: '2단계 인증', value: caps.mfa.length ? caps.mfa.join(', ') : '없음', description: '사용자가 계정 화면에서 켠다' },
            { name: '가입 방식', value: caps.signup_modes.join(', '), description: '테넌트마다 정한다' },
            { name: '기기 로그인', value: yesNo(caps.device_authorization), description: 'RFC 8628' },
            { name: '토큰 교환 · CIBA', value: `${yesNo(caps.token_exchange)} · ${yesNo(caps.ciba)}`, description: '아직 없다' },
          ]
        : [],
    },
  ]

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900">설정</h1>
        <p className="mt-2 text-sm text-gray-700">
          Authway 시스템의 설정을 확인하고 관리할 수 있습니다.
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
                  현재 테넌트
                </h3>
                <p className="mt-1 max-w-2xl text-sm text-gray-500">
                  현재 작업 중인 테넌트 정보입니다.
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
                      {selectedTenant.active ? 'Active' : 'Inactive'}
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
                  테넌트 전환
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
            <h3 className="text-lg leading-6 font-medium text-gray-900">테넌트 설정</h3>
            <p className="mt-1 max-w-2xl text-sm text-gray-500">
              이 테넌트의 사용자에게 적용되는 규칙입니다.
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
            관리자 세션
          </h3>
          <p className="mt-1 max-w-2xl text-sm text-gray-500">
            콘솔 계정으로 들어오면 그 계정의 멤버십이 관리할 수 있는 테넌트를 정합니다. 공유 관리자
            비밀번호는 모든 테넌트를 관리합니다.
          </p>
        </div>
        <div className="px-4 py-5 sm:px-6">
          <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-gray-500">로그인 방식</dt>
              <dd className="mt-1 text-sm text-gray-900">{signedInAs}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">관리 범위</dt>
              <dd className="mt-1 text-sm text-gray-900">
                {!me ? '-' : me.operator ? '모든 테넌트(운영자)' : `테넌트 ${me.memberships.length} 개`}
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
                                정상
                              </span>
                            )}
                            {setting.status === 'error' && (
                              <span className="ml-2 inline-flex px-2 py-1 text-xs font-semibold rounded-full bg-red-100 text-red-800">
                                확인 필요
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
              도움이 필요하신가요?
            </h3>
            <div className="mt-2 text-sm text-blue-700">
              <p>
                Authway는 Ory Hydra를 기반으로 한 3층 아키텍처 인증 플랫폼입니다.
                자세한 사용법은{' '}
                <a
                  href="https://github.com/iyulab/authway"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium underline"
                >
                  공식 문서
                </a>
                를 참고하세요.
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
                  Danger Zone
                </h3>
                <p className="mt-1 max-w-2xl text-sm text-red-600">
                  끄면 그 테넌트의 로그인이 모두 멈춥니다. 삭제는 되돌릴 수 없습니다.
                </p>
              </div>
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6 border-b border-red-100">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-900">
                  {selectedTenant.active ? '테넌트 끄기' : '테넌트 켜기'}
                </h4>
                <p className="mt-1 text-sm text-gray-500">
                  {selectedTenant.active
                    ? '이 테넌트의 어떤 클라이언트로도 로그인할 수 없고 토큰도 발급되지 않습니다. 가진 토큰은 끝납니다. 기본 테넌트는 끌 수 없습니다.'
                    : '각 클라이언트가 자기 설정대로 다시 동작합니다. 꺼져 있던 동안 끝난 세션은 돌아오지 않습니다.'}
                </p>
              </div>
              {selectedTenant.active ? (
                <Button variant="danger" size="sm" onClick={() => setShowTurnOffConfirm(true)}>
                  테넌트 끄기
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => activeMutation.mutate(true)}
                  isLoading={activeMutation.isPending}
                >
                  테넌트 켜기
                </Button>
              )}
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-900">
                  테넌트 삭제
                </h4>
                <p className="mt-1 text-sm text-gray-500">
                  사용자·클라이언트·서비스 자격증명이 남아 있으면 지울 수 없습니다 — 먼저 그것들을 지우세요. 기본
                  테넌트는 지울 수 없습니다.
                </p>
              </div>
              <Button
                variant="danger"
                size="sm"
                leftIcon={<TrashIcon className="h-4 w-4" />}
                onClick={() => setShowDeleteModal(true)}
              >
                테넌트 삭제
              </Button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        isOpen={showTurnOffConfirm}
        onClose={() => setShowTurnOffConfirm(false)}
        onConfirm={() => activeMutation.mutate(false)}
        title="테넌트 끄기"
        message={`"${selectedTenant?.name}" 의 사용자는 어떤 클라이언트로도 로그인할 수 없고, 클라이언트가 가진 토큰은 끝납니다. 다시 켜도 끝난 세션은 돌아오지 않습니다. 계속할까요?`}
        confirmText="끄기"
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
        title="테넌트 삭제"
        size="md"
      >
        <div className="space-y-4">
          <div className="bg-red-50 border border-red-200 rounded-lg p-4">
            <div className="flex">
              <ExclamationTriangleIcon className="h-5 w-5 text-red-500 mt-0.5" />
              <div className="ml-3">
                <h3 className="text-sm font-medium text-red-800">
                  경고: 이 작업은 되돌릴 수 없습니다
                </h3>
                <div className="mt-2 text-sm text-red-700">
                  <p>
                    테넌트 <strong className="font-mono">{selectedTenant?.name}</strong>을(를) 지웁니다. 빈 테넌트만
                    지워집니다:
                  </p>
                  <ul className="list-disc list-inside mt-2 space-y-1">
                    <li>사용자·클라이언트·서비스 자격증명이 남아 있으면 거절됩니다</li>
                    <li>Authway 가 만든 계정 화면 클라이언트는 함께 지워집니다</li>
                    <li>약관 문서·설정은 더 쓰이지 않습니다</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              확인을 위해 테넌트 슬러그 <span className="font-mono text-red-600">{selectedTenant?.slug}</span>을(를) 입력하세요:
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
              취소
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteTenant}
              disabled={deleteConfirmText !== selectedTenant?.slug}
              isLoading={deleteMutation.isPending}
            >
              테넌트 삭제
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default SettingsPage
