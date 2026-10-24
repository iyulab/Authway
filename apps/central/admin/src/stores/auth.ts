import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * How the console signed in: with the admin password (a console session), or
 * as a console account through Authway's own sign-in (its access token).
 * Signing out of an account also ends its session at Authway.
 */
export type SignInMethod = 'password' | 'account'

interface AuthState {
  token: string | null
  expiresAt: string | null
  method: SignInMethod | null
  /**
   * The browser is leaving for Authway's sign-out. The console shows nothing
   * else meanwhile: a sign-in started from a page still on screen would cancel
   * that navigation, and Authway's session would outlive the sign-out. Never
   * persisted — a page loaded afresh is not leaving.
   */
  signingOut: boolean
  login: (token: string, expiresAt: string, method?: SignInMethod) => void
  logout: () => void
  /** Drops the token like logout, and holds the console on the signing-out screen. */
  beginSignOut: () => void
}

/**
 * The store is the only home of the admin token.
 *
 * It used to also write `localStorage['authway_admin_token']` for the axios
 * interceptor to read, and to persist a separate `isAuthenticated` boolean.
 * Three copies of one fact drift apart, and they did: the 401 interceptor
 * cleared the standalone key but not the store, so the app kept believing it
 * was signed in while sending no credentials. `/login` then bounced straight
 * back to `/dashboard`, whose first request 401'd and hard-navigated to
 * `/login` again — an endless full-page reload loop that only clearing
 * localStorage by hand could escape.
 */
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      expiresAt: null,
      method: null,
      signingOut: false,
      login: (token, expiresAt, method = 'password') => set({ token, expiresAt, method, signingOut: false }),
      logout: () => set({ token: null, expiresAt: null, method: null, signingOut: false }),
      beginSignOut: () => set({ token: null, expiresAt: null, method: null, signingOut: true }),
    }),
    {
      name: 'authway-admin-auth',
      // Only these facts.
      partialize: (state) => ({
        token: state.token,
        expiresAt: state.expiresAt,
        method: state.method,
      }),
      // Read back exactly those and nothing else. The default merge is a
      // shallow spread, so a stale `isAuthenticated: true` written by an older
      // build would be reinstated into the live store on every load — the very
      // value that caused the loop. Naming the fields drops it for good.
      merge: (persisted, current) => {
        const saved = (persisted ?? {}) as Partial<AuthState>
        return {
          ...current,
          token: saved.token ?? null,
          expiresAt: saved.expiresAt ?? null,
          method: saved.method ?? (saved.token ? 'password' : null),
        }
      },
    }
  )
)

/**
 * Authentication is DERIVED, never stored — a stored boolean can contradict the
 * token, and that contradiction was the bug.
 *
 * The expiry comparison is what heals browsers already stuck in the reload
 * loop: their persisted token is still present (only the mirrored copy was
 * cleared), so `token !== null` alone would keep them signed in and looping.
 * Their `expiresAt` is long past, so checking it lands them on the login page
 * on the very first render, with no network round trip.
 */
export const selectIsAuthenticated = (state: AuthState): boolean => {
  if (!state.token || !state.expiresAt) return false
  const expiry = new Date(state.expiresAt).getTime()
  return Number.isFinite(expiry) && expiry > Date.now()
}
