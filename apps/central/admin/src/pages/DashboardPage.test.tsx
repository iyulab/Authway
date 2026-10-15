import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@/test/utils'
import DashboardPage from './DashboardPage'
import { clientsApi, healthApi, usersApi } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

// The dashboard counts what the API counts and shows only what /health says.

const answer = <T,>(data: T) => ({ data }) as never

beforeEach(() => {
  vi.restoreAllMocks()
  useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Tenant', slug: 'tenant', active: true } as never })
  vi.spyOn(clientsApi, 'list').mockResolvedValue(
    answer({ clients: [{ id: 'c1', name: 'Notes', client_id: 'notes', active: true }], total: 137, limit: 5, offset: 0 }),
  )
  vi.spyOn(usersApi, 'list').mockResolvedValue(answer({ users: [{ id: 'u1', email: 'a@example.com', active: true }], total: 4210, limit: 5, offset: 0 }))
  vi.spyOn(healthApi, 'get').mockResolvedValue(answer({ status: 'ok', version: 'v20261015', migrations: ['041', '042'] }))
})

describe('dashboard', () => {
  it('shows the totals the API counts, not the length of one page', async () => {
    render(<DashboardPage />)
    expect(await screen.findByText('137')).toBeInTheDocument()
    expect(screen.getByText('4,210')).toBeInTheDocument()
    expect(clientsApi.list).toHaveBeenCalledWith({ limit: 5, tenant_id: 't1' })
  })

  it('shows what /health says and nothing it does not', async () => {
    render(<DashboardPage />)
    expect(await screen.findByText('v20261015')).toBeInTheDocument()
    expect(screen.getByText('migration 042')).toBeInTheDocument()
    expect(screen.queryByText('v1.0.0')).not.toBeInTheDocument()
    expect(screen.queryByText(/Redis/)).not.toBeInTheDocument()
  })
})
