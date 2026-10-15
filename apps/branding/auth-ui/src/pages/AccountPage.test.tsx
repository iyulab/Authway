import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import AccountPage from './AccountPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams] }
})

// The SDK the page signs in with: a signed-in user unless a test says otherwise.
const sdk = { signedIn: true, session: false, logins: 0, configs: [] as unknown[] }
vi.mock('@authway/client', () => ({
  AuthwayClient: class {
    constructor(config: unknown) {
      sdk.configs.push(config)
    }
    isAuthenticated = async () => sdk.signedIn
    checkSession = async () => sdk.session
    loginWithRedirect = async () => {
      sdk.logins++
    }
    getUser = async () => ({ email: 'person@example.com' })
    getAccessToken = async () => 'user-token'
  },
}))

const API = 'http://localhost:8080'
const entry = { client_id: 'authway_account', redirect_uri: 'http://localhost:3001/account/callback', scope: 'openid email profile', app: { name: 'Notes', client_uri: 'https://notes.example' } }
const problem = (status: number, code: string) =>
  HttpResponse.json({ type: 'about:blank', title: 'x', status, detail: code, code }, { status, headers: { 'Content-Type': 'application/problem+json' } })

// A user arrives from an application, is signed in with the tenant's account
// client, and turns the second factor on and off themselves.
describe('AccountPage', () => {
  const user = userEvent.setup()
  let enabled: boolean

  beforeEach(() => {
    i18n.changeLanguage('en')
    enabled = false
    Object.assign(sdk, { signedIn: true, session: false, logins: 0, configs: [] })
    vi.mocked(window.sessionStorage.setItem).mockClear()
    server.use(
      http.get(`${API}/api/v1/account-entry`, ({ request }) =>
        new URL(request.url).searchParams.get('client_id') === 'notes' ? HttpResponse.json(entry) : problem(404, 'unknown_client')),
      http.get(`${API}/api/v1/users/mfa/status`, ({ request }) =>
        request.headers.get('Authorization') === 'Bearer user-token'
          ? HttpResponse.json({ enabled, recovery_codes_left: enabled ? 10 : 0 })
          : problem(401, 'unauthorized')),
      http.post(`${API}/api/v1/users/mfa/setup`, () =>
        HttpResponse.json({ secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/x', qr_image: 'data:image/png;base64,iVBORw0KGgo=', issuer: 'Authway', account_name: 'person@example.com' })),
    )
  })

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
    i18n.changeLanguage('ko')
  })

  it('asks to be opened from an application when it names none', async () => {
    render(<AccountPage />)
    expect(await screen.findByText('Open this page from the application you use.')).toBeInTheDocument()
  })

  it('says so when the application is not registered', async () => {
    mockSearchParams.set('client_id', 'nobody')
    render(<AccountPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('The application that sent you here is not registered.')
  })

  it('signs in with the account client when there is no session', async () => {
    mockSearchParams.set('client_id', 'notes')
    sdk.signedIn = false
    render(<AccountPage />)
    await waitFor(() => expect(sdk.logins).toBe(1))
    expect(sdk.configs.at(-1)).toMatchObject({ clientId: 'authway_account', redirectUri: entry.redirect_uri, cacheLocation: 'sessionstorage' })
    // Kept for the comeback to /account/callback (the test environment's storage is a mock).
    expect(window.sessionStorage.setItem).toHaveBeenCalledWith('authway.account.entry', JSON.stringify({ appClientId: 'notes', entry }))
  })

  it('turns the second factor on: QR code and key, a code, then the recovery codes', async () => {
    mockSearchParams.set('client_id', 'notes')
    let sentCode: unknown
    server.use(
      http.post(`${API}/api/v1/users/mfa/verify`, async ({ request }) => {
        sentCode = ((await request.json()) as { code: string }).code
        enabled = true
        return HttpResponse.json({ recovery_codes: ['aaaa-bbbb', 'cccc-dddd'], sessions_ended: 'others' })
      }),
    )
    render(<AccountPage />)
    expect(await screen.findByText('person@example.com')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Back to Notes' })).toHaveAttribute('href', 'https://notes.example/')

    await user.click(await screen.findByRole('button', { name: 'Turn on two-step verification' }))
    expect(await screen.findByAltText('QR code for your authenticator app')).toHaveAttribute('src', 'data:image/png;base64,iVBORw0KGgo=')
    expect(screen.getByText('JBSWY3DPEHPK3PXP')).toBeInTheDocument()

    await user.type(screen.getByLabelText('6-digit code'), ' 123456 ')
    await user.click(screen.getByRole('button', { name: 'Turn on' }))

    expect(await screen.findByRole('list', { name: 'Recovery codes' })).toHaveTextContent('aaaa-bbbb')
    expect(sentCode).toBe('123456')
    expect(await screen.findByText('Two-step verification is on. 10 recovery codes left.')).toBeInTheDocument()
  })

  it('asks to sign in again when turning it on ended this session too', async () => {
    mockSearchParams.set('client_id', 'notes')
    server.use(http.post(`${API}/api/v1/users/mfa/verify`, () => HttpResponse.json({ recovery_codes: ['aaaa-bbbb'], sessions_ended: 'all' })))
    render(<AccountPage />)
    await user.click(await screen.findByRole('button', { name: 'Turn on two-step verification' }))
    await user.type(await screen.findByLabelText('6-digit code'), '123456')
    await user.click(screen.getByRole('button', { name: 'Turn on' }))

    expect(await screen.findByRole('list', { name: 'Recovery codes' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Sign in again' }))
    expect(sdk.logins).toBe(1)
  })

  it('turns it off only with a right code', async () => {
    mockSearchParams.set('client_id', 'notes')
    enabled = true
    server.use(
      http.delete(`${API}/api/v1/users/mfa`, async ({ request }) => {
        if (((await request.json()) as { code: string }).code !== '654321') return problem(400, 'invalid_code')
        enabled = false
        return HttpResponse.json({ message: 'off' })
      }),
    )
    render(<AccountPage />)
    await user.click(await screen.findByRole('button', { name: 'Turn off' }))
    await user.type(screen.getByLabelText('6-digit code'), '111111')
    await user.click(screen.getByRole('button', { name: 'Turn off' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('That code is not right.')

    await user.clear(screen.getByLabelText('6-digit code'))
    await user.type(screen.getByLabelText('6-digit code'), '654321')
    await user.click(screen.getByRole('button', { name: 'Turn off' }))
    expect(await screen.findByRole('button', { name: 'Turn on two-step verification' })).toBeInTheDocument()
  })

  it('offers no way back to an address that is not https', async () => {
    mockSearchParams.set('client_id', 'notes')
    server.use(http.get(`${API}/api/v1/account-entry`, () => HttpResponse.json({ ...entry, app: { name: 'Notes', client_uri: 'javascript:alert(1)' } })))
    render(<AccountPage />)
    expect(await screen.findByText('person@example.com')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Back to Notes' })).not.toBeInTheDocument()
  })
})
