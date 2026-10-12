import { describe, expect, it } from 'vitest'
import { AuthenticationError, LoginRequiredError } from '@authway/client'
import type { User } from '@authway/client'
import { initialize, type SessionClient } from './initialize'

const APP = 'https://app.example.com'
const user = { sub: 'user-1' } as User

/** A client in a given state, recording what was asked of it. */
function clientWith(state: {
  signedIn?: boolean
  /** What handling the answer in the address does. */
  answer?: 'tokens' | 'not-signed-in' | 'refused'
  /** Whether a session check leaves the page. */
  checkLeaves?: boolean
}) {
  const asked: string[] = []
  let signedIn = state.signedIn ?? false
  const client: SessionClient = {
    waitForReady: async () => {},
    handleRedirectCallback: async () => {
      asked.push('handleRedirectCallback')
      if (state.answer === 'not-signed-in') throw new LoginRequiredError('login_required', { returnTo: '/inbox' })
      if (state.answer === 'refused') throw new AuthenticationError('no account', 'access_denied')
      signedIn = true
      return { accessToken: 'a', idToken: 'i', expiresIn: 3600, user, appState: { returnTo: '/inbox' } }
    },
    isAuthenticated: async () => signedIn,
    getUser: async () => (signedIn ? user : null),
    checkSession: async () => {
      asked.push('checkSession')
      return state.checkLeaves ?? false
    },
  }
  return { client, asked }
}

const options = (url: string, overrides: Partial<Parameters<typeof initialize>[1]> = {}) => ({
  url,
  skipRedirectCallback: false,
  checkSessionOnLoad: true,
  ...overrides,
})

describe('working out who is signed in on a page load', () => {
  it('is signed in with tokens at hand, without asking the provider', async () => {
    const { client, asked } = clientWith({ signedIn: true })
    expect(await initialize(client, options(`${APP}/reports`))).toEqual({ kind: 'signed-in', user, cameBack: false, appState: undefined })
    expect(asked).toEqual([])
  })

  it('leaves for the provider when there are no tokens and a session may be there', async () => {
    const { client, asked } = clientWith({ checkLeaves: true })
    expect(await initialize(client, options(`${APP}/reports`))).toEqual({ kind: 'leaving' })
    expect(asked).toEqual(['checkSession'])
  })

  it('is signed out when there is nothing to look for', async () => {
    const { client } = clientWith({ checkLeaves: false })
    expect(await initialize(client, options(`${APP}/reports`))).toEqual({ kind: 'signed-out', cameBack: false, appState: undefined })
  })

  it('does not look when the application turned that off', async () => {
    const { client, asked } = clientWith({ checkLeaves: true })
    expect(await initialize(client, options(`${APP}/reports`, { checkSessionOnLoad: false }))).toEqual({
      kind: 'signed-out',
      cameBack: false,
      appState: undefined,
    })
    expect(asked).toEqual([])
  })

  it('comes back signed in, with where the user was', async () => {
    const { client, asked } = clientWith({ answer: 'tokens' })
    let cleared = 0
    const outcome = await initialize(client, options(`${APP}/callback?code=c&state=s`, { onAnswer: () => cleared++ }))
    expect(outcome).toEqual({ kind: 'signed-in', user, cameBack: true, appState: { returnTo: '/inbox' } })
    expect(cleared).toBe(1)
    expect(asked).toEqual(['handleRedirectCallback'])
  })

  it('comes back signed out — not in error — when the provider says nobody is signed in, and does not ask again', async () => {
    const { client, asked } = clientWith({ answer: 'not-signed-in', checkLeaves: true })
    const outcome = await initialize(client, options(`${APP}/callback?error=login_required&state=s`))
    expect(outcome).toEqual({ kind: 'signed-out', cameBack: true, appState: { returnTo: '/inbox' } })
    expect(asked).toEqual(['handleRedirectCallback'])
  })

  it('fails when the provider refuses for another reason', async () => {
    const { client } = clientWith({ answer: 'refused' })
    await expect(initialize(client, options(`${APP}/callback?error=access_denied&state=s`))).rejects.toMatchObject({ code: 'access_denied' })
  })

  it('leaves an answer in the address alone when the application handles it itself', async () => {
    const { client, asked } = clientWith({ answer: 'tokens' })
    const outcome = await initialize(client, options(`${APP}/callback?code=c&state=s`, { skipRedirectCallback: true }))
    expect(outcome.kind).toBe('signed-out')
    expect(asked).toEqual(['checkSession'])
  })
})
