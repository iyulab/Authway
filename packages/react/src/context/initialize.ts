import { LoginRequiredError } from '@authway/client'
import type { AuthwayClient, User } from '@authway/client'

/** What the provider needs of the client to find out who is signed in. */
export type SessionClient = Pick<AuthwayClient, 'waitForReady' | 'handleRedirectCallback' | 'isAuthenticated' | 'getUser' | 'checkSession'>

/** Where a page load ends up. */
export type Initialized =
  /** `cameBack`: this page load is the return from the provider; `appState` is what the request carried. */
  | { kind: 'signed-in'; user: User | null; cameBack: boolean; appState?: any }
  | { kind: 'signed-out'; cameBack: boolean; appState?: any }
  /** The browser is on its way to the provider to ask for the session; this page is about to go away. */
  | { kind: 'leaving' }

export interface InitializeOptions {
  /** The address the page was loaded at. */
  url: string
  /** Leave an answer in the address alone (the application handles it itself). */
  skipRedirectCallback: boolean
  /** Look for the user's session at the provider when the page has no tokens. */
  checkSessionOnLoad: boolean
  /** Called before the answer in the address is handled, so that it is handled once. */
  onAnswer?: () => void
}

/**
 * Works out who is signed in on a page load: handles an answer from the
 * provider if the address carries one, and otherwise — with no tokens at
 * hand — may send the browser to ask for the session.
 *
 * An answer of "not signed in" to that question is not an error: the page
 * loads signed out, with the `appState` of the request so the application can
 * put the user back where they were.
 */
export async function initialize(client: SessionClient, options: InitializeOptions): Promise<Initialized> {
  await client.waitForReady()

  let cameBack = false
  let appState: any
  const params = new URL(options.url).searchParams
  // OAuth error redirects (e.g. access_denied) carry `error=` without `code=`.
  if (!options.skipRedirectCallback && (params.has('code') || params.has('error'))) {
    cameBack = true
    options.onAnswer?.()
    try {
      appState = (await client.handleRedirectCallback(options.url)).appState
    } catch (error) {
      if (!(error instanceof LoginRequiredError)) throw error
      return { kind: 'signed-out', cameBack, appState: error.appState }
    }
  }

  if (await client.isAuthenticated()) {
    return { kind: 'signed-in', user: await client.getUser(), cameBack, appState }
  }
  // Not straight after an answer: the provider has just been asked.
  if (!cameBack && options.checkSessionOnLoad && (await client.checkSession())) {
    return { kind: 'leaving' }
  }
  return { kind: 'signed-out', cameBack, appState }
}
