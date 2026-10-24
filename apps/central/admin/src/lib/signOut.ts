import { useAuthStore } from '@/stores/auth'
import { authApi } from './api'
import { signOutOfAccount } from './consoleSignIn'

/**
 * Signs the console out the way it signed in. A console account's session
 * ends at Authway too — the browser leaves for Authway's sign-out and comes
 * back to the console; resolves true when it is leaving, and until the page
 * goes the console shows only the signing-out screen. An admin session is
 * ended at the API.
 */
export async function signOut(): Promise<boolean> {
  const { method, logout, beginSignOut } = useAuthStore.getState()
  if (method === 'account') {
    beginSignOut()
    try {
      if (await signOutOfAccount()) return true
    } catch (error) {
      console.error('Sign-out error:', error)
    }
    // Nowhere to leave for: this browser's sign-in is all there was to end.
    logout()
    return false
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
