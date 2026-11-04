import { api } from './api'
import type { AuditLog, Invitation, Webhook, WebhookDelivery } from './contract'

// The shapes below are the contract's (contract.ts); the screens import them from here.
export type { AuditLog, Invitation, Webhook, WebhookDelivery }

// Webhooks API
export const webhooksApi = {
  list: (params?: { tenant_id?: string }) =>
    api.get<{ webhooks: Webhook[] }>('/api/v1/webhooks', { params }),

  get: (id: string) =>
    api.get<{ webhook: Webhook }>(`/api/v1/webhooks/${id}`),

  // Created in the selected tenant, which the API client sends as X-Tenant-ID.
  create: (data: {
    name: string
    url: string
    events: string[]
    enabled?: boolean
    retry_count?: number
    timeout_secs?: number
  }) =>
    api.post<{ webhook: Webhook; secret: string }>('/api/v1/webhooks', data),

  // PATCH: the API changes only the fields the body names.
  update: (id: string, data: Partial<Webhook>) =>
    api.patch<{ webhook: Webhook }>(`/api/v1/webhooks/${id}`, data),

  delete: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/webhooks/${id}`),

  // The old secret stops signing as soon as this answers.
  rotateSecret: (id: string) =>
    api.post<{ secret: string }>(`/api/v1/webhooks/${id}/rotate-secret`),

  // Sends one test event; a receiver that refuses or cannot be reached still
  // answers 200, and the delivery says how it went.
  test: (id: string) =>
    api.post<{ delivery: WebhookDelivery }>(`/api/v1/webhooks/${id}/test`),

  deliveries: (id: string, params?: { limit?: number }) =>
    api.get<{ deliveries: WebhookDelivery[] }>(`/api/v1/webhooks/${id}/deliveries`, { params }),
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

  userActivity: (userId: string, params?: { tenant_id?: string; limit?: number }) =>
    api.get<{ logs: AuditLog[] }>(`/api/v1/audit/users/${userId}/activity`, { params }),

  security: (params?: { tenant_id?: string; hours?: number }) =>
    api.get<{ logs: AuditLog[]; hours: number }>('/api/v1/audit/security', { params }),

  summary: (params?: { tenant_id?: string }) =>
    api.get<{ summary: { total_24h: number; total_7d: number; total_30d: number; security_events: number; failed_operations: number } }>('/api/v1/audit/summary', { params }),

  actions: () =>
    api.get<{ actions: { action: string; description: string }[]; severities: { severity: string; description: string }[] }>('/api/v1/audit/actions'),
}

// Invitations API
export const invitationsApi = {
  list: (params?: { tenant_id?: string; status?: string; limit?: number; offset?: number }) =>
    api.get<{ invitations: Invitation[]; total: number; limit: number; offset: number }>('/api/v1/invitations', { params }),

  get: (id: string) =>
    api.get<{ invitation: Invitation }>(`/api/v1/invitations/${id}`),

  // Created in the selected tenant, which the API client sends as X-Tenant-ID.
  create: (data: {
    email: string
    message?: string
    // The OAuth client_id of the application the invitation is for: after
    // accepting, the user is sent to its sign-in start URL (or home page).
    client_id?: string
    // The language of the invitation email (`ko`, `en`): the invited person's,
    // which the administrator knows and the API does not.
    locale?: string
  }) =>
    api.post<{ invitation: Invitation; message: string }>('/api/v1/invitations', data),

  revoke: (id: string) =>
    api.delete<{ message: string }>(`/api/v1/invitations/${id}`),

  resend: (id: string) =>
    api.post<{ message: string }>(`/api/v1/invitations/${id}/resend`),
}
