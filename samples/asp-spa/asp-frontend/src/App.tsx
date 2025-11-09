import { AuthwayProvider, useAuth } from '@authway/react'
import { WelcomeScreen } from './components/WelcomeScreen'
import { Dashboard } from './components/Dashboard'
import { API_BASE_URL } from './config'
import './App.css'

// Authway Configuration
// domain: Auth Backend URL (port 8081)
// Auth Backend acts as a proxy to Central API and handles CORS
// The SDK will auto-detect OAuth server (Hydra on port 4444)
const authConfig = {
  domain: 'http://localhost:8081',  // Auth Backend (proxies to Central API on 8080)
  clientId: 'authway_spa_sample_local',
  useDPoP: false
}

function App() {
  return (
    <AuthwayProvider
      config={authConfig}
      onRedirectCallback={(appState) => {
        console.log('✅ Redirect callback:', appState)
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
  const { isAuthenticated, user, logout, isLoading } = useAuth()

  return (
    <header className="app-header">
      <div className="header-content">
        <div className="logo">
          <span className="icon">🔐</span>
          <h1>Authway ASP.NET SPA Sample</h1>
          <span className="badge">@authway/react</span>
        </div>

        <nav className="auth-nav">
          {isLoading ? (
            <span className="loading-text">Loading...</span>
          ) : isAuthenticated ? (
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
          ) : null}
        </nav>
      </div>
    </header>
  )
}

function Main() {
  const { isAuthenticated, isLoading, error } = useAuth()

  if (error) {
    return (
      <main className="main">
        <div className="error-card">
          <h2>⚠️ Error</h2>
          <p>{error.message}</p>
          <pre>{JSON.stringify(error, null, 2)}</pre>
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

  return (
    <main className="main">
      {!isAuthenticated ? <WelcomeScreen /> : <Dashboard />}
    </main>
  )
}

function Footer() {
  return (
    <footer className="app-footer">
      <div className="footer-content">
        <p>
          <strong>Authway React SDK</strong> with ASP.NET Backend
        </p>
        <p className="tech-stack">
          <span>@authway/client</span>
          <span>•</span>
          <span>@authway/react</span>
          <span>•</span>
          <span>ASP.NET Core</span>
          <span>•</span>
          <span>API: {API_BASE_URL}</span>
        </p>
      </div>
    </footer>
  )
}

export default App
