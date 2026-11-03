import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw/http'
import { render } from '../test/utils'
import i18n from '../i18n'
import LoginPage from './LoginPage'
import { server } from '../test/mocks/server'

const mockNavigate = vi.fn()
const mockSearchParams = new URLSearchParams()
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams], useNavigate: () => mockNavigate }
})

// Following the flow's redirect leaves the page; record where it would go.
const mockFollowRedirect = vi.fn()
vi.mock('../utils/loginFlow', async () => {
  const actual = await vi.importActual<typeof import('../utils/loginFlow')>('../utils/loginFlow')
  return { ...actual, followRedirect: (...args: unknown[]) => mockFollowRedirect(...args) }
})

const API = 'http://localhost:8080/api/v1/login-flows'
const bytes = (...values: number[]) => new Uint8Array(values).buffer

const answer = {
  id: 'AQID',
  rawId: bytes(1, 2, 3),
  type: 'public-key',
  authenticatorAttachment: 'platform',
  response: { clientDataJSON: bytes(9), authenticatorData: bytes(7), signature: bytes(6), userHandle: bytes(5) },
  getClientExtensionResults: () => ({}),
}

/** The login flow of a client that offers these ways of signing in. */
function flowOffering(methods: string[]) {
  server.use(
    http.get(`${API}/:flow`, () =>
      HttpResponse.json({ next: 'form', flow: 'test-flow', client_name: 'Acme', requested_scope: [], client: { client_id: 'acme', sign_in_methods: methods } }),
    ),
  )
}

/** What the server answers to starting, and to finishing, a passkey sign-in. */
function passkeyServer(finish: () => Response | Promise<Response>) {
  const finished = vi.fn()
  server.use(
    http.post(`${API}/:flow/passkey/begin`, () => HttpResponse.json({ ceremony_id: 'c-1', options: { challenge: 'AQID', rpId: 'auth.example.com', userVerification: 'required' } })),
    http.post(`${API}/:flow/passkey/finish`, async ({ request }) => {
      finished(await request.json())
      return finish()
    }),
  )
  return finished
}

describe('LoginPage with a passkey', () => {
  const credentials = { get: vi.fn(), create: vi.fn() }
  let conditional = false

  beforeEach(() => {
    vi.clearAllMocks()
    i18n.changeLanguage('en')
    mockSearchParams.set('flow', 'test-flow')
    conditional = false
    vi.stubGlobal(
      'PublicKeyCredential',
      class {
        static isConditionalMediationAvailable = () => Promise.resolve(conditional)
      },
    )
    Object.defineProperty(navigator, 'credentials', { value: credentials, configurable: true })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
  })

  it('offers a passkey where the server lists it, and signs in with the button', async () => {
    flowOffering(['email', 'passkey'])
    const finished = passkeyServer(() => HttpResponse.json({ next: 'redirect', redirect_to: 'https://app.example/callback' }))
    credentials.get.mockResolvedValue(answer)
    render(<LoginPage />)

    await userEvent.click(await screen.findByRole('button', { name: 'Sign in with a passkey' }))

    await waitFor(() => expect(mockFollowRedirect).toHaveBeenCalledWith('https://app.example/callback'))
    // The browser was asked with the server's options, in its own buffers, in a prompt of its own.
    const request = credentials.get.mock.calls[0][0]
    expect(request.publicKey.challenge).toBeInstanceOf(ArrayBuffer)
    expect(request.mediation).toBeUndefined()
    // The server got the device's answer as WebAuthn JSON, for the ceremony it began.
    expect(finished).toHaveBeenCalledWith({
      ceremony_id: 'c-1',
      remember: false,
      credential: expect.objectContaining({ id: 'AQID', rawId: 'AQID', response: expect.objectContaining({ userHandle: 'BQ', signature: 'Bg' }) }),
    })
  })

  it('goes on to the second factor, as a password would', async () => {
    flowOffering(['email', 'passkey'])
    passkeyServer(() => HttpResponse.json({ next: 'mfa', mfa_challenge: 'm-1', flow: 'test-flow' }))
    credentials.get.mockResolvedValue(answer)
    render(<LoginPage />)

    await userEvent.click(await screen.findByRole('button', { name: 'Sign in with a passkey' }))

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/mfa/verify?flow=test-flow&mfa_challenge=m-1'))
  })

  it('says so when the server does not accept the passkey', async () => {
    flowOffering(['email', 'passkey'])
    passkeyServer(() => HttpResponse.json({ code: 'passkey_rejected', detail: 'The passkey could not be verified.' }, { status: 401 }))
    credentials.get.mockResolvedValue(answer)
    render(<LoginPage />)

    await userEvent.click(await screen.findByRole('button', { name: 'Sign in with a passkey' }))

    expect(await screen.findByText(i18n.t('auth:errors.passkeyRejected'))).toBeInTheDocument()
    expect(mockFollowRedirect).not.toHaveBeenCalled()
  })

  it('says so when the user closes the prompt, and lets them try again', async () => {
    flowOffering(['email', 'passkey'])
    const finished = passkeyServer(() => HttpResponse.json({ next: 'redirect', redirect_to: 'https://app.example/callback' }))
    credentials.get.mockRejectedValueOnce(new DOMException('closed', 'NotAllowedError')).mockResolvedValue(answer)
    render(<LoginPage />)

    await userEvent.click(await screen.findByRole('button', { name: 'Sign in with a passkey' }))
    expect(await screen.findByText(i18n.t('auth:errors.passkeyCancelled'))).toBeInTheDocument()
    expect(finished).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Sign in with a passkey' }))
    await waitFor(() => expect(mockFollowRedirect).toHaveBeenCalled())
  })

  it('offers none where the client does not list it, or the browser cannot make one', async () => {
    flowOffering(['email', 'google'])
    const { unmount } = render(<LoginPage />)
    await screen.findByLabelText('Email')
    expect(screen.queryByRole('button', { name: 'Sign in with a passkey' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('Email')).toHaveAttribute('autocomplete', 'username')
    unmount()

    flowOffering(['email', 'passkey'])
    vi.stubGlobal('PublicKeyCredential', undefined)
    render(<LoginPage />)
    await screen.findByLabelText('Email')
    expect(screen.queryByRole('button', { name: 'Sign in with a passkey' })).not.toBeInTheDocument()
  })

  it('offers the passkeys in the address field too, where the browser can, and signs in on choosing one', async () => {
    conditional = true
    flowOffering(['email', 'passkey'])
    const finished = passkeyServer(() => HttpResponse.json({ next: 'redirect', redirect_to: 'https://app.example/callback' }))
    credentials.get.mockResolvedValue(answer)
    render(<LoginPage />)

    expect(await screen.findByLabelText('Email')).toHaveAttribute('autocomplete', 'username webauthn')
    await waitFor(() => expect(mockFollowRedirect).toHaveBeenCalledWith('https://app.example/callback'))
    expect(credentials.get.mock.calls[0][0].mediation).toBe('conditional')
    expect(finished).toHaveBeenCalledTimes(1)
  })

  it('does not wait behind the field where the browser cannot', async () => {
    conditional = false
    flowOffering(['email', 'passkey'])
    passkeyServer(() => HttpResponse.json({}))
    render(<LoginPage />)

    await screen.findByLabelText('Email')
    // Give an unwanted request time to be made.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(credentials.get).not.toHaveBeenCalled()
  })
})
