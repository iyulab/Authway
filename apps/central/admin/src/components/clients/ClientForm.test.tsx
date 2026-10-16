import { describe, it, expect, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import { ClientForm, usesBrowserRedirect } from './ClientForm'
import { capabilitiesApi, type Client } from '@/lib/api'

/**
 * These tests pin the two client-config rules the console has to mirror from the
 * server, because getting either wrong is invisible until a real registration
 * fails:
 *
 *  1. redirect_uris is required for redirect-based grants only — a
 *     machine-to-machine client must be registrable without one.
 *  2. access_token_strategy is three-state: '' (inherit) / 'opaque' / 'jwt', and
 *     '' has to survive the round-trip because that is how a pin gets cleared.
 */

const noop = () => {}

const renderForm = (props: Partial<React.ComponentProps<typeof ClientForm>> = {}) => {
  const onSubmit = vi.fn()
  render(<ClientForm onSubmit={onSubmit} onCancel={noop} submitLabel="Save" {...props} />)
  return { onSubmit }
}

const submit = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole('button', { name: 'Save' }))
}

describe('ClientForm — redirect_uris is conditional on grant type', () => {
  it('rejects an authorization_code client with no redirect URI', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await submit(user)

    expect(
      await screen.findByText(
        'At least one Redirect URI is required for Authorization Code'
      )
    ).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('accepts a client_credentials-only client with no redirect URI', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'Batch Worker')
    // Swap the default authorization_code grant for client_credentials.
    await user.click(screen.getByLabelText('Authorization Code'))
    await user.click(screen.getByLabelText('Client Credentials'))
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      name: 'Batch Worker',
      grant_types: ['client_credentials'],
      redirect_uris: '',
    })
  })

  it('drops the required marker once no redirect-based grant is selected', async () => {
    const user = userEvent.setup()
    renderForm()

    expect(screen.getByLabelText('Redirect URIs *')).toBeInTheDocument()

    await user.click(screen.getByLabelText('Authorization Code'))

    expect(screen.getByLabelText('Redirect URIs')).toBeInTheDocument()
    expect(
      screen.getByText(/Not used by machine-to-machine clients/)
    ).toBeInTheDocument()
  })
})

describe('ClientForm — allowed_origins is conditional on public + authorization_code', () => {
  it('rejects a public authorization_code client with no allowed origin', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'SPA App')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'https://app.example/cb')
    await user.click(screen.getByLabelText('Public Client (No Client Secret)'))
    await submit(user)

    expect(
      await screen.findByText(
        'Public clients with Authorization Code need at least one Allowed Origin for browser CORS'
      )
    ).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('accepts a public authorization_code client once an allowed origin is set', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'SPA App')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'https://app.example/cb')
    await user.click(screen.getByLabelText('Public Client (No Client Secret)'))
    await user.type(screen.getByLabelText('Allowed Origins (CORS) *'), 'https://app.example')
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0].allowed_origins).toBe('https://app.example')
  })

  it('does not require an allowed origin for a native client on a loopback address', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'Desktop App')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'http://127.0.0.1/callback')
    await user.click(screen.getByLabelText('Public Client (No Client Secret)'))
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  })

  it('does not require an allowed origin for a confidential client', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'Server App')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'https://app.example/cb')
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  })

  it('preloads an existing client’s allowed origins', () => {
    const existing = {
      id: 'c3',
      client_id: 'spa_app',
      name: 'SPA App',
      redirect_uris: ['https://app.example/cb'],
      allowed_origins: ['https://app.example'],
      grant_types: ['authorization_code'],
      scopes: ['openid'],
      public: true,
    } as unknown as Client

    renderForm({ initialData: existing })

    expect(screen.getByLabelText('Allowed Origins (CORS) *')).toHaveValue('https://app.example')
  })
})

describe('ClientForm — access token strategy', () => {
  it('defaults to inheriting the server setting', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    const select = screen.getByLabelText('Access Token Format') as HTMLSelectElement
    expect(select.value).toBe('')

    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'https://app.example/cb')
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0].access_token_strategy).toBe('')
  })

  it('submits an explicit jwt opt-in', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()

    await user.type(screen.getByLabelText('Client Name *'), 'API Consumer')
    await user.type(screen.getByLabelText('Redirect URIs *'), 'https://app.example/cb')
    await user.selectOptions(screen.getByLabelText('Access Token Format'), 'jwt')
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0].access_token_strategy).toBe('jwt')
  })

  it('preloads an existing pin and can clear it back to inherit', async () => {
    const user = userEvent.setup()
    const existing = {
      id: 'c1',
      client_id: 'api_consumer',
      name: 'API Consumer',
      redirect_uris: ['https://app.example/cb'],
      grant_types: ['authorization_code'],
      scopes: ['openid'],
      public: false,
      access_token_strategy: 'jwt',
    } as unknown as Client
    const { onSubmit } = renderForm({ initialData: existing })

    const select = screen.getByLabelText('Access Token Format') as HTMLSelectElement
    expect(select.value).toBe('jwt')

    await user.selectOptions(select, '')
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    // '' — not undefined — so the update request clears the pin server-side.
    expect(onSubmit.mock.calls[0][0].access_token_strategy).toBe('')
  })

  it('tolerates an existing M2M client that has no redirect_uris', () => {
    const m2m = {
      id: 'c2',
      client_id: 'batch_worker',
      name: 'Batch Worker',
      grant_types: ['client_credentials'],
      scopes: ['api'],
      public: false,
    } as unknown as Client

    // Regression: initialData.redirect_uris used to be dereferenced unconditionally.
    expect(() => renderForm({ initialData: m2m })).not.toThrow()
    expect(screen.getByLabelText('Redirect URIs')).toHaveValue('')
  })
})

describe('ClientForm — social providers follow the server capabilities', () => {
  const baseClient: Client = {
    id: 'c1',
    tenant_id: 't1',
    client_id: 'app',
    name: 'App',
    redirect_uris: ['https://app.example.com/callback'],
    grant_types: ['authorization_code'],
    scopes: ['openid'],
    public: false,
    active: true,
    enabled_auth_providers: ['email'],
  } as Client
  const capabilities = {
    multi_tenant: true,
    providers: ['google'],
    magic_link: true,
    mfa: ['totp'],
    signup_modes: ['invite_only', 'open'],
    token_exchange: false,
    ciba: false,
  }

  it('disables a provider nobody has credentials for', async () => {
    vi.spyOn(capabilitiesApi, 'get').mockResolvedValue({ data: capabilities } as Awaited<ReturnType<typeof capabilitiesApi.get>>)
    renderForm()

    await waitFor(() => expect(screen.getByLabelText('GitHub')).toBeDisabled())
    expect(screen.getByLabelText('Google')).toBeEnabled()
    expect(screen.getAllByText('Not configured on this server').length).toBe(3) // GitHub, Microsoft, Apple
    expect(screen.getByLabelText('Email link (passwordless)')).toBeEnabled()
  })

  it('keeps a provider the client has its own credentials for', async () => {
    vi.spyOn(capabilitiesApi, 'get').mockResolvedValue({ data: capabilities } as Awaited<ReturnType<typeof capabilitiesApi.get>>)
    renderForm({ initialData: { ...baseClient, github_oauth_enabled: true, apple_oauth_enabled: true } })

    await waitFor(() => expect(screen.getByLabelText('Microsoft')).toBeDisabled())
    expect(screen.getByLabelText('GitHub')).toBeEnabled()
    // Apple and Microsoft follow the same rule as Google and GitHub.
    expect(screen.getByLabelText('Apple')).toBeEnabled()
  })
})

describe('usesBrowserRedirect', () => {
  it.each([
    ['https://app.example/cb', true],
    ['http://localhost:3000/cb', true],
    ['http://app.example/cb', true],
    ['not a uri', true],
    ['http://127.0.0.1/callback', false],
    ['http://127.0.0.1:51234/callback', false],
    ['http://[::1]/callback', false],
    ['com.example.app:/oauth2redirect', false],
    ['http://127.0.0.1/callback\nhttps://app.example/cb', true],
    ['', false],
  ])('%s -> %s', (uris, expected) => {
    expect(usesBrowserRedirect(uris)).toBe(expected)
  })
})

// The two settings a client that works with other parts of a deployment
// needs: which APIs its tokens are for, and where its backend is told about a
// logout. Both were reachable through the API only.
describe('ClientForm — API audiences and the back-channel logout address', () => {
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
    audience: ['https://api.example.com', 'https://billing.example.com'],
    backchannel_logout_uri: 'https://app.example.com/backchannel-logout',
  } as Client

  it('shows what an existing client has and submits it unchanged', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: existing })

    expect(screen.getByLabelText('API Audiences (Optional)')).toHaveValue('https://api.example.com\nhttps://billing.example.com')
    expect(screen.getByLabelText('Back-Channel Logout URL (Optional)')).toHaveValue('https://app.example.com/backchannel-logout')

    await submit(user)
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      audience: 'https://api.example.com\nhttps://billing.example.com',
      backchannel_logout_uri: 'https://app.example.com/backchannel-logout',
    })
  })

  it('lets both be cleared', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: existing })

    await user.clear(screen.getByLabelText('API Audiences (Optional)'))
    await user.clear(screen.getByLabelText('Back-Channel Logout URL (Optional)'))
    await submit(user)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ audience: '', backchannel_logout_uri: '' })
  })

  it('rejects an audience with a space in it, and one named twice', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: { ...existing, audience: [] } })
    const field = screen.getByLabelText('API Audiences (Optional)')
    const refusal = 'Each audience must be a distinct identifier without spaces, at most 255 characters'

    await user.type(field, 'https://api.example.com billing')
    await submit(user)
    expect(await screen.findByText(refusal)).toBeInTheDocument()

    await user.clear(field)
    await user.type(field, 'https://api.example.com{enter}https://api.example.com')
    await submit(user)
    expect(await screen.findByText(refusal)).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('rejects a logout address that is not one absolute URL without a fragment', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: { ...existing, backchannel_logout_uri: '' } })
    const field = screen.getByLabelText('Back-Channel Logout URL (Optional)')

    for (const bad of ['/backchannel-logout', 'app.example.com/logout', 'https://app.example.com/logout#done', 'myapp://logout']) {
      await user.clear(field)
      await user.type(field, bad)
      await submit(user)
      expect(await screen.findByText('Enter one absolute https URL without a fragment (#…)'), bad).toBeInTheDocument()
    }
    expect(onSubmit).not.toHaveBeenCalled()

    // Whether an address may be called is the server's to say: the form lets
    // a well-formed one through and shows the server's answer.
    await user.clear(field)
    await user.type(field, 'https://app.example.com/backchannel-logout')
    await submit(user)
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
  })
})

describe('ClientForm — the home page and the sign-in start URL', () => {
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
    client_uri: 'https://app.example.com',
    initiate_login_uri: 'https://app.example.com/login',
  } as Client

  it('shows what an existing client has and submits it unchanged', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: existing })

    expect(screen.getByLabelText('Home Page')).toHaveValue('https://app.example.com')
    expect(screen.getByLabelText('Sign-In Start URL (Optional)')).toHaveValue('https://app.example.com/login')

    await submit(user)
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      client_uri: 'https://app.example.com',
      initiate_login_uri: 'https://app.example.com/login',
    })
  })

  it('takes only an absolute https URL without a fragment — users are sent there', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({ initialData: { ...existing, client_uri: '', initiate_login_uri: '' } })

    for (const label of ['Home Page', 'Sign-In Start URL (Optional)']) {
      const field = screen.getByLabelText(label)
      for (const bad of ['http://app.example.com', 'javascript:alert(1)', 'https://app.example.com/#top', '/login']) {
        await user.clear(field)
        await user.type(field, bad)
        await submit(user)
        expect(await screen.findByText('Enter one absolute https URL without a fragment (#…)'), `${label}: ${bad}`).toBeInTheDocument()
      }
      await user.clear(field)
    }
    expect(onSubmit).not.toHaveBeenCalled()
  })
})


describe('ClientForm — a client keeps a way to sign in', () => {
  it('refuses to save with every sign-in method unchecked', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm()
    await user.type(screen.getByLabelText('Client Name *'), 'Web App')
    await user.type(screen.getByPlaceholderText(/callback/i), 'https://app.example/callback')
    for (const name of [/email/i, /google/i]) {
      const box = screen.getAllByRole('checkbox', { name }).find((b) => (b as HTMLInputElement).checked)
      if (box) await user.click(box)
    }
    await submit(user)
    expect(await screen.findByText('Choose at least one way to sign in')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})

// The server refuses some input only it can judge (a grant it does not serve, a
// key address that is private). The refusal names the field; the form shows it
// there rather than only in a passing toast.
describe('ClientForm — a refusal from the server', () => {
  it('is shown at the field it names', async () => {
    renderForm({
      serverError: {
        field: 'redirect_uris',
        detail: '"app://x#y" is not a usable redirect URI',
        hint: 'Use an absolute URI without a fragment.',
      },
    })
    expect(
      await screen.findByText('"app://x#y" is not a usable redirect URI Use an absolute URI without a fragment.')
    ).toBeInTheDocument()
  })

  it('names nothing on the form when the field is not one of its own', async () => {
    renderForm({ serverError: { field: 'client_secret', detail: 'Not this form' } })
    await waitFor(() => expect(screen.queryByText('Not this form')).not.toBeInTheDocument())
  })
})
