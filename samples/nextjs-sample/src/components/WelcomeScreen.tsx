'use client'

import { useState } from 'react'
import { useAuth } from '@authway/react'

export default function WelcomeScreen() {
  const { loginWithRedirect, loginWithPopup } = useAuth()
  const [loading, setLoading] = useState(false)
  const [popupError, setPopupError] = useState<string | null>(null)

  const handlePopupLogin = async () => {
    setLoading(true)
    setPopupError(null)
    try {
      // Use /callback route - @authway/client/popup-callback handles postMessage automatically
      await loginWithPopup({
        redirectUri: window.location.origin + '/callback',
      })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Unknown error'
      setPopupError(message)
      console.error('Popup login failed:', err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="welcome">
      <h2>Welcome to Authway Next.js Sample</h2>
      <p className="subtitle">
        This sample demonstrates OAuth 2.0 authentication integration<br />
        using @authway/client with Next.js App Router.
      </p>

      <div className="features">
        <div className="feature">
          <span className="feature-icon">🔐</span>
          <h3>OAuth 2.0 + PKCE</h3>
          <p>Secure Authorization Code Flow</p>
        </div>
        <div className="feature">
          <span className="feature-icon">⚡</span>
          <h3>Next.js 15</h3>
          <p>App Router & Server Components</p>
        </div>
        <div className="feature">
          <span className="feature-icon">🔄</span>
          <h3>Auto Token Refresh</h3>
          <p>Automatic token renewal</p>
        </div>
      </div>

      <h3 style={{ marginTop: '2rem', marginBottom: '1rem' }}>Choose Login Method:</h3>

      <div className="login-buttons">
        <button
          className="btn btn-large btn-primary"
          onClick={() => loginWithRedirect()}
        >
          🔄 Redirect Login (Recommended)
        </button>

        <button
          className="btn btn-large btn-secondary"
          onClick={handlePopupLogin}
          disabled={loading}
        >
          {loading ? 'Logging in...' : '🪟 Popup Login'}
        </button>
      </div>

      {popupError && (
        <div className="error-box" style={{ maxWidth: '500px', margin: '1rem auto' }}>
          <strong>Login Failed:</strong> {popupError}
        </div>
      )}

      <div style={{
        marginTop: '1.5rem',
        padding: '0.75rem',
        background: '#f0f0f0',
        borderRadius: '8px',
        fontSize: '0.9rem',
        maxWidth: '600px',
        marginLeft: 'auto',
        marginRight: 'auto'
      }}>
        <strong>🔒 OAuth 2.0 + PKCE</strong>: Both methods provide the same security level.<br />
        <strong>🔄 Redirect</strong>: Full page navigation (traditional)<br />
        <strong>🪟 Popup</strong>: Maintains app state (recommended by Auth0, Okta)
      </div>

      <div className="tech-stack">
        <span>@authway/client</span>
        <span>Next.js 15</span>
        <span>React 18</span>
        <span>TypeScript</span>
        <span>App Router</span>
      </div>
    </div>
  )
}
