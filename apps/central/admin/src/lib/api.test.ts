import { describe, it, expect, beforeEach } from 'vitest'
import { http, HttpResponse } from 'msw/http'
import { server } from '@/test/mocks/server'
import { api } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'
import type { Tenant } from '@/lib/api'

// Tenant-scoped admin calls act on the tenant named by X-Tenant-ID. Two of the
// console's calls once left it out, the API refused them, and the console
// treated the refusal as a lost session.
describe('tenant header', () => {
  let seen: string | null

  beforeEach(() => {
    seen = 'unset'
    useTenantStore.setState({ selectedTenant: null })
    server.use(
      http.post('http://localhost:8080/api/v1/invitations', ({ request }) => {
        seen = request.headers.get('X-Tenant-ID')
        return HttpResponse.json({ invitation: {} }, { status: 201 })
      })
    )
  })

  it('names the selected tenant on every request', async () => {
    useTenantStore.setState({ selectedTenant: { id: 'tenant-a' } as Tenant })
    await api.post('/api/v1/invitations', { email: 'a@example.com' })
    expect(seen).toBe('tenant-a')
  })

  it('sends none when no tenant is selected', async () => {
    await api.post('/api/v1/invitations', { email: 'a@example.com' })
    expect(seen).toBeNull()
  })
})
