import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import TenantSelectionPage from './TenantSelectionPage'
import { consoleApi, supportAccessApi, tenantsApi, type ConsoleMe, type SupportAccess } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

// An operator signed in with a console account enters a tenant through
// support access: it gives a reason, and the tenant's audit log records it.
// The admin password enters any tenant as before.

const answer = <T,>(data: T) => ({ data }) as never
const tenant = { id: 't1', name: 'Acme', slug: 'acme', active: true } as never
const access: SupportAccess = {
  id: 'sa1',
  tenant_id: 't1',
  reason: 'ticket 7',
  started_at: '2026-10-24T08:00:00Z',
  expires_at: new Date(Date.now() + 3_600_000).toISOString(),
}
const me = (over: Partial<ConsoleMe>): ConsoleMe => ({ actor_type: 'console_user', operator: true, memberships: [], ...over })

beforeEach(() => {
  vi.restoreAllMocks()
  useTenantStore.setState({ selectedTenant: null, supportAccess: null })
  vi.spyOn(tenantsApi, 'list').mockResolvedValue(answer([tenant]))
})

describe('entering a tenant', () => {
  it('asks an operator’s account for a reason and opens support access', async () => {
    vi.spyOn(consoleApi, 'me').mockResolvedValue(answer(me({})))
    vi.spyOn(supportAccessApi, 'list').mockResolvedValue(answer({ support_access: [] }))
    const start = vi.spyOn(supportAccessApi, 'start').mockResolvedValue(answer({ support_access: access }))
    const user = userEvent.setup()
    render(<TenantSelectionPage />)

    await waitFor(() => expect(supportAccessApi.list).toHaveBeenCalled())
    await user.click(await screen.findByRole('button', { name: /acme/i }))
    expect(useTenantStore.getState().selectedTenant).toBeNull()
    const open = screen.getByRole('button', { name: 'Open support access' })
    expect(open).toBeDisabled()
    await user.type(screen.getByLabelText('Reason'), 'ticket 7')
    await user.click(open)

    await waitFor(() => expect(useTenantStore.getState().supportAccess).toEqual(access))
    expect(start).toHaveBeenCalledWith('t1', 'ticket 7')
    expect(useTenantStore.getState().selectedTenant).toMatchObject({ id: 't1' })
  })

  it('reuses support access the operator already holds open', async () => {
    vi.spyOn(consoleApi, 'me').mockResolvedValue(answer(me({})))
    vi.spyOn(supportAccessApi, 'list').mockResolvedValue(answer({ support_access: [access] }))
    const start = vi.spyOn(supportAccessApi, 'start')
    const user = userEvent.setup()
    render(<TenantSelectionPage />)

    await waitFor(() => expect(supportAccessApi.list).toHaveBeenCalled())
    await waitFor(async () => {
      await user.click(await screen.findByRole('button', { name: /acme/i }))
      expect(useTenantStore.getState().supportAccess).toEqual(access)
    })
    expect(start).not.toHaveBeenCalled()
  })

  it('lets the admin password in without support access', async () => {
    vi.spyOn(consoleApi, 'me').mockResolvedValue(answer(me({ actor_type: 'admin_session' })))
    const list = vi.spyOn(supportAccessApi, 'list')
    const user = userEvent.setup()
    render(<TenantSelectionPage />)

    await waitFor(() => expect(consoleApi.me).toHaveBeenCalled())
    await user.click(await screen.findByRole('button', { name: /acme/i }))
    expect(useTenantStore.getState().selectedTenant).toMatchObject({ id: 't1' })
    expect(useTenantStore.getState().supportAccess).toBeNull()
    expect(list).not.toHaveBeenCalled()
  })
})
