import React, { useEffect, useState, useRef } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'

interface LogoutErrorState {
  message: string
  fallbackRedirect: string | null
}

const LogoutPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<LogoutErrorState | null>(null)
  const redirectTimerRef = useRef<number | null>(null)

  // Extract origin from URL for fallback
  const extractOrigin = (url: string): string => {
    try {
      const parsed = new URL(url)
      return parsed.origin
    } catch {
      // Simple fallback: find third slash
      const matches = url.match(/^(https?:\/\/[^/]+)/)
      return matches ? matches[1] : url
    }
  }

  // Determine fallback redirect URL
  const getFallbackUrl = (data: Record<string, unknown>): string | null => {
    // Priority: fallback_redirect from API > post_logout_redirect_uri param > referrer origin
    if (data.fallback_redirect && typeof data.fallback_redirect === 'string') {
      return data.fallback_redirect
    }

    const postLogoutUri = searchParams.get('post_logout_redirect_uri')
    if (postLogoutUri) {
      return extractOrigin(postLogoutUri)
    }

    if (document.referrer) {
      return extractOrigin(document.referrer)
    }

    return null
  }

  // Handle redirect with countdown
  useEffect(() => {
    if (error?.fallbackRedirect) {
      redirectTimerRef.current = window.setTimeout(() => {
        window.location.href = error.fallbackRedirect!
      }, 1000)

      return () => {
        if (redirectTimerRef.current) {
          clearTimeout(redirectTimerRef.current)
        }
      }
    }
  }, [error])

  useEffect(() => {
    const logoutChallenge = searchParams.get('logout_challenge')
    const postLogoutUri = searchParams.get('post_logout_redirect_uri')

    if (!logoutChallenge) {
      // No challenge - redirect to referrer or post_logout_redirect_uri
      const fallback = postLogoutUri ? extractOrigin(postLogoutUri) :
                       document.referrer ? extractOrigin(document.referrer) : null

      console.error('[Authway Logout] Missing logout_challenge parameter', {
        post_logout_redirect_uri: postLogoutUri,
        referrer: document.referrer,
        fallback_redirect: fallback
      })

      setError({
        message: 'Logout challenge parameter is missing',
        fallbackRedirect: fallback
      })
      return
    }

    // Auto-accept logout by calling backend
    const performLogout = async () => {
      try {
        // Use backend URL in production, relative path in development (proxied by Vite)
        const baseUrl = import.meta.env.VITE_AUTH_BACKEND_URL || ''
        const url = postLogoutUri
          ? `${baseUrl}/logout?logout_challenge=${logoutChallenge}&post_logout_redirect_uri=${encodeURIComponent(postLogoutUri)}`
          : `${baseUrl}/logout?logout_challenge=${logoutChallenge}`

        const response = await fetch(url, {
          redirect: 'manual' // Don't auto-follow redirects
        })

        // Check for redirect response (status 3xx)
        if (response.type === 'opaqueredirect' || (response.status >= 300 && response.status < 400)) {
          const redirectUrl = response.headers.get('Location') || response.url
          if (redirectUrl) {
            window.location.href = redirectUrl
            return
          }
        }

        const data = await response.json()

        if (data.redirect_to) {
          // Redirect to Hydra's logout completion URL
          window.location.href = data.redirect_to
        } else if (data.error) {
          // Log detailed error for developers
          console.error('[Authway Logout] Logout failed', {
            error: data.error,
            error_description: data.error_description,
            client_id: data.client_id,
            fallback_redirect: data.fallback_redirect,
            post_logout_redirect_uri: postLogoutUri,
            logout_challenge: logoutChallenge,
            hint: 'Ensure post_logout_redirect_uris is configured in Authway client settings'
          })

          const fallback = getFallbackUrl(data)
          setError({
            message: data.error_description || data.error,
            fallbackRedirect: fallback
          })
        }
      } catch (err) {
        // Log error for developers
        console.error('[Authway Logout] Network or parsing error', {
          error: err,
          logout_challenge: logoutChallenge,
          post_logout_redirect_uri: postLogoutUri,
          referrer: document.referrer
        })

        const fallback = postLogoutUri ? extractOrigin(postLogoutUri) :
                         document.referrer ? extractOrigin(document.referrer) : null

        setError({
          message: 'Logout failed due to network error',
          fallbackRedirect: fallback
        })
      }
    }

    performLogout()
  }, [searchParams])

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center max-w-md px-4">
          <h1 className="text-2xl font-bold text-gray-900 mb-4">{t('auth:logout.error')}</h1>
          <p className="text-red-600 mb-4 text-sm">{error.message}</p>
          {error.fallbackRedirect ? (
            <p className="text-gray-600">{t('auth:logout.redirecting', 'Redirecting you back...')}</p>
          ) : (
            <p className="text-gray-600">{t('auth:logout.closeWindow', 'Please close this window or return to your application.')}</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mb-4"></div>
        <h1 className="text-2xl font-bold text-gray-900 mb-2">{t('auth:logout.loggingOut')}</h1>
        <p className="text-gray-600">{t('auth:logout.pleaseWait')}</p>
      </div>
    </div>
  )
}

export default LogoutPage
