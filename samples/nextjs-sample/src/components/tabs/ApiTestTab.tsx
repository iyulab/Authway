'use client'

import { useState } from 'react'
import { useAuth } from '@authway/react'

export default function ApiTestTab() {
  const { getAccessToken } = useAuth()
  const [apiResponse, setApiResponse] = useState<object | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const testApi = async (endpoint: string) => {
    setLoading(true)
    setError(null)
    setApiResponse(null)

    try {
      const token = await getAccessToken()
      const response = await fetch(`http://localhost:8081${endpoint}`, {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      const data = await response.json()
      setApiResponse(data)
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'API request failed'
      setError(message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="card">
      <h2>API Test</h2>
      <p style={{ marginBottom: '1rem', color: 'var(--secondary)' }}>
        Test authenticated requests to Authway API endpoints.
      </p>

      <div className="api-buttons">
        <button
          className="btn btn-primary"
          onClick={() => testApi('/health')}
          disabled={loading}
        >
          /health
        </button>
        <button
          className="btn btn-primary"
          onClick={() => testApi('/api/v1/profile/me')}
          disabled={loading}
        >
          /api/v1/profile/me
        </button>
      </div>

      {loading && (
        <div className="loading" style={{ padding: '1rem' }}>
          <div className="spinner"></div>
          <p style={{ marginTop: '0.5rem' }}>Requesting...</p>
        </div>
      )}

      {error && (
        <div className="error-box">
          <strong>Error:</strong> {error}
        </div>
      )}

      {apiResponse && (
        <details open style={{ marginTop: '1rem' }}>
          <summary>Response Data</summary>
          <pre>{JSON.stringify(apiResponse, null, 2)}</pre>
        </details>
      )}
    </div>
  )
}
