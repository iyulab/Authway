import { describe, expect, it } from 'vitest'
import { resolveProvider } from './discovery'
import { ConfigurationError } from '../types/errors'

type Routes = Record<string, { status?: number; body?: unknown }>

function fakeFetch(routes: Routes) {
  const calls: string[] = []
  const impl = (async (input: RequestInfo | URL) => {
    const url = String(input)
    calls.push(url)
    const route = routes[url]
    if (!route) return new Response('not found', { status: 404 })
    return new Response(JSON.stringify(route.body ?? {}), { status: route.status ?? 200 })
  }) as typeof fetch
  return { impl, calls }
}

const metadata = (issuer: string) => ({
  issuer,
  authorization_endpoint: `${issuer}/oauth2/auth`,
  token_endpoint: `${issuer}/oauth2/token`,
  end_session_endpoint: `${issuer}/oauth2/sessions/logout`,
  userinfo_endpoint: `${issuer}/userinfo`,
})

describe('resolveProvider', () => {
  it('follows the Authway API to a separate issuer and takes every endpoint from its metadata', async () => {
    const { impl } = fakeFetch({
      'https://api.example.com/.well-known/authway-config': {
        body: { oauth_url: 'https://login.example.com', api_url: 'https://api.example.com' },
      },
      'https://login.example.com/.well-known/openid-configuration': { body: metadata('https://login.example.com') },
    })

    const resolved = await resolveProvider('https://api.example.com/', undefined, impl)

    expect(resolved.apiUrl).toBe('https://api.example.com')
    expect(resolved.endpoints.authorizationEndpoint).toBe('https://login.example.com/oauth2/auth')
    expect(resolved.endpoints.tokenEndpoint).toBe('https://login.example.com/oauth2/token')
    expect(resolved.endpoints.endSessionEndpoint).toBe('https://login.example.com/oauth2/sessions/logout')
  })

  it('treats a deployment without the Authway document as its own issuer', async () => {
    const { impl } = fakeFetch({
      'https://id.example.com/.well-known/openid-configuration': {
        body: { ...metadata('https://id.example.com'), authorization_endpoint: 'https://id.example.com/connect/authorize' },
      },
    })

    const resolved = await resolveProvider('https://id.example.com', undefined, impl)

    expect(resolved.apiUrl).toBe('https://id.example.com')
    expect(resolved.endpoints.authorizationEndpoint).toBe('https://id.example.com/connect/authorize')
  })

  it('uses an explicit issuer without asking the Authway API', async () => {
    const { impl, calls } = fakeFetch({
      'https://login.example.com/.well-known/openid-configuration': { body: metadata('https://login.example.com') },
    })

    await resolveProvider('https://api.example.com', 'https://login.example.com', impl)

    expect(calls).toEqual(['https://login.example.com/.well-known/openid-configuration'])
  })

  it('rejects metadata whose issuer does not match the issuer it was fetched for', async () => {
    const { impl } = fakeFetch({
      'https://login.example.com/.well-known/openid-configuration': { body: metadata('https://evil.example.com') },
    })

    await expect(resolveProvider('https://api.example.com', 'https://login.example.com', impl)).rejects.toBeInstanceOf(
      ConfigurationError,
    )
  })

  it('fails instead of guessing when discovery is unavailable', async () => {
    const { impl } = fakeFetch({})
    await expect(resolveProvider('https://id.example.com', undefined, impl)).rejects.toThrow(/OIDC discovery failed/)
  })

  it('reports an end-session endpoint as absent when the provider publishes none', async () => {
    const withoutLogout: Partial<ReturnType<typeof metadata>> = metadata('https://id.example.com')
    delete withoutLogout.end_session_endpoint
    const { impl } = fakeFetch({
      'https://id.example.com/.well-known/openid-configuration': { body: withoutLogout },
    })

    const resolved = await resolveProvider('https://id.example.com', undefined, impl)
    expect(resolved.endpoints.endSessionEndpoint).toBeUndefined()
  })
})
