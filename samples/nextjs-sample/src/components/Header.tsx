'use client'

import Image from 'next/image'
import { useAuth } from '@authway/react'

export default function Header() {
  const { isAuthenticated, isLoading, user, loginWithRedirect, logout } = useAuth()

  return (
    <header className="header">
      <div className="header-content">
        <div className="logo">
          <span className="logo-icon">⚡</span>
          <h1>Authway Next.js</h1>
          <span className="badge">Sample</span>
        </div>

        <nav className="nav">
          {isLoading ? (
            <span>Loading...</span>
          ) : !isAuthenticated ? (
            <button
              className="btn btn-primary"
              onClick={() => loginWithRedirect({ appState: { returnTo: '/' } })}
            >
              Login
            </button>
          ) : (
            <div className="user-info">
              {user?.picture && (
                <Image
                  src={user.picture}
                  alt={user.name || 'User'}
                  width={32}
                  height={32}
                  className="avatar"
                />
              )}
              <span className="user-name">{user?.name || user?.email}</span>
              <button
                className="btn btn-secondary"
                onClick={() => logout({ returnTo: window.location.origin })}
              >
                Logout
              </button>
            </div>
          )}
        </nav>
      </div>
    </header>
  )
}
