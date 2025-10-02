import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { User } from '../lib/api'

interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
  login: (user: User, token: string) => void
  logout: () => void
  updateUser: (user: User) => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: null,
      isAuthenticated: false,
      login: (user, token) => {
        localStorage.setItem('authway_admin_token', token)
        set({ user, token, isAuthenticated: true })
      },
      logout: () => {
        localStorage.removeItem('authway_admin_token')
        set({ user: null, token: null, isAuthenticated: false })
      },
      updateUser: (user) => set({ user }),
    }),
    {
      name: 'authway-admin-auth',
      partialize: (state) => ({ user: state.user, isAuthenticated: state.isAuthenticated }),
    }
  )
)