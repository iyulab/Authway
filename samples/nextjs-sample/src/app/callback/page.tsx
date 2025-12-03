'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'

/**
 * OAuth Callback Page
 *
 * This page handles OAuth callback for both redirect and popup flows.
 * AuthwayProvider automatically:
 * - Handles popup callbacks via postMessage (closes popup automatically)
 * - Processes redirect callbacks and exchanges authorization code for tokens
 *
 * This page only needs to show loading state and handle errors.
 */
export default function CallbackPage() {
  const router = useRouter()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search)
    const errorParam = searchParams.get('error')
    const errorDescription = searchParams.get('error_description')

    if (errorParam) {
      setError(errorDescription || errorParam)
    }
    // AuthwayProvider handles successful callbacks automatically
  }, [])

  if (error) {
    return (
      <div style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100vh',
        padding: '2rem',
      }}>
        <div className="card error-box" style={{ maxWidth: '500px' }}>
          <h2>Authentication Error</h2>
          <p>{error}</p>
          <button
            className="btn btn-primary"
            onClick={() => router.replace('/')}
            style={{ marginTop: '1rem' }}
          >
            Return Home
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
    }}>
      <div className="spinner"></div>
      <p style={{ marginTop: '1rem' }}>Processing authentication...</p>
    </div>
  )
}
