import { AuthwayClient } from '@authway/client'
import { consolePath, getConfig } from '../config'

/** What GET /api/v1/console/entry answers: the client console accounts sign in with. */
export interface ConsoleEntry {
  client_id: string
  redirect_uri: string
  scope: string
  audience: string
}

/** Where the entry is kept between leaving for sign-in and coming back to /callback. */
const ENTRY_KEY = 'authway.console.entry'

/**
 * The console's sign-in client, or null when the deployment offers none (no
 * console address or no system tenant configured) — the console then signs
 * in with the admin password only.
 */
export async function fetchConsoleEntry(fetcher: typeof fetch = fetch): Promise<ConsoleEntry | null> {
  try {
    const res = await fetcher(`${getConfig().apiUrl}/api/v1/console/entry`)
    if (!res.ok) return null
    return (await res.json()) as ConsoleEntry
  } catch {
    return null
  }
}

const clientFor = (entry: ConsoleEntry) =>
  new AuthwayClient({
    domain: getConfig().apiUrl || window.location.origin,
    clientId: entry.client_id,
    redirectUri: entry.redirect_uri,
    scope: entry.scope,
    audience: entry.audience,
    cacheLocation: 'sessionstorage',
  })

const readEntry = (): ConsoleEntry | null => {
  try {
    const raw = sessionStorage.getItem(ENTRY_KEY)
    return raw ? (JSON.parse(raw) as ConsoleEntry) : null
  } catch {
    return null
  }
}

/** Leaves for the sign-in screen; the browser comes back to /callback. */
export async function signInWithAccount(entry: ConsoleEntry): Promise<void> {
  sessionStorage.setItem(ENTRY_KEY, JSON.stringify(entry))
  await clientFor(entry).loginWithRedirect()
}

/** The console account's token, from the sign-in the browser came back from. */
export interface SignedIn {
  token: string
  /** ISO time, as the admin session's expiry is kept. */
  expiresAt: string
}

/** Completes the sign-in on /callback. Throws when this page started none, or it failed. */
export async function completeSignIn(url = window.location.href): Promise<SignedIn> {
  const entry = readEntry()
  if (!entry) throw new Error('No sign-in was started from this console.')
  const result = await clientFor(entry).handleRedirectCallback(url)
  return {
    token: result.accessToken,
    expiresAt: new Date(Date.now() + result.expiresIn * 1000).toISOString(),
  }
}

/**
 * Ends the console account's session at Authway too, and comes back to the
 * console's sign-in page. Without the entry (it was cleared) only this
 * browser's tokens are dropped.
 */
export async function signOutOfAccount(): Promise<void> {
  const entry = readEntry()
  if (!entry) return
  sessionStorage.removeItem(ENTRY_KEY)
  await clientFor(entry).logout({ returnTo: new URL(consolePath(''), window.location.origin).toString() })
}
