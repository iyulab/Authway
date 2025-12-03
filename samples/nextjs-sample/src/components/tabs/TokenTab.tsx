'use client'

import { useState } from 'react'
import { useAuth } from '@authway/react'

interface TokenPayload {
  iss?: string
  aud?: string | string[]
  exp?: number
  iat?: number
  sub?: string
  [key: string]: unknown
}

export default function TokenTab() {
  const { getAccessToken } = useAuth()
  const [token, setToken] = useState<string | null>(null)
  const [tokenInfo, setTokenInfo] = useState<TokenPayload | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleGetToken = async () => {
    setLoading(true)
    setError(null)
    try {
      const accessToken = await getAccessToken()
      setToken(accessToken)

      // Decode token (for display only, not verification)
      const parts = accessToken.split('.')
      if (parts.length === 3) {
        const payload = JSON.parse(atob(parts[1])) as TokenPayload
        setTokenInfo(payload)
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to get token'
      setError(message)
      console.error('Failed to get token:', err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="card">
      <h2>Access Token</h2>

      <button
        className="btn btn-primary"
        onClick={handleGetToken}
        disabled={loading}
      >
        {loading ? 'Loading...' : 'Get Token'}
      </button>

      {error && (
        <div className="error-box">
          <strong>Error:</strong> {error}
        </div>
      )}

      {token && (
        <>
          <div style={{ marginTop: '1rem' }}>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
              Access Token:
            </label>
            <textarea
              readOnly
              value={token}
              rows={5}
              className="token-textarea"
            />
          </div>

          {tokenInfo && (
            <details open style={{ marginTop: '1rem' }}>
              <summary>Token Payload (Decoded)</summary>
              <pre>{JSON.stringify(tokenInfo, null, 2)}</pre>
              <div style={{ marginTop: '1rem', fontSize: '0.9rem' }}>
                <p><strong>Issuer:</strong> {tokenInfo.iss}</p>
                <p><strong>Audience:</strong> {Array.isArray(tokenInfo.aud) ? tokenInfo.aud.join(', ') : tokenInfo.aud}</p>
                {tokenInfo.exp && (
                  <p><strong>Expires:</strong> {new Date(tokenInfo.exp * 1000).toLocaleString()}</p>
                )}
                {tokenInfo.iat && (
                  <p><strong>Issued:</strong> {new Date(tokenInfo.iat * 1000).toLocaleString()}</p>
                )}
              </div>
            </details>
          )}
        </>
      )}
    </div>
  )
}
