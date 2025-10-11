import axios from 'axios'

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

// Create axios instance
export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
})

// Request interceptor to add auth token
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('authway_admin_token')
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
      // Clear token and redirect to login
      localStorage.removeItem('authway_admin_token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// API types
export interface User {
  id: string
  email: string
  first_name: string
  last_name: string
  avatar?: string
  email_verified: boolean
  active: boolean
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface Client {
  id: string
  client_id: string
  name: string
  description?: string
  website?: string
  logo?: string
  redirect_uris: string[]
  grant_types: string[]
  scopes: string[]
  public: boolean
  active: boolean
  created_at: string
  updated_at: string
}

export interface Tenant {
  id: string
  name: string
  slug: string
  description?: string
  settings?: Record<string, any>
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

// Users API (Note: This is for admin dashboard - not implemented in backend yet)
export const usersApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<{ users: User[]; total: number; limit: number; offset: number }>('/api/users', { params }),

  get: (id: string) =>
    api.get<{ user: User }>(`/api/users/${id}`),

  update: (id: string, data: Partial<User>) =>
    api.put<{ message: string; user: User }>(`/api/users/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/users/${id}`),
}

// Clients API
export const clientsApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<{ clients: Client[]; total: number; limit: number; offset: number }>('/api/v1/clients', { params }),

  get: (id: string) =>
    api.get<{ client: Client }>(`/api/v1/clients/${id}`),

  create: (data: {
    name: string
    description?: string
    website?: string
    logo?: string
    redirect_uris: string[]
    grant_types: string[]
    scopes: string[]
    public: boolean
  }) =>
    api.post<{ message: string; client: Client; credentials: { client_id: string; client_secret: string } }>('/api/v1/clients', data),

  update: (id: string, data: Partial<Client>) =>
    api.put<{ message: string; client: Client }>(`/api/v1/clients/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/clients/${id}`),

  regenerateSecret: (id: string) =>
    api.post<{ message: string; credentials: { client_id: string; client_secret: string } }>(`/api/v1/clients/${id}/regenerate-secret`),
}

// Tenants API
export const tenantsApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<Tenant[]>('/api/v1/tenants', {
      params,
      headers: { 'X-Admin-API-Key': import.meta.env.VITE_ADMIN_API_KEY || '' }
    }),

  get: (id: string) =>
    api.get<Tenant>(`/api/v1/tenants/${id}`, {
      headers: { 'X-Admin-API-Key': import.meta.env.VITE_ADMIN_API_KEY || '' }
    }),

  create: (data: {
    name: string
    slug: string
    description?: string
    settings?: Record<string, any>
  }) =>
    api.post<Tenant>('/api/v1/tenants', data, {
      headers: { 'X-Admin-API-Key': import.meta.env.VITE_ADMIN_API_KEY || '' }
    }),

  update: (id: string, data: Partial<Tenant>) =>
    api.put<Tenant>(`/api/v1/tenants/${id}`, data, {
      headers: { 'X-Admin-API-Key': import.meta.env.VITE_ADMIN_API_KEY || '' }
    }),

  delete: (id: string) =>
    api.delete<void>(`/api/v1/tenants/${id}`, {
      headers: { 'X-Admin-API-Key': import.meta.env.VITE_ADMIN_API_KEY || '' }
    }),
}