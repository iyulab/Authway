import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '@/test/utils'
import toast from 'react-hot-toast'
import UsersPage from './UsersPage'
import WebhooksPage from './WebhooksPage'
import { usersApi } from '@/lib/api'
import { webhooksApi } from '@/lib/features-api'
import { useTenantStore } from '@/stores/tenant'

const answer = <T,>(data: T) => ({ data }) as never

beforeEach(() => {
  vi.restoreAllMocks()
  useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Tenant', slug: 'tenant', active: true } as never })
})

describe('users', () => {
  // The badge shows the provider accounts that actually sign in — the stored
  // provider said "local" for everyone.
  it('shows the accounts each user is connected to', async () => {
    vi.spyOn(usersApi, 'list').mockResolvedValue(
      answer({
        users: [
          { id: 'u1', tenant_id: 't1', email: 'g@example.com', name: 'G', email_verified: true, active: true, connections: ['google', 'microsoft'], created_at: '', updated_at: '' },
          { id: 'u2', tenant_id: 't1', email: 'p@example.com', name: 'P', email_verified: true, active: true, connections: [], created_at: '', updated_at: '' },
        ],
        total: 2,
        limit: 10,
        offset: 0,
      }),
    )
    render(<UsersPage />)
    expect(await screen.findByText('Google')).toBeInTheDocument()
    expect(screen.getByText('Microsoft')).toBeInTheDocument()
    expect(screen.getByText('Password / link')).toBeInTheDocument()
  })
})

describe('webhooks', () => {
  it('does not send a blank retry count as "the default"', async () => {
    const user = userEvent.setup()
    const error = vi.spyOn(toast, 'error')
    vi.spyOn(webhooksApi, 'list').mockResolvedValue(answer({ webhooks: [], total: 0, limit: 10, offset: 0 }))
    const create = vi.spyOn(webhooksApi, 'create').mockResolvedValue(answer({}))
    render(<WebhooksPage />)

    await user.click(await screen.findByRole('button', { name: /add webhook/i }))
    await user.type(screen.getByPlaceholderText('My Webhook'), 'Hook')
    await user.type(screen.getByPlaceholderText('https://example.com/webhook'), 'https://example.com/hook')
    const retry = screen.getAllByRole('spinbutton')[0]
    await user.clear(retry)
    await user.click(screen.getByRole('button', { name: /^create$|create webhook|save/i }))
    expect(error).toHaveBeenCalledWith('Retry count must be a number from 0 to 10')
    expect(create).not.toHaveBeenCalled()
  })
})
