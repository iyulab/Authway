import axios from 'axios'
import { useAuthStore } from '@/stores/auth'
import { useTenantStore } from '@/stores/tenant'
import { consolePath, getConfig } from '../config'

const API_BASE_URL = getConfig().apiUrl

// Create axios instance
export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000, // 30 seconds for operations like OAuth client creation
})

// Request interceptor to add the auth token and the selected tenant.
// Reads the stores rather than mirrored localStorage keys, so there is exactly
// one answer to "are we authenticated" — see stores/auth.ts.
//
// Tenant-scoped admin calls (invitations, webhooks, audit) act on
// the tenant named by X-Tenant-ID. Sending it on every request means no single
// call can forget it; a list call's tenant_id query parameter still decides its
// filter.
api.interceptors.request.use(
  (config) => {
    const token = useAuthStore.getState().token
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    const tenant = useTenantStore.getState().selectedTenant
    if (tenant && !config.headers['X-Tenant-ID']) {
      config.headers['X-Tenant-ID'] = tenant.id
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Response interceptor to handle errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // logout() clears token AND expiry together. Clearing only the token
      // while the app still considered itself signed in is what produced the
      // /login -> /dashboard -> 401 -> /login reload loop.
      useAuthStore.getState().logout()
      // Already on the login page? Then this 401 is a failed sign-in attempt,
      // and reloading would throw away the error the user needs to see.
      if (window.location.pathname !== consolePath('login')) {
        window.location.href = consolePath('login')
      }
    }
    return Promise.reject(error)
  }
)

/**
 * The API's refusal (RFC 9457 problem details). `field` names the request
 * member at fault — a member inside an object by its path
 * (`settings.allowed_domains`).
 */
export interface ServerRefusal {
  code?: string
  field?: string
  detail?: string
  hint?: string
}

/** The refusal behind a failed request, or null when the API gave none. */
export function serverRefusal(error: unknown): ServerRefusal | null {
  const data = (error as { response?: { data?: unknown } } | null | undefined)?.response?.data
  return data && typeof data === 'object' ? (data as ServerRefusal) : null
}

/** A refusal as a form shows it at its field: what is wrong, then what to do. */
export function refusalMessage(refusal: ServerRefusal): string {
  return [refusal.detail, refusal.hint].filter(Boolean).join(' ') || 'Refused by the server'
}

/** What the refusal behind `error` says about the request member `field`, if it names it. */
export function refusalAt(error: unknown, field: string): string | undefined {
  const refusal = serverRefusal(error)
  return refusal?.field === field ? refusalMessage(refusal) : undefined
}

// API types
export interface User {
  id: string
  tenant_id: string
  email: string
  name: string
  avatar_url?: string
  email_verified: boolean
  active: boolean
  /** The sign-in providers the account is linked to. */
  connections?: string[]
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface Client {
  /** Part of Authway — the tenant's account screen client; it cannot be changed or deleted. */
  system?: boolean
  id: string
  tenant_id: string
  client_id: string
  name: string
  description?: string
  // The application's home page — where a user goes back to from Authway's
  // screens. An absolute https URL; '' on update clears it.
  client_uri?: string
  // Where the application starts a sign-in when Authway sends a user to it
  // (an accepted invitation), with iss and login_hint. '' on update clears it.
  initiate_login_uri?: string
  logo?: string
  redirect_uris: string[]
  post_logout_redirect_uris?: string[]
  // CORS allow-list for browser-based OAuth flows. Required by the API for a
  // public client using authorization_code — without it, the reverse proxy
  // has nothing to validate a cross-origin token request against.
  allowed_origins?: string[]
  // The APIs this client's access tokens are for (their `aud` claim).
  audience?: string[]
  // Where the client's backend is told that a user's session has ended.
  // '' on update clears it.
  backchannel_logout_uri?: string
  grant_types: string[]
  scopes: string[]
  public: boolean
  active: boolean
  // Authentication Provider Settings
  enabled_auth_providers?: string[]
  // Consent Flow Configuration (first-party clients bypass consent/logout screens)
  skip_consent?: boolean
  skip_logout_consent?: boolean
  // Access token format. null/undefined inherits the deployment-wide strategy.
  access_token_strategy?: 'jwt' | 'opaque' | null
  // Social OAuth Settings
  google_oauth_enabled?: boolean
  github_oauth_enabled?: boolean
  microsoft_oauth_enabled?: boolean
  apple_oauth_enabled?: boolean
  created_at: string
  updated_at: string
}

export interface TenantSettings {
  // The least password length the tenant asks for, when more than 8.
  password_min_length?: number
  session_timeout?: number
  // The only email domains accounts can be created for; empty allows any.
  allowed_domains?: string[]
  // signup_mode: 'invite_only' (default — a first-time sign-in needs a
  // pending invitation) or 'open' (any address auto-provisions). Absent,
  // empty, or any other value behaves as 'invite_only'.
  signup_mode?: 'invite_only' | 'open' | string
}

export interface Tenant {
  id: string
  name: string
  slug: string
  description?: string
  settings?: TenantSettings
  active: boolean
  created_at: string
  updated_at: string
}

export interface LoginRequest {
  password: string
}

export interface LoginResponse {
  token: string
  expires_at: string
}

export interface AdminInfo {
  authenticated: boolean
  version: string
}

// Admin Auth API
export const authApi = {
  login: (data: LoginRequest) =>
    api.post<LoginResponse>('/admin/login', data),

  logout: () =>
    api.post<{ message: string }>('/admin/logout'),

  validate: () =>
    api.get<{ valid: boolean; info: AdminInfo }>('/admin/validate'),

  info: () =>
    api.get<AdminInfo>('/admin/info'),
}

// Users API
export const usersApi = {
  list: (params?: { limit?: number; offset?: number; tenant_id?: string }) =>
    api.get<{ users: User[]; total: number; limit: number; offset: number }>('/api/v1/users', { params }),

  get: (id: string) =>
    api.get<{ user: User }>(`/api/v1/users/${id}`),

  update: (id: string, data: { name?: string; avatar_url?: string; active?: boolean }) =>
    api.put<{ message: string; user: User }>(`/api/v1/users/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/users/${id}`),
}

/** What this deployment offers (GET /api/v1/capabilities). */
export interface Capabilities {
  multi_tenant: boolean
  /** Social providers this deployment has credentials for. */
  providers: string[]
  magic_link: boolean
  mfa: string[]
  signup_modes: string[]
  account_deletion?: boolean
  backchannel_logout?: boolean
  token_introspection?: boolean
  device_authorization?: boolean
  terms_acceptance?: boolean
  token_exchange: boolean
  ciba: boolean
}

export const capabilitiesApi = {
  get: () => api.get<Capabilities>('/api/v1/capabilities'),
}

/** What the API says about itself (GET /health): the deployed version and the database migrations it ships. */
export interface Health {
  status: string
  version: string
  migrations?: string[]
}

export const healthApi = {
  get: () => api.get<Health>('/health'),
}

// Clients API
export const clientsApi = {
  list: (params?: { limit?: number; offset?: number; tenant_id?: string }) =>
    api.get<{ clients: Client[]; total: number; limit: number; offset: number }>('/api/v1/clients', { params }),

  get: (id: string) =>
    api.get<{ client: Client }>(`/api/v1/clients/${id}`),

  create: (data: {
    tenant_id: string
    name: string
    description?: string
    client_uri?: string
    initiate_login_uri?: string
    logo?: string
    redirect_uris: string[]
    post_logout_redirect_uris?: string[]
    allowed_origins?: string[]
    audience?: string[]
    backchannel_logout_uri?: string
    grant_types: string[]
    scopes: string[]
    public: boolean
    enabled_auth_providers?: string[]
    skip_consent?: boolean
    skip_logout_consent?: boolean
    access_token_strategy?: string
  }) =>
    api.post<{ message: string; client: Client; credentials: { client_id: string; client_secret: string } }>('/api/v1/clients', data),

  // access_token_strategy accepts '' on update, which the API reads as "clear the
  // pin and go back to inheriting the server setting". The response never uses ''.
  update: (
    id: string,
    data: Partial<Omit<Client, 'access_token_strategy'>> & {
      access_token_strategy?: 'jwt' | 'opaque' | ''
    }
  ) => api.put<{ message: string; client: Client }>(`/api/v1/clients/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/clients/${id}`),

  regenerateSecret: (id: string) =>
    api.post<{ message: string; credentials: { client_id: string; client_secret: string } }>(`/api/v1/clients/${id}/regenerate-secret`),
}

// Tenants API
export const tenantsApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<Tenant[]>('/api/v1/tenants', { params }),

  get: (id: string) =>
    api.get<Tenant>(`/api/v1/tenants/${id}`),

  create: (data: {
    name: string
    slug: string
    description?: string
    settings?: TenantSettings
  }) =>
    api.post<Tenant>('/api/v1/tenants', data),

  update: (id: string, data: Partial<Tenant>) =>
    api.put<Tenant>(`/api/v1/tenants/${id}`, data),

  delete: (id: string) =>
    api.delete<void>(`/api/v1/tenants/${id}`),
}

