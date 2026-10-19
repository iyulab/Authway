import { describe, it, expect, beforeEach, vi } from 'vitest'
import { http, HttpResponse } from 'msw/http'
import { server } from '@/test/mocks/server'
import { useAuthStore } from '@/stores/auth'
import { signOut } from './signOut'

const { signOutOfAccount } = vi.hoisted(() => ({ signOutOfAccount: vi.fn() }))
vi.mock('./consoleSignIn', () => ({ signOutOfAccount }))

const later = () => new Date(Date.now() + 60 * 60 * 1000).toISOString()

beforeEach(() => {
  signOutOfAccount.mockReset()
  useAuthStore.setState({ token: null, expiresAt: null, method: null })
})

describe('signOut', () => {
  it('ends a console account’s session at Authway, not at the API', async () => {
    let apiLogout = false
    server.use(http.post('http://localhost:8080/admin/logout', () => {
      apiLogout = true
      return HttpResponse.json({})
    }))
    useAuthStore.getState().login('account-token', later(), 'account')
    expect(await signOut()).toBe(true)
    expect(signOutOfAccount).toHaveBeenCalledOnce()
    expect(apiLogout).toBe(false)
    expect(useAuthStore.getState()).toMatchObject({ token: null, method: null })
  })

  it('ends an admin session at the API', async () => {
    let apiLogout = false
    server.use(http.post('http://localhost:8080/admin/logout', () => {
      apiLogout = true
      return HttpResponse.json({})
    }))
    useAuthStore.getState().login('session-token', later())
    expect(useAuthStore.getState().method).toBe('password')
    expect(await signOut()).toBe(false)
    expect(apiLogout).toBe(true)
    expect(signOutOfAccount).not.toHaveBeenCalled()
    expect(useAuthStore.getState().token).toBeNull()
  })
})
