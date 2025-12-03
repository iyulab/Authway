'use client'

import { useAuth } from '@authway/react'
import Header from '@/components/Header'
import Footer from '@/components/Footer'
import WelcomeScreen from '@/components/WelcomeScreen'
import Dashboard from '@/components/Dashboard'

export default function Home() {
  const { isAuthenticated, isLoading, error } = useAuth()

  if (error) {
    return (
      <>
        <Header />
        <main className="main">
          <div className="container">
            <div className="card error-box">
              <h2>Authentication Error</h2>
              <p>{error.message}</p>
            </div>
          </div>
        </main>
        <Footer />
      </>
    )
  }

  if (isLoading) {
    return (
      <>
        <Header />
        <main className="main">
          <div className="container">
            <div className="loading">
              <div className="spinner"></div>
              <p style={{ marginTop: '1rem' }}>Checking authentication...</p>
            </div>
          </div>
        </main>
        <Footer />
      </>
    )
  }

  return (
    <>
      <Header />
      <main className="main">
        <div className="container">
          {isAuthenticated ? <Dashboard /> : <WelcomeScreen />}
        </div>
      </main>
      <Footer />
    </>
  )
}
