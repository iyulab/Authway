import React, { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Input, Textarea, Select, Checkbox, CheckboxGroup, Button } from '@/components/ui'
import { useQuery } from '@tanstack/react-query'
import { Client, ServerRefusal, capabilitiesApi, refusalMessage } from '@/lib/api'

/** The non-empty lines of a one-per-line field, trimmed. */
export function lines(value?: string): string[] {
  return (value ?? '')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

/**
 * Whether any redirect URI returns the user to a web page — an https URL, or
 * http to a named host. A native app is called back on a loopback IP literal
 * or a private-use URI scheme and has no browser origin to allow. Mirrors the
 * server rule; `localhost` counts as a web page there too.
 */
export function usesBrowserRedirect(redirectUris: string): boolean {
  return lines(redirectUris).some((uri) => {
      const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(uri)?.[1].toLowerCase()
      if (scheme === 'https') return true
      if (scheme !== 'http') return scheme === undefined
      const host = /^http:\/\/(\[[^\]]*\]|[^/:?#]*)/i.exec(uri)?.[1] ?? ''
      return !(host === '[::1]' || /^127(\.\d{1,3}){3}$/.test(host))
    })
}

/** An absolute https URL without a fragment — what the server accepts as client_uri, initiate_login_uri and logo. */
export function isApplicationAddress(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && url.host !== '' && url.hash === '' && !/\s/.test(value)
  } catch {
    return false
  }
}

// Client form validation schema
export const clientFormSchema = z.object({
  name: z.string().min(1, 'Client name is required'),
  description: z.string().optional(),
  // Addresses the application publishes for users — see the superRefine below.
  client_uri: z.string().optional(),
  initiate_login_uri: z.string().optional(),
  logo: z.string().optional(),
  // Required only for redirect-based grants — see the superRefine below. A
  // machine-to-machine client (client_credentials only) never redirects, and
  // forcing a dummy URI here pollutes its logout configuration.
  redirect_uris: z.string(),
  post_logout_redirect_uris: z.string().optional(),
  // Required only for a public client using authorization_code — see the
  // superRefine below, mirroring the server rule
  // (public_client_missing_allowed_origins).
  allowed_origins: z.string().optional(),
  // One identifier per line — see the superRefine below.
  audience: z.string().optional(),
  // A single address; the server decides whether it may be called.
  backchannel_logout_uri: z.string().optional(),
  grant_types: z.array(z.string()).min(1, 'At least one Grant Type is required'),
  scopes: z.array(z.string()).min(1, 'At least one Scope is required'),
  public: z.boolean(),
  // Authentication Provider Settings
  // A client nobody can sign in to is refused by the API (invalid_enabled_auth_providers).
  enabled_auth_providers: z.array(z.string()).min(1, 'Choose at least one way to sign in').optional(),
  allow_email_login: z.boolean().optional(),
  // Consent Flow Configuration
  skip_consent: z.boolean().optional(),
  skip_logout_consent: z.boolean().optional(),
  // '' = inherit the deployment-wide strategy.
  access_token_strategy: z.enum(['', 'jwt', 'opaque']).optional(),
}).superRefine((data, ctx) => {
  // Mirrors the server rule (validateRedirectURIRequirement): redirect_uris is
  // mandatory exactly for the grants that redirect the user-agent back.
  const redirectsUserAgent = data.grant_types.includes('authorization_code')
  if (redirectsUserAgent && data.redirect_uris.trim() === '') {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['redirect_uris'],
      message: 'At least one Redirect URI is required for Authorization Code',
    })
  }

  // Mirrors the server rule (public_client_missing_allowed_origins): a public
  // client using authorization_code needs at least one allowed origin, or the
  // reverse proxy has nothing to validate a browser's cross-origin token
  // request against.
  const needsAllowedOrigins =
    data.public && data.grant_types.includes('authorization_code') && usesBrowserRedirect(data.redirect_uris)
  if (needsAllowedOrigins && (data.allowed_origins ?? '').trim() === '') {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['allowed_origins'],
      message: 'Public clients with Authorization Code need at least one Allowed Origin for browser CORS',
    })
  }

  // Mirrors the server rule (invalid_audience): each identifier is distinct,
  // has no whitespace and is at most 255 characters.
  const audiences = lines(data.audience)
  if (audiences.some((a) => /\s/.test(a) || a.length > 255) || new Set(audiences).size !== audiences.length) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['audience'],
      message: 'Each audience must be a distinct identifier without spaces, at most 255 characters',
    })
  }

  // Mirrors the server rule (invalid_client_uri, invalid_initiate_login_uri,
  // invalid_logo): users are sent to or shown these addresses, so only an
  // absolute https URL without a fragment is accepted.
  for (const field of ['client_uri', 'initiate_login_uri', 'logo'] as const) {
    const address = (data[field] ?? '').trim()
    if (address !== '' && !isApplicationAddress(address)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: [field],
        message: 'Enter one absolute https URL without a fragment (#…)',
      })
    }
  }

  // The shape only (invalid_backchannel_logout_uri). Whether the address may
  // be called — https, not a local or private address — is the server's to
  // say: it knows what the deployment allows and what the name resolves to.
  const logoutAddress = (data.backchannel_logout_uri ?? '').trim()
  if (logoutAddress !== '') {
    let parsed: URL | undefined
    try {
      parsed = new URL(logoutAddress)
    } catch {
      parsed = undefined
    }
    if (!parsed || !['https:', 'http:'].includes(parsed.protocol) || parsed.hash !== '' || /\s/.test(logoutAddress)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['backchannel_logout_uri'],
        message: 'Enter one absolute https URL without a fragment (#…)',
      })
    }
  }
})

export type ClientFormData = z.infer<typeof clientFormSchema>

export const AVAILABLE_GRANT_TYPES = [
  { value: 'authorization_code', label: 'Authorization Code' },
  { value: 'client_credentials', label: 'Client Credentials' },
  { value: 'refresh_token', label: 'Refresh Token' },
]

export const AVAILABLE_SCOPES = [
  { value: 'openid', label: 'OpenID Connect' },
  { value: 'profile', label: 'Profile' },
  { value: 'email', label: 'Email' },
  { value: 'offline_access', label: 'Offline Access' },
]

export const ACCESS_TOKEN_STRATEGIES = [
  { value: '', label: 'Default (inherit server setting)' },
  { value: 'opaque', label: 'Opaque - reference token, revocable' },
  { value: 'jwt', label: 'JWT - self-contained, offline validation' },
]

export const AVAILABLE_AUTH_PROVIDERS = [
  { value: 'email', label: 'Email/Password' },
  { value: 'magic_link', label: 'Email link (passwordless)' },
  { value: 'google', label: 'Google' },
  { value: 'github', label: 'GitHub' },
  { value: 'microsoft', label: 'Microsoft' },
  { value: 'apple', label: 'Apple' },
]

export interface ClientFormProps {
  initialData?: Client
  /** The server's refusal of the last submission; shown at the field it names. */
  serverError?: ServerRefusal | null
  onSubmit: (data: ClientFormData) => void
  onCancel: () => void
  isSubmitting?: boolean
  submitLabel?: string
}

export const ClientForm: React.FC<ClientFormProps> = ({
  initialData,
  serverError,
  onSubmit,
  onCancel,
  isSubmitting = false,
  submitLabel = 'Submit',
}) => {
  const {
    register,
    handleSubmit,
    formState: { errors },
    setValue,
    setError,
    watch,
  } = useForm<ClientFormData>({
    resolver: zodResolver(clientFormSchema),
    defaultValues: initialData
      ? {
          name: initialData.name,
          description: initialData.description || '',
          client_uri: initialData.client_uri || '',
          logo: initialData.logo || '',
          initiate_login_uri: initialData.initiate_login_uri || '',
          // M2M clients legitimately have no redirect_uris — the API may omit the field.
          redirect_uris: (initialData.redirect_uris || []).join('\n'),
          post_logout_redirect_uris: (initialData.post_logout_redirect_uris || []).join('\n'),
          allowed_origins: (initialData.allowed_origins || []).join('\n'),
          audience: (initialData.audience || []).join('\n'),
          backchannel_logout_uri: initialData.backchannel_logout_uri || '',
          grant_types: initialData.grant_types,
          scopes: initialData.scopes,
          public: initialData.public,
          enabled_auth_providers: initialData.enabled_auth_providers || ['email', 'google'],
          allow_email_login: initialData.allow_email_login ?? true,
          access_token_strategy: initialData.access_token_strategy || '',
          skip_consent: initialData.skip_consent || false,
          skip_logout_consent: initialData.skip_logout_consent || false,
        }
      : {
          grant_types: ['authorization_code'],
          scopes: ['openid'],
          public: false,
          enabled_auth_providers: ['email', 'google'],
          allow_email_login: true,
          skip_consent: false,
          skip_logout_consent: false,
          access_token_strategy: '',
        },
  })

  // A refusal that names one of this form's fields is shown at that field.
  useEffect(() => {
    const field = serverError?.field
    if (!field || !(field in clientFormSchema.shape)) return
    setError(field as keyof ClientFormData, { type: 'server', message: refusalMessage(serverError) }, { shouldFocus: true })
  }, [serverError, setError])

  const grantTypes = watch('grant_types') || []
  const scopes = watch('scopes') || []
  const enabledAuthProviders = watch('enabled_auth_providers') || []
  const isPublic = watch('public')
  const redirectsUserAgent = grantTypes.includes('authorization_code')
  const needsAllowedOrigins =
    isPublic && grantTypes.includes('authorization_code') && usesBrowserRedirect(watch('redirect_uris') || '')

  const handleGrantTypeChange = (values: string[]) => {
    setValue('grant_types', values, { shouldValidate: true })
  }

  const handleScopeChange = (values: string[]) => {
    setValue('scopes', values, { shouldValidate: true })
  }

  // A social provider is only worth enabling if someone has credentials for
  // it: this deployment, or the client itself (its own Google/GitHub app).
  // Until capabilities load — or if they cannot — nothing is disabled; the
  // server refuses an unusable provider at sign-in either way.
  const { data: capabilities } = useQuery({
    queryKey: ['capabilities'],
    queryFn: async () => (await capabilitiesApi.get()).data,
    staleTime: 5 * 60 * 1000,
  })
  const clientHasOwnCredentials: Record<string, boolean> = {
    google: !!initialData?.google_oauth_enabled,
    github: !!initialData?.github_oauth_enabled,
  }
  const authProviderOptions = AVAILABLE_AUTH_PROVIDERS.map((option) => {
    const unusable =
      capabilities !== undefined &&
      (option.value === 'magic_link'
        ? !capabilities.magic_link
        : option.value !== 'email' &&
          !capabilities.providers.includes(option.value) &&
          !clientHasOwnCredentials[option.value])
    return unusable
      ? {
          ...option,
          description: 'Not configured on this server',
          // Leave a selected one clickable so it can be turned off.
          disabled: !enabledAuthProviders.includes(option.value),
        }
      : option
  })

  const handleAuthProviderChange = (values: string[]) => {
    setValue('enabled_auth_providers', values, { shouldValidate: true })
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      {/* Basic Information */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Input
          {...register('name')}
          label="Client Name *"
          placeholder="My Application"
          error={errors.name?.message}
        />
        <Input
          {...register('client_uri')}
          label="Home Page"
          placeholder="https://app.example.com"
          helperText="Where users go back to from Authway's screens (the account screen, an accepted invitation when there is no sign-in start URL). https only."
          error={errors.client_uri?.message}
        />
      </div>

      <Input
        {...register('logo')}
        label="Logo URL (Optional)"
        placeholder="https://app.example.com/logo.png"
        helperText="Shown with the application's name on the sign-in and consent screens. https only."
        error={errors.logo?.message}
      />

      <Textarea
        {...register('description')}
        label="Description"
        placeholder="Enter a description for the client"
        rows={2}
      />

      <Input
        {...register('initiate_login_uri')}
        label="Sign-In Start URL (Optional)"
        placeholder="https://app.example.com/login"
        helperText="Where the application starts a sign-in. A user who accepts an invitation to this application is sent here with iss and login_hint, and the application sends them on to Authway. https only. Empty: they go to the home page."
        error={errors.initiate_login_uri?.message}
      />

      {/* Redirect URIs */}
      <Textarea
        {...register('redirect_uris')}
        label={redirectsUserAgent ? 'Redirect URIs *' : 'Redirect URIs'}
        placeholder={`http://localhost:3000/callback\nhttps://example.com/callback`}
        rows={3}
        helperText={
          redirectsUserAgent
            ? 'Enter each URI on a new line.'
            : 'Not used by machine-to-machine clients — leave empty unless you add a redirect-based grant.'
        }
        error={errors.redirect_uris?.message}
      />

      {/* Post-Logout Redirect URIs */}
      <Textarea
        {...register('post_logout_redirect_uris')}
        label="Post-Logout Redirect URIs (Optional)"
        placeholder={`http://localhost:3000\nhttps://example.com`}
        rows={3}
        helperText="Enter each URI on a new line. Sign-out may only return to one of these addresses (exact match). If empty, Redirect URIs will be used."
        error={errors.post_logout_redirect_uris?.message}
      />

      {/* Allowed Origins (CORS) */}
      <Textarea
        {...register('allowed_origins')}
        label={needsAllowedOrigins ? 'Allowed Origins (CORS) *' : 'Allowed Origins (CORS)'}
        placeholder={`http://localhost:3000\nhttps://example.com`}
        rows={2}
        helperText={
          needsAllowedOrigins
            ? 'Enter each browser origin on a new line (scheme + host + port, no path). Required for a public client using Authorization Code, so the reverse proxy can validate cross-origin token requests.'
            : 'Enter each browser origin on a new line. Only needed for a public client (SPA) using Authorization Code — a native app called back on http://127.0.0.1/… or its own URI scheme needs none.'
        }
        error={errors.allowed_origins?.message}
      />

      {/* API audiences */}
      <Textarea
        {...register('audience')}
        label="API Audiences (Optional)"
        placeholder={`https://api.example.com`}
        rows={2}
        helperText="Enter the identifier of each API this client's access tokens are for, one per line (usually the API's URL). They become the token's aud claim, which the API checks for its own identifier. Empty: the tokens name no API."
        error={errors.audience?.message}
      />

      {/* Back-channel logout */}
      <Input
        {...register('backchannel_logout_uri')}
        label="Back-Channel Logout URL (Optional)"
        placeholder="https://app.example.com/backchannel-logout"
        helperText="For an application that keeps its own server-side session: a logout token is POSTed here whenever a session of one of its users ends. Must be a public https address — the call is made from the server, not from a browser. Empty: the application is not told."
        error={errors.backchannel_logout_uri?.message}
      />

      {/* Consent Flow */}
      <Checkbox
        {...register('skip_consent')}
        label="Skip Consent Screen"
        description="Bypass the OAuth consent screen on login. Enable only for first-party / trusted clients you control — never for third-party apps."
      />
      <Checkbox
        {...register('skip_logout_consent')}
        label="Skip Logout Confirmation"
        description="Bypass the logout confirmation screen. Recommended together with Skip Consent Screen for first-party clients."
      />

      {/* Access Token Format */}
      <Select
        {...register('access_token_strategy')}
        label="Access Token Format"
        options={ACCESS_TOKEN_STRATEGIES}
        helperText="JWT lets a resource server validate this client's tokens offline via OIDC discovery (JWKS) — but a resource server that validates it offline keeps accepting it until it expires, even after the user signs out or is suspended. Leave on Default unless an API needs offline validation."
        error={errors.access_token_strategy?.message}
      />

      {/* Grant Types */}
      <CheckboxGroup
        label="Grant Types *"
        options={AVAILABLE_GRANT_TYPES}
        value={grantTypes}
        onChange={handleGrantTypeChange}
        error={errors.grant_types?.message}
        required
      />

      {/* Scopes */}
      <CheckboxGroup
        label="Scopes *"
        options={AVAILABLE_SCOPES}
        value={scopes}
        onChange={handleScopeChange}
        error={errors.scopes?.message}
        required
      />

      {/* Public Client */}
      <Checkbox
        {...register('public')}
        label="Public Client (No Client Secret)"
        description="Select for SPAs or mobile apps that cannot securely store a Client Secret."
      />

      {/* Authentication Provider Settings */}
      <div className="border-t border-gray-200 pt-4 mt-4">
        <h3 className="text-lg font-medium text-gray-900 mb-4">Authentication Provider Settings</h3>

        <CheckboxGroup
          label="Enabled Authentication Providers"
          options={authProviderOptions}
          value={enabledAuthProviders}
          onChange={handleAuthProviderChange}
          error={errors.enabled_auth_providers?.message}
        />

        <div className="mt-4 space-y-3">
          <Checkbox
            {...register('allow_email_login')}
            label="Allow Email/Password Login"
            description="Allow existing users to login with email and password."
          />
        </div>
      </div>

      {/* Buttons */}
      <div className="flex justify-end space-x-3 pt-4">
        <Button type="button" variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" isLoading={isSubmitting}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}

export default ClientForm
