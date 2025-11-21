import { useState } from 'react'
import { useAuth } from '@authway/react'

export function WelcomeScreen() {
  const { loginWithRedirect, loginWithPopup } = useAuth()
  const [popupLoading, setPopupLoading] = useState(false)
  const [popupError, setPopupError] = useState<string | null>(null)

  const handlePopupLogin = async () => {
    setPopupLoading(true)
    setPopupError(null)

    try {
      console.log('🚀 Starting popup login...')
      console.log('Redirect URI:', window.location.origin)

      // Popup login - SDK auto-handles callback (no callback.html needed!)
      const result = await loginWithPopup()

      console.log('✅ Popup login successful!')
      console.log('Result:', {
        hasAccessToken: !!result?.accessToken,
        hasIdToken: !!result?.idToken,
        hasUser: !!result?.user
      })
    } catch (err: any) {
      console.error('❌ Popup login failed:', err)
      console.error('Error details:', {
        name: err.name,
        message: err.message,
        code: err.code,
        statusCode: err.statusCode,
        stack: err.stack
      })
      setPopupError(err.message || 'Login failed')
    } finally {
      setPopupLoading(false)
    }
  }

  const handleRedirectLogin = () => {
    loginWithRedirect({
      appState: { returnTo: window.location.pathname }
    })
  }

  return (
    <div className="welcome-screen">
      <div className="welcome-content">
        <div className="welcome-hero">
          <span className="hero-icon">🚀</span>
          <h1>Welcome to Authway ASP.NET SPA!</h1>
          <p className="subtitle">
            Powerful OAuth 2.0 authentication with @authway/react SDK
          </p>
        </div>

        <div className="features-grid">
          <div className="feature-card">
            <span className="feature-icon">🪟</span>
            <h3>Popup Login</h3>
            <p>로그인 팝업을 통해 앱 상태를 유지하면서 인증</p>
          </div>
          <div className="feature-card">
            <span className="feature-icon">🎭</span>
            <h3>Dynamic Claims</h3>
            <p>재인증 없이 런타임에 클레임 업데이트</p>
          </div>
          <div className="feature-card">
            <span className="feature-icon">🔐</span>
            <h3>OAuth 2.0 + PKCE</h3>
            <p>안전한 Authorization Code Flow</p>
          </div>
          <div className="feature-card">
            <span className="feature-icon">⚡</span>
            <h3>Auto Token Refresh</h3>
            <p>만료된 토큰 자동 갱신</p>
          </div>
        </div>

        <div className="login-section">
          <h2>로그인 방식 선택</h2>
          <p className="login-description">
            두 방식 모두 동일한 OAuth 2.0 + PKCE 보안을 제공합니다
          </p>

          <div className="login-buttons">
            <button
              className="btn btn-primary btn-large"
              onClick={handleRedirectLogin}
            >
              <span className="btn-icon">🔄</span>
              <div className="btn-content">
                <div className="btn-title">Redirect Login</div>
                <div className="btn-subtitle">전체 페이지 이동 (전통적)</div>
              </div>
            </button>

            <button
              className="btn btn-secondary btn-large"
              onClick={handlePopupLogin}
              disabled={popupLoading}
            >
              <span className="btn-icon">🪟</span>
              <div className="btn-content">
                <div className="btn-title">
                  {popupLoading ? 'Logging in...' : 'Popup Login'}
                </div>
                <div className="btn-subtitle">앱 상태 유지 (권장)</div>
              </div>
            </button>
          </div>

          {popupError && (
            <div className="error-box">
              <strong>❌ Popup Login Failed:</strong> {popupError}
            </div>
          )}

          <div className="info-box">
            <h4>🔒 Security Features</h4>
            <ul>
              <li>✅ OAuth 2.0 Authorization Code Flow with PKCE</li>
              <li>✅ SDK auto-handles popup callback (no callback.html needed!)</li>
              <li>✅ Auto-discovery of OAuth endpoints from Auth Backend</li>
              <li>✅ JWT tokens with automatic refresh</li>
            </ul>
          </div>
        </div>

        <div className="tech-info">
          <h3>🛠️ Tech Stack</h3>
          <div className="tech-badges">
            <span className="tech-badge">@authway/client</span>
            <span className="tech-badge">@authway/react</span>
            <span className="tech-badge">ASP.NET Core</span>
            <span className="tech-badge">React 18</span>
            <span className="tech-badge">TypeScript</span>
          </div>
        </div>
      </div>
    </div>
  )
}
