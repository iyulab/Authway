import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { useAuthStore } from '@/stores/auth'
import App from './App'

beforeEach(() => {
  useAuthStore.setState({ token: null, expiresAt: null, method: null, signingOut: false })
})

describe('App while signing out', () => {
  it('shows only the signing-out screen, with no sign-in to start', () => {
    useAuthStore.getState().beginSignOut()
    render(
      <MemoryRouter initialEntries={['/login']}>
        <App />
      </MemoryRouter>,
    )
    expect(screen.getByRole('status')).toHaveTextContent('로그아웃 중')
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows the sign-in page again when the browser comes back to it from history', () => {
    useAuthStore.getState().beginSignOut()
    render(
      <MemoryRouter initialEntries={['/login']}>
        <App />
      </MemoryRouter>,
    )
    const restored = new Event('pageshow') as PageTransitionEvent
    Object.defineProperty(restored, 'persisted', { value: true })
    window.dispatchEvent(restored)
    expect(useAuthStore.getState().signingOut).toBe(false)
  })
})
