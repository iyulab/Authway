import axios from 'axios'
import { useAuthStore } from '@/stores/auth'
import { useTenantStore } from '@/stores/tenant'
import { consolePath, getConfig } from '../config'
import i18n from '@/i18n'
import type { Client, SupportAccess, Tenant, TenantSettings } from './contract'
import { route } from './route'

// The shapes of the API are the contract's (contract.ts); the screens import them from here.
export type { Client, SupportAccess, Tenant, TenantSettings }

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
//
// The language of the console goes with every request: a request that sends
// mail (an invitation) is worded in the language the administrator reads.
api.interceptors.request.use(
  (config) => {
    config.headers['Accept-Language'] = i18n.resolvedLanguage ?? i18n.language
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
    // An operator's support access ended (expired, or ended elsewhere): back
    // to the tenant list, where it can open another.
    if (error.response?.status === 403 && error.response?.data?.code === 'support_access_required') {
      useTenantStore.getState().clearSelectedTenant()
    }
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

// The API's refusals, and their words in the console's language.
export { serverRefusal, refusalMessage, refusalAt, apiErrorMessage } from '@/i18n/problem'
export type { ServerRefusal } from '@/i18n/problem'

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

export interface LoginRequest {
  password: string
}

export interface LoginResponse {
  token: string
  expires_at: string
}

// Admin Auth API
export const authApi = {
  login: (data: LoginRequest) =>
    api.post<LoginResponse>('/admin/login', data),

  logout: () =>
    api.post<{ message: string }>('/admin/logout'),
}

/** Who the caller is (GET /api/v1/console/me), whichever way it came in. */
export interface ConsoleMe {
  // console_user (a console account), admin_session (the admin password) or api_key
  actor_type: string
  operator: boolean
  user_id?: string
  memberships: { tenant_id: string | null; role: string }[]
}

export const consoleApi = {
  me: () => api.get<ConsoleMe>(route('get', '/api/v1/console/me')),
}

/**
 * An operator's entry into one tenant's contents: an hour, with a reason the
 * tenant's audit log records. An operator signed in with a console account
 * reaches into a tenant only while it holds one open.
 */
/** Whether the caller reaches into a tenant only through support access. */
export const needsSupportAccess = (me: ConsoleMe | undefined) =>
  Boolean(me && me.operator && me.actor_type === 'console_user')

export const supportAccessApi = {
  list: () => api.get<{ support_access: SupportAccess[] }>(route('get', '/api/v1/console/support-access')),
  start: (tenantId: string, reason: string) =>
    api.post<{ support_access: SupportAccess }>(route('post', '/api/v1/console/support-access'), { tenant_id: tenantId, reason }),
  end: (id: string) => api.delete<void>(route('delete', '/api/v1/console/support-access/{id}', { id })),
}

// Users API
export const usersApi = {
  list: (params?: { limit?: number; offset?: number; tenant_id?: string }) =>
    api.get<{ users: User[]; total: number; limit: number; offset: number }>(route('get', '/api/v1/users'), { params }),

  get: (id: string) =>
    api.get<{ user: User }>(route('get', '/api/v1/users/{id}', { id })),

  update: (id: string, data: { name?: string; avatar_url?: string; active?: boolean }) =>
    api.put<{ message: string; user: User }>(route('put', '/api/v1/users/{id}', { id }), data),

  delete: (id: string) =>
    api.delete<{ message: string }>(route('delete', '/api/v1/users/{id}', { id })),
}

/** What this deployment offers (GET /api/v1/capabilities). */
export interface Capabilities {
  multi_tenant: boolean
  /** Social providers this deployment has credentials for. */
  providers: string[]
  magic_link: boolean
  /** Users can register passkeys and sign in with them. */
  passkeys?: boolean
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
  get: () => api.get<Capabilities>(route('get', '/api/v1/capabilities')),
}

/** What the API says about itself (GET /health): the deployed version and the database migrations it ships. */
export interface Health {
  status: string
  version: string
  migrations?: string[]
}

export const healthApi = {
  get: () => api.get<Health>(route('get', '/health')),
}

// Clients API
export const clientsApi = {
  list: (params?: { limit?: number; offset?: number; tenant_id?: string }) =>
    api.get<{ clients: Client[]; total: number; limit: number; offset: number }>(route('get', '/api/v1/clients'), { params }),

  get: (id: string) =>
    api.get<{ client: Client }>(route('get', '/api/v1/clients/{id}', { id })),

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
    api.post<{ message: string; client: Client; credentials: { client_id: string; client_secret: string } }>(route('post', '/api/v1/clients'), data),

  // access_token_strategy accepts '' on update, which the API reads as "clear the
  // pin and go back to inheriting the server setting". The response never uses ''.
  update: (
    id: string,
    data: Partial<Omit<Client, 'access_token_strategy'>> & {
      access_token_strategy?: 'jwt' | 'opaque' | ''
    }
  ) => api.put<{ message: string; client: Client }>(route('put', '/api/v1/clients/{id}', { id }), data),

  delete: (id: string) =>
    api.delete<{ message: string }>(route('delete', '/api/v1/clients/{id}', { id })),

  regenerateSecret: (id: string) =>
    api.post<{ message: string; credentials: { client_id: string; client_secret: string } }>(route('post', '/api/v1/clients/{id}/regenerate-secret', { id })),
}

// Tenants API
export const tenantsApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<Tenant[]>(route('get', '/api/v1/tenants'), { params }),

  get: (id: string) =>
    api.get<Tenant>(route('get', '/api/v1/tenants/{id}', { id })),

  create: (data: {
    name: string
    slug: string
    description?: string
    settings?: TenantSettings
  }) =>
    api.post<Tenant>(route('post', '/api/v1/tenants'), data),

  update: (id: string, data: Partial<Tenant>) =>
    api.put<Tenant>(route('put', '/api/v1/tenants/{id}', { id }), data),

  delete: (id: string) =>
    api.delete<void>(route('delete', '/api/v1/tenants/{id}', { id })),
}

