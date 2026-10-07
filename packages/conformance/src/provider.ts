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
 * admin API. Login-flow answers carry `next`: "form", "redirect" (with
 * `redirect_to`) or "mfa" (with `mfa_challenge`).
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

  /** Registers a public client; `signInMethods` overrides the provider's default sign-in methods. */
  async createPublicClient(options: { signInMethods?: string[] } = {}): Promise<TestClient> {
    const res = await this.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        ...(options.signInMethods ? { enabled_auth_providers: options.signInMethods } : {}),
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

  /** Extracts the opaque flow id from the login screen's address. */
  loginFlowFrom(location: URL): string {
    const flow = location.searchParams.get('flow')
    if (!flow) throw new Error(`The login screen was opened without a flow id: ${location}`)
    return flow
  }

  /**
   * Follows the authorization server's redirects to the login screen. A
   * provider may pass through its own backend first; the screen is the first
   * address that carries a flow id.
   */
  async openLoginScreen(browser: Browser, authorizeUrl: string): Promise<URL> {
    return this.followUntil(browser, authorizeUrl, (u) => u.searchParams.has('flow'))
  }

  /**
   * Follows redirects from `url` until `done` holds for the address reached —
   * a screen of the login UI (it carries a flow id) or the client's redirect.
   */
  private async followUntil(browser: Browser, url: string, done: (u: URL) => boolean): Promise<URL> {
    let next = await browser.redirectFrom(url)
    for (let hops = 0; hops < 5 && !done(next); hops++) {
      next = await browser.redirectFrom(next.toString())
    }
    return next
  }

  /** URL of a login-flow endpoint of the login UI's backend. */
  loginFlowUrl(flow: string, path = ''): string {
    return this.flowUrl('login-flows', flow, path)
  }

  flowUrl(kind: 'login-flows' | 'consent-flows' | 'logout-flows', flow: string, path = ''): string {
    return `${this.config.api}/api/v1/${kind}/${encodeURIComponent(flow)}${path}`
  }

  consentFlowFrom(location: URL): string {
    const flow = location.searchParams.get('flow')
    if (!flow) throw new Error(`The consent screen was opened without a flow id: ${location}`)
    return flow
  }

  async submitPassword(browser: Browser, flow: string, user: TestUser, remember = false): Promise<Response> {
    return browser.fetch(this.loginFlowUrl(flow, '/password'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user.email, password: user.password, remember }),
    })
  }

  async acceptConsent(browser: Browser, flow: string, scopes: string[]): Promise<Response> {
    return browser.fetch(this.flowUrl('consent-flows', flow, '/accept'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ grant_scope: scopes }),
    })
  }

  /**
   * RP-initiated logout (OIDC RP-Initiated Logout 1.0) through the logout
   * screen's backend, in the browser that holds the login session. The screen
   * only knows its flow id, as the real one does. Returns where the browser
   * finally lands.
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
    const issuer = new URL(this.config.issuer).origin
    const api = new URL(this.config.api).origin
    // Through the authorization server and the provider's backend to the logout screen.
    const logoutScreen = await this.followUntil(
      browser,
      start.toString(),
      (u) => u.searchParams.has('flow') || (u.origin !== issuer && u.origin !== api),
    )
    const flow = logoutScreen.searchParams.get('flow')
    if (!flow) throw new Error(`The logout screen was opened without a flow id: ${logoutScreen}`)
    const res = await browser.fetch(this.flowUrl('logout-flows', flow), { method: 'POST' })
    const body = await res.text()
    if (!res.ok) throw new Error(`Completing the logout flow failed: ${res.status} ${body}`)
    const { next: step, redirect_to: redirectTo } = JSON.parse(body) as { next?: string; redirect_to?: string }
    if (step !== 'redirect' || !redirectTo) throw new Error(`Logout flow did not redirect: ${body}`)
    // Follow the authorization server's own hops until it hands the browser back to the client.
    return this.followUntil(browser, redirectTo, (u) => u.origin !== issuer)
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
    const loginScreen = await this.openLoginScreen(browser, authorize.toString())
    return { client, scopes, browser, pkce, flow: this.loginFlowFrom(loginScreen) }
  }

  /** Submits a password on an open login screen and, if accepted, finishes the flow. */
  async continueLogin(attempt: LoginAttempt, user: TestUser): Promise<LoginOutcome> {
    const { browser, flow, remember } = attempt
    const loginRes = await this.submitPassword(browser, flow, user, remember)
    const loginBody = await loginRes.text()
    if (!loginRes.ok) return { kind: 'rejected', status: loginRes.status, body: loginBody }
    const { next: step, redirect_to: afterLogin } = JSON.parse(loginBody) as { next?: string; redirect_to?: string }
    if (step !== 'redirect' || !afterLogin) return { kind: 'rejected', status: loginRes.status, body: loginBody }
    return this.finishAuthorization(attempt, afterLogin, loginRes.status)
  }

  /**
   * Signs in with an emailed link: asks the login flow to send one, reads it
   * from the mail capture, and redeems it in the browser that started the
   * sign-in.
   */
  async continueWithMagicLink(attempt: LoginAttempt, email: string): Promise<LoginOutcome> {
    const { browser, flow } = attempt
    const sendRes = await browser.fetch(this.loginFlowUrl(flow, '/magic-link'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    })
    const sendBody = await sendRes.text()
    if (!sendRes.ok) return { kind: 'rejected', status: sendRes.status, body: sendBody }
    if ((JSON.parse(sendBody) as { next?: string }).next !== 'email_sent') throw new Error(`Unexpected answer: ${sendBody}`)

    const link = await waitForLink(this.config.mailApi, email, '/magic-link')
    const token = link.searchParams.get('token')
    if (!token) throw new Error(`Sign-in link carries no token: ${link}`)
    const redeemRes = await browser.fetch(`${this.config.api}/api/v1/magic-links/redeem`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    })
    const redeemBody = await redeemRes.text()
    if (!redeemRes.ok) return { kind: 'rejected', status: redeemRes.status, body: redeemBody }
    const { next: step, redirect_to: afterLogin } = JSON.parse(redeemBody) as { next?: string; redirect_to?: string }
    if (step !== 'redirect' || !afterLogin) return { kind: 'rejected', status: redeemRes.status, body: redeemBody }
    return this.finishAuthorization(attempt, afterLogin, redeemRes.status)
  }

  /** From the authorization server's answer to an accepted login to tokens: consent if asked, then the code exchange. */
  private async finishAuthorization(attempt: LoginAttempt, afterLogin: string, status: number): Promise<LoginOutcome> {
    const { browser, client, scopes, pkce } = attempt

    // After login the authorization server either answers the client or opens the consent screen.
    let next = await this.followUntil(browser, afterLogin, (u) => u.searchParams.has('flow') || u.searchParams.has('code') || u.searchParams.has('error'))
    if (next.searchParams.has('error')) {
      // The provider ended the whole authorization request instead of answering the login screen.
      return { kind: 'rejected', status, body: next.toString() }
    }
    if (!next.searchParams.has('code')) {
      const consentRes = await this.acceptConsent(browser, this.consentFlowFrom(next), scopes)
      const consentBody = await consentRes.text()
      if (!consentRes.ok) throw new Error(`Consent failed: ${consentRes.status} ${consentBody}`)
      const { redirect_to: afterConsent } = JSON.parse(consentBody) as { redirect_to: string }
      next = await this.followUntil(browser, afterConsent, (u) => u.searchParams.has('code') || u.searchParams.has('error'))
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
