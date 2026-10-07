/**
 * Target provider configuration, read from the environment.
 *
 * The suite is black-box: it only knows the provider's public surface (the OIDC
 * issuer, the login-UI backend and the admin API) plus an SMTP capture service
 * that receives the provider's outgoing mail. Nothing here reaches into a
 * database or depends on the implementation language of the provider.
 */
export interface ConformanceConfig {
  /** OIDC issuer — discovery, authorize, token, userinfo. */
  issuer: string
  /** Backend the login UI talks to (login, consent, logout, invitations). */
  api: string
  /** Admin credential for provisioning test clients and invitations. */
  adminKey: string
  /** MailHog-compatible HTTP API that captured the provider's mail. */
  mailApi: string
  /** Tenant to provision into; resolved from the admin API when omitted. */
  tenantId?: string
}

const REQUIRED = {
  issuer: 'CONFORMANCE_ISSUER',
  api: 'CONFORMANCE_API',
  adminKey: 'CONFORMANCE_ADMIN_KEY',
  mailApi: 'CONFORMANCE_MAIL_API',
} as const

export function loadConfig(env: NodeJS.ProcessEnv = process.env): ConformanceConfig {
  const missing = Object.values(REQUIRED).filter((name) => !env[name])
  if (missing.length > 0) {
    // Fail loudly instead of skipping: a suite that silently skips reports
    // green while having verified nothing.
    throw new Error(
      `Conformance target is not configured. Set: ${missing.join(', ')}. See packages/conformance/README.md.`,
    )
  }
  const trim = (v: string) => v.replace(/\/+$/, '')
  return {
    issuer: trim(env[REQUIRED.issuer]!),
    api: trim(env[REQUIRED.api]!),
    adminKey: env[REQUIRED.adminKey]!,
    mailApi: trim(env[REQUIRED.mailApi]!),
    tenantId: env.CONFORMANCE_TENANT_ID || undefined,
  }
}
