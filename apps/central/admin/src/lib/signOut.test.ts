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
  useAuthStore.setState({ token: null, expiresAt: null, method: null, signingOut: false })
})

describe('signOut', () => {
  it('ends a console account’s session at Authway, not at the API', async () => {
    let apiLogout = false
    server.use(http.post('http://localhost:8080/admin/logout', () => {
      apiLogout = true
      return HttpResponse.json({})
    }))
    signOutOfAccount.mockResolvedValue(true)
    useAuthStore.getState().login('account-token', later(), 'account')
    expect(await signOut()).toBe(true)
    expect(signOutOfAccount).toHaveBeenCalledOnce()
    expect(apiLogout).toBe(false)
    // Leaving: the token is gone, and the console holds on the signing-out
    // screen rather than a sign-in page whose click would cancel the leaving.
    expect(useAuthStore.getState()).toMatchObject({ token: null, method: null, signingOut: true })
  })

  it('holds the signing-out screen before it starts to leave', async () => {
    let signingOutWhenLeaving: boolean | undefined
    signOutOfAccount.mockImplementation(async () => {
      signingOutWhenLeaving = useAuthStore.getState().signingOut
      return true
    })
    useAuthStore.getState().login('account-token', later(), 'account')
    await signOut()
    expect(signingOutWhenLeaving).toBe(true)
  })

  it('shows the sign-in page when there is nowhere to leave for', async () => {
    signOutOfAccount.mockResolvedValue(false)
    useAuthStore.getState().login('account-token', later(), 'account')
    expect(await signOut()).toBe(false)
    expect(useAuthStore.getState()).toMatchObject({ token: null, signingOut: false })
  })

  it('shows the sign-in page when leaving fails', async () => {
    signOutOfAccount.mockRejectedValue(new Error('discovery unreachable'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    useAuthStore.getState().login('account-token', later(), 'account')
    expect(await signOut()).toBe(false)
    expect(useAuthStore.getState()).toMatchObject({ token: null, signingOut: false })
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
