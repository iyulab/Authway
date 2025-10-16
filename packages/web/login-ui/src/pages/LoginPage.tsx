import React, { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation } from '@tanstack/react-query'
import GoogleLoginButton from '../components/GoogleLoginButton'

// Validation schema
const loginSchema = z.object({
  email: z.string().email('올바른 이메일을 입력해주세요'),
  password: z.string().min(6, '비밀번호는 최소 6자 이상이어야 합니다'),
  remember: z.boolean().optional(),
})

type LoginFormData = z.infer<typeof loginSchema>

interface LoginRequest {
  challenge: string
  email: string
  password: string
  remember: boolean
}

interface LoginResponse {
  redirect_to?: string
  error?: string
}

interface LoginPageInfo {
  challenge: string
  client_name: string
  requested_scope: string[]
}

const LoginPage: React.FC = () => {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [loginInfo, setLoginInfo] = useState<LoginPageInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [clientId, setClientId] = useState<string | null>(null)

  const challenge = searchParams.get('login_challenge')

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      remember: false,
    },
  })

  // Fetch login challenge info
  useEffect(() => {
    if (!challenge) {
      setError('Login challenge가 누락되었습니다.')
      setIsLoading(false)
      return
    }

    // Use POST if challenge is long (>1500 chars) to avoid HTTP 431 errors
    const usePost = challenge.length > 1500
    const fetchOptions = usePost
      ? {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ login_challenge: challenge })
        }
      : { method: 'GET' }

    const url = usePost
      ? `${import.meta.env.VITE_API_URL}/login`
      : `${import.meta.env.VITE_API_URL}/login?login_challenge=${challenge}`

    fetch(url, fetchOptions)
      .then(res => res.json())
      .then(data => {
        // Handle SSO auto-login or session cleared - both need redirect
        if (data.redirect_to) {
          if (data.sso) {
            console.log('SSO auto-login, redirecting to:', data.redirect_to)
          } else if (data.session_cleared) {
            console.log('Session cleared, redirecting to:', data.redirect_to)
          }
          window.location.href = data.redirect_to
          return
        }

        if (data.error) {
          setError(data.error)
        } else {
          setLoginInfo(data)
          // Extract client_id from login info if available
          if (data.client && data.client.client_id) {
            setClientId(data.client.client_id)
          }
        }
      })
      .catch(err => {
        console.error('Login challenge fetch error:', err)
        setError('로그인 정보를 가져오는데 실패했습니다.')
      })
      .finally(() => {
        setIsLoading(false)
      })
  }, [challenge])

  // Login mutation
  const loginMutation = useMutation({
    mutationFn: async (data: LoginFormData): Promise<LoginResponse> => {
      const response = await fetch(`${import.meta.env.VITE_API_URL}/authenticate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          challenge,
          email: data.email,
          password: data.password,
          remember: data.remember,
        } as LoginRequest),
      })

      return response.json()
    },
    onSuccess: (data) => {
      if (data.redirect_to) {
        // Hydra redirect URL로 이동
        window.location.href = data.redirect_to
      } else if (data.error) {
        setError(data.error)
      }
    },
    onError: (error) => {
      console.error('Login error:', error)
      setError('로그인 중 오류가 발생했습니다.')
    },
  })

  const onSubmit = (data: LoginFormData) => {
    setError(null)
    loginMutation.mutate(data)
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600" data-testid="loading-spinner"></div>
      </div>
    )
  }

  if (error && !loginInfo) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">오류 발생</h2>
            <p className="mt-2 text-sm text-red-600">{error}</p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-md w-full space-y-8">
        <div>
          <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
            <svg
              className="h-6 w-6 text-indigo-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            로그인
          </h2>
          {loginInfo && (
            <div className="mt-2 text-center">
              <p className="text-sm text-gray-600">
                <span className="font-medium">{loginInfo.client_name}</span>에 로그인하시겠습니까?
              </p>
              {loginInfo.requested_scope.length > 0 && (
                <p className="text-xs text-gray-500 mt-1">
                  요청된 권한: {loginInfo.requested_scope.join(', ')}
                </p>
              )}
            </div>
          )}
        </div>

        <form className="mt-8 space-y-6" onSubmit={handleSubmit(onSubmit)}>
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label htmlFor="email" className="block text-sm font-medium text-gray-700">
                이메일
              </label>
              <input
                {...register('email')}
                type="email"
                autoComplete="email"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder="이메일을 입력하세요"
              />
              {errors.email && (
                <p className="mt-1 text-sm text-red-600">{errors.email.message}</p>
              )}
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium text-gray-700">
                비밀번호
              </label>
              <input
                {...register('password')}
                type="password"
                autoComplete="current-password"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder="비밀번호를 입력하세요"
              />
              {errors.password && (
                <p className="mt-1 text-sm text-red-600">{errors.password.message}</p>
              )}
            </div>

            <div className="flex items-center">
              <input
                {...register('remember')}
                type="checkbox"
                className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
              />
              <label htmlFor="remember" className="ml-2 block text-sm text-gray-900">
                로그인 상태 유지
              </label>
            </div>
          </div>

          <div>
            <button
              type="submit"
              disabled={isSubmitting || loginMutation.isPending}
              className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSubmitting || loginMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  로그인 중...
                </div>
              ) : (
                '로그인'
              )}
            </button>
          </div>

          <div className="mt-6">
            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-gray-300" />
              </div>
              <div className="relative flex justify-center text-sm">
                <span className="px-2 bg-gray-50 text-gray-500">또는</span>
              </div>
            </div>

            <div className="mt-6">
              <GoogleLoginButton
                onError={(error) => setError(error)}
                disabled={isSubmitting || loginMutation.isPending}
                clientId={clientId || undefined}
              />
            </div>
          </div>

          <div className="text-center">
            <button
              type="button"
              onClick={() => navigate('/register')}
              className="text-sm text-indigo-600 hover:text-indigo-500"
            >
              계정이 없으신가요? 회원가입
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default LoginPage