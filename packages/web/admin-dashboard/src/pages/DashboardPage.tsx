import React, { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { tenantsApi, clientsApi, Tenant, Client } from '@/lib/api'
import {
  BuildingOfficeIcon,
  KeyIcon,
  CheckCircleIcon,
} from '@heroicons/react/24/outline'

interface DashboardStats {
  totalTenants: number
  totalClients: number
  activeTenants: number
  activeClients: number
}

const DashboardPage: React.FC = () => {
  const [stats, setStats] = useState<DashboardStats>({
    totalTenants: 0,
    totalClients: 0,
    activeTenants: 0,
    activeClients: 0,
  })

  // 테넌트 목록 조회
  const { data: tenantsData, isLoading: tenantsLoading } = useQuery({
    queryKey: ['tenants'],
    queryFn: () => tenantsApi.list({ limit: 100 }),
  })

  // 클라이언트 목록 조회
  const { data: clientsData, isLoading: clientsLoading } = useQuery({
    queryKey: ['clients'],
    queryFn: () => clientsApi.list({ limit: 100 }),
  })

  // 통계 계산
  useEffect(() => {
    if (tenantsData && clientsData) {
      const tenants = tenantsData.data || []
      const clients = clientsData.data.clients || []

      const activeTenants = tenants.filter((tenant: Tenant) => tenant.active).length
      const activeClients = clients.filter((client: Client) => client.active).length

      setStats({
        totalTenants: tenants.length,
        totalClients: clients.length,
        activeTenants,
        activeClients,
      })
    }
  }, [tenantsData, clientsData])

  const isLoading = tenantsLoading || clientsLoading

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
      </div>
    )
  }

  const statCards = [
    {
      name: '총 테넌트',
      value: stats.totalTenants,
      icon: BuildingOfficeIcon,
      color: 'bg-blue-500',
      bgColor: 'bg-blue-50',
      textColor: 'text-blue-600',
    },
    {
      name: '활성 테넌트',
      value: stats.activeTenants,
      icon: CheckCircleIcon,
      color: 'bg-green-500',
      bgColor: 'bg-green-50',
      textColor: 'text-green-600',
    },
    {
      name: '총 앱(클라이언트)',
      value: stats.totalClients,
      icon: KeyIcon,
      color: 'bg-purple-500',
      bgColor: 'bg-purple-50',
      textColor: 'text-purple-600',
    },
    {
      name: '활성 앱',
      value: stats.activeClients,
      icon: CheckCircleIcon,
      color: 'bg-green-500',
      bgColor: 'bg-green-50',
      textColor: 'text-green-600',
    },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">대시보드</h2>
        <p className="mt-2 text-sm text-gray-600">
          Authway 관리 콘솔에 오신 것을 환영합니다.
        </p>
      </div>

      {/* 통계 카드 */}
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
        {statCards.map((stat) => {
          const Icon = stat.icon
          return (
            <div
              key={stat.name}
              className={`relative overflow-hidden rounded-lg ${stat.bgColor} px-4 py-5 shadow sm:px-6`}
            >
              <div className="flex items-center">
                <div className="flex-shrink-0">
                  <Icon className={`h-8 w-8 ${stat.textColor}`} />
                </div>
                <div className="ml-5 w-0 flex-1">
                  <dl>
                    <dt className={`text-sm font-medium ${stat.textColor} truncate`}>
                      {stat.name}
                    </dt>
                    <dd className="flex items-baseline">
                      <div className={`text-2xl font-semibold ${stat.textColor}`}>
                        {stat.value.toLocaleString()}
                      </div>
                    </dd>
                  </dl>
                </div>
              </div>
            </div>
          )
        })}
      </div>

      {/* 최근 활동 */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* 최근 생성된 테넌트 */}
        <div className="bg-white overflow-hidden shadow rounded-lg">
          <div className="px-4 py-5 sm:px-6">
            <h3 className="text-lg leading-6 font-medium text-gray-900">
              최근 생성된 테넌트
            </h3>
            <p className="mt-1 max-w-2xl text-sm text-gray-500">
              최근에 생성된 테넌트 목록입니다.
            </p>
          </div>
          <div className="border-t border-gray-200">
            <div className="divide-y divide-gray-200">
              {tenantsData?.data?.slice(0, 5).map((tenant: Tenant) => (
                <div key={tenant.id} className="px-4 py-4 sm:px-6">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <BuildingOfficeIcon className="h-8 w-8 text-blue-500" />
                      </div>
                      <div className="ml-4">
                        <div className="text-sm font-medium text-gray-900">
                          {tenant.name}
                        </div>
                        <div className="text-sm text-gray-500">{tenant.slug}</div>
                      </div>
                    </div>
                    <div className="flex items-center">
                      {tenant.active ? (
                        <CheckCircleIcon className="h-5 w-5 text-green-500" />
                      ) : (
                        <div className="h-5 w-5 rounded-full bg-gray-300"></div>
                      )}
                    </div>
                  </div>
                </div>
              )) || (
                <div className="px-4 py-8 text-center text-gray-500">
                  생성된 테넌트가 없습니다.
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 최근 생성된 클라이언트 */}
        <div className="bg-white overflow-hidden shadow rounded-lg">
          <div className="px-4 py-5 sm:px-6">
            <h3 className="text-lg leading-6 font-medium text-gray-900">
              최근 생성된 앱(클라이언트)
            </h3>
            <p className="mt-1 max-w-2xl text-sm text-gray-500">
              최근에 생성된 앱 목록입니다.
            </p>
          </div>
          <div className="border-t border-gray-200">
            <div className="divide-y divide-gray-200">
              {clientsData?.data.clients?.slice(0, 5).map((client: Client) => (
                <div key={client.id} className="px-4 py-4 sm:px-6">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <KeyIcon className="h-8 w-8 text-purple-500" />
                      </div>
                      <div className="ml-4">
                        <div className="text-sm font-medium text-gray-900">
                          {client.name}
                        </div>
                        <div className="text-sm text-gray-500">{client.client_id}</div>
                      </div>
                    </div>
                    <div className="flex items-center">
                      {client.active ? (
                        <CheckCircleIcon className="h-5 w-5 text-green-500" />
                      ) : (
                        <div className="h-5 w-5 rounded-full bg-gray-300"></div>
                      )}
                    </div>
                  </div>
                </div>
              )) || (
                <div className="px-4 py-8 text-center text-gray-500">
                  생성된 앱이 없습니다.
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* 시스템 정보 */}
      <div className="bg-white overflow-hidden shadow rounded-lg">
        <div className="px-4 py-5 sm:px-6">
          <h3 className="text-lg leading-6 font-medium text-gray-900">
            시스템 정보
          </h3>
          <p className="mt-1 max-w-2xl text-sm text-gray-500">
            Authway OAuth 2.0 서버 시스템 정보입니다.
          </p>
        </div>
        <div className="border-t border-gray-200 px-4 py-5 sm:px-6">
          <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-gray-500">서버 버전</dt>
              <dd className="mt-1 text-sm text-gray-900">v1.0.0</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Ory Hydra 연동</dt>
              <dd className="mt-1 text-sm text-gray-900 flex items-center">
                <CheckCircleIcon className="h-5 w-5 text-green-500 mr-2" />
                정상
              </dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">데이터베이스</dt>
              <dd className="mt-1 text-sm text-gray-900">PostgreSQL</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Redis 캐시</dt>
              <dd className="mt-1 text-sm text-gray-900 flex items-center">
                <CheckCircleIcon className="h-5 w-5 text-green-500 mr-2" />
                연결됨
              </dd>
            </div>
          </dl>
        </div>
      </div>
    </div>
  )
}

export default DashboardPage
