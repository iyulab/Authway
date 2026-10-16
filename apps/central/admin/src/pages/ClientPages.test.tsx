import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import ClientCreatePage from './ClientCreatePage'
import ClientDetailPage from './ClientDetailPage'
import { capabilitiesApi, clientsApi, type Client } from '@/lib/api'
import { useTenantStore } from '@/stores/tenant'

// What the pages send is what the API acts on: these follow a client's API
// audiences and back-channel logout address from the form to the request.

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useParams: () => ({ clientId: 'c1' }), useNavigate: () => vi.fn() }
})

const answer = <T,>(data: T) => ({ data }) as never

const existing = {
  id: 'c1',
  tenant_id: 't1',
  client_id: 'app',
  name: 'Web App',
  redirect_uris: ['https://app.example.com/callback'],
  grant_types: ['authorization_code'],
  scopes: ['openid'],
  public: false,
  active: true,
  audience: ['https://api.example.com'],
  backchannel_logout_uri: 'https://app.example.com/backchannel-logout',
} as Client

beforeEach(() => {
  vi.restoreAllMocks()
  vi.spyOn(capabilitiesApi, 'get').mockResolvedValue(answer({ providers: [] }))
  useTenantStore.setState({ selectedTenant: { id: 't1', name: 'Tenant', slug: 'tenant' } as never })
})

describe('creating a client', () => {
  it('sends the audiences as a list and the logout address as given', async () => {
    const create = vi
      .spyOn(clientsApi, 'create')
      .mockResolvedValue(answer({ message: 'ok', client: existing, credentials: { client_id: 'app', client_secret: 's' } }))
    const user = userEvent.setup()
    render(<ClientCreatePage />)

    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await user.type(screen.getByLabelText(/^Redirect URIs/), 'https://app.example.com/callback')
    await user.type(screen.getByLabelText('API Audiences (Optional)'), 'https://api.example.com{enter} https://billing.example.com ')
    await user.type(screen.getByLabelText('Back-Channel Logout URL (Optional)'), ' https://app.example.com/backchannel-logout ')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create.mock.calls[0][0]).toMatchObject({
      tenant_id: 't1',
      audience: ['https://api.example.com', 'https://billing.example.com'],
      backchannel_logout_uri: 'https://app.example.com/backchannel-logout',
    })
  })

  it('sends neither when they are left empty', async () => {
    const create = vi
      .spyOn(clientsApi, 'create')
      .mockResolvedValue(answer({ message: 'ok', client: existing, credentials: { client_id: 'app', client_secret: 's' } }))
    const user = userEvent.setup()
    render(<ClientCreatePage />)

    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await user.type(screen.getByLabelText(/^Redirect URIs/), 'https://app.example.com/callback')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    const sent = create.mock.calls[0][0]
    expect(sent.audience).toBeUndefined()
    expect(sent.backchannel_logout_uri).toBeUndefined()
  })
})

describe('where users are sent', () => {
  it('sends the home page and the sign-in start URL when creating, trimmed', async () => {
    const create = vi
      .spyOn(clientsApi, 'create')
      .mockResolvedValue(answer({ message: 'ok', client: existing, credentials: { client_id: 'app', client_secret: 's' } }))
    const user = userEvent.setup()
    render(<ClientCreatePage />)

    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await user.type(screen.getByLabelText('Home Page'), ' https://app.example.com ')
    await user.type(screen.getByLabelText('Sign-In Start URL (Optional)'), 'https://app.example.com/login')
    await user.type(screen.getByLabelText('Logo URL (Optional)'), 'https://app.example.com/logo.png')
    await user.type(screen.getByLabelText(/^Redirect URIs/), 'https://app.example.com/callback')
    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create.mock.calls[0][0]).toMatchObject({
      client_uri: 'https://app.example.com',
      initiate_login_uri: 'https://app.example.com/login',
      logo: 'https://app.example.com/logo.png',
    })
  })

  it('clears them by sending them empty when changing a client', async () => {
    vi.spyOn(clientsApi, 'get').mockResolvedValue(
      answer({ client: { ...existing, client_uri: 'https://app.example.com', initiate_login_uri: 'https://app.example.com/login', logo: 'https://app.example.com/logo.png' } }),
    )
    const update = vi.spyOn(clientsApi, 'update').mockResolvedValue(answer({ message: 'ok', client: existing }))
    const user = userEvent.setup()
    render(<ClientDetailPage />)

    await user.clear(await screen.findByLabelText('Home Page'))
    await user.clear(screen.getByLabelText('Sign-In Start URL (Optional)'))
    await user.clear(screen.getByLabelText('Logo URL (Optional)'))
    await user.click(screen.getByRole('button', { name: /save|update/i }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update.mock.calls[0][1]).toMatchObject({ client_uri: '', initiate_login_uri: '', logo: '' })
  })
})

describe('changing a client', () => {
  it('clears the audiences and the logout address by sending them empty', async () => {
    vi.spyOn(clientsApi, 'get').mockResolvedValue(answer({ client: existing }))
    const update = vi.spyOn(clientsApi, 'update').mockResolvedValue(answer({ message: 'ok', client: existing }))
    const user = userEvent.setup()
    render(<ClientDetailPage />)

    const audiences = await screen.findByLabelText('API Audiences (Optional)')
    expect(audiences).toHaveValue('https://api.example.com')
    await user.clear(audiences)
    await user.clear(screen.getByLabelText('Back-Channel Logout URL (Optional)'))
    await user.click(screen.getByRole('button', { name: /save|update/i }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    // Left out, the API would keep what was there.
    expect(update.mock.calls[0][1]).toMatchObject({ audience: [], backchannel_logout_uri: '' })
  })
})

describe('turning a client off', () => {
  it('asks first, then sends only active: false', async () => {
    vi.spyOn(clientsApi, 'get').mockResolvedValue(answer({ client: existing }))
    const update = vi.spyOn(clientsApi, 'update').mockResolvedValue(answer({ message: 'ok', client: existing }))
    const user = userEvent.setup()
    render(<ClientDetailPage />)

    await user.click(await screen.findByRole('button', { name: 'Turn Off' }))
    expect(update).not.toHaveBeenCalled()
    const confirm = screen.getAllByRole('button', { name: 'Turn Off' }).at(-1)!
    await user.click(confirm)
    await waitFor(() => expect(update).toHaveBeenCalledWith('c1', { active: false }))
  })

  it('turns a client that is off back on', async () => {
    vi.spyOn(clientsApi, 'get').mockResolvedValue(answer({ client: { ...existing, active: false } }))
    const update = vi.spyOn(clientsApi, 'update').mockResolvedValue(answer({ message: 'ok', client: existing }))
    const user = userEvent.setup()
    render(<ClientDetailPage />)

    await user.click(await screen.findByRole('button', { name: 'Turn On' }))
    await waitFor(() => expect(update).toHaveBeenCalledWith('c1', { active: true }))
  })
})

describe('the account screen client', () => {
  it('is shown as managed by Authway, without a form to change it or a way to delete it', async () => {
    vi.spyOn(clientsApi, 'get').mockResolvedValue(answer({ client: { ...existing, name: 'Account', public: true, system: true } }))
    render(<ClientDetailPage />)

    expect(await screen.findByText('Managed by Authway')).toBeInTheDocument()
    expect(screen.getByText('System')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save|update/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /delete client/i })).not.toBeInTheDocument()
  })
})
