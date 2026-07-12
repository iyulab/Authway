import React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Input, Textarea, Select, Checkbox, CheckboxGroup, Button } from '@/components/ui'
import { Client } from '@/lib/api'

// Client form validation schema
export const clientFormSchema = z.object({
  name: z.string().min(1, 'Client name is required'),
  description: z.string().optional(),
  website: z.string().url('Please enter a valid URL').optional().or(z.literal('')),
  redirect_uris: z.string().min(1, 'At least one Redirect URI is required'),
  post_logout_redirect_uris: z.string().optional(),
  logout_redirect_policy: z.enum(['strict', 'lenient', 'disabled']).optional(),
  default_logout_uri: z.string().url('Please enter a valid URL').optional().or(z.literal('')),
  allow_wildcard_logout: z.boolean().optional(),
  grant_types: z.array(z.string()).min(1, 'At least one Grant Type is required'),
  scopes: z.array(z.string()).min(1, 'At least one Scope is required'),
  public: z.boolean(),
  // Authentication Provider Settings
  enabled_auth_providers: z.array(z.string()).optional(),
  allow_email_signup: z.boolean().optional(),
  allow_email_login: z.boolean().optional(),
  // Consent Flow Configuration
  skip_consent: z.boolean().optional(),
  skip_logout_consent: z.boolean().optional(),
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

export const LOGOUT_REDIRECT_POLICIES = [
  { value: 'strict', label: 'Strict (Default) - URI required + validation' },
  { value: 'lenient', label: 'Lenient - URI optional + validation' },
  { value: 'disabled', label: 'Disabled - No validation (dev only)' },
]

export const AVAILABLE_AUTH_PROVIDERS = [
  { value: 'email', label: 'Email/Password' },
  { value: 'google', label: 'Google' },
  { value: 'github', label: 'GitHub' },
  { value: 'microsoft', label: 'Microsoft' },
  { value: 'apple', label: 'Apple' },
]

export interface ClientFormProps {
  initialData?: Client
  onSubmit: (data: ClientFormData) => void
  onCancel: () => void
  isSubmitting?: boolean
  submitLabel?: string
}

export const ClientForm: React.FC<ClientFormProps> = ({
  initialData,
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
    watch,
  } = useForm<ClientFormData>({
    resolver: zodResolver(clientFormSchema),
    defaultValues: initialData
      ? {
          name: initialData.name,
          description: initialData.description || '',
          website: initialData.website || '',
          redirect_uris: initialData.redirect_uris.join('\n'),
          post_logout_redirect_uris: (initialData.post_logout_redirect_uris || []).join('\n'),
          logout_redirect_policy: initialData.logout_redirect_policy || 'strict',
          default_logout_uri: initialData.default_logout_uri || '',
          allow_wildcard_logout: initialData.allow_wildcard_logout || false,
          grant_types: initialData.grant_types,
          scopes: initialData.scopes,
          public: initialData.public,
          enabled_auth_providers: initialData.enabled_auth_providers || ['email', 'google'],
          allow_email_signup: initialData.allow_email_signup ?? true,
          allow_email_login: initialData.allow_email_login ?? true,
          skip_consent: initialData.skip_consent || false,
          skip_logout_consent: initialData.skip_logout_consent || false,
        }
      : {
          grant_types: ['authorization_code'],
          scopes: ['openid'],
          public: false,
          logout_redirect_policy: 'strict',
          allow_wildcard_logout: false,
          enabled_auth_providers: ['email', 'google'],
          allow_email_signup: true,
          allow_email_login: true,
          skip_consent: false,
          skip_logout_consent: false,
        },
  })

  const grantTypes = watch('grant_types') || []
  const scopes = watch('scopes') || []
  const enabledAuthProviders = watch('enabled_auth_providers') || []

  const handleGrantTypeChange = (values: string[]) => {
    setValue('grant_types', values, { shouldValidate: true })
  }

  const handleScopeChange = (values: string[]) => {
    setValue('scopes', values, { shouldValidate: true })
  }

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
          {...register('website')}
          type="url"
          label="Website URL"
          placeholder="https://example.com"
          error={errors.website?.message}
        />
      </div>

      <Textarea
        {...register('description')}
        label="Description"
        placeholder="Enter a description for the client"
        rows={2}
      />

      {/* Redirect URIs */}
      <Textarea
        {...register('redirect_uris')}
        label="Redirect URIs *"
        placeholder={`http://localhost:3000/callback\nhttps://example.com/callback`}
        rows={3}
        helperText="Enter each URI on a new line."
        error={errors.redirect_uris?.message}
      />

      {/* Post-Logout Redirect URIs */}
      <Textarea
        {...register('post_logout_redirect_uris')}
        label="Post-Logout Redirect URIs (Optional)"
        placeholder={`http://localhost:3000\nhttps://example.com`}
        rows={3}
        helperText="Enter each URI on a new line. If empty, Redirect URIs will be used."
        error={errors.post_logout_redirect_uris?.message}
      />

      {/* Logout Redirect Policy */}
      <Select
        {...register('logout_redirect_policy')}
        label="Logout Redirect Policy"
        options={LOGOUT_REDIRECT_POLICIES}
        helperText="Strict: Required + validation (production). Lenient: Optional + validation. Disabled: No validation (dev only)."
        error={errors.logout_redirect_policy?.message}
      />

      {/* Default Logout URI */}
      <Input
        {...register('default_logout_uri')}
        type="url"
        label="Default Logout URI (Optional)"
        placeholder="https://example.com"
        helperText="Used when post_logout_redirect_uri is not provided in Lenient mode."
        error={errors.default_logout_uri?.message}
      />

      {/* Allow Wildcard Logout */}
      <Checkbox
        {...register('allow_wildcard_logout')}
        label="Allow Wildcard Patterns"
        description="Enable wildcard patterns in Post-Logout Redirect URIs (e.g., http://localhost:*, https://*.example.com)"
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
          options={AVAILABLE_AUTH_PROVIDERS}
          value={enabledAuthProviders}
          onChange={handleAuthProviderChange}
          error={errors.enabled_auth_providers?.message}
        />

        <div className="mt-4 space-y-3">
          <Checkbox
            {...register('allow_email_signup')}
            label="Allow Email/Password Signup"
            description="Allow new users to register with email and password. Disable to only allow social login."
          />
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
