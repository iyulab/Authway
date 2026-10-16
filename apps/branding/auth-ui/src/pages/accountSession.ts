import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AuthwayClient } from '@authway/client'
import { getConfig } from '../config'

/** What GET /api/v1/account-entry answers: the client this screen signs in with, and the application to go back to. */
export interface AccountEntry {
  client_id: string
  redirect_uri: string
  scope: string
  app: { name: string; client_uri?: string }
}

/** Where the entry is kept between leaving for sign-in and coming back to /account/callback. */
const ENTRY_KEY = 'authway.account.entry'

export const apiBase = () => getConfig().apiUrl || window.location.origin

/** The application's address when it is an https URL — the only kind it registers, and the only kind followed. */
export const httpsUrl = (raw?: string): string | undefined => {
  try {
    const u = raw ? new URL(raw) : undefined
    return u && u.protocol === 'https:' ? u.toString() : undefined
  } catch {
    return undefined
  }
}

export const signIn = (entry: AccountEntry) =>
  new AuthwayClient({
    domain: apiBase(),
    clientId: entry.client_id,
    redirectUri: entry.redirect_uri,
    scope: entry.scope,
    cacheLocation: 'sessionstorage',
  })

export interface Stored {
  appClientId: string
  entry: AccountEntry
  /** The screen to come back to after sign-in: a path on this site. */
  returnTo?: string
}

export const readStored = (): Stored | null => {
  try {
    const raw = sessionStorage.getItem(ENTRY_KEY)
    return raw ? (JSON.parse(raw) as Stored) : null
  } catch {
    return null
  }
}

/** Where the comeback from sign-in goes: the screen that sent the user, when it is a path on this site. */
export const returnPath = (stored: Stored): string =>
  // A browser reads "/\host" as "//host": another site.
  stored.returnTo?.startsWith('/') && !stored.returnTo.startsWith('//') && !stored.returnTo.includes('\\')
    ? stored.returnTo
    : `/account?client_id=${encodeURIComponent(stored.appClientId)}`

/** A refusal's message, or null when the answer is not one. */
export const refusal = async (res: Response): Promise<{ code?: string; detail?: string }> => {
  try {
    return (await res.json()) as { code?: string; detail?: string }
  } catch {
    return {}
  }
}

/**
 * Signs the user in to the tenant's account application, found from the
 * application that sent them (appClientId) — no screen at all when their
 * session is still there — and calls the account API with their token. After
 * a sign-in that leaves the page, the comeback returns to returnTo.
 */
export function useAccountSession(appClientId: string | null, returnTo: string) {
  const { t } = useTranslation(['auth', 'common'])
  const [entry, setEntry] = useState<AccountEntry | null>(null)
  const [email, setEmail] = useState<string | null>(null)
  const [fatal, setFatal] = useState<string | null>(null)

  const client = useMemo(() => (entry ? signIn(entry) : null), [entry])

  /** Calls the account API with the user's token. */
  const call = useCallback(
    async (path: string, init: RequestInit = {}) => {
      const token = await client!.getAccessToken()
      return fetch(`${apiBase()}${path}`, {
        ...init,
        headers: { Authorization: `Bearer ${token}`, ...(init.body ? { 'Content-Type': 'application/json' } : {}) },
      })
    },
    [client],
  )

  // Find the account client for the application, then make sure the user is signed in to it.
  useEffect(() => {
    if (!appClientId) return
    let cancelled = false
    ;(async () => {
      const res = await fetch(`${apiBase()}/api/v1/account-entry?client_id=${encodeURIComponent(appClientId)}`)
      if (!res.ok) {
        if (!cancelled) setFatal(res.status === 404 ? t('auth:account.unknownApp') : t('common:errorOccurred'))
        return
      }
      const answer = (await res.json()) as AccountEntry
      sessionStorage.setItem(ENTRY_KEY, JSON.stringify({ appClientId, entry: answer, returnTo } satisfies Stored))
      const sdk = signIn(answer)
      if (!(await sdk.isAuthenticated()) && !(await sdk.checkSession().catch(() => false))) {
        // A run that was cleaned up must not start a sign-in: two would each
        // store their own state, and the one that comes back would not match.
        if (!cancelled) await sdk.loginWithRedirect()
        return
      }
      if (cancelled) return
      setEntry(answer)
      setEmail((await sdk.getUser())?.email ?? null)
    })().catch(() => !cancelled && setFatal(t('common:errorOccurred')))
    return () => {
      cancelled = true
    }
  }, [appClientId, returnTo, t])

  return { entry, email, client, call, fatal, setFatal }
}
