import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  token: string | null
  expiresAt: string | null
  isAuthenticated: boolean
  login: (token: string, expiresAt: string) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      expiresAt: null,
      isAuthenticated: false,
      login: (token, expiresAt) => {
        localStorage.setItem('authway_admin_token', token)
        set({ token, expiresAt, isAuthenticated: true })
      },
      logout: () => {
        localStorage.removeItem('authway_admin_token')
        set({ token: null, expiresAt: null, isAuthenticated: false })
      },
    }),
    {
      name: 'authway-admin-auth',
      partialize: (state) => ({
        token: state.token,
        expiresAt: state.expiresAt,
        isAuthenticated: state.isAuthenticated
      }),
    }
  )
)