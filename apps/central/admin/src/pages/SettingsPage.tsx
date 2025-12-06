import React, { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
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
import { tenantsApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import { Button, Modal, Input } from '@/components/ui'

const SettingsPage: React.FC = () => {
  const queryClient = useQueryClient()
  const { selectedTenant, clearSelectedTenant } = useTenantStore()

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
      const errorMessage =
        error.response?.data?.error ||
        error.response?.data?.details ||
        'Failed to delete tenant'
      toast.error(errorMessage)
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

  // TODO: Fetch user data from API
  const user = {
    first_name: 'Admin',
    last_name: 'User',
    email: 'admin@authway.com',
    email_verified: true,
    active: true
  }

  const settingSections = [
    {
      id: 'system',
      title: '시스템 설정',
      description: 'Authway 시스템 전반적인 설정을 관리합니다.',
      icon: CogIcon,
      settings: [
        {
          name: 'API Base URL',
          value: import.meta.env.VITE_API_URL || 'http://localhost:8080',
          description: 'Authway API 서버 URL',
        },
        {
          name: 'Environment',
          value: 'Development',
          description: '현재 실행 환경',
        },
      ],
    },
    {
      id: 'hydra',
      title: 'Ory Hydra 연동',
      description: 'Ory Hydra OAuth2 서버와의 연동 설정을 확인합니다.',
      icon: ServerIcon,
      settings: [
        {
          name: 'Hydra Public URL',
          value: import.meta.env.VITE_HYDRA_PUBLIC_URL || 'http://localhost:4444',
          description: 'Hydra Public Endpoint (OAuth2 토큰)',
        },
        {
          name: 'Hydra Admin URL',
          value: 'http://localhost:4445',
          description: 'Hydra Admin Endpoint (관리 API)',
        },
        {
          name: 'Connection Status',
          value: '연결됨',
          description: 'Hydra 서버 연결 상태',
          status: 'success',
        },
      ],
    },
    {
      id: 'security',
      title: '보안 설정',
      description: '인증 및 보안 관련 설정을 관리합니다.',
      icon: ShieldCheckIcon,
      settings: [
        {
          name: 'Access Token 만료 시간',
          value: '15분',
          description: 'OAuth2 Access Token의 기본 만료 시간',
        },
        {
          name: 'Refresh Token 만료 시간',
          value: '7일',
          description: 'OAuth2 Refresh Token의 기본 만료 시간',
        },
        {
          name: 'PKCE 요구',
          value: '활성화',
          description: 'Authorization Code Flow에서 PKCE 사용 강제',
          status: 'success',
        },
      ],
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
        <div className="bg-white shadow rounded-lg">
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
                <div className="w-16 h-16 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-xl flex items-center justify-center flex-shrink-0">
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

      {/* 관리자 정보 */}
      <div className="bg-white shadow rounded-lg">
        <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
          <h3 className="text-lg leading-6 font-medium text-gray-900">
            관리자 정보
          </h3>
          <p className="mt-1 max-w-2xl text-sm text-gray-500">
            현재 로그인한 관리자 계정 정보입니다.
          </p>
        </div>
        <div className="px-4 py-5 sm:px-6">
          <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-gray-500">이름</dt>
              <dd className="mt-1 text-sm text-gray-900">
                {user?.first_name} {user?.last_name}
              </dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">이메일</dt>
              <dd className="mt-1 text-sm text-gray-900">{user?.email}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">이메일 인증</dt>
              <dd className="mt-1 text-sm text-gray-900">
                <span
                  className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full ${
                    user?.email_verified
                      ? 'bg-green-100 text-green-800'
                      : 'bg-red-100 text-red-800'
                  }`}
                >
                  {user?.email_verified ? '인증됨' : '미인증'}
                </span>
              </dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">계정 상태</dt>
              <dd className="mt-1 text-sm text-gray-900">
                <span
                  className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full ${
                    user?.active
                      ? 'bg-green-100 text-green-800'
                      : 'bg-red-100 text-red-800'
                  }`}
                >
                  {user?.active ? '활성' : '비활성'}
                </span>
              </dd>
            </div>
          </dl>
        </div>
      </div>

      {/* 설정 섹션들 */}
      {settingSections.map((section) => {
        const Icon = section.icon
        return (
          <div key={section.id} className="bg-white shadow rounded-lg">
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

      {/* 버전 정보 */}
      <div className="bg-white shadow rounded-lg">
        <div className="px-4 py-5 sm:px-6 border-b border-gray-200">
          <div className="flex items-center">
            <InformationCircleIcon className="h-6 w-6 text-gray-400 mr-3" />
            <div>
              <h3 className="text-lg leading-6 font-medium text-gray-900">
                버전 정보
              </h3>
              <p className="mt-1 max-w-2xl text-sm text-gray-500">
                Authway와 관련 컴포넌트의 버전 정보입니다.
              </p>
            </div>
          </div>
        </div>
        <div className="px-4 py-5 sm:px-6">
          <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-gray-500">Authway</dt>
              <dd className="mt-1 text-sm text-gray-900 font-mono">v1.0.0-alpha</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Ory Hydra</dt>
              <dd className="mt-1 text-sm text-gray-900 font-mono">v2.2.0</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Admin Dashboard</dt>
              <dd className="mt-1 text-sm text-gray-900 font-mono">v1.0.0</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Login UI</dt>
              <dd className="mt-1 text-sm text-gray-900 font-mono">v1.0.0</dd>
            </div>
          </dl>
        </div>
      </div>

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

      {/* Danger Zone - 테넌트 삭제 */}
      {selectedTenant && (
        <div className="bg-white shadow rounded-lg border-2 border-red-200">
          <div className="px-4 py-5 sm:px-6 border-b border-red-200 bg-red-50">
            <div className="flex items-center">
              <ExclamationTriangleIcon className="h-6 w-6 text-red-500 mr-3" />
              <div>
                <h3 className="text-lg leading-6 font-medium text-red-900">
                  Danger Zone
                </h3>
                <p className="mt-1 max-w-2xl text-sm text-red-600">
                  이 작업은 되돌릴 수 없습니다. 신중하게 진행하세요.
                </p>
              </div>
            </div>
          </div>
          <div className="px-4 py-5 sm:px-6">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-medium text-gray-900">
                  테넌트 삭제
                </h4>
                <p className="mt-1 text-sm text-gray-500">
                  이 테넌트와 관련된 모든 데이터(클라이언트, 사용자 등)가 영구적으로 삭제됩니다.
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
                    테넌트 <strong className="font-mono">{selectedTenant?.name}</strong>을(를) 삭제하면
                    다음 데이터가 모두 영구적으로 삭제됩니다:
                  </p>
                  <ul className="list-disc list-inside mt-2 space-y-1">
                    <li>모든 OAuth2 클라이언트</li>
                    <li>모든 사용자 계정</li>
                    <li>모든 세션 및 토큰</li>
                    <li>관련된 모든 설정</li>
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
