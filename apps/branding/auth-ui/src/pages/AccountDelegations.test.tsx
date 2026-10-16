import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import AccountDelegations from './AccountDelegations'

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const agent = {
  id: 'd1',
  user_id: 'u1',
  client_id: 'authway_agent',
  client_name: 'Orders agent',
  scopes: ['openid', 'orders.read'],
  created_at: '2026-10-16T09:00:00Z',
}

describe('AccountDelegations', () => {
  const user = userEvent.setup()
  beforeEach(() => {
    i18n.changeLanguage('en')
  })

  it('lists the apps acting for the user and revokes one after confirming', async () => {
    let listed = [agent]
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        listed = listed.filter((d) => !path.endsWith(`/${d.id}`))
        return new Response(null, { status: 204 })
      }
      return json(200, { delegations: listed })
    })
    render(<AccountDelegations call={call} />)

    expect(await screen.findByText('Orders agent')).toBeInTheDocument()
    expect(screen.getByText(/openid, orders\.read/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Revoke' }))
    await user.click(screen.getByRole('button', { name: 'Stop Orders agent' }))
    await waitFor(() => expect(screen.queryByText('Orders agent')).not.toBeInTheDocument())
    expect(call).toHaveBeenCalledWith('/api/v1/users/delegations/d1', { method: 'DELETE' })
  })

  it('says what went wrong when the revocation fails', async () => {
    const call = vi.fn(async (_path: string, init?: RequestInit) =>
      init?.method === 'DELETE'
        ? json(502, { code: 'authorization_server_unavailable', detail: 'The delegation could not be revoked. Try again shortly.' })
        : json(200, { delegations: [agent] }))
    render(<AccountDelegations call={call} />)

    await user.click(await screen.findByRole('button', { name: 'Revoke' }))
    await user.click(screen.getByRole('button', { name: 'Stop Orders agent' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('The delegation could not be revoked. Try again shortly.')
    expect(screen.getByText('Orders agent')).toBeInTheDocument()
  })

  it('shows nothing when no app acts for the user', async () => {
    const call = vi.fn(async () => json(200, { delegations: [] }))
    const { container } = render(<AccountDelegations call={call} />)
    await waitFor(() => expect(call).toHaveBeenCalled())
    expect(container.querySelector('section')).toBeNull()
  })
})
