import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import toast from 'react-hot-toast'
import { clientsApi, Client } from '@/lib/api'
import {
  PlusIcon,
  KeyIcon,
  EyeIcon,
  EyeSlashIcon,
  ClipboardDocumentIcon,
  PencilIcon,
  TrashIcon,
  CheckCircleIcon,
  XCircleIcon,
} from '@heroicons/react/24/outline'

// 클라이언트 생성 폼 스키마
const createClientSchema = z.object({
  name: z.string().min(1, '클라이언트 이름은 필수입니다'),
  description: z.string().optional(),
  website: z.string().url('올바른 URL을 입력해주세요').optional().or(z.literal('')),
  redirect_uris: z.string().min(1, '최소 하나의 Redirect URI가 필요합니다'),
  grant_types: z.array(z.string()).min(1, '최소 하나의 Grant Type을 선택해주세요'),
  scopes: z.array(z.string()).min(1, '최소 하나의 Scope를 선택해주세요'),
  public: z.boolean(),
})

type CreateClientFormData = z.infer<typeof createClientSchema>

const availableGrantTypes = [
  { value: 'authorization_code', label: 'Authorization Code' },
  { value: 'client_credentials', label: 'Client Credentials' },
  { value: 'refresh_token', label: 'Refresh Token' },
]

const availableScopes = [
  { value: 'openid', label: 'OpenID Connect' },
  { value: 'profile', label: 'Profile' },
  { value: 'email', label: 'Email' },
  { value: 'offline_access', label: 'Offline Access' },
]

const ClientsPage: React.FC = () => {
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showSecrets, setShowSecrets] = useState<{ [key: string]: boolean }>({})
  const queryClient = useQueryClient()

  // 클라이언트 목록 조회
  const { data: clientsData, isLoading, error, refetch } = useQuery({
    queryKey: ['clients'],
    queryFn: () => clientsApi.list(),
  })

  // 클라이언트 생성 폼
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
    watch,
  } = useForm<CreateClientFormData>({
    resolver: zodResolver(createClientSchema),
    defaultValues: {
      grant_types: ['authorization_code'],
      scopes: ['openid'],
      public: false,
    },
  })

  const watchedGrantTypes = watch('grant_types')
  const watchedScopes = watch('scopes')

  // 클라이언트 생성 뮤테이션
  const createClientMutation = useMutation({
    mutationFn: (data: CreateClientFormData) => {
      const redirectUris = data.redirect_uris
        .split('\n')
        .map(uri => uri.trim())
        .filter(uri => uri.length > 0)

      return clientsApi.create({
        name: data.name,
        description: data.description,
        website: data.website || undefined,
        redirect_uris: redirectUris,
        grant_types: data.grant_types,
        scopes: data.scopes,
        public: data.public,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      setShowCreateModal(false)
      reset()
      toast.success('클라이언트가 생성되었습니다')
    },
    onError: () => {
      toast.error('클라이언트 생성에 실패했습니다')
    },
  })

  // 클라이언트 삭제 뮤테이션
  const deleteClientMutation = useMutation({
    mutationFn: (clientId: string) => clientsApi.delete(clientId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success('클라이언트가 삭제되었습니다')
    },
    onError: () => {
      toast.error('클라이언트 삭제에 실패했습니다')
    },
  })

  // 시크릿 재생성 뮤테이션
  const regenerateSecretMutation = useMutation({
    mutationFn: (clientId: string) => clientsApi.regenerateSecret(clientId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['clients'] })
      toast.success('Client Secret이 재생성되었습니다')
    },
    onError: () => {
      toast.error('Client Secret 재생성에 실패했습니다')
    },
  })

  const clients = clientsData?.data.clients || []

  const toggleSecretVisibility = (clientId: string) => {
    setShowSecrets(prev => ({
      ...prev,
      [clientId]: !prev[clientId],
    }))
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast.success('클립보드에 복사되었습니다')
  }

  const handleDelete = (client: Client) => {
    if (confirm(`정말로 "${client.name}" 클라이언트를 삭제하시겠습니까?`)) {
      deleteClientMutation.mutate(client.id)
    }
  }

  const handleRegenerateSecret = (client: Client) => {
    if (confirm(`정말로 "${client.name}" 클라이언트의 시크릿을 재생성하시겠습니까?`)) {
      regenerateSecretMutation.mutate(client.id)
    }
  }

  const onSubmit = (data: CreateClientFormData) => {
    createClientMutation.mutate(data)
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="text-red-600">클라이언트 목록을 불러오는데 실패했습니다.</p>
        <button
          onClick={() => refetch()}
          className="mt-4 px-4 py-2 bg-indigo-600 text-white rounded-md hover:bg-indigo-700"
        >
          다시 시도
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div className="sm:flex sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">OAuth 클라이언트 관리</h1>
          <p className="mt-2 text-sm text-gray-700">
            총 {clients.length}개의 OAuth 클라이언트가 등록되어 있습니다.
          </p>
        </div>
        <div className="mt-4 sm:mt-0">
          <button
            onClick={() => setShowCreateModal(true)}
            className="inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
          >
            <PlusIcon className="-ml-1 mr-2 h-5 w-5" />
            새 클라이언트 생성
          </button>
        </div>
      </div>

      {/* 클라이언트 목록 */}
      <div className="bg-white shadow overflow-hidden sm:rounded-lg">
        {clients.length === 0 ? (
          <div className="text-center py-12">
            <KeyIcon className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900">클라이언트가 없습니다</h3>
            <p className="mt-1 text-sm text-gray-500">
              새 OAuth 클라이언트를 생성하여 시작하세요.
            </p>
            <div className="mt-6">
              <button
                onClick={() => setShowCreateModal(true)}
                className="inline-flex items-center px-4 py-2 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700"
              >
                <PlusIcon className="-ml-1 mr-2 h-5 w-5" />
                새 클라이언트 생성
              </button>
            </div>
          </div>
        ) : (
          <div className="divide-y divide-gray-200">
            {clients.map((client: Client) => (
              <div key={client.id} className="p-6">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center">
                      <h3 className="text-lg font-medium text-gray-900">{client.name}</h3>
                      <span
                        className={`ml-3 inline-flex px-2 py-1 text-xs font-semibold rounded-full ${
                          client.active
                            ? 'bg-green-100 text-green-800'
                            : 'bg-red-100 text-red-800'
                        }`}
                      >
                        {client.active ? '활성' : '비활성'}
                      </span>
                      {client.public && (
                        <span className="ml-2 inline-flex px-2 py-1 text-xs font-semibold rounded-full bg-blue-100 text-blue-800">
                          Public
                        </span>
                      )}
                    </div>
                    {client.description && (
                      <p className="mt-1 text-sm text-gray-600">{client.description}</p>
                    )}
                    <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div>
                        <dt className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                          Client ID
                        </dt>
                        <dd className="mt-1 text-sm text-gray-900 font-mono flex items-center">
                          {client.client_id}
                          <button
                            onClick={() => copyToClipboard(client.client_id)}
                            className="ml-2 text-gray-400 hover:text-gray-600"
                          >
                            <ClipboardDocumentIcon className="h-4 w-4" />
                          </button>
                        </dd>
                      </div>
                      {!client.public && (
                        <div>
                          <dt className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Client Secret
                          </dt>
                          <dd className="mt-1 text-sm text-gray-900 font-mono flex items-center">
                            {showSecrets[client.id] ? (
                              <span>{'*'.repeat(32)}</span>
                            ) : (
                              <span>{'*'.repeat(32)}</span>
                            )}
                            <button
                              onClick={() => toggleSecretVisibility(client.id)}
                              className="ml-2 text-gray-400 hover:text-gray-600"
                            >
                              {showSecrets[client.id] ? (
                                <EyeSlashIcon className="h-4 w-4" />
                              ) : (
                                <EyeIcon className="h-4 w-4" />
                              )}
                            </button>
                            <button
                              onClick={() => handleRegenerateSecret(client)}
                              className="ml-2 text-orange-400 hover:text-orange-600"
                              title="시크릿 재생성"
                            >
                              <KeyIcon className="h-4 w-4" />
                            </button>
                          </dd>
                        </div>
                      )}
                    </div>
                    <div className="mt-4">
                      <dt className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Redirect URIs
                      </dt>
                      <dd className="mt-1">
                        {client.redirect_uris.map((uri, index) => (
                          <span
                            key={index}
                            className="inline-block mr-2 mb-1 px-2 py-1 text-xs bg-gray-100 text-gray-800 rounded"
                          >
                            {uri}
                          </span>
                        ))}
                      </dd>
                    </div>
                    <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
                      <div>
                        <dt className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                          Grant Types
                        </dt>
                        <dd className="mt-1">
                          {client.grant_types.map((type, index) => (
                            <span
                              key={index}
                              className="inline-block mr-2 mb-1 px-2 py-1 text-xs bg-indigo-100 text-indigo-800 rounded"
                            >
                              {type}
                            </span>
                          ))}
                        </dd>
                      </div>
                      <div>
                        <dt className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                          Scopes
                        </dt>
                        <dd className="mt-1">
                          {client.scopes.map((scope, index) => (
                            <span
                              key={index}
                              className="inline-block mr-2 mb-1 px-2 py-1 text-xs bg-green-100 text-green-800 rounded"
                            >
                              {scope}
                            </span>
                          ))}
                        </dd>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={() => handleDelete(client)}
                      className="text-red-600 hover:text-red-900"
                      title="삭제"
                    >
                      <TrashIcon className="h-5 w-5" />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 클라이언트 생성 모달 */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
          <div className="relative top-20 mx-auto p-5 border w-full max-w-2xl shadow-lg rounded-md bg-white">
            <div className="mt-3">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-medium text-gray-900">새 OAuth 클라이언트 생성</h3>
                <button
                  onClick={() => setShowCreateModal(false)}
                  className="text-gray-400 hover:text-gray-600"
                >
                  <XCircleIcon className="h-6 w-6" />
                </button>
              </div>

              <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                {/* 기본 정보 */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700">
                      클라이언트 이름 *
                    </label>
                    <input
                      {...register('name')}
                      type="text"
                      className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500"
                      placeholder="My Application"
                    />
                    {errors.name && (
                      <p className="mt-1 text-sm text-red-600">{errors.name.message}</p>
                    )}
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">
                      웹사이트 URL
                    </label>
                    <input
                      {...register('website')}
                      type="url"
                      className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500"
                      placeholder="https://example.com"
                    />
                    {errors.website && (
                      <p className="mt-1 text-sm text-red-600">{errors.website.message}</p>
                    )}
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700">
                    설명
                  </label>
                  <textarea
                    {...register('description')}
                    rows={2}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500"
                    placeholder="클라이언트에 대한 설명을 입력하세요"
                  />
                </div>

                {/* Redirect URIs */}
                <div>
                  <label className="block text-sm font-medium text-gray-700">
                    Redirect URIs *
                  </label>
                  <textarea
                    {...register('redirect_uris')}
                    rows={3}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500"
                    placeholder={`http://localhost:3000/callback\nhttps://example.com/callback`}
                  />
                  <p className="mt-1 text-sm text-gray-500">
                    각 URI를 새 줄에 입력하세요.
                  </p>
                  {errors.redirect_uris && (
                    <p className="mt-1 text-sm text-red-600">{errors.redirect_uris.message}</p>
                  )}
                </div>

                {/* Grant Types */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    Grant Types *
                  </label>
                  <div className="space-y-2">
                    {availableGrantTypes.map((grantType) => (
                      <label key={grantType.value} className="flex items-center">
                        <input
                          {...register('grant_types')}
                          type="checkbox"
                          value={grantType.value}
                          className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                        />
                        <span className="ml-2 text-sm text-gray-700">{grantType.label}</span>
                      </label>
                    ))}
                  </div>
                  {errors.grant_types && (
                    <p className="mt-1 text-sm text-red-600">{errors.grant_types.message}</p>
                  )}
                </div>

                {/* Scopes */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    Scopes *
                  </label>
                  <div className="space-y-2">
                    {availableScopes.map((scope) => (
                      <label key={scope.value} className="flex items-center">
                        <input
                          {...register('scopes')}
                          type="checkbox"
                          value={scope.value}
                          className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                        />
                        <span className="ml-2 text-sm text-gray-700">{scope.label}</span>
                      </label>
                    ))}
                  </div>
                  {errors.scopes && (
                    <p className="mt-1 text-sm text-red-600">{errors.scopes.message}</p>
                  )}
                </div>

                {/* Public Client */}
                <div>
                  <label className="flex items-center">
                    <input
                      {...register('public')}
                      type="checkbox"
                      className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                    />
                    <span className="ml-2 text-sm text-gray-700">
                      Public Client (Client Secret 없음)
                    </span>
                  </label>
                  <p className="mt-1 text-sm text-gray-500">
                    SPA나 모바일 앱처럼 Client Secret을 안전하게 보관할 수 없는 경우 선택하세요.
                  </p>
                </div>

                {/* 버튼 */}
                <div className="flex justify-end space-x-3 pt-4">
                  <button
                    type="button"
                    onClick={() => setShowCreateModal(false)}
                    className="px-4 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 hover:bg-gray-50"
                  >
                    취소
                  </button>
                  <button
                    type="submit"
                    disabled={createClientMutation.isPending}
                    className="px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50"
                  >
                    {createClientMutation.isPending ? '생성 중...' : '생성'}
                  </button>
                </div>
              </form>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default ClientsPage
