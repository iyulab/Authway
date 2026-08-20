import { describe, it, expect, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@/test/utils'
import { ClientForm } from './ClientForm'
import type { Client } from '@/lib/api'

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
        'At least one Redirect URI is required for Authorization Code / Implicit'
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
