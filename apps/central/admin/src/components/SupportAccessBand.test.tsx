import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import SupportAccessBand from './SupportAccessBand'
import { supportAccessApi, type SupportAccess } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

const access = (expiresInMs: number): SupportAccess => ({
  id: 'sa1',
  tenant_id: 't1',
  reason: 'ticket 7',
  started_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + expiresInMs).toISOString(),
})

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('the support access band', () => {
  it('says which tenant, why and how long, and ends it', async () => {
    useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Acme', slug: 'acme' } as never, supportAccess: access(30 * 60_000) })
    const end = vi.spyOn(supportAccessApi, 'end').mockResolvedValue({ data: undefined } as never)
    const user = userEvent.setup()
    render(<SupportAccessBand />)

    const band = screen.getByRole('status')
    expect(band).toHaveTextContent('Acme')
    expect(band).toHaveTextContent('ticket 7')
    expect(band).toHaveTextContent('30분 남음')
    await user.click(screen.getByRole('button', { name: '지원 접근 끝내기' }))
    await waitFor(() => expect(useTenantStore.getState().selectedTenant).toBeNull())
    expect(end).toHaveBeenCalledWith('sa1')
  })

  it('goes back to the tenant list once the hour is up', async () => {
    useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Acme', slug: 'acme' } as never, supportAccess: access(-1000) })
    render(<SupportAccessBand />)
    await waitFor(() => expect(useTenantStore.getState().selectedTenant).toBeNull())
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('shows nothing without support access', () => {
    useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Acme', slug: 'acme' } as never, supportAccess: null })
    render(<SupportAccessBand />)
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
