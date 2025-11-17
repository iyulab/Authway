import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

const LogoutPage: React.FC = () => {
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string>('')

  useEffect(() => {
    const logoutChallenge = searchParams.get('logout_challenge')

    if (!logoutChallenge) {
      setError('Logout challenge parameter is missing')
      return
    }

    // Auto-accept logout by calling backend
    const performLogout = async () => {
      try {
        const response = await fetch(`/logout?logout_challenge=${logoutChallenge}`, {
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
          console.error('Logout API error:', data.error)
          setError(`Logout failed: ${data.error_description || data.error}`)
        }
      } catch (err) {
        console.error('Logout error:', err)
        setError('Logout failed. Please close this window or return to your application.')
      }
    }

    performLogout()
  }, [searchParams])

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center">
          <h1 className="text-4xl font-bold text-gray-900 mb-4">Logout Error</h1>
          <p className="text-red-600 mb-4">{error}</p>
          <p className="text-gray-600">Redirecting to home...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mb-4"></div>
        <h1 className="text-2xl font-bold text-gray-900 mb-2">Logging out...</h1>
        <p className="text-gray-600">Please wait while we sign you out</p>
      </div>
    </div>
  )
}

export default LogoutPage
