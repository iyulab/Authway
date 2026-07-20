import { describe, it, expect, beforeEach, vi } from 'vitest'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/mocks/server'
import { useAuthStore, selectIsAuthenticated } from './auth'
import { api } from '@/lib/api'

const future = () => new Date(Date.now() + 60 * 60 * 1000).toISOString()
const past = () => new Date(Date.now() - 60 * 60 * 1000).toISOString()

beforeEach(() => {
  useAuthStore.setState({ token: null, expiresAt: null })
})

describe('selectIsAuthenticated', () => {
  it('is false with no token', () => {
    expect(selectIsAuthenticated(useAuthStore.getState())).toBe(false)
  })

  it('is true for a token that has not expired', () => {
    useAuthStore.getState().login('t', future())
    expect(selectIsAuthenticated(useAuthStore.getState())).toBe(true)
  })

  // The production incident: admin.authway.in reloaded forever because the app
  // considered itself signed in while holding a token that expired months
  // earlier. Presence of a token must never be enough on its own.
  it('is false for a token whose expiry has passed', () => {
    useAuthStore.setState({ token: 'stale-but-present', expiresAt: past() })
    expect(selectIsAuthenticated(useAuthStore.getState())).toBe(false)
  })

  it('is false when the expiry is missing or unparseable', () => {
    useAuthStore.setState({ token: 't', expiresAt: null })
    expect(selectIsAuthenticated(useAuthStore.getState())).toBe(false)

    useAuthStore.setState({ token: 't', expiresAt: 'not a date' })
    expect(selectIsAuthenticated(useAuthStore.getState())).toBe(false)
  })
})

describe('rehydration from a stuck browser', () => {
  // Verbatim shape of the poisoned localStorage entry observed on production:
  // a persisted `isAuthenticated: true` next to a long-expired token. Loading
  // this must NOT sign the user in, or the fix would not heal anyone who is
  // already stuck — it would only help people who cleared storage by hand.
  it('ignores a persisted isAuthenticated flag and honours the expiry', async () => {
    const poisoned = JSON.stringify({
      state: {
        token: 'exg-TDZCq444mgpWFIfMzLNCn9fDsyhBC2RXTxO85d0=',
        expiresAt: '2026-04-04T08:33:54.046672638Z',
        isAuthenticated: true,
      },
      version: 0,
    })
    vi.mocked(window.localStorage.getItem).mockReturnValue(poisoned)

    await useAuthStore.persist.rehydrate()

    const state = useAuthStore.getState()
    expect(state.token).toBe('exg-TDZCq444mgpWFIfMzLNCn9fDsyhBC2RXTxO85d0=')
    expect(selectIsAuthenticated(state)).toBe(false)
    expect('isAuthenticated' in state).toBe(false)
  })
})

describe('outgoing requests', () => {
  // The token moved from a standalone localStorage key into the store, and the
  // request interceptor moved with it. Everything else here tests the signed-out
  // direction; this is the one that proves a signed-in user can still talk to
  // the API. Getting it wrong would trade one reload loop for another.
  it('attaches the bearer token from the store', async () => {
    useAuthStore.getState().login('tok-123', future())
    let seen: string | null = null
    server.use(
      http.get('http://localhost:8080/api/clients', ({ request }) => {
        seen = request.headers.get('authorization')
        return HttpResponse.json({ clients: [], total: 0 })
      })
    )

    await api.get('/api/clients')

    expect(seen).toBe('Bearer tok-123')
  })

  it('sends no Authorization header when signed out', async () => {
    let seen: string | null = 'unset'
    server.use(
      http.get('http://localhost:8080/api/clients', ({ request }) => {
        seen = request.headers.get('authorization')
        return HttpResponse.json({ clients: [], total: 0 })
      })
    )

    await api.get('/api/clients')

    expect(seen).toBeNull()
  })
})

describe('401 handling', () => {
  it('clears the whole auth state, not just the token', async () => {
    useAuthStore.getState().login('t', future())
    server.use(
      http.get('http://localhost:8080/api/clients', () =>
        HttpResponse.json({ error: 'unauthorized' }, { status: 401 })
      )
    )

    await expect(api.get('/api/clients')).rejects.toThrow()

    const state = useAuthStore.getState()
    expect(state.token).toBeNull()
    expect(state.expiresAt).toBeNull()
    // The loop was: token cleared, "signed in" still true, /login bounces back
    // to /dashboard, whose first request 401s again.
    expect(selectIsAuthenticated(state)).toBe(false)
  })
})
