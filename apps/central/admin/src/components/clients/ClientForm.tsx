import React, { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
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
  name: z.string().min(1, { error: () => i18n.t('clients:form.validation.nameRequired') }),
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
  grant_types: z.array(z.string()).min(1, { error: () => i18n.t('clients:form.validation.grantRequired') }),
  scopes: z.array(z.string()).min(1, { error: () => i18n.t('clients:form.validation.scopeRequired') }),
  public: z.boolean(),
  // Authentication Provider Settings
  // A client nobody can sign in to is refused by the API (invalid_enabled_auth_providers).
  enabled_auth_providers: z.array(z.string()).min(1, { error: () => i18n.t('clients:form.validation.providerRequired') }).optional(),
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
      message: i18n.t('clients:form.validation.redirectRequired'),
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
      message: i18n.t('clients:form.validation.originsRequired'),
    })
  }

  // Mirrors the server rule (invalid_audience): each identifier is distinct,
  // has no whitespace and is at most 255 characters.
  const audiences = lines(data.audience)
  if (audiences.some((a) => /\s/.test(a) || a.length > 255) || new Set(audiences).size !== audiences.length) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['audience'],
      message: i18n.t('clients:form.validation.audience'),
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
        message: i18n.t('clients:form.validation.httpsUrl'),
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
        message: i18n.t('clients:form.validation.httpsUrl'),
      })
    }
  }
})

export type ClientFormData = z.infer<typeof clientFormSchema>

// The options a form offers; each one's words are `clients:form.<group>.<value>`.
export const AVAILABLE_GRANT_TYPES = [
  { value: 'authorization_code' },
  { value: 'client_credentials' },
  { value: 'refresh_token' },
]

export const AVAILABLE_SCOPES = [
  { value: 'openid' },
  { value: 'profile' },
  { value: 'email' },
  { value: 'offline_access' },
]

export const ACCESS_TOKEN_STRATEGIES = [
  { value: '', key: 'default' },
  { value: 'opaque', key: 'opaque' },
  { value: 'jwt', key: 'jwt' },
]

export const AVAILABLE_AUTH_PROVIDERS = [
  { value: 'email' },
  { value: 'magic_link' },
  { value: 'passkey' },
  { value: 'google' },
  { value: 'github' },
  { value: 'microsoft' },
  { value: 'apple' },
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
  submitLabel,
}) => {
  const { t } = useTranslation('clients')
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
          access_token_strategy: initialData.access_token_strategy || '',
          skip_consent: initialData.skip_consent || false,
          skip_logout_consent: initialData.skip_logout_consent || false,
        }
      : {
          grant_types: ['authorization_code'],
          scopes: ['openid'],
          public: false,
          enabled_auth_providers: ['email', 'google'],
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
  // it: this deployment, or the client itself (its own provider app — an
  // enabled one always has its credentials).
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
    microsoft: !!initialData?.microsoft_oauth_enabled,
    apple: !!initialData?.apple_oauth_enabled,
  }
  const authProviderOptions = AVAILABLE_AUTH_PROVIDERS.map((provider) => {
    const option = { value: provider.value, label: t(`form.providers.${provider.value}`) }
    const unusable =
      capabilities !== undefined &&
      (option.value === 'magic_link'
        ? !capabilities.magic_link
        : option.value === 'passkey'
          ? capabilities.passkeys !== true
          : option.value !== 'email' &&
          !capabilities.providers.includes(option.value) &&
          !clientHasOwnCredentials[option.value])
    return unusable
      ? {
          ...option,
          description: t('form.providers.notConfigured'),
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
          label={t('form.name')}
          placeholder={t('form.namePlaceholder')}
          error={errors.name?.message}
        />
        <Input
          {...register('client_uri')}
          label={t('form.homePage')}
          placeholder={t('form.homePagePlaceholder')}
          helperText={t('form.homePageHelp')}
          error={errors.client_uri?.message}
        />
      </div>

      <Input
        {...register('logo')}
        label={t('form.logo')}
        placeholder={t('form.logoPlaceholder')}
        helperText={t('form.logoHelp')}
        error={errors.logo?.message}
      />

      <Textarea
        {...register('description')}
        label={t('form.description')}
        placeholder={t('form.descriptionPlaceholder')}
        rows={2}
      />

      <Input
        {...register('initiate_login_uri')}
        label={t('form.initiateLogin')}
        placeholder={t('form.initiateLoginPlaceholder')}
        helperText={t('form.initiateLoginHelp')}
        error={errors.initiate_login_uri?.message}
      />

      {/* Redirect URIs */}
      <Textarea
        {...register('redirect_uris')}
        label={redirectsUserAgent ? t('form.redirectUrisRequired') : t('form.redirectUris')}
        placeholder={t('form.redirectUrisPlaceholder')}
        rows={3}
        helperText={redirectsUserAgent ? t('form.redirectUrisHelp') : t('form.redirectUrisHelpM2m')}
        error={errors.redirect_uris?.message}
      />

      {/* Post-Logout Redirect URIs */}
      <Textarea
        {...register('post_logout_redirect_uris')}
        label={t('form.postLogout')}
        placeholder={t('form.postLogoutPlaceholder')}
        rows={3}
        helperText={t('form.postLogoutHelp')}
        error={errors.post_logout_redirect_uris?.message}
      />

      {/* Allowed Origins (CORS) */}
      <Textarea
        {...register('allowed_origins')}
        label={needsAllowedOrigins ? t('form.originsRequired') : t('form.origins')}
        placeholder={t('form.originsPlaceholder')}
        rows={2}
        helperText={needsAllowedOrigins ? t('form.originsHelpRequired') : t('form.originsHelp')}
        error={errors.allowed_origins?.message}
      />

      {/* API audiences */}
      <Textarea
        {...register('audience')}
        label={t('form.audience')}
        placeholder={t('form.audiencePlaceholder')}
        rows={2}
        helperText={t('form.audienceHelp')}
        error={errors.audience?.message}
      />

      {/* Back-channel logout */}
      <Input
        {...register('backchannel_logout_uri')}
        label={t('form.backchannel')}
        placeholder={t('form.backchannelPlaceholder')}
        helperText={t('form.backchannelHelp')}
        error={errors.backchannel_logout_uri?.message}
      />

      {/* Consent Flow */}
      <Checkbox
        {...register('skip_consent')}
        label={t('form.skipConsent')}
        description={t('form.skipConsentHelp')}
      />
      <Checkbox
        {...register('skip_logout_consent')}
        label={t('form.skipLogoutConsent')}
        description={t('form.skipLogoutConsentHelp')}
      />

      {/* Access Token Format */}
      <Select
        {...register('access_token_strategy')}
        label={t('form.tokenFormat')}
        options={ACCESS_TOKEN_STRATEGIES.map((s) => ({ value: s.value, label: t(`form.tokenStrategies.${s.key}`) }))}
        helperText={t('form.tokenFormatHelp')}
        error={errors.access_token_strategy?.message}
      />

      {/* Grant Types */}
      <CheckboxGroup
        label={t('form.grantTypesLabel')}
        options={AVAILABLE_GRANT_TYPES.map((g) => ({ value: g.value, label: t(`form.grantTypes.${g.value}`) }))}
        value={grantTypes}
        onChange={handleGrantTypeChange}
        error={errors.grant_types?.message}
        required
      />

      {/* Scopes */}
      <CheckboxGroup
        label={t('form.scopesLabel')}
        options={AVAILABLE_SCOPES.map((s) => ({ value: s.value, label: t(`form.scopes.${s.value}`) }))}
        value={scopes}
        onChange={handleScopeChange}
        error={errors.scopes?.message}
        required
      />

      {/* Public Client */}
      <Checkbox
        {...register('public')}
        label={t('form.public')}
        description={t('form.publicHelp')}
      />

      {/* Authentication Provider Settings */}
      <div className="border-t border-gray-200 pt-4 mt-4">
        <h3 className="text-lg font-medium text-gray-900 mb-4">{t('form.providersTitle')}</h3>

        <CheckboxGroup
          label={t('form.providersLabel')}
          options={authProviderOptions}
          value={enabledAuthProviders}
          onChange={handleAuthProviderChange}
          error={errors.enabled_auth_providers?.message}
        />

      </div>

      {/* Buttons */}
      <div className="flex justify-end space-x-3 pt-4">
        <Button type="button" variant="secondary" onClick={onCancel}>
          {t('form.cancel')}
        </Button>
        <Button type="submit" isLoading={isSubmitting}>
          {submitLabel ?? t('form.submit')}
        </Button>
      </div>
    </form>
  )
}

export default ClientForm
