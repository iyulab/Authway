import React from 'react'
import { useAuthStore } from '@/stores/auth'
import {
  CogIcon,
  ServerIcon,
  ShieldCheckIcon,
  InformationCircleIcon,
} from '@heroicons/react/24/outline'

const SettingsPage: React.FC = () => {
  const { user } = useAuthStore()

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
                  href="https://github.com/authway/authway"
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
    </div>
  )
}

export default SettingsPage
