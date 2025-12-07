import { api } from './api'

// Webhook types
export interface Webhook {
  id: string
  tenant_id: string
  name: string
  url: string
  events: string[]
  enabled: boolean
  retry_count: number
  timeout_secs: number
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  webhook_id: string
  event_type: string
  payload: string
  status_code: number
  response_body: string
  attempt: number
  delivered_at: string
  success: boolean
  error_message: string
}

// Audit Log types
export interface AuditLog {
  id: string
  tenant_id: string
  actor_id?: string
  actor_email?: string
  action: string
  resource_type: string
  resource_id: string
  description: string
  ip_address: string
  user_agent: string
  severity: 'info' | 'warning' | 'error' | 'critical'
  success: boolean
  metadata?: Record<string, any>
  created_at: string
}

// Invitation types
export interface Invitation {
  id: string
  tenant_id: string
  email: string
  role: string
  inviter_id: string
  inviter_email?: string
  status: 'pending' | 'accepted' | 'expired' | 'revoked'
  message?: string
  expires_at: string
  accepted_at?: string
  created_at: string
  updated_at: string
}

// Impersonation types
export interface ImpersonationSession {
  id: string
  admin_id: string
  admin_email: string
  target_user_id: string
  target_user_email: string
  reason: string
  started_at: string
  ended_at?: string
  active: boolean
}

// Webhooks API
export const webhooksApi = {
  list: (params?: { tenant_id?: string }) =>
    api.get<{ webhooks: Webhook[] }>('/api/v1/webhooks', { params }),

  get: (id: string) =>
    api.get<{ webhook: Webhook }>(`/api/v1/webhooks/${id}`),

  create: (data: {
    tenant_id: string
    name: string
    url: string
    events: string[]
    enabled?: boolean
    retry_count?: number
    timeout_secs?: number
  }) =>
    api.post<{ webhook: Webhook }>('/api/v1/webhooks', data),

  update: (id: string, data: Partial<Webhook>) =>
    api.put<{ webhook: Webhook }>(`/api/v1/webhooks/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/webhooks/${id}`),

  test: (id: string) =>
    api.post<{ success: boolean; status_code?: number; error?: string }>(`/api/v1/webhooks/${id}/test`),

  deliveries: (id: string, params?: { limit?: number; offset?: number }) =>
    api.get<{ deliveries: WebhookDelivery[]; total: number }>(`/api/v1/webhooks/${id}/deliveries`, { params }),
}

// Audit Logs API
export const auditLogsApi = {
  list: (params?: {
    tenant_id?: string
    actor_id?: string
    action?: string
    resource_type?: string
    severity?: string
    success?: boolean
    start_time?: string
    end_time?: string
    limit?: number
    offset?: number
  }) =>
    api.get<{ logs: AuditLog[]; total: number; limit: number; offset: number }>('/api/v1/audit/logs', { params }),

  get: (id: string) =>
    api.get<{ log: AuditLog }>(`/api/v1/audit/logs/${id}`),

  userActivity: (userId: string, params?: { limit?: number }) =>
    api.get<{ logs: AuditLog[]; count: number; user_id: string }>(`/api/v1/audit/users/${userId}/activity`, { params }),

  security: (params?: { hours?: number }) =>
    api.get<{ logs: AuditLog[]; count: number; hours: number }>('/api/v1/audit/security', { params }),

  summary: () =>
    api.get<{ summary: { total_24h: number; total_7d: number; total_30d: number; security_events: number; failed_operations: number } }>('/api/v1/audit/summary'),

  actions: () =>
    api.get<{ actions: { action: string; description: string }[]; severities: { severity: string; description: string }[] }>('/api/v1/audit/actions'),
}

// Invitations API
export const invitationsApi = {
  list: (params?: { tenant_id?: string; status?: string; limit?: number; offset?: number }) =>
    api.get<{ invitations: Invitation[]; count: number }>('/api/v1/invitations', { params }),

  get: (id: string) =>
    api.get<{ invitation: Invitation }>(`/api/v1/invitations/${id}`),

  create: (data: {
    email: string
    role?: string
    message?: string
    expires_in_hours?: number
  }) =>
    api.post<{ invitation: Invitation; message: string }>('/api/v1/invitations', data),

  revoke: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/invitations/${id}`),

  resend: (id: string) =>
    api.post<{ message: string }>(`/api/v1/invitations/${id}/resend`),
}

// Impersonation API
// Backend routes are under /api/v1/admin/impersonate
export const impersonationApi = {
  start: (data: { target_user_id: string; reason: string }) =>
    api.post<{ message: string; token: string; expires_at: string; target_user: { id: string; email: string; name: string } }>('/api/v1/admin/impersonate', data),

  end: (sessionId: string) =>
    api.post<{ message: string }>(`/api/v1/admin/impersonate/${sessionId}/end`),

  validate: (token: string) =>
    api.post<{ valid: boolean; session_id: string; admin: { id: string; email: string }; target_user: { id: string; email: string }; expires_at: string }>('/api/v1/admin/impersonate/validate', { token }),

  activeSessions: (params?: { limit?: number }) =>
    api.get<{ sessions: ImpersonationSession[]; count: number }>('/api/v1/admin/impersonate/sessions', { params }),

  history: (params?: { limit?: number }) =>
    api.get<{ sessions: ImpersonationSession[]; count: number }>('/api/v1/admin/impersonate/history', { params }),
}
