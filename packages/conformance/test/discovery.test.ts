import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

describe('OIDC discovery', () => {
  it('publishes the endpoints a relying party needs', async () => {
    const d = await provider.discovery()
    expect(d.issuer.replace(/\/+$/, '')).toBe(provider.config.issuer)
    for (const key of ['authorization_endpoint', 'token_endpoint', 'userinfo_endpoint', 'jwks_uri'] as const) {
      expect(d[key], key).toMatch(/^https?:\/\//)
    }
  })

  it('supports the authorization-code flow with S256 PKCE', async () => {
    const d = await provider.discovery()
    expect(d.response_types_supported).toContain('code')
    expect(d.code_challenge_methods_supported).toContain('S256')
  })

  // Discovery is how a relying party learns what it can ask for and what it
  // will be told, so it has to name the claims and scopes of the contract.
  it('advertises the scopes and claims of the contract', async () => {
    const d = await provider.discovery()
    for (const scope of ['openid', 'profile', 'email', 'offline_access']) {
      expect(d.scopes_supported, `scopes_supported: ${scope}`).toContain(scope)
    }
    for (const claim of ['sub', 'tenant_id', 'email', 'email_verified', 'name', 'auth_time', 'amr']) {
      expect(d.claims_supported, `claims_supported: ${claim}`).toContain(claim)
    }
  })

  it('serves a JWKS with at least one signing key', async () => {
    const d = await provider.discovery()
    const res = await fetch(d.jwks_uri)
    expect(res.status).toBe(200)
    const { keys } = (await res.json()) as { keys: unknown[] }
    expect(keys.length).toBeGreaterThan(0)
  })
})

// Authway's own statement of the provider's metadata: the authorization
// server's document with what Authway answers added and what no client can be
// registered for taken out. Reached at the issuer's address only where the
// deployment routes it to the API; here it is read at the API.
describe('discovery as Authway offers it', () => {
  const read = async (path: string) => {
    const res = await fetch(`${provider.config.api}${path}`, { headers: { Origin: 'https://any-page.example' } })
    expect(res.status, path).toBe(200)
    expect(res.headers.get('access-control-allow-origin'), `${path} is readable from any page`).toBeTruthy()
    return (await res.json()) as Record<string, unknown>
  }

  it('keeps the issuer and says what a client can actually use', async () => {
    const d = await read('/.well-known/openid-configuration')
    const engine = (await provider.discovery()) as unknown as Record<string, unknown>
    expect(d.issuer).toBe(engine.issuer)
    expect(d.token_endpoint).toBe(engine.token_endpoint)
    expect(d.response_types_supported).toEqual(['code'])
    expect(d.grant_types_supported).not.toContain('implicit')
    expect(d.token_endpoint_auth_methods_supported).not.toContain('client_secret_basic')
    expect(d.token_endpoint_auth_signing_alg_values_supported).toContain('ES256')
    expect(d.frontchannel_logout_supported).toBe(false)
    expect(d.request_uri_parameter_supported).toBe(false)
    expect(d.introspection_endpoint).toBe(`${provider.config.api}/api/v1/oauth2/introspect`)

    const caps = (await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()) as Record<string, unknown>
    if (caps.token_exchange) expect(d.grant_types_supported).toContain('urn:ietf:params:oauth:grant-type:token-exchange')
    if (caps.approval_requests) {
      expect(d.grant_types_supported).toContain('urn:authway:params:oauth:grant-type:approval')
      expect(d.approval_request_endpoint).toBe(`${provider.config.api}/oauth2/approval-requests`)
    }
  })

  it('is the same document at the RFC 8414 address', async () => {
    expect(await read('/.well-known/oauth-authorization-server')).toEqual(await read('/.well-known/openid-configuration'))
  })
})
