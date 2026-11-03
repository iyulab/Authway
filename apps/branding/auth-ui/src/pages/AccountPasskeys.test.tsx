import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import AccountPasskeys from './AccountPasskeys'

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const bytes = (...values: number[]) => new Uint8Array(values).buffer

const laptop = { id: 'p-1', name: 'Work laptop', created_at: '2026-10-01T09:00:00Z', last_used_at: '2026-11-02T09:00:00Z', synced: true }
const phone = { id: 'p-2', name: 'Phone', created_at: '2026-10-05T09:00:00Z', last_used_at: null, synced: false }

describe('AccountPasskeys', () => {
  const user = userEvent.setup()
  const credentials = { create: vi.fn(), get: vi.fn() }

  beforeEach(() => {
    vi.clearAllMocks()
    i18n.changeLanguage('en')
    vi.stubGlobal('PublicKeyCredential', class {})
    Object.defineProperty(navigator, 'credentials', { value: credentials, configurable: true })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('lists the passkeys with when they were added and last used', async () => {
    const call = vi.fn(async () => json(200, { passkeys: [laptop, phone] }))
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    expect(await screen.findByText('Work laptop')).toBeInTheDocument()
    expect(screen.getByText('Phone')).toBeInTheDocument()
    expect(screen.getByText(/Synced across your devices/)).toBeInTheDocument()
    expect(screen.getByText(/Not used yet/)).toBeInTheDocument()
  })

  it('shows nothing where the server has no passkeys', async () => {
    const call = vi.fn(async () => json(404, { code: 'passkeys_unavailable' }))
    const { container } = render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await waitFor(() => expect(call).toHaveBeenCalled())
    expect(container.querySelector('section')).toBeNull()
  })

  it('adds a passkey on this device: the server begins, the device creates, the server saves', async () => {
    let passkeys = [laptop]
    const calls: Array<[string, RequestInit | undefined]> = []
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      calls.push([path, init])
      if (path.endsWith('/register/begin')) {
        return json(200, { ceremony_id: 'c-1', options: { challenge: 'AQID', user: { id: 'BAUG', name: 'pat@example.com', displayName: 'pat@example.com' } } })
      }
      if (path.endsWith('/register/finish')) {
        passkeys = [...passkeys, phone]
        return json(201, { passkey: phone })
      }
      return json(200, { passkeys })
    })
    credentials.create.mockResolvedValue({
      id: 'AQID',
      rawId: bytes(1, 2, 3),
      type: 'public-key',
      response: { clientDataJSON: bytes(9), attestationObject: bytes(8), getTransports: () => ['internal'] },
      getClientExtensionResults: () => ({}),
    })
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await screen.findByText('Work laptop')
    await user.type(screen.getByLabelText('Name this passkey'), '  Phone ')
    await user.click(screen.getByRole('button', { name: 'Add a passkey' }))

    expect(await screen.findByText('Passkey added.')).toBeInTheDocument()
    expect(screen.getAllByText('Phone').length).toBeGreaterThan(0)
    const finish = calls.find(([path]) => path.endsWith('/register/finish'))!
    expect(JSON.parse(finish[1]!.body as string)).toEqual({
      ceremony_id: 'c-1',
      name: 'Phone',
      credential: expect.objectContaining({ id: 'AQID', response: expect.objectContaining({ attestationObject: 'CA', transports: ['internal'] }) }),
    })
  })

  it('says nothing was added when the user closes the prompt, and tells the server nothing', async () => {
    const call = vi.fn(async (path: string) =>
      path.endsWith('/register/begin')
        ? json(200, { ceremony_id: 'c-1', options: { challenge: 'AQID', user: { id: 'BAUG', name: 'a', displayName: 'a' } } })
        : json(200, { passkeys: [laptop] }),
    )
    credentials.create.mockRejectedValue(new DOMException('closed', 'NotAllowedError'))
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await user.click(await screen.findByRole('button', { name: 'Add a passkey' }))

    expect(await screen.findByText('No passkey was added.')).toBeInTheDocument()
    expect(call.mock.calls.some(([path]) => String(path).endsWith('/register/finish'))).toBe(false)
  })

  it('says the device already has a passkey, rather than that adding failed', async () => {
    const call = vi.fn(async (path: string) =>
      path.endsWith('/register/begin')
        ? json(200, { ceremony_id: 'c-1', options: { challenge: 'AQID', user: { id: 'BAUG', name: 'a', displayName: 'a' }, excludeCredentials: [{ id: 'AQID', type: 'public-key' }] } })
        : json(200, { passkeys: [laptop] }),
    )
    credentials.create.mockRejectedValue(new DOMException('exists', 'InvalidStateError'))
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await user.click(await screen.findByRole('button', { name: 'Add a passkey' }))

    expect(await screen.findByText('This device already has a passkey for this account.')).toBeInTheDocument()
  })

  it('sends the user through sign-in again when a change needs a recent one', async () => {
    const signInAgain = vi.fn()
    const call = vi.fn(async (path: string, init?: RequestInit) =>
      init?.method === 'POST' ? json(401, { code: 'insufficient_user_authentication' }) : json(200, { passkeys: [laptop] }),
    )
    render(<AccountPasskeys call={call} signInAgain={signInAgain} />)

    await user.click(await screen.findByRole('button', { name: 'Add a passkey' }))
    expect(await screen.findByText('To change your passkeys, sign in again first.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18n.t('auth:account.signInAgainButton') }))
    expect(signInAgain).toHaveBeenCalledWith(true)
    expect(credentials.create).not.toHaveBeenCalled()
  })

  it('removes a passkey after confirming, and says when it is the only way to sign in', async () => {
    let passkeys = [laptop, phone]
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        if (path.endsWith('/p-2')) {
          passkeys = passkeys.filter((p) => p.id !== 'p-2')
          return json(200, { message: 'Passkey removed' })
        }
        return json(409, { code: 'last_sign_in_method' })
      }
      return json(200, { passkeys })
    })
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await screen.findByText('Phone')
    const removes = screen.getAllByRole('button', { name: 'Remove' })
    await user.click(removes[1])
    await user.click(screen.getByRole('button', { name: 'Remove Phone' }))
    await waitFor(() => expect(screen.queryByText('Phone')).not.toBeInTheDocument())
    expect(call).toHaveBeenCalledWith('/api/v1/users/passkeys/p-2', { method: 'DELETE' })

    await user.click(screen.getByRole('button', { name: 'Remove' }))
    await user.click(screen.getByRole('button', { name: 'Remove Work laptop' }))
    expect(await screen.findByText('This is the only way you sign in. Set a password first.')).toBeInTheDocument()
  })

  it('cannot add in a browser that cannot make a passkey, but still lists them', async () => {
    vi.stubGlobal('PublicKeyCredential', undefined)
    const call = vi.fn(async () => json(200, { passkeys: [laptop] }))
    render(<AccountPasskeys call={call} signInAgain={vi.fn()} />)

    await screen.findByText('Work laptop')
    expect(screen.queryByRole('button', { name: 'Add a passkey' })).not.toBeInTheDocument()
  })
})
