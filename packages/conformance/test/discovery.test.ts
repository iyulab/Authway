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
