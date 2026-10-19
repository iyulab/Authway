import { useAuthStore } from '@/stores/auth'
import { authApi } from './api'
import { signOutOfAccount } from './consoleSignIn'

/**
 * Signs the console out the way it signed in. A console account's session
 * ends at Authway too — the browser leaves for Authway's sign-out and comes
 * back to the console; resolves true when it is leaving. An admin session is
 * ended at the API.
 */
export async function signOut(): Promise<boolean> {
  const { method, logout } = useAuthStore.getState()
  if (method === 'account') {
    logout()
    await signOutOfAccount()
    return true
  }
  try {
    await authApi.logout()
  } catch (error) {
    console.error('Logout error:', error)
  } finally {
    logout()
  }
  return false
}
