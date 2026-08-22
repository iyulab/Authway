import axios from 'axios'
import { useAuthStore } from '@/stores/auth'

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

// Create axios instance
export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000, // 30 seconds for operations like OAuth client creation
})

// Request interceptor to add auth token.
// Reads the store rather than a mirrored localStorage key, so there is exactly
// one answer to "are we authenticated" — see stores/auth.ts.
api.interceptors.request.use(
  (config) => {
    const token = useAuthStore.getState().token
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
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
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  }
)

// API types
export interface User {
  id: string
  tenant_id: string
  email: string
  name: string
  avatar_url?: string
  email_verified: boolean
  active: boolean
  provider: 'local' | 'google' | 'github'
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface Client {
  id: string
  tenant_id: string
  client_id: string
  name: string
  description?: string
  website?: string
  logo?: string
  redirect_uris: string[]
  post_logout_redirect_uris?: string[]
  // CORS allow-list for browser-based OAuth flows. Required by the API for a
  // public client using authorization_code — without it, the reverse proxy
  // has nothing to validate a cross-origin token request against.
  allowed_origins?: string[]
  logout_redirect_policy?: 'strict' | 'lenient' | 'disabled'
  default_logout_uri?: string
  allow_wildcard_logout?: boolean
  grant_types: string[]
  scopes: string[]
  public: boolean
  active: boolean
  // Authentication Provider Settings
  enabled_auth_providers?: string[]
  allow_email_signup?: boolean
  allow_email_login?: boolean
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
  require_email_verification?: boolean
  password_min_length?: number
  session_timeout?: number
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

  update: (id: string, data: { name?: string; avatar_url?: string }) =>
    api.put<{ message: string; user: User }>(`/api/v1/users/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/users/${id}`),
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
    website?: string
    logo?: string
    redirect_uris: string[]
    post_logout_redirect_uris?: string[]
    allowed_origins?: string[]
    logout_redirect_policy?: 'strict' | 'lenient' | 'disabled'
    default_logout_uri?: string
    allow_wildcard_logout?: boolean
    grant_types: string[]
    scopes: string[]
    public: boolean
    enabled_auth_providers?: string[]
    allow_email_signup?: boolean
    allow_email_login?: boolean
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
// Note: Admin API Key authentication is currently disabled in development mode
// When AUTHWAY_ADMIN_API_KEY is set in backend, update VITE_ADMIN_API_KEY in .env.production
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

