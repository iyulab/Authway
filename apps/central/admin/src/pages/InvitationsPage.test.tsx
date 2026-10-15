import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import InvitationsPage from './InvitationsPage'
import { clientsApi, type Client } from '@/lib/api'
import { invitationsApi } from '@/lib/features-api'
import { useTenantStore } from '@/stores/tenant'

// An invitation can name the application it is for; after accepting, the
// user is sent on to that application to sign in.

const answer = <T,>(data: T) => ({ data }) as never

const app = (over: Partial<Client>) =>
  ({
    id: over.client_id,
    tenant_id: 't1',
    redirect_uris: [],
    grant_types: ['authorization_code'],
    scopes: ['openid'],
    public: false,
    active: true,
    ...over,
  }) as Client

beforeEach(() => {
  vi.restoreAllMocks()
  useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Tenant', slug: 'tenant' } as never })
  vi.spyOn(invitationsApi, 'list').mockResolvedValue(answer({ invitations: [], total: 0, limit: 10, offset: 0 }))
  vi.spyOn(clientsApi, 'list').mockResolvedValue(
    answer({
      clients: [
        app({ client_id: 'notes', name: 'Notes', initiate_login_uri: 'https://notes.example/login' }),
        app({ client_id: 'bare', name: 'Bare' }),
        app({ client_id: 'authway_account', name: 'Account', system: true }),
      ],
      total: 3,
      limit: 200,
      offset: 0,
    }),
  )
})

const openDialog = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(await screen.findByRole('button', { name: /invite user/i }))
  await user.type(screen.getByPlaceholderText('user@example.com'), 'person@example.com')
}

describe('inviting a user to an application', () => {
  it('offers the tenant’s own applications, not the account screen’s', async () => {
    const user = userEvent.setup()
    render(<InvitationsPage />)
    await openDialog(user)

    expect(await screen.findByRole('option', { name: 'Notes' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Bare' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Account' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('Application (optional)')).toHaveValue('')
  })

  it('sends the chosen application’s client_id', async () => {
    const create = vi.spyOn(invitationsApi, 'create').mockResolvedValue(answer({ invitation: {}, message: 'ok' }))
    const user = userEvent.setup()
    render(<InvitationsPage />)
    await openDialog(user)

    await user.selectOptions(screen.getByLabelText('Application (optional)'), await screen.findByRole('option', { name: 'Notes' }))
    await user.click(screen.getByRole('button', { name: 'Send Invitation' }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create.mock.calls[0][0]).toMatchObject({ email: 'person@example.com', client_id: 'notes' })
  })

  it('sends no client_id when none is chosen', async () => {
    const create = vi.spyOn(invitationsApi, 'create').mockResolvedValue(answer({ invitation: {}, message: 'ok' }))
    const user = userEvent.setup()
    render(<InvitationsPage />)
    await openDialog(user)
    await user.click(screen.getByRole('button', { name: 'Send Invitation' }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create.mock.calls[0][0]).not.toHaveProperty('client_id')
  })

  it('says so when the chosen application has nowhere to send the user', async () => {
    const user = userEvent.setup()
    render(<InvitationsPage />)
    await openDialog(user)

    await user.selectOptions(screen.getByLabelText('Application (optional)'), await screen.findByRole('option', { name: 'Bare' }))
    expect(screen.getByText(/has no sign-in start URL or home page yet/)).toBeInTheDocument()
  })

  it('shows a refusal at the field it names, until that is changed', async () => {
    vi.spyOn(invitationsApi, 'create').mockRejectedValue({
      response: { status: 400, data: { code: 'email_domain_not_allowed', field: 'email', detail: 'this address is not in a domain the organization allows' } },
    })
    const user = userEvent.setup()
    render(<InvitationsPage />)
    await openDialog(user)
    await user.click(screen.getByRole('button', { name: 'Send Invitation' }))

    expect(await screen.findByText('this address is not in a domain the organization allows')).toBeInTheDocument()
    await user.type(screen.getByPlaceholderText('user@example.com'), 'x')
    expect(screen.queryByText('this address is not in a domain the organization allows')).not.toBeInTheDocument()
  })
})

describe('the invitation list', () => {
  it('shows the application each invitation is for, and offers no role', async () => {
    const user = userEvent.setup()
    vi.spyOn(invitationsApi, 'list').mockResolvedValue(
      answer({
        invitations: [
          { id: 'i1', tenant_id: 't1', email: 'a@example.com', inviter_id: null, inviter_name: 'system', tenant_name: 'Tenant', status: 'pending', expires_at: '2026-10-22T00:00:00Z', created_at: '2026-10-15T00:00:00Z', client: { client_id: 'notes', name: 'Notes' } },
        ],
        total: 1,
        limit: 10,
        offset: 0,
      }),
    )
    render(<InvitationsPage />)
    expect(await screen.findByText('a@example.com')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Application' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'Notes' })).toBeInTheDocument()

    await openDialog(user)
    expect(screen.queryByText('Role')).not.toBeInTheDocument()
    expect(screen.queryByText(/Expires In/)).not.toBeInTheDocument()
  })
})
