import type { ConformanceConfig } from './config.js'
import { Browser } from './browser.js'
import { waitForLink } from './mail.js'
import { createPkce, randomToken, type Pkce } from './pkce.js'

export const REDIRECT_URI = 'http://localhost:9999/conformance-callback'
export const POST_LOGOUT_REDIRECT_URI = 'http://localhost:9999/conformance-signed-out'

export interface Discovery {
  issuer: string
  authorization_endpoint: string
  token_endpoint: string
  userinfo_endpoint: string
  jwks_uri: string
  [key: string]: unknown
}

export interface TestClient {
  id: string
  clientId: string
}

export interface TestUser {
  email: string
  password: string
}

export interface Tokens {
  access_token: string
  id_token?: string
  token_type: string
}

/** An authorization request waiting at the login screen. */
export interface LoginAttempt {
  client: TestClient
  scopes: string[]
  browser: Browser
  pkce: Pkce
  flow: string
  /** Ask the provider to keep the user signed in (a persistent login session). */
  remember?: boolean
}

/** What a login attempt produced at the step where it stopped. */
export type LoginOutcome =
  | { kind: 'code'; code: string; tokens: Tokens; sub: string; browser: Browser }
  | { kind: 'rejected'; status: number; body: string }

/**
 * Drives one provider through its public surfaces. The login-UI backend calls
 * (`submitPassword`, `acceptConsent`) are the only part that encodes how a
 * provider's own login screens talk to it; everything else is OIDC or the
 * admin API.
 */
export class Provider {
  private discoveryDoc?: Discovery
  private tenant?: string

  constructor(readonly config: ConformanceConfig) {}

  async discovery(): Promise<Discovery> {
    if (!this.discoveryDoc) {
      const res = await fetch(`${this.config.issuer}/.well-known/openid-configuration`)
      if (!res.ok) throw new Error(`Discovery failed: ${res.status}`)
      this.discoveryDoc = (await res.json()) as Discovery
    }
    return this.discoveryDoc
  }

  // ---- admin provisioning -------------------------------------------------

  private async admin(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers)
    headers.set('Authorization', `Bearer ${this.config.adminKey}`)
    headers.set('X-Tenant-ID', await this.tenantId())
    if (init.body) headers.set('Content-Type', 'application/json')
    return fetch(`${this.config.api}${path}`, { ...init, headers })
  }

  async tenantId(): Promise<string> {
    if (this.config.tenantId) return this.config.tenantId
    if (!this.tenant) {
      const res = await fetch(`${this.config.api}/api/v1/tenants`, {
        headers: { Authorization: `Bearer ${this.config.adminKey}` },
      })
      if (!res.ok) throw new Error(`Listing tenants failed: ${res.status} ${await res.text()}`)
      const body = (await res.json()) as { tenants?: { id: string; slug?: string }[] } | { id: string }[]
      const tenants = Array.isArray(body) ? body : (body.tenants ?? [])
      if (tenants.length === 0) throw new Error('Provider has no tenant to provision into')
      this.tenant = (tenants.find((t) => 'slug' in t && t.slug === 'default') ?? tenants[0]).id
    }
    return this.tenant
  }

  async createPublicClient(): Promise<TestClient> {
    const res = await this.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await this.tenantId(),
        name: `conformance-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        post_logout_redirect_uris: [POST_LOGOUT_REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid', 'profile', 'email'],
      }),
    })
    const text = await res.text()
    if (!res.ok) throw new Error(`Creating client failed: ${res.status} ${text}`)
    const { client } = JSON.parse(text) as { client: { id: string; client_id: string } }
    return { id: client.id, clientId: client.client_id }
  }

  async deleteClient(client: TestClient): Promise<void> {
    await this.admin(`/api/v1/clients/${client.id}`, { method: 'DELETE' })
  }

  /** Provisions a user the way a real one arrives: an admin invitation accepted from the mailed link. */
  async inviteAndAccept(): Promise<TestUser> {
    const email = `conformance-${randomToken(6)}@example.test`
    const password = `Cf-${randomToken(12)}`
    const invite = await this.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email }),
    })
    if (!invite.ok) throw new Error(`Creating invitation failed: ${invite.status} ${await invite.text()}`)

    const link = await waitForLink(this.config.mailApi, email, '/invitation/accept')
    const token = link.searchParams.get('token')
    if (!token) throw new Error(`Invitation link carries no token: ${link}`)

    const accept = await fetch(`${this.config.api}/api/v1/invitations/accept`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token, password, name: 'Conformance User' }),
    })
    if (!accept.ok) throw new Error(`Accepting invitation failed: ${accept.status} ${await accept.text()}`)
    return { email, password }
  }

  async deleteUser(sub: string): Promise<void> {
    await this.admin(`/api/v1/users/${sub}`, { method: 'DELETE' })
  }

  // ---- login-UI backend (provider-specific protocol) ----------------------

  /** Extracts the opaque flow id from the redirect to the login screen. */
  loginFlowFrom(location: URL): string {
    const flow = location.searchParams.get('flow') ?? location.searchParams.get('login_challenge')
    if (!flow) throw new Error(`Redirect to the login screen carries no flow id: ${location}`)
    return flow
  }

  consentFlowFrom(location: URL): string {
    const flow = location.searchParams.get('flow') ?? location.searchParams.get('consent_challenge')
    if (!flow) throw new Error(`Redirect to the consent screen carries no flow id: ${location}`)
    return flow
  }

  async submitPassword(browser: Browser, flow: string, user: TestUser, remember = false): Promise<Response> {
    return browser.fetch(`${this.config.api}/authenticate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ challenge: flow, email: user.email, password: user.password, remember }),
    })
  }

  async acceptConsent(browser: Browser, flow: string, scopes: string[]): Promise<Response> {
    return browser.fetch(`${this.config.api}/consent/accept`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ challenge: flow, grant_scope: scopes }),
    })
  }

  /**
   * RP-initiated logout (OIDC RP-Initiated Logout 1.0) through the logout
   * screen's backend, in the browser that holds the login session. Returns
   * where the browser finally lands.
   */
  async rpInitiatedLogout(browser: Browser, idToken: string): Promise<URL> {
    const d = await this.discovery()
    const endSession = d.end_session_endpoint as string | undefined
    if (!endSession) throw new Error('Discovery publishes no end_session_endpoint')
    const start = new URL(endSession)
    start.search = new URLSearchParams({
      id_token_hint: idToken,
      post_logout_redirect_uri: POST_LOGOUT_REDIRECT_URI,
      state: randomToken(),
    }).toString()
    const logoutScreen = await browser.redirectFrom(start.toString())
    const flow = logoutScreen.searchParams.get('flow') ?? logoutScreen.searchParams.get('logout_challenge')
    if (!flow) throw new Error(`Redirect to the logout screen carries no flow id: ${logoutScreen}`)
    const backend = new URL(`${this.config.api}/logout`)
    backend.search = new URLSearchParams({
      logout_challenge: flow,
      post_logout_redirect_uri: POST_LOGOUT_REDIRECT_URI,
    }).toString()
    let next = await browser.redirectFrom(backend.toString())
    // Follow the authorization server's own hops until it hands the browser back to the client.
    for (let hops = 0; hops < 5 && next.origin === new URL(this.config.issuer).origin; hops++) {
      next = await browser.redirectFrom(next.toString())
    }
    return next
  }

  async logout(accessToken: string): Promise<Response> {
    return fetch(`${this.config.api}/api/v1/logout`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${accessToken}` },
    })
  }

  // ---- authorization-code + PKCE login ------------------------------------

  /** Starts an authorization-code + PKCE request and stops at the login screen. */
  async startLogin(client: TestClient, scopes = ['openid', 'profile', 'email']): Promise<LoginAttempt> {
    const d = await this.discovery()
    const browser = new Browser()
    const pkce = createPkce()
    const authorize = new URL(d.authorization_endpoint)
    authorize.search = new URLSearchParams({
      client_id: client.clientId,
      response_type: 'code',
      redirect_uri: REDIRECT_URI,
      scope: scopes.join(' '),
      state: randomToken(),
      nonce: randomToken(),
      code_challenge: pkce.challenge,
      code_challenge_method: 'S256',
    }).toString()
    const loginScreen = await browser.redirectFrom(authorize.toString())
    return { client, scopes, browser, pkce, flow: this.loginFlowFrom(loginScreen) }
  }

  /** Submits a password on an open login screen and, if accepted, finishes the flow. */
  async continueLogin(attempt: LoginAttempt, user: TestUser): Promise<LoginOutcome> {
    const { browser, client, scopes, pkce, flow, remember } = attempt
    const loginRes = await this.submitPassword(browser, flow, user, remember)
    const loginBody = await loginRes.text()
    if (!loginRes.ok) return { kind: 'rejected', status: loginRes.status, body: loginBody }
    const { redirect_to: afterLogin } = JSON.parse(loginBody) as { redirect_to?: string }
    if (!afterLogin) return { kind: 'rejected', status: loginRes.status, body: loginBody }

    let next = await browser.redirectFrom(afterLogin)
    if (next.searchParams.has('error')) {
      // The provider ended the whole authorization request instead of answering the login screen.
      return { kind: 'rejected', status: loginRes.status, body: next.toString() }
    }
    if (!next.searchParams.has('code')) {
      const consentRes = await this.acceptConsent(browser, this.consentFlowFrom(next), scopes)
      const consentBody = await consentRes.text()
      if (!consentRes.ok) throw new Error(`Consent failed: ${consentRes.status} ${consentBody}`)
      const { redirect_to: afterConsent } = JSON.parse(consentBody) as { redirect_to: string }
      next = await browser.redirectFrom(afterConsent)
    }
    const code = next.searchParams.get('code')
    if (!code) throw new Error(`Authorization did not return a code: ${next}`)

    const d = await this.discovery()
    const tokenRes = await fetch(d.token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        code,
        redirect_uri: REDIRECT_URI,
        client_id: client.clientId,
        code_verifier: pkce.verifier,
      }),
    })
    if (!tokenRes.ok) throw new Error(`Token exchange failed: ${tokenRes.status} ${await tokenRes.text()}`)
    const tokens = (await tokenRes.json()) as Tokens
    const info = await this.userinfo(tokens.access_token)
    if (info.status !== 200) throw new Error(`userinfo rejected a fresh token: ${info.status}`)
    return { kind: 'code', code, tokens, sub: (info.body as { sub: string }).sub, browser }
  }

  /** Full authorization-code + PKCE login with password and consent. */
  async login(client: TestClient, user: TestUser, options: { scopes?: string[]; remember?: boolean } = {}): Promise<LoginOutcome> {
    const attempt = await this.startLogin(client, options.scopes)
    return this.continueLogin({ ...attempt, remember: options.remember }, user)
  }

  async userinfo(accessToken: string): Promise<{ status: number; body: unknown }> {
    const d = await this.discovery()
    const res = await fetch(d.userinfo_endpoint, { headers: { Authorization: `Bearer ${accessToken}` } })
    const text = await res.text()
    let body: unknown = text
    try {
      body = JSON.parse(text)
    } catch {
      // non-JSON error bodies are kept as text
    }
    return { status: res.status, body }
  }
}
