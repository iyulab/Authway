import type { ConformanceConfig } from './config.js'
import { Browser } from './browser.js'
import { conform } from './contract.js'
import { waitForLink } from './mail.js'
import { createPkce, randomToken, type Pkce } from './pkce.js'
import { type StandInIdentity, type StandInProvider } from './standin.js'

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
  /** A confidential client's secret, sent in the token request body (client_secret_post). */
  secret?: string
}

/** The grant a client exchanges a delegation with (RFC 8693). */
export const TOKEN_EXCHANGE_GRANT = 'urn:ietf:params:oauth:grant-type:token-exchange'

export interface TestUser {
  email: string
  password: string
}

/** A document the tenant asks its users to accept, as a sign-in shows it. */
export interface TermsDocument {
  kind: string
  version: string
  url: string
  title?: string
}

export interface Tokens {
  access_token: string
  id_token?: string
  /** Present when the client may use the refresh_token grant and the request asked for offline_access. */
  refresh_token?: string
  token_type: string
  scope?: string
}

/** An authorization request waiting at the login screen. */
export interface LoginAttempt {
  client: TestClient
  scopes: string[]
  browser: Browser
  pkce: Pkce
  flow: string
  /** The user ticks "keep me signed in": the session is to outlast the provider's usual one. */
  remember?: boolean
  /** The user ticks "remember this choice" on the consent screen. */
  rememberConsent?: boolean
}

/** The provider's answer to an authorization request that must not show the user anything (`prompt=none`). */
export type SilentOutcome =
  | { kind: 'code'; tokens: Tokens; sub: string }
  /** The OAuth error the application was handed: `login_required`, `consent_required`, … */
  | { kind: 'error'; error: string }

/** What a login attempt produced at the step where it stopped. */
export type LoginOutcome =
  | {
      kind: 'code'
      code: string
      tokens: Tokens
      sub: string
      browser: Browser
      /**
       * What the consent step did: 'asked' — the provider had the consent
       * screen put the question to the user; 'skipped' — it continued without
       * asking; 'none' — the authorization server never opened the screen.
       */
      consent: 'asked' | 'skipped' | 'none'
    }
  | { kind: 'rejected'; status: number; body: string }
  /**
   * The first factor passed and the user has a second one: the login screen
   * must ask for it before the flow goes on. `flow` is the flow the provider
   * named in its answer.
   */
  | { kind: 'second_factor'; attempt: LoginAttempt; flow: string; challenge: string }
  /**
   * Every factor passed and the user owes the tenant's documents: the login
   * screen shows `terms` and the user accepts or declines them, quoting
   * `challenge`.
   */
  | { kind: 'terms'; attempt: LoginAttempt; flow: string; challenge: string; terms: TermsDocument[] }

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

  /** Calls the admin API with the configured admin credential and tenant. */
  async admin(path: string, init: RequestInit = {}): Promise<Response> {
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
      await conform('GET', '/api/v1/tenants', res)
      if (!res.ok) throw new Error(`Listing tenants failed: ${res.status} ${await res.text()}`)
      const body = (await res.json()) as { tenants?: { id: string; slug?: string }[] } | { id: string }[]
      const tenants = Array.isArray(body) ? body : (body.tenants ?? [])
      if (tenants.length === 0) throw new Error('Provider has no tenant to provision into')
      this.tenant = (tenants.find((t) => 'slug' in t && t.slug === 'default') ?? tenants[0]).id
    }
    return this.tenant
  }

  /**
   * Registers a public client. `signInMethods` overrides the provider's
   * default sign-in methods; `skipConsent` registers it as an application
   * whose users are not asked for consent.
   */
  async createPublicClient(options: { signInMethods?: string[]; skipConsent?: boolean; scopes?: string[] } = {}): Promise<TestClient> {
    const res = await this.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        ...(options.signInMethods ? { enabled_auth_providers: options.signInMethods } : {}),
        ...(options.skipConsent ? { skip_consent: true } : {}),
        tenant_id: await this.tenantId(),
        name: `conformance-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        post_logout_redirect_uris: [POST_LOGOUT_REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: options.scopes ?? ['openid', 'profile', 'email'],
      }),
    })
    await conform('POST', '/api/v1/clients', res)
    const text = await res.text()
    if (!res.ok) throw new Error(`Creating client failed: ${res.status} ${text}`)
    const { client } = JSON.parse(text) as { client: { id: string; client_id: string } }
    return { id: client.id, clientId: client.client_id }
  }

  /** Exchanges a refresh token at the token endpoint (a public client: no secret). */
  async refresh(client: TestClient, refreshToken: string): Promise<{ status: number; tokens?: Tokens; error?: string }> {
    const d = await this.discovery()
    const res = await fetch(d.token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'refresh_token', refresh_token: refreshToken, client_id: client.clientId }),
    })
    const body = (await res.json()) as Tokens & { error?: string }
    return res.ok ? { status: res.status, tokens: body } : { status: res.status, error: body.error }
  }

  /** Registers a confidential machine-to-machine client, which holds a secret. */
  async createConfidentialClient(): Promise<TestClient> {
    const res = await this.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await this.tenantId(),
        name: `conformance-${randomToken(4)}`,
        public: false,
        grant_types: ['client_credentials'],
        scopes: ['openid'],
      }),
    })
    await conform('POST', '/api/v1/clients', res)
    const text = await res.text()
    if (!res.ok) throw new Error(`Creating client failed: ${res.status} ${text}`)
    const { client } = JSON.parse(text) as { client: { id: string; client_id: string } }
    return { id: client.id, clientId: client.client_id }
  }

  /**
   * Registers a confidential client that may act for users who delegate to it:
   * the token exchange grant and the delegation scope, beside a sign-in of its
   * own (authorization code).
   */
  async createDelegatingClient(options: { skipConsent?: boolean } = {}): Promise<TestClient> {
    const res = await this.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        ...(options.skipConsent ? { skip_consent: true } : {}),
        tenant_id: await this.tenantId(),
        name: `conformance-agent-${randomToken(4)}`,
        public: false,
        redirect_uris: [REDIRECT_URI],
        grant_types: ['authorization_code', TOKEN_EXCHANGE_GRANT],
        scopes: ['openid', 'profile', 'email', 'delegation'],
      }),
    })
    await conform('POST', '/api/v1/clients', res)
    const text = await res.text()
    if (!res.ok) throw new Error(`Creating client failed: ${res.status} ${text}`)
    const { client, credentials } = JSON.parse(text) as { client: { id: string; client_id: string }; credentials: { client_secret: string } }
    return { id: client.id, clientId: client.client_id, secret: credentials.client_secret }
  }

  async deleteClient(client: TestClient): Promise<void> {
    await conform('DELETE', '/api/v1/clients/{id}', await this.admin(`/api/v1/clients/${client.id}`, { method: 'DELETE' }))
  }

  /** Provisions a user the way a real one arrives: an admin invitation accepted from the mailed link. */
  async inviteAndAccept(): Promise<TestUser> {
    const email = `conformance-${randomToken(6)}@example.test`
    const password = `Cf-${randomToken(12)}`
    const invite = await this.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email }),
    })
    await conform('POST', '/api/v1/invitations', invite)
    if (!invite.ok) throw new Error(`Creating invitation failed: ${invite.status} ${await invite.text()}`)

    const link = await waitForLink(this.config.mailApi, email, '/invitation/accept')
    const token = link.searchParams.get('token')
    if (!token) throw new Error(`Invitation link carries no token: ${link}`)

    // The acceptance screen shows the invitation first, then accepts it.
    const shown = await fetch(`${this.config.api}/api/v1/invitations/token/${encodeURIComponent(token)}`)
    await conform('GET', '/api/v1/invitations/token/{token}', shown)
    if (!shown.ok) throw new Error(`Showing the invitation failed: ${shown.status} ${await shown.text()}`)

    const accept = await fetch(`${this.config.api}/api/v1/invitations/accept`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token, password, name: 'Conformance User' }),
    })
    await conform('POST', '/api/v1/invitations/accept', accept)
    if (!accept.ok) throw new Error(`Accepting invitation failed: ${accept.status} ${await accept.text()}`)
    return { email, password }
  }

  async deleteUser(sub: string): Promise<void> {
    await conform('DELETE', '/api/v1/users/{id}', await this.admin(`/api/v1/users/${sub}`, { method: 'DELETE' }))
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
   * Follows the authorization server's redirects to where they stop: a screen
   * of the login UI (the address carries a flow id), or the application, when
   * nothing is asked of the user.
   */
  async afterLogin(browser: Browser, redirectTo: string): Promise<URL> {
    return this.followUntil(browser, redirectTo, (u) => u.searchParams.has('flow') || u.searchParams.has('code') || u.searchParams.has('error'))
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
    const res = await browser.fetch(this.loginFlowUrl(flow, '/password'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user.email, password: user.password, remember }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/password', res)
    return res
  }

  async acceptConsent(browser: Browser, flow: string, scopes: string[], remember = false): Promise<Response> {
    const res = await browser.fetch(this.flowUrl('consent-flows', flow, '/accept'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ grant_scope: scopes, ...(remember ? { remember: true } : {}) }),
    })
    await conform('POST', '/api/v1/consent-flows/{flow}/accept', res)
    return res
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
    await conform('POST', '/api/v1/logout-flows/{flow}', res)
    const body = await res.text()
    if (!res.ok) throw new Error(`Completing the logout flow failed: ${res.status} ${body}`)
    const { next: step, redirect_to: redirectTo } = JSON.parse(body) as { next?: string; redirect_to?: string }
    if (step !== 'redirect' || !redirectTo) throw new Error(`Logout flow did not redirect: ${body}`)
    // Follow the authorization server's own hops until it hands the browser back to the client.
    return this.followUntil(browser, redirectTo, (u) => u.origin !== issuer)
  }

  async logout(accessToken: string): Promise<Response> {
    const res = await fetch(`${this.config.api}/api/v1/logout`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${accessToken}` },
    })
    await conform('POST', '/api/v1/logout', res)
    return res
  }

  // ---- authorization-code + PKCE login ------------------------------------

  /**
   * Starts an authorization-code + PKCE request and stops at the login screen.
   * Pass the browser of an earlier login to start in its session.
   */
  async startLogin(
    client: TestClient,
    scopes = ['openid', 'profile', 'email'],
    browser = new Browser(),
    /** Further parameters of the authorization request. */
    extra: Record<string, string> = {},
  ): Promise<LoginAttempt> {
    const pkce = createPkce()
    const loginScreen = await this.openLoginScreen(browser, await this.authorizeUrl(client, scopes, pkce, extra))
    return { client, scopes, browser, pkce, flow: this.loginFlowFrom(loginScreen) }
  }

  /**
   * Opens an authorization-code + PKCE request and follows it to where it
   * stops: a screen of the login UI (a flow id), or the application's redirect
   * with a code or an error.
   */
  async authorize(client: TestClient, scopes = ['openid', 'profile', 'email'], browser = new Browser()): Promise<URL> {
    return this.afterLogin(browser, await this.authorizeUrl(client, scopes, createPkce()))
  }

  /** The address of an authorization-code + PKCE request; `extra` adds request parameters. */
  private async authorizeUrl(client: TestClient, scopes: string[], pkce: Pkce, extra: Record<string, string> = {}): Promise<string> {
    const d = await this.discovery()
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
      ...extra,
    }).toString()
    return authorize.toString()
  }

  /** Submits a password on an open login screen and, if accepted, finishes the flow. */
  async continueLogin(attempt: LoginAttempt, user: TestUser): Promise<LoginOutcome> {
    const { browser, flow, remember } = attempt
    const loginRes = await this.submitPassword(browser, flow, user, remember)
    return this.afterFirstFactor(attempt, loginRes.status, await loginRes.text())
  }

  /** Reads the provider's answer to a first factor: rejected, a second factor owed, or on to the tokens. */
  private async afterFirstFactor(attempt: LoginAttempt, status: number, body: string): Promise<LoginOutcome> {
    if (status < 200 || status > 299) return { kind: 'rejected', status, body }
    const step = JSON.parse(body) as { next?: string; redirect_to?: string; mfa_challenge?: string; flow?: string }
    if (step.next === 'mfa' && step.mfa_challenge && step.flow) {
      return { kind: 'second_factor', attempt, flow: step.flow, challenge: step.mfa_challenge }
    }
    const owed = this.termsOwed(attempt, body)
    if (owed) return owed
    if (step.next !== 'redirect' || !step.redirect_to) return { kind: 'rejected', status, body }
    return this.finishAuthorization(attempt, step.redirect_to, status)
  }

  /**
   * Gives the second factor a sign-in owes — a code from the user's
   * authenticator, or one of their recovery codes — and, if it is accepted,
   * finishes the flow.
   */
  async continueWithSecondFactor(
    pending: Extract<LoginOutcome, { kind: 'second_factor' }>,
    code: string,
    kind: 'totp' | 'recovery' = 'totp',
  ): Promise<LoginOutcome> {
    const path = kind === 'totp' ? '/mfa' : '/mfa/recovery'
    const res = await pending.attempt.browser.fetch(this.loginFlowUrl(pending.flow, path), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mfa_challenge: pending.challenge, code }),
    })
    await conform('POST', `/api/v1/login-flows/{flow}${path}`, res)
    const body = await res.text()
    if (!res.ok) return { kind: 'rejected', status: res.status, body }
    const owed = this.termsOwed(pending.attempt, body)
    if (owed) return owed
    const { next, redirect_to: afterLogin } = JSON.parse(body) as { next?: string; redirect_to?: string }
    if (next !== 'redirect' || !afterLogin) return { kind: 'rejected', status: res.status, body }
    return this.finishAuthorization(pending.attempt, afterLogin, res.status)
  }

  /** The terms step, when `body` is a login-flow answer that asks for it. */
  termsOwed(attempt: LoginAttempt, body: string): Extract<LoginOutcome, { kind: 'terms' }> | undefined {
    const step = JSON.parse(body) as { next?: string; flow?: string; terms_challenge?: string; terms?: TermsDocument[] }
    if (step.next !== 'terms') return undefined
    if (!step.flow || !step.terms_challenge || !step.terms) throw new Error(`A terms step without flow, challenge or terms: ${body}`)
    return { kind: 'terms', attempt, flow: step.flow, challenge: step.terms_challenge, terms: step.terms }
  }

  /** Reads the documents a parked sign-in owes, as a screen reached by navigation does. */
  async owedTerms(attempt: LoginAttempt, flow: string, challenge: string): Promise<Response> {
    const res = await attempt.browser.fetch(`${this.loginFlowUrl(flow, '/terms')}?${new URLSearchParams({ terms_challenge: challenge })}`)
    await conform('GET', '/api/v1/login-flows/{flow}/terms', res.clone())
    return res
  }

  /**
   * Accepts the documents a sign-in owes — those it was shown, unless
   * `accepted` names others — and, if the acceptance holds, finishes the flow.
   */
  async acceptTerms(
    pending: Extract<LoginOutcome, { kind: 'terms' }>,
    accepted: { kind: string; version: string }[] = pending.terms.map(({ kind, version }) => ({ kind, version })),
  ): Promise<LoginOutcome> {
    const res = await pending.attempt.browser.fetch(this.loginFlowUrl(pending.flow, '/terms'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ terms_challenge: pending.challenge, accepted }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/terms', res.clone())
    const body = await res.text()
    if (!res.ok) return { kind: 'rejected', status: res.status, body }
    const { next, redirect_to: afterLogin } = JSON.parse(body) as { next?: string; redirect_to?: string }
    if (next !== 'redirect' || !afterLogin) return { kind: 'rejected', status: res.status, body }
    return this.finishAuthorization(pending.attempt, afterLogin, res.status)
  }

  /** Declines the documents a sign-in owes; the application is told how that ended. */
  async declineTerms(pending: Extract<LoginOutcome, { kind: 'terms' }>): Promise<LoginOutcome> {
    const res = await pending.attempt.browser.fetch(this.loginFlowUrl(pending.flow, '/terms/decline'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ terms_challenge: pending.challenge }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/terms/decline', res.clone())
    const body = await res.text()
    if (!res.ok) return { kind: 'rejected', status: res.status, body }
    const { redirect_to: afterLogin } = JSON.parse(body) as { redirect_to?: string }
    if (!afterLogin) return { kind: 'rejected', status: res.status, body }
    return this.finishAuthorization(pending.attempt, afterLogin, res.status)
  }

  /**
   * Signs in with an emailed link: asks the login flow to send one, reads it
   * from the mail capture, and redeems it in the browser that started the
   * sign-in.
   */
  /**
   * Asks the login flow to email a sign-in link and returns the link from the
   * mail — the newest one to `email`.
   */
  async sendMagicLink(attempt: LoginAttempt, email: string): Promise<URL> {
    const sendRes = await attempt.browser.fetch(this.loginFlowUrl(attempt.flow, '/magic-link'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/magic-link', sendRes)
    const sendBody = await sendRes.text()
    if (!sendRes.ok) throw new Error(`Sending the link was refused (${sendRes.status}): ${sendBody}`)
    if ((JSON.parse(sendBody) as { next?: string }).next !== 'email_sent') throw new Error(`Unexpected answer: ${sendBody}`)
    const link = await waitForLink(this.config.mailApi, email, '/magic-link', 15_000, (url) => {
      const token = url.searchParams.get('token')
      return token !== null && !this.usedLinks.has(token)
    })
    this.usedLinks.add(link.searchParams.get('token')!)
    return link
  }

  /** Links this run has already taken from the mailbox — each mail is read once. */
  private readonly usedLinks = new Set<string>()

  /** Types `code` — from a mail sendMagicLink fetched — into the sign-in that asked for it. */
  async continueWithMagicLinkCode(attempt: LoginAttempt, code: string): Promise<LoginOutcome> {
    const res = await attempt.browser.fetch(this.loginFlowUrl(attempt.flow, '/magic-link/code'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/magic-link/code', res.clone())
    return this.afterFirstFactor(attempt, res.status, await res.text())
  }

  async continueWithMagicLink(attempt: LoginAttempt, email: string): Promise<LoginOutcome> {
    const { browser } = attempt
    const link = await this.sendMagicLink(attempt, email)
    const token = link.searchParams.get('token')
    if (!token) throw new Error(`Sign-in link carries no token: ${link}`)
    const redeemRes = await browser.fetch(`${this.config.api}/api/v1/magic-links/redeem`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    })
    await conform('POST', '/api/v1/magic-links/redeem', redeemRes)
    return this.afterFirstFactor(attempt, redeemRes.status, await redeemRes.text())
  }

  /**
   * Signs in through another identity provider, played by the suite's
   * stand-in: starts the provider step of the login flow, and — as the user
   * who has just signed in at the provider — comes back with a code that
   * stands for `identity`.
   *
   * How a provider answers that return is its own business: a redirect, or a
   * page that navigates onward. Either leads to the application, to the login
   * UI's second factor step, or back to the application with an error.
   */
  async continueWithProvider(attempt: LoginAttempt, standIn: StandInProvider, identity: StandInIdentity): Promise<LoginOutcome> {
    const { browser, flow } = attempt
    const toProvider = await browser.redirectFrom(this.loginFlowUrl(flow, `/social/${this.config.idpStandInAs}`))
    if (toProvider.origin !== standIn.origin) {
      throw new Error(`The provider step went to ${toProvider.origin}${toProvider.pathname}, not to the stand-in at ${standIn.origin}`)
    }
    const state = toProvider.searchParams.get('state')
    const redirectUri = toProvider.searchParams.get('redirect_uri')
    if (!state || !redirectUri) throw new Error(`The provider step carries no state or redirect_uri: ${toProvider}`)

    const back = new URL(redirectUri)
    back.searchParams.set('code', standIn.codeFor(identity))
    back.searchParams.set('state', state)
    const res = await browser.fetch(back.toString())
    const next = await navigationFrom(res, back)

    if (next.pathname.endsWith('/mfa/verify')) {
      const owedFlow = next.searchParams.get('flow')
      const challenge = next.searchParams.get('mfa_challenge')
      if (!owedFlow || !challenge) throw new Error(`The second factor step was opened without a flow or challenge: ${next}`)
      const { auth_ui: loginUi } = (await (await fetch(`${this.config.api}/.well-known/authway-config`)).json()) as { auth_ui: string }
      if (next.origin !== new URL(loginUi).origin) throw new Error(`The second factor step is not on the login UI (${loginUi}): ${next}`)
      return { kind: 'second_factor', attempt, flow: owedFlow, challenge }
    }
    if (next.pathname.endsWith('/terms')) {
      const owedFlow = next.searchParams.get('flow')
      const challenge = next.searchParams.get('terms_challenge')
      if (!owedFlow || !challenge) throw new Error(`The terms step was opened without a flow or challenge: ${next}`)
      const res = await this.owedTerms(attempt, owedFlow, challenge)
      const body = await res.text()
      if (!res.ok) return { kind: 'rejected', status: res.status, body }
      return this.termsOwed(attempt, body) ?? { kind: 'rejected', status: res.status, body }
    }
    // Straight to an error screen: no one was signed in and the request could not be handed back.
    if (next.searchParams.has('error')) return { kind: 'rejected', status: res.status, body: next.toString() }
    // Otherwise on by way of the authorization server — which answers the
    // application with a code, or with the error of a refused sign-in.
    return this.finishAuthorization(attempt, next.toString(), res.status)
  }

  /** From the authorization server's answer to an accepted login to tokens: consent if asked, then the code exchange. */
  private async finishAuthorization(attempt: LoginAttempt, afterLogin: string, status: number): Promise<LoginOutcome> {
    // After login the authorization server either answers the client or opens the consent screen.
    return this.fromConsentOn(attempt, await this.afterLogin(attempt.browser, afterLogin), status)
  }

  /** Goes on from where the browser stands once the login step is behind it: the consent screen, or the application. */
  private async fromConsentOn(attempt: LoginAttempt, stop: URL, status: number): Promise<LoginOutcome> {
    const { browser, client, scopes, pkce } = attempt
    let next = stop
    if (next.searchParams.has('error')) {
      // The provider ended the whole authorization request instead of answering the login screen.
      return { kind: 'rejected', status, body: next.toString() }
    }
    let consent: 'asked' | 'skipped' | 'none' = 'none'
    if (!next.searchParams.has('code')) {
      // As the consent screen does: first ask the provider whether there is a
      // question to put to the user, and only then answer it.
      const flow = this.consentFlowFrom(next)
      const stepRes = await browser.fetch(this.flowUrl('consent-flows', flow))
      await conform('GET', '/api/v1/consent-flows/{flow}', stepRes.clone())
      const stepBody = await stepRes.text()
      if (!stepRes.ok) throw new Error(`Consent flow failed: ${stepRes.status} ${stepBody}`)
      const step = JSON.parse(stepBody) as { next: string; redirect_to?: string }
      let afterConsent: string
      if (step.next === 'redirect' && step.redirect_to) {
        consent = 'skipped'
        afterConsent = step.redirect_to
      } else {
        consent = 'asked'
        const consentRes = await this.acceptConsent(browser, flow, scopes, attempt.rememberConsent)
        const consentBody = await consentRes.text()
        if (!consentRes.ok) throw new Error(`Consent failed: ${consentRes.status} ${consentBody}`)
        afterConsent = (JSON.parse(consentBody) as { redirect_to: string }).redirect_to
      }
      next = await this.followUntil(browser, afterConsent, (u) => u.searchParams.has('code') || u.searchParams.has('error'))
      // The provider may end the request at the consent step instead of answering it.
      if (next.searchParams.has('error')) return { kind: 'rejected', status, body: next.toString() }
    }
    const code = next.searchParams.get('code')
    if (!code) throw new Error(`Authorization did not return a code: ${next}`)

    const tokens = await this.exchangeCode(client, pkce, code)
    const info = await this.userinfo(tokens.access_token)
    if (info.status !== 200) throw new Error(`userinfo rejected a fresh token: ${info.status}`)
    return { kind: 'code', code, tokens, sub: (info.body as { sub: string }).sub, browser, consent }
  }

  /**
   * A second authorization in a browser that already has a session (single
   * sign-on): the user is not asked to sign in. A provider may settle the
   * login step before any screen opens; if it opens the login screen, the
   * screen asks what to show, as it does for every flow, and is told to
   * continue without a form.
   */
  async reauthorize(browser: Browser, client: TestClient, scopes = ['openid', 'profile', 'email']): Promise<Tokens> {
    const reached = await this.pastTheLoginStep(browser, client, scopes)
    if (!reached) throw new Error('The session was not reused — the user was asked to sign in')
    const outcome = await this.fromConsentOn(reached.attempt, reached.stop, 200)
    if (outcome.kind !== 'code') throw new Error(`Reusing the session failed: ${JSON.stringify(outcome)}`)
    return outcome.tokens
  }

  /** Whether the provider still knows the browser: an authorization request there goes on without the sign-in form. */
  async isSignedIn(browser: Browser, client: TestClient): Promise<boolean> {
    return (await this.pastTheLoginStep(browser, client, ['openid'])) !== undefined
  }

  /**
   * Starts an authorization request in `browser` and takes it past the login
   * step if the provider lets it through without a form: returns where the
   * browser then stands — the consent screen or the application. Undefined
   * when the user would have to sign in.
   */
  async pastTheLoginStep(
    browser: Browser,
    client: TestClient,
    scopes = ['openid', 'profile', 'email'],
  ): Promise<{ attempt: LoginAttempt; stop: URL } | undefined> {
    const pkce = createPkce()
    const attempt: LoginAttempt = { client, scopes, browser, pkce, flow: '' }
    const stop = await this.afterLogin(browser, await this.authorizeUrl(client, scopes, pkce))
    if (!stop.searchParams.has('flow') || stop.pathname.endsWith('/consent')) return { attempt, stop }
    // The login screen was opened: it asks what to show.
    const flow = this.loginFlowFrom(stop)
    const res = await browser.fetch(this.loginFlowUrl(flow))
    await conform('GET', '/api/v1/login-flows/{flow}', res.clone())
    const step = (await res.json()) as { next?: string; redirect_to?: string }
    if (step.next !== 'redirect' || !step.redirect_to) return undefined
    return { attempt: { ...attempt, flow }, stop: await this.afterLogin(browser, step.redirect_to) }
  }

  /**
   * An authorization request that must not show the user anything
   * (`prompt=none`), in a browser that may or may not hold a session. The
   * provider answers the application either way: with a code, or with an
   * OAuth error. If it opens the login screen on the way, the screen asks what
   * to show — as it does for every flow — and must be told to go on; a form
   * here is a failure.
   */
  async silentAuthorize(browser: Browser, client: TestClient, scopes = ['openid', 'profile', 'email']): Promise<SilentOutcome> {
    const pkce = createPkce()
    const settled = (u: URL) => u.searchParams.has('flow') || u.searchParams.has('code') || u.searchParams.has('error')
    let next = await this.followUntil(browser, await this.authorizeUrl(client, scopes, pkce, { prompt: 'none' }), settled)
    for (let screens = 0; screens < 2 && next.searchParams.has('flow'); screens++) {
      // The login screen, or the consent screen after it.
      const kind = next.pathname.endsWith('/consent') ? 'consent-flows' : 'login-flows'
      const res = await browser.fetch(this.flowUrl(kind, next.searchParams.get('flow')!))
      await conform('GET', `/api/v1/${kind}/{flow}`, res.clone())
      const step = (await res.json()) as { next?: string; redirect_to?: string }
      if (step.next !== 'redirect' || !step.redirect_to) {
        throw new Error(`A request with prompt=none was shown a ${kind === 'login-flows' ? 'sign-in' : 'consent'} form: ${JSON.stringify(step)}`)
      }
      next = await this.followUntil(browser, step.redirect_to, settled)
    }
    const error = next.searchParams.get('error')
    if (error) return { kind: 'error', error }
    const code = next.searchParams.get('code')
    if (!code) throw new Error(`A request with prompt=none ended at neither a code nor an error: ${next}`)
    const tokens = await this.exchangeCode(client, pkce, code)
    const info = await this.userinfo(tokens.access_token)
    if (info.status !== 200) throw new Error(`userinfo rejected a fresh token: ${info.status}`)
    return { kind: 'code', tokens, sub: (info.body as { sub: string }).sub }
  }

  private async exchangeCode(client: TestClient, pkce: Pkce, code: string): Promise<Tokens> {
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
        ...(client.secret ? { client_secret: client.secret } : {}),
      }),
    })
    if (!tokenRes.ok) throw new Error(`Token exchange failed: ${tokenRes.status} ${await tokenRes.text()}`)
    return (await tokenRes.json()) as Tokens
  }

  /** Full authorization-code + PKCE login with password and consent. */
  async login(
    client: TestClient,
    user: TestUser,
    options: { scopes?: string[]; remember?: boolean; rememberConsent?: boolean } = {},
  ): Promise<LoginOutcome> {
    const attempt = await this.startLogin(client, options.scopes)
    return this.continueLogin({ ...attempt, remember: options.remember, rememberConsent: options.rememberConsent }, user)
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

/** Where a response sends the browser: a redirect's Location, or the address a returned page navigates to. */
async function navigationFrom(res: Response, from: URL): Promise<URL> {
  const location = res.headers.get('location')
  if (res.status >= 300 && res.status < 400 && location) return new URL(location, from)
  const page = await res.text()
  // The address is a script string literal: read it as the browser does, escapes and all.
  const scripted = /window\.location(?:\.href)?\s*=\s*("(?:[^"\\]|\\.)*")/.exec(page)
  const refreshed = /http-equiv=["']refresh["'][^>]*content=["'][^"']*url=([^"']+)["']/i.exec(page)
  const target = (scripted ? (JSON.parse(scripted[1]) as string) : undefined) ?? refreshed?.[1]
  if (!target) throw new Error(`Expected a redirect or a page that navigates onward from ${from}, got ${res.status}: ${page.slice(0, 300)}`)
  return new URL(target, from)
}
