import { AuthwayProvider, useAuth, useAccessToken, useClaims, LoginButton, LogoutButton, UserProfile } from '@authway/react'
import { useState } from 'react'

// Authway SDK Configuration
// 🎯 Only specify Auth Backend URL - all other endpoints are auto-discovered!
const config = {
  domain: 'http://localhost:8081',  // Auth Backend URL (auto-discovers Hydra, etc.)
  clientId: 'react-sdk-sample-client',
  useDPoP: false  // DPoP (Demonstrating Proof-of-Possession) - 토큰 보안 강화
}

function App() {
  return (
    <AuthwayProvider
      config={config}
      onRedirectCallback={(appState) => {
        console.log('Redirect callback:', appState)
        // Redirect to app state or default route
        window.history.replaceState({}, document.title, appState?.returnTo || '/')
      }}
    >
      <div className="app">
        <Header />
        <Main />
        <Footer />
      </div>
    </AuthwayProvider>
  )
}

function Header() {
  const { isAuthenticated, user, loginWithRedirect, logout, isLoading } = useAuth()

  return (
    <header className="header">
      <div className="header-content">
        <div className="logo">
          <span className="icon">🚀</span>
          <h1>Authway React SDK</h1>
          <span className="badge">Sample</span>
        </div>

        <nav className="auth-nav">
          {isLoading ? (
            <span className="loading-text">Loading...</span>
          ) : !isAuthenticated ? (
            <button
              className="btn btn-primary"
              onClick={() => loginWithRedirect({
                appState: { returnTo: window.location.pathname }
              })}
            >
              로그인
            </button>
          ) : (
            <div className="user-menu">
              {user?.picture && (
                <img
                  src={user.picture}
                  alt={user.name || 'User'}
                  className="avatar"
                />
              )}
              <span className="user-name">{user?.name || user?.email}</span>
              <button
                className="btn btn-secondary"
                onClick={() => logout({ returnTo: window.location.origin })}
              >
                로그아웃
              </button>
            </div>
          )}
        </nav>
      </div>
    </header>
  )
}

function Main() {
  const { isAuthenticated, isLoading, user, error } = useAuth()

  if (error) {
    // Safely serialize error for display
    const errorInfo = (() => {
      try {
        return JSON.stringify(error, null, 2)
      } catch {
        // Fallback for errors that can't be serialized (e.g., containing Window objects)
        return JSON.stringify({
          name: error.name,
          message: error.message,
          ...(error as any).code && { code: (error as any).code }
        }, null, 2)
      }
    })()

    return (
      <main className="main">
        <div className="error-card">
          <h2>⚠️ Error</h2>
          <p>{error.message}</p>
          <pre>{errorInfo}</pre>
        </div>
      </main>
    )
  }

  if (isLoading) {
    return (
      <main className="main">
        <div className="loading-card">
          <div className="spinner"></div>
          <p>인증 확인 중...</p>
        </div>
      </main>
    )
  }

  if (!isAuthenticated) {
    return (
      <main className="main">
        <WelcomeScreen />
      </main>
    )
  }

  return (
    <main className="main">
      <UserDashboard user={user} />
    </main>
  )
}

function WelcomeScreen() {
  const { loginWithRedirect, loginWithPopup } = useAuth()
  const [loading, setLoading] = useState(false)
  const [popupError, setPopupError] = useState<string | null>(null)

  const handlePopupLogin = async () => {
    setLoading(true)
    setPopupError(null)
    try {
      // No redirectUri needed - uses default redirect_uri (same as redirect login)
      // SDK's handlePopupCallback() automatically detects popup context and closes
      await loginWithPopup()
    } catch (err: any) {
      setPopupError(err.message)
      console.error('Popup login failed:', err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="welcome">
      <div className="welcome-content">
        <h2>Authway React SDK 샘플에 오신 것을 환영합니다</h2>
        <p className="subtitle">
          이 샘플은 @authway/client와 @authway/react 패키지를 사용하여<br />
          OAuth 2.0 인증을 구현하는 방법을 보여줍니다.
        </p>

        <div className="features">
          <div className="feature">
            <span className="feature-icon">🔐</span>
            <h3>OAuth 2.0 + PKCE</h3>
            <p>안전한 Authorization Code Flow</p>
          </div>
          <div className="feature">
            <span className="feature-icon">🔄</span>
            <h3>자동 토큰 갱신</h3>
            <p>만료된 토큰 자동 리프레시</p>
          </div>
          <div className="feature">
            <span className="feature-icon">⚛️</span>
            <h3>React Hooks</h3>
            <p>useAuth, useUser, useAccessToken</p>
          </div>
        </div>

        <h3 style={{ marginTop: '2rem', marginBottom: '1rem', fontSize: '1.1rem' }}>로그인 방식 선택:</h3>

        <div style={{ display: 'flex', gap: '1rem', justifyContent: 'center', flexWrap: 'wrap' }}>
          <button
            className="btn btn-large btn-primary"
            onClick={() => loginWithRedirect()}
          >
            🔄 리다이렉트 로그인 (권장)
          </button>

          <button
            className="btn btn-large btn-secondary"
            onClick={handlePopupLogin}
            disabled={loading}
            title="postMessage를 사용하여 COOP 정책 문제를 해결했습니다"
          >
            {loading ? '로그인 중...' : '🪟 팝업 로그인'}
          </button>
        </div>

        {popupError && (
          <div style={{
            marginTop: '1rem',
            padding: '1rem',
            backgroundColor: '#fee',
            border: '1px solid #fcc',
            borderRadius: '8px',
            color: '#c33'
          }}>
            <strong>로그인 실패:</strong> {popupError}
          </div>
        )}

        <div style={{ marginTop: '1.5rem', padding: '0.75rem', backgroundColor: '#f0f0f0', borderRadius: '8px', fontSize: '0.9rem' }}>
          <strong>🔒 OAuth 2.0 + PKCE</strong>: 두 방식 모두 동일한 보안 수준을 제공합니다.<br />
          <strong>🔄 리다이렉트</strong>: 전체 페이지 이동 (전통적 방식)<br />
          <strong>🪟 팝업</strong>: 앱 상태 유지 (Auth0, Okta 권장 방식)
        </div>

        <div className="tech-stack">
          <span>@authway/client</span>
          <span>@authway/react</span>
          <span>React 18</span>
          <span>TypeScript</span>
          <span>Vite</span>
        </div>
      </div>
    </div>
  )
}

function UserDashboard({ user }: { user: any }) {
  const [activeTab, setActiveTab] = useState<'profile' | 'token' | 'api' | 'claims' | 'components' | 'advanced' | 'hoc'>('profile')

  return (
    <div className="dashboard">
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'profile' ? 'active' : ''}`}
          onClick={() => setActiveTab('profile')}
        >
          👤 프로필
        </button>
        <button
          className={`tab ${activeTab === 'token' ? 'active' : ''}`}
          onClick={() => setActiveTab('token')}
        >
          🎫 토큰
        </button>
        <button
          className={`tab ${activeTab === 'claims' ? 'active' : ''}`}
          onClick={() => setActiveTab('claims')}
        >
          🎭 Dynamic Claims
        </button>
        <button
          className={`tab ${activeTab === 'advanced' ? 'active' : ''}`}
          onClick={() => setActiveTab('advanced')}
        >
          🚀 Advanced
        </button>
        <button
          className={`tab ${activeTab === 'hoc' ? 'active' : ''}`}
          onClick={() => setActiveTab('hoc')}
        >
          ⚛️ HOCs
        </button>
        <button
          className={`tab ${activeTab === 'api' ? 'active' : ''}`}
          onClick={() => setActiveTab('api')}
        >
          🔌 API 테스트
        </button>
        <button
          className={`tab ${activeTab === 'components' ? 'active' : ''}`}
          onClick={() => setActiveTab('components')}
        >
          🎨 UI Components
        </button>
      </div>

      <div className="tab-content">
        {activeTab === 'profile' && <ProfileTab user={user} />}
        {activeTab === 'token' && <TokenTab />}
        {activeTab === 'claims' && <ClaimsTab />}
        {activeTab === 'advanced' && <AdvancedTab />}
        {activeTab === 'hoc' && <HocTab />}
        {activeTab === 'api' && <ApiTestTab />}
        {activeTab === 'components' && <ComponentsTab />}
      </div>
    </div>
  )
}

function ProfileTab({ user }: { user: any }) {
  return (
    <div className="profile-tab">
      <h2>사용자 프로필</h2>

      {user?.picture && (
        <img src={user.picture} alt={user.name} className="profile-avatar" />
      )}

      <div className="info-grid">
        <div className="info-item">
          <label>이름</label>
          <span>{user?.name || 'N/A'}</span>
        </div>
        <div className="info-item">
          <label>이메일</label>
          <span>{user?.email}</span>
        </div>
        <div className="info-item">
          <label>이메일 인증</label>
          <span className={user?.email_verified ? 'verified' : 'unverified'}>
            {user?.email_verified ? '✅ 인증됨' : '❌ 미인증'}
          </span>
        </div>
        <div className="info-item">
          <label>사용자 ID</label>
          <code>{user?.sub}</code>
        </div>
      </div>

      <details className="claims-details">
        <summary>전체 Claims 보기</summary>
        <pre className="claims-json">
          {JSON.stringify(user, null, 2)}
        </pre>
      </details>
    </div>
  )
}

function TokenTab() {
  const { token, getToken } = useAccessToken()
  const [tokenInfo, setTokenInfo] = useState<any>(null)

  const handleGetToken = async () => {
    try {
      const accessToken = await getToken()
      // Decode token (for display only, not for verification)
      const parts = accessToken.split('.')
      if (parts.length === 3) {
        const payload = JSON.parse(atob(parts[1]))
        setTokenInfo(payload)
      }
    } catch (error) {
      console.error('Failed to get token:', error)
    }
  }

  return (
    <div className="token-tab">
      <h2>액세스 토큰</h2>

      <button className="btn btn-primary" onClick={handleGetToken}>
        토큰 가져오기
      </button>

      {token && (
        <>
          <div className="token-display">
            <label>Access Token:</label>
            <textarea
              readOnly
              value={token}
              rows={5}
              className="token-textarea"
            />
          </div>

          {tokenInfo && (
            <details className="token-details" open>
              <summary>토큰 페이로드 (Decoded)</summary>
              <pre className="token-json">
                {JSON.stringify(tokenInfo, null, 2)}
              </pre>
              <div className="token-info">
                <p><strong>발급자:</strong> {tokenInfo.iss}</p>
                <p><strong>대상:</strong> {tokenInfo.aud}</p>
                <p><strong>만료:</strong> {new Date(tokenInfo.exp * 1000).toLocaleString('ko-KR')}</p>
                <p><strong>발급일:</strong> {new Date(tokenInfo.iat * 1000).toLocaleString('ko-KR')}</p>
              </div>
            </details>
          )}
        </>
      )}
    </div>
  )
}

function ApiTestTab() {
  const { getAccessToken } = useAuth()
  const [apiResponse, setApiResponse] = useState<any>(null)
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
          'Authorization': `Bearer ${token}`
        }
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      const data = await response.json()
      setApiResponse(data)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="api-test-tab">
      <h2>API 테스트</h2>
      <p>인증된 요청으로 Authway API를 테스트해보세요.</p>

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

      {loading && <div className="loading">요청 중...</div>}

      {error && (
        <div className="error-box">
          <strong>Error:</strong> {error}
        </div>
      )}

      {apiResponse && (
        <details className="api-response" open>
          <summary>응답 데이터</summary>
          <pre className="response-json">
            {JSON.stringify(apiResponse, null, 2)}
          </pre>
        </details>
      )}
    </div>
  )
}

function ClaimsTab() {
  const { claims, isLoading, error, refreshClaims } = useClaims()
  const { client } = useAuth()

  const [claimKey, setClaimKey] = useState('')
  const [claimValue, setClaimValue] = useState('')
  const [updating, setUpdating] = useState(false)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  const handleUpdateClaim = async () => {
    if (!claimKey.trim()) {
      setUpdateError('Key는 필수입니다')
      return
    }

    setUpdating(true)
    setUpdateError(null)
    setSuccessMessage(null)

    try {
      // Try to parse value as JSON if it looks like JSON
      let parsedValue: any = claimValue
      if (claimValue.trim().startsWith('{') || claimValue.trim().startsWith('[')) {
        try {
          parsedValue = JSON.parse(claimValue)
        } catch {
          // Keep as string if JSON parsing fails
        }
      }

      // Use updateUserClaims (NO re-authentication!)
      await client.updateUserClaims({ [claimKey]: parsedValue })

      setClaimKey('')
      setClaimValue('')
      setSuccessMessage('✅ 사용자 클레임이 추가되었습니다! (재인증 불필요)')

      // Refresh claims to show the new user claim
      await refreshClaims()
    } catch (err: any) {
      setUpdateError(err.message)
    } finally {
      setUpdating(false)
    }
  }

  return (
    <div className="claims-tab">
      <h2>Dynamic Claims</h2>
      <p>Authway의 동적 클레임 시스템을 테스트해보세요. 재인증 없이 런타임에 클레임을 업데이트할 수 있습니다!</p>

      {isLoading && <div className="loading">로딩 중...</div>}

      {error && (
        <div className="error-box">
          <strong>Error:</strong> {error.message}
        </div>
      )}

      <div className="section">
        <h3>✏️ Update Claims</h3>
        <p>클레임을 추가하거나 업데이트하세요. (업데이트 후 토큰이 자동 갱신됩니다)</p>

        <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', flexWrap: 'wrap' }}>
          <input
            type="text"
            placeholder="Claim Key (예: role, department)"
            value={claimKey}
            onChange={(e) => setClaimKey(e.target.value)}
            style={{
              flex: '1',
              minWidth: '200px',
              padding: '0.625rem',
              border: '1px solid var(--border)',
              borderRadius: '8px',
              fontSize: '0.95rem'
            }}
          />
          <input
            type="text"
            placeholder="Claim Value (예: admin, engineering)"
            value={claimValue}
            onChange={(e) => setClaimValue(e.target.value)}
            style={{
              flex: '2',
              minWidth: '200px',
              padding: '0.625rem',
              border: '1px solid var(--border)',
              borderRadius: '8px',
              fontSize: '0.95rem'
            }}
          />
          <button
            className="btn btn-primary"
            onClick={handleUpdateClaim}
            disabled={updating || !claimKey.trim()}
          >
            {updating ? '업데이트 중...' : '🔄 Update Claim'}
          </button>
        </div>

        {updateError && (
          <div className="error-box" style={{ marginTop: '0.5rem' }}>
            <strong>Error:</strong> {updateError}
          </div>
        )}

        {successMessage && (
          <div className="success-box" style={{ marginTop: '0.5rem', padding: '0.75rem', backgroundColor: '#d4edda', border: '1px solid #c3e6cb', borderRadius: '8px', color: '#155724' }}>
            {successMessage}
          </div>
        )}

        <div style={{ fontSize: '0.85rem', color: 'var(--secondary)' }}>
          💡 <strong>Tip:</strong> JSON 값도 지원합니다. 예: <code>{`{"permissions":["read","write"]}`}</code>
        </div>
      </div>

      <div className="section">
        <h3>🎭 Current Claims</h3>
        <button className="btn btn-secondary" onClick={refreshClaims}>
          🔄 Refresh Claims
        </button>
        <pre className="claims-json">
          {JSON.stringify(claims, null, 2)}
        </pre>
      </div>

      <div className="section">
        <h3>💡 How It Works</h3>
        <ul>
          <li><code>useClaims()</code> - ID 토큰에서 클레임을 자동으로 추출합니다</li>
          <li><code>updateUserClaims()</code> - 재인증 없이 사용자 클레임 업데이트</li>
          <li>클레임 업데이트 후 토큰이 자동으로 갱신됩니다</li>
          <li>새로운 클레임이 즉시 ID 토큰에 반영됩니다</li>
          <li><strong>예시 패턴</strong>: workspace_id, organization_id, project_id 등 자유롭게 추가 가능</li>
        </ul>
      </div>
    </div>
  )
}

function ComponentsTab() {
  return (
    <div className="components-tab">
      <h2>Pre-built UI Components</h2>
      <p>@authway/react에서 제공하는 사전 제작 컴포넌트들을 테스트해보세요.</p>

      <div className="section">
        <h3>🔘 LoginButton / LogoutButton</h3>
        <p>즉시 사용 가능한 로그인/로그아웃 버튼 컴포넌트</p>
        <div className="component-demo">
          <LoginButton className="btn btn-primary">
            Custom Login Text
          </LoginButton>
          <LogoutButton className="btn btn-secondary" logoutOptions={{ returnTo: window.location.origin }}>
            Custom Logout Text
          </LogoutButton>
        </div>
        <pre className="code-example">{`<LoginButton className="btn btn-primary">
  Custom Login Text
</LoginButton>

<LogoutButton
  className="btn btn-secondary"
  logoutOptions={{ returnTo: window.location.origin }}
>
  Custom Logout Text
</LogoutButton>`}</pre>
      </div>

      <div className="section">
        <h3>👤 UserProfile</h3>
        <p>사용자 정보를 표시하는 프로필 컴포넌트</p>
        <div className="component-demo">
          <UserProfile
            showAvatar={true}
            showEmail={true}
            className="user-profile-demo"
          />
        </div>
        <pre className="code-example">{`<UserProfile
  showAvatar={true}
  showEmail={true}
  className="user-profile-demo"
/>`}</pre>
      </div>

      <div className="section">
        <h3>💡 Component Features</h3>
        <ul>
          <li>완전한 TypeScript 타입 지원</li>
          <li>className을 통한 스타일 커스터마이징</li>
          <li>자동 로딩 상태 관리</li>
          <li>접근성 (accessibility) 기본 지원</li>
          <li>React 16.8+ 호환</li>
        </ul>
      </div>

      <div className="section">
        <h3>🎨 Customization</h3>
        <p>모든 컴포넌트는 className prop으로 스타일을 커스터마이징할 수 있으며,
        children prop으로 내용을 변경할 수 있습니다.</p>
        <p>또는 useAuth, useUser 등의 hooks를 사용하여 완전히 커스텀 UI를 구축할 수 있습니다.</p>
      </div>
    </div>
  )
}

function AdvancedTab() {
  const { getIdTokenClaims, getAccessTokenWithPopup } = useAuth()
  const [idTokenClaims, setIdTokenClaims] = useState<any>(null)
  // const [linkedAccounts, setLinkedAccounts] = useState<any[]>([]) // Backend not implemented
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const handleGetIdTokenClaims = async () => {
    setLoading(true)
    setError(null)
    try {
      const claims = await getIdTokenClaims()
      setIdTokenClaims(claims)
      setSuccess('✅ ID Token Claims를 성공적으로 가져왔습니다!')
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleGetAccessTokenWithPopup = async () => {
    setLoading(true)
    setError(null)
    setSuccess(null)
    try {
      const token = await getAccessTokenWithPopup()
      setSuccess(`✅ 새로운 Access Token을 팝업으로 획득했습니다! (길이: ${token.length} chars)`)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  // Note: Account linking functions removed - backend not implemented yet
  // Available in SDK: getLinkedAccounts, linkAccount, unlinkAccount

  return (
    <div className="advanced-tab">
      <h2>🚀 Advanced Features (Phase 2 & 3)</h2>
      <p>Auth0 호환 고급 기능들을 테스트해보세요.</p>

      {error && (
        <div className="error-box" style={{ marginBottom: '1rem' }}>
          <strong>Error:</strong> {error}
        </div>
      )}

      {success && (
        <div className="success-box" style={{ marginBottom: '1rem', padding: '0.75rem', backgroundColor: '#d4edda', border: '1px solid #c3e6cb', borderRadius: '8px', color: '#155724' }}>
          {success}
        </div>
      )}

      <div className="section">
        <h3>🎫 getIdTokenClaims()</h3>
        <p>ID Token의 전체 Claims를 조회합니다 (Auth0 호환)</p>
        <button
          className="btn btn-primary"
          onClick={handleGetIdTokenClaims}
          disabled={loading}
        >
          {loading ? '로딩 중...' : 'Get ID Token Claims'}
        </button>
        {idTokenClaims && (
          <details className="claims-details" open style={{ marginTop: '1rem' }}>
            <summary>ID Token Claims</summary>
            <pre className="claims-json">
              {JSON.stringify(idTokenClaims, null, 2)}
            </pre>
          </details>
        )}
      </div>

      <div className="section">
        <h3>🪟 getAccessTokenWithPopup()</h3>
        <p>리프레시 토큰 없이 팝업으로 새 Access Token을 획득합니다</p>
        <button
          className="btn btn-primary"
          onClick={handleGetAccessTokenWithPopup}
          disabled={loading}
        >
          {loading ? '로딩 중...' : 'Get Token with Popup'}
        </button>
        <div style={{ fontSize: '0.85rem', color: 'var(--secondary)', marginTop: '0.5rem' }}>
          💡 Refresh Token이 없거나 만료된 경우 유용합니다
        </div>
      </div>

      <div className="section">
        <h3>🔗 Account Linking</h3>
        <p>여러 OAuth 제공자 계정을 하나의 사용자 프로필에 연결할 수 있습니다</p>

        <div style={{
          padding: '1rem',
          backgroundColor: '#fff3cd',
          border: '1px solid #ffc107',
          borderRadius: '8px',
          marginBottom: '1rem'
        }}>
          <strong>⚠️ 백엔드 구현 필요</strong>
          <p style={{ margin: '0.5rem 0 0 0', fontSize: '0.9rem' }}>
            Account Linking 기능은 SDK에 구현되어 있지만, 백엔드 API가 아직 구현되지 않았습니다.
          </p>
          <details style={{ marginTop: '0.5rem', fontSize: '0.85rem' }}>
            <summary style={{ cursor: 'pointer', color: '#856404' }}>구현 필요한 엔드포인트</summary>
            <ul style={{ marginTop: '0.5rem', paddingLeft: '1.5rem' }}>
              <li><code>GET /api/v1/user/identities</code> - 연결된 계정 조회</li>
              <li><code>POST /api/v1/user/identities/link</code> - 계정 연결</li>
              <li><code>DELETE /api/v1/user/identities/:provider/:userId</code> - 연결 해제</li>
            </ul>
          </details>
        </div>

        <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', flexWrap: 'wrap', opacity: 0.5, pointerEvents: 'none' }}>
          <button
            className="btn btn-secondary"
            disabled={true}
          >
            🔍 연결된 계정 조회
          </button>
          <button
            className="btn btn-primary"
            disabled={true}
          >
            🔗 Google 계정 연결
          </button>
          <button
            className="btn btn-primary"
            disabled={true}
          >
            🔗 GitHub 계정 연결
          </button>
        </div>

        <div style={{ fontSize: '0.85rem', color: 'var(--secondary)', marginTop: '1rem' }}>
          💡 <strong>사용 사례:</strong> Google로 가입 후 GitHub 계정도 연결하여 두 방식 모두로 로그인 가능
        </div>

        <details style={{ marginTop: '1rem', fontSize: '0.9rem' }}>
          <summary style={{ cursor: 'pointer' }}>SDK 구현 완료 (백엔드 구현 대기)</summary>
          <pre style={{ backgroundColor: '#f5f5f5', padding: '0.75rem', borderRadius: '4px', overflow: 'auto', marginTop: '0.5rem' }}>
{`// SDK에 구현된 메서드들
const {
  getLinkedAccounts,  // 연결된 계정 목록
  linkAccount,        // 새 계정 연결
  unlinkAccount       // 계정 연결 해제
} = useAuth()

// 사용 예시
const identities = await getLinkedAccounts()
await linkAccount({ provider: 'google' })
await unlinkAccount('google', 'user-id')`}
          </pre>
        </details>
      </div>

      <div className="section">
        <h3>🔐 DPoP Support (RFC 9449)</h3>
        <p>Demonstrating Proof-of-Possession: 토큰 탈취 공격 방지</p>
        <div style={{
          padding: '1rem',
          backgroundColor: '#f8f9fa',
          borderRadius: '8px',
          fontSize: '0.9rem'
        }}>
          <p><strong>DPoP란?</strong></p>
          <ul>
            <li>토큰이 특정 암호화 키에 바인딩됩니다</li>
            <li>각 요청마다 소유권을 증명해야 합니다</li>
            <li>토큰이 탈취되어도 키 없이는 사용 불가</li>
          </ul>
          <p><strong>설정 방법:</strong></p>
          <pre style={{ backgroundColor: '#fff', padding: '0.5rem', borderRadius: '4px', overflow: 'auto' }}>
{`<AuthwayProvider
  config={{
    domain: 'http://localhost:8080',
    clientId: 'your-client-id',
    useDPoP: true  // DPoP 활성화
  }}
>`}
          </pre>
          <p style={{ marginTop: '0.5rem' }}>
            ℹ️ 현재 설정: <strong>{config.useDPoP ? '✅ 활성화' : '❌ 비활성화'}</strong>
          </p>
        </div>
      </div>
    </div>
  )
}

function HocTab() {
  return (
    <div className="hoc-tab">
      <h2>⚛️ Higher-Order Components (HOCs)</h2>
      <p>Auth0 스타일의 HOC를 사용하여 클래스 컴포넌트와 라우트 보호를 구현할 수 있습니다.</p>

      <div className="section">
        <h3>🔒 withAuthRequired</h3>
        <p>인증이 필요한 라우트를 보호하는 HOC (Auth0의 withAuthenticationRequired와 동일)</p>
        <pre className="code-example">{`import { withAuthRequired } from '@authway/react'

// 기본 사용
const ProtectedProfile = withAuthRequired(ProfilePage)

// 옵션 사용
const ProtectedProfile = withAuthRequired(ProfilePage, {
  onRedirecting: () => <div>Loading...</div>,
  returnTo: '/profile',
  loginOptions: {}
})

// 라우터와 함께 사용
<Route path="/profile" element={<ProtectedProfile />} />`}</pre>

        <div style={{
          padding: '1rem',
          backgroundColor: '#f8f9fa',
          borderRadius: '8px',
          marginTop: '1rem'
        }}>
          <p><strong>작동 방식:</strong></p>
          <ol style={{ fontSize: '0.9rem' }}>
            <li>사용자가 인증되지 않은 경우 자동으로 로그인 페이지로 리다이렉트</li>
            <li>로그인 후 원래 페이지로 돌아옴 (returnTo 사용)</li>
            <li>로딩 중에는 커스텀 로딩 컴포넌트 표시 가능</li>
          </ol>
        </div>
      </div>

      <div className="section">
        <h3>⚛️ withAuthway</h3>
        <p>클래스 컴포넌트에 auth context를 주입하는 HOC (Auth0의 withAuth0와 동일)</p>
        <pre className="code-example">{`import { withAuthway, WithAuthwayProps } from '@authway/react'

// 클래스 컴포넌트 정의
class ProfilePage extends React.Component<WithAuthwayProps> {
  render() {
    const { auth } = this.props

    if (auth.isLoading) {
      return <div>Loading...</div>
    }

    if (!auth.isAuthenticated) {
      return <div>Not authenticated</div>
    }

    return (
      <div>
        <h1>Welcome, {auth.user?.name}!</h1>
        <button onClick={() => auth.logout()}>
          Logout
        </button>
      </div>
    )
  }
}

// HOC로 감싸기
export default withAuthway(ProfilePage)`}</pre>

        <div style={{
          padding: '1rem',
          backgroundColor: '#f8f9fa',
          borderRadius: '8px',
          marginTop: '1rem'
        }}>
          <p><strong>주입되는 auth 객체:</strong></p>
          <ul style={{ fontSize: '0.9rem' }}>
            <li><code>auth.user</code> - 사용자 정보</li>
            <li><code>auth.isAuthenticated</code> - 인증 상태</li>
            <li><code>auth.isLoading</code> - 로딩 상태</li>
            <li><code>auth.loginWithRedirect()</code> - 로그인 메서드</li>
            <li><code>auth.logout()</code> - 로그아웃 메서드</li>
            <li><code>auth.getAccessToken()</code> - 토큰 획득 메서드</li>
            <li>그 외 모든 useAuth() 메서드</li>
          </ul>
        </div>
      </div>

      <div className="section">
        <h3>🆚 HOC vs Hooks</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
          <div style={{ padding: '1rem', border: '1px solid var(--border)', borderRadius: '8px' }}>
            <h4 style={{ marginTop: 0 }}>HOCs (withAuthRequired, withAuthway)</h4>
            <p style={{ fontSize: '0.9rem' }}>✅ 클래스 컴포넌트에서 사용 가능</p>
            <p style={{ fontSize: '0.9rem' }}>✅ Auth0에서 마이그레이션하기 쉬움</p>
            <p style={{ fontSize: '0.9rem' }}>✅ 라우트 보호에 적합</p>
            <p style={{ fontSize: '0.9rem' }}>⚠️ 함수형 컴포넌트에서는 불필요</p>
          </div>
          <div style={{ padding: '1rem', border: '1px solid var(--border)', borderRadius: '8px' }}>
            <h4 style={{ marginTop: 0 }}>Hooks (useAuth, useUser)</h4>
            <p style={{ fontSize: '0.9rem' }}>✅ 함수형 컴포넌트에서 직접 사용</p>
            <p style={{ fontSize: '0.9rem' }}>✅ 더 간단하고 직관적</p>
            <p style={{ fontSize: '0.9rem' }}>✅ React 모던 패턴</p>
            <p style={{ fontSize: '0.9rem' }}>⚠️ 클래스 컴포넌트에서 사용 불가</p>
          </div>
        </div>
      </div>

      <div className="section">
        <h3>💡 권장 사용법</h3>
        <ul>
          <li><strong>새로운 프로젝트</strong>: Hooks (useAuth, useUser) 사용 권장</li>
          <li><strong>Auth0에서 마이그레이션</strong>: HOC로 시작하여 점진적으로 Hooks로 전환</li>
          <li><strong>클래스 컴포넌트 레거시 코드</strong>: withAuthway 사용</li>
          <li><strong>라우트 보호</strong>: withAuthRequired 또는 커스텀 ProtectedRoute 컴포넌트</li>
        </ul>
      </div>
    </div>
  )
}

function Footer() {
  return (
    <footer className="footer">
      <p>
        <strong>Authway React SDK Sample</strong> |
        로컬 개발 환경: <code>http://localhost:8081</code> (Auth Backend)
      </p>
      <p className="footer-links">
        <a href="http://localhost:3000" target="_blank" rel="noopener">Admin Dashboard</a>
        <span>•</span>
        <a href="http://localhost:3001" target="_blank" rel="noopener">Login UI</a>
        <span>•</span>
        <a href="http://localhost:8025" target="_blank" rel="noopener">MailHog</a>
      </p>
    </footer>
  )
}

export default App
