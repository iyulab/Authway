import React, { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'

interface ConsentRequest {
  challenge: string
  grant_scope: string[]
  remember: boolean
  remember_for: number
}

interface ConsentResponse {
  redirect_to?: string
  error?: string
}

interface ConsentPageInfo {
  challenge: string
  client_name: string
  requested_scope: string[]
  user: {
    email: string
    name: string
  }
}

const scopeDescriptions: Record<string, { name: string; description: string }> = {
  openid: {
    name: 'OpenID 인증',
    description: '기본 사용자 인증 정보에 접근합니다.',
  },
  profile: {
    name: '프로필 정보',
    description: '사용자 이름, 프로필 사진 등 기본 프로필 정보에 접근합니다.',
  },
  email: {
    name: '이메일 주소',
    description: '사용자의 이메일 주소에 접근합니다.',
  },
  offline_access: {
    name: '오프라인 접근',
    description: '사용자가 오프라인일 때도 정보에 접근할 수 있습니다.',
  },
}

const ConsentPage: React.FC = () => {
  const [searchParams] = useSearchParams()
  const [consentInfo, setConsentInfo] = useState<ConsentPageInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [rememberConsent, setRememberConsent] = useState(true) // Default to true for better UX

  const challenge = searchParams.get('consent_challenge')

  // Helper function to handle popup mode redirect
  const handlePopupRedirect = (redirectUrl: string) => {
    // Check both window.opener AND sessionStorage (survives cross-origin redirects)
    const hasWindowOpener = window.opener !== null && window.opener !== window
    const isSessionStoragePopup = sessionStorage.getItem('authway_popup_mode') === 'true'
    const isPopupMode = hasWindowOpener || isSessionStoragePopup

    if (isPopupMode) {
      console.log('[ConsentPage] Popup mode detected via', hasWindowOpener ? 'window.opener' : 'sessionStorage')
      console.log('[ConsentPage] Popup mode - will follow Hydra redirect in hidden iframe')

      try {
        // Parse the redirect URL
        const url = new URL(redirectUrl)

        // Extract redirect_uri to get parent origin
        const redirectUriParam = url.searchParams.get('redirect_uri') || url.searchParams.get('redirectUri')
        let parentOrigin = '*'

        if (redirectUriParam) {
          try {
            const redirectUriUrl = new URL(redirectUriParam)
            parentOrigin = redirectUriUrl.origin
            console.log('[ConsentPage] Parent origin:', parentOrigin)
          } catch (e) {
            console.warn('[ConsentPage] Failed to parse redirect_uri')
          }
        }

        // Create hidden iframe to follow Hydra redirect
        // This maintains the popup context while allowing Hydra to generate the code
        const iframe = document.createElement('iframe')
        iframe.style.display = 'none'
        document.body.appendChild(iframe)

        // Declare interval variable first for use in messageHandler
        let checkCount = 0
        const maxChecks = 50 // 5 seconds max
        let checkInterval: ReturnType<typeof setInterval>

        // Listen for postMessage from iframe (callback.html sends this)
        const messageHandler = (event: MessageEvent) => {
          console.log('[ConsentPage] Received postMessage:', event.data)

          if (event.data?.type === 'authway-callback') {
            console.log('[ConsentPage] ✅ Received code from iframe via postMessage')

            const { code, state, error, error_description } = event.data

            console.log('[ConsentPage] Extracted from postMessage:', {
              code: code ? `${code.substring(0, 10)}...` : null,
              state,
              error
            })

            // Clean up
            window.removeEventListener('message', messageHandler)
            if (checkInterval) clearInterval(checkInterval)
            document.body.removeChild(iframe)

            // Send to parent window (main app)
            const targetWindow = window.opener || window.parent
            if (targetWindow && targetWindow !== window) {
              targetWindow.postMessage({
                type: 'authway-callback',
                code,
                state,
                error,
                error_description
              }, parentOrigin)

              console.log('[ConsentPage] ✅ Sent code to parent via postMessage')
            } else {
              console.error('[ConsentPage] ❌ No parent window found for postMessage')
            }

            // Clean up sessionStorage and close
            sessionStorage.removeItem('authway_popup_mode')
            setTimeout(() => window.close(), 500)
          }
        }

        window.addEventListener('message', messageHandler)

        // Fallback: Listen for the iframe to navigate to callback URL (in case postMessage fails)
        checkInterval = setInterval(() => {
          checkCount++
          try {
            // Check if iframe has navigated to callback URL
            const iframeUrl = iframe.contentWindow?.location.href
            if (iframeUrl && (iframeUrl.includes('/callback.html') || iframeUrl.includes('code='))) {
              console.log('[ConsentPage] Iframe reached callback URL (fallback method)')
              clearInterval(checkInterval)
              window.removeEventListener('message', messageHandler)

              // Extract code and state from iframe URL
              const callbackUrl = new URL(iframeUrl)
              const code = callbackUrl.searchParams.get('code')
              const state = callbackUrl.searchParams.get('state')
              const error = callbackUrl.searchParams.get('error')
              const errorDescription = callbackUrl.searchParams.get('error_description')

              console.log('[ConsentPage] Extracted from iframe:', {
                code: code ? `${code.substring(0, 10)}...` : null,
                state,
                error
              })

              // Send to parent window
              // Use window.opener if available, otherwise try window.parent (for iframe scenarios)
              const targetWindow = window.opener || window.parent
              if (targetWindow && targetWindow !== window) {
                targetWindow.postMessage({
                  type: 'authway-callback',
                  code,
                  state,
                  error,
                  error_description: errorDescription
                }, parentOrigin)

                console.log('[ConsentPage] ✅ Sent code to parent via postMessage')
              } else {
                console.error('[ConsentPage] ❌ No parent window found for postMessage')
              }

              // Clean up sessionStorage and close
              sessionStorage.removeItem('authway_popup_mode')
              document.body.removeChild(iframe)
              setTimeout(() => window.close(), 500)
            }
          } catch (e) {
            // Cross-origin iframe access will throw - this is expected
            // Keep checking until we can access it (same-origin callback URL)
          }

          // Timeout after max checks
          if (checkCount >= maxChecks) {
            console.error('[ConsentPage] Timeout waiting for callback URL')
            clearInterval(checkInterval)
            window.removeEventListener('message', messageHandler)
            document.body.removeChild(iframe)
            // Fall back to normal redirect
            window.location.href = redirectUrl
          }
        }, 100)

        // Load Hydra URL in iframe
        console.log('[ConsentPage] Loading Hydra URL in iframe')
        iframe.src = redirectUrl

        return true // Indicate that popup redirect was handled
      } catch (err) {
        console.error('[ConsentPage] Failed to handle popup redirect:', err)
        return false
      }
    }

    return false // Not in popup mode
  }

  // Fetch consent challenge info
  useEffect(() => {
    const hasWindowOpener = window.opener !== null && window.opener !== window
    const hasSessionStorage = sessionStorage.getItem('authway_popup_mode') === 'true'

    console.log('[ConsentPage] useEffect - checking popup mode:', {
      hasOpener: window.opener !== null,
      isSelfReference: window.opener === window,
      hasSessionStorage,
      isPopupMode: hasWindowOpener || hasSessionStorage,
      detectionMethod: hasWindowOpener ? 'window.opener' : (hasSessionStorage ? 'sessionStorage' : 'none')
    })

    if (!challenge) {
      setError('Consent challenge가 누락되었습니다.')
      setIsLoading(false)
      return
    }

    console.log('[ConsentPage] Fetching consent info with challenge:', challenge.substring(0, 20) + '...')

    // Use POST to avoid HTTP 431 with long consent_challenge
    fetch(`${import.meta.env.VITE_API_URL}/consent`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        consent_challenge: challenge,
      }),
    })
      .then(res => res.json())
      .then(data => {
        if (data.error) {
          setError(data.error)
          setIsLoading(false)
        } else if (data.redirect_to && data.auto_accepted) {
          // Auto-accepted by skip_consent - redirect immediately
          // Keep loading state to prevent rendering
          console.log('[ConsentPage] Auto-accepted consent, redirecting...')
          console.log('[ConsentPage] redirect_to URL:', data.redirect_to)

          // Handle popup mode redirect
          console.log('[ConsentPage] Calling handlePopupRedirect...')
          const popupHandled = handlePopupRedirect(data.redirect_to)
          console.log('[ConsentPage] handlePopupRedirect returned:', popupHandled)

          // If not in popup mode or popup handling failed, do normal redirect
          if (!popupHandled) {
            window.location.href = data.redirect_to
          }
        } else {
          setConsentInfo(data)
          // 기본적으로 모든 요청된 scope를 선택
          setSelectedScopes(data.requested_scope || [])
          setIsLoading(false)
        }
      })
      .catch(err => {
        console.error('Consent challenge fetch error:', err)
        setError('동의 정보를 가져오는데 실패했습니다.')
        setIsLoading(false)
      })
  }, [challenge])

  // Accept consent mutation
  const acceptMutation = useMutation({
    mutationFn: async (): Promise<ConsentResponse> => {
      const response = await fetch(`${import.meta.env.VITE_API_URL}/consent/accept`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          challenge,
          grant_scope: selectedScopes,
          remember: rememberConsent,
          remember_for: rememberConsent ? 3600 : 0, // 1 hour
        } as ConsentRequest),
      })

      return response.json()
    },
    onSuccess: (data) => {
      if (data.redirect_to) {
        console.log('[ConsentPage] Consent accepted, redirecting...')
        console.log('[ConsentPage] redirect_to URL:', data.redirect_to)

        // Handle popup mode redirect
        console.log('[ConsentPage] Calling handlePopupRedirect (accept)...')
        const popupHandled = handlePopupRedirect(data.redirect_to)
        console.log('[ConsentPage] handlePopupRedirect returned:', popupHandled)

        // If not in popup mode or popup handling failed, do normal redirect
        if (!popupHandled) {
          console.log('[ConsentPage] Not popup mode, doing normal redirect')
          window.location.href = data.redirect_to
        }
      } else if (data.error) {
        setError(data.error)
      }
    },
    onError: (error) => {
      console.error('Consent accept error:', error)
      setError('동의 처리 중 오류가 발생했습니다.')
    },
  })

  // Reject consent mutation
  const rejectMutation = useMutation({
    mutationFn: async (): Promise<ConsentResponse> => {
      const response = await fetch(
        `${import.meta.env.VITE_API_URL}/consent/reject`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            consent_challenge: challenge,
          }),
        }
      )

      return response.json()
    },
    onSuccess: (data) => {
      if (data.redirect_to) {
        console.log('[ConsentPage] Consent rejected, redirecting...')
        console.log('[ConsentPage] redirect_to URL:', data.redirect_to)

        // Handle popup mode redirect
        console.log('[ConsentPage] Calling handlePopupRedirect (reject)...')
        const popupHandled = handlePopupRedirect(data.redirect_to)
        console.log('[ConsentPage] handlePopupRedirect returned:', popupHandled)

        // If not in popup mode or popup handling failed, do normal redirect
        if (!popupHandled) {
          console.log('[ConsentPage] Not popup mode, doing normal redirect')
          window.location.href = data.redirect_to
        }
      } else if (data.error) {
        setError(data.error)
      }
    },
    onError: (error) => {
      console.error('Consent reject error:', error)
      setError('거부 처리 중 오류가 발생했습니다.')
    },
  })

  const handleScopeToggle = (scope: string) => {
    setSelectedScopes(prev =>
      prev.includes(scope)
        ? prev.filter(s => s !== scope)
        : [...prev, scope]
    )
  }

  const handleAccept = () => {
    setError(null)
    acceptMutation.mutate()
  }

  const handleReject = () => {
    setError(null)
    rejectMutation.mutate()
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600" data-testid="loading-spinner"></div>
      </div>
    )
  }

  if (error && !consentInfo) {
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

  if (!consentInfo) {
    return null
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-lg w-full space-y-8">
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
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            앱 권한 승인
          </h2>
          <div className="mt-4 text-center">
            <p className="text-sm text-gray-600">
              안녕하세요, <span className="font-medium">{consentInfo.user.name || consentInfo.user.email}</span>님
            </p>
            <p className="text-sm text-gray-600 mt-2">
              <span className="font-medium">{consentInfo.client_name}</span>에서 다음 권한을 요청하고 있습니다.
            </p>
          </div>
        </div>

        <div className="mt-8 space-y-6">
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          {/* 권한 목록 */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium text-gray-900">요청된 권한</h3>
            <div className="space-y-3">
              {consentInfo.requested_scope.map((scope) => {
                const scopeInfo = scopeDescriptions[scope] || {
                  name: scope,
                  description: `${scope} 권한에 접근합니다.`,
                }

                return (
                  <div key={scope} className="flex items-start">
                    <div className="flex items-center h-5">
                      <input
                        id={scope}
                        type="checkbox"
                        checked={selectedScopes.includes(scope)}
                        onChange={() => handleScopeToggle(scope)}
                        className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                      />
                    </div>
                    <div className="ml-3 text-sm">
                      <label htmlFor={scope} className="font-medium text-gray-700">
                        {scopeInfo.name}
                      </label>
                      <p className="text-gray-500">{scopeInfo.description}</p>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          {/* 동의 저장 옵션 */}
          <div className="flex items-center">
            <input
              id="remember"
              type="checkbox"
              checked={rememberConsent}
              onChange={(e) => setRememberConsent(e.target.checked)}
              className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
            />
            <label htmlFor="remember" className="ml-2 block text-sm text-gray-900">
              이 선택을 기억하기 (1시간)
            </label>
          </div>

          {/* 버튼들 */}
          <div className="flex space-x-4">
            <button
              onClick={handleReject}
              disabled={rejectMutation.isPending}
              className="flex-1 flex justify-center py-2 px-4 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {rejectMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600 mr-2"></div>
                  거부 중...
                </div>
              ) : (
                '거부'
              )}
            </button>

            <button
              onClick={handleAccept}
              disabled={acceptMutation.isPending || selectedScopes.length === 0}
              className="flex-1 flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {acceptMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  승인 중...
                </div>
              ) : (
                `승인 (${selectedScopes.length}개 권한)`
              )}
            </button>
          </div>

          <div className="text-center">
            <p className="text-xs text-gray-500">
              승인하면 선택한 권한에 대해 {consentInfo.client_name}에서 접근할 수 있습니다.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ConsentPage