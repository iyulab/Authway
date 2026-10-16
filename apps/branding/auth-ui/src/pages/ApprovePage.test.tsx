import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import ApprovePage from './ApprovePage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams] }
})

// The SDK the page signs in with: a signed-in user unless a test says otherwise.
const sdk = { signedIn: true, logins: [] as unknown[] }
vi.mock('@authway/client', () => ({
  AuthwayClient: class {
    isAuthenticated = async () => sdk.signedIn
    checkSession = async () => false
    loginWithRedirect = async (options?: unknown) => {
      sdk.logins.push(options ?? {})
    }
    getUser = async () => ({ email: 'person@example.com' })
    getAccessToken = async () => 'user-token'
  },
}))

const API = 'http://localhost:8080'
const REQUEST = '7f9c2a40-0000-4000-8000-000000000001'
const entry = { client_id: 'authway_account', redirect_uri: 'http://localhost:3001/account/callback', scope: 'openid email', app: { name: 'Mate', client_uri: 'https://mate.example' } }
const problem = (status: number, code: string) =>
  HttpResponse.json({ type: 'about:blank', title: 'x', status, detail: code, code }, { status, headers: { 'Content-Type': 'application/problem+json' } })

// A user away from an application that acts for them is asked, by a link the
// application sends, to approve one action: they sign in to their account,
// compare the code, and answer.
describe('ApprovePage', () => {
  const user = userEvent.setup()
  let status: string

  beforeEach(() => {
    i18n.changeLanguage('en')
    status = 'pending'
    Object.assign(sdk, { signedIn: true, logins: [] })
    mockSearchParams.set('request', REQUEST)
    mockSearchParams.set('client_id', 'mate')
    server.use(
      http.get(`${API}/api/v1/account-entry`, () => HttpResponse.json(entry)),
      http.get(`${API}/api/v1/users/approvals/:id`, ({ params }) =>
        params.id === REQUEST
          ? HttpResponse.json({
              id: REQUEST,
              client_id: 'mate',
              client_name: 'Mate',
              binding_message: 'K-7Q2',
              scopes: ['mail.send'],
              authorization_details: [{ type: 'mail', ref: 'opaque-1' }, { type: 'mail', ref: 'opaque-2' }],
              status,
              created_at: '2026-10-16T12:00:00Z',
              expires_at: '2026-10-16T12:05:00Z',
            })
          : problem(404, 'not_found')),
    )
  })

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
    i18n.changeLanguage('ko')
  })

  it('shows the code to compare, the kind of action and the permissions — not the action itself', async () => {
    render(<ApprovePage />)
    expect(await screen.findByLabelText('Code')).toHaveTextContent('K-7Q2')
    expect(screen.getByText('Check that Mate shows this same code:')).toBeInTheDocument()
    expect(screen.getByRole('list', { name: 'Kind of action' })).toHaveTextContent('mail')
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
    expect(screen.getByText('mail.send')).toBeInTheDocument()
    expect(screen.queryByText('opaque-1')).not.toBeInTheDocument()
  })

  it('approves', async () => {
    let answered = ''
    server.use(http.post(`${API}/api/v1/users/approvals/:id/:decision`, ({ params }) => {
      answered = params.decision as string
      return new HttpResponse(null, { status: 204 })
    }))
    render(<ApprovePage />)
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Approved. Mate can now carry out the action.')
    expect(answered).toBe('approve')
  })

  it('denies', async () => {
    server.use(http.post(`${API}/api/v1/users/approvals/:id/deny`, () => new HttpResponse(null, { status: 204 })))
    render(<ApprovePage />)
    await user.click(await screen.findByRole('button', { name: 'Deny' }))
    expect(await screen.findByRole('status')).toHaveTextContent('Denied.')
  })

  it('asks for a fresh sign-in when the last one is too old, and comes back here', async () => {
    server.use(http.post(`${API}/api/v1/users/approvals/:id/approve`, () => problem(401, 'insufficient_user_authentication')))
    render(<ApprovePage />)
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    await user.click(await screen.findByRole('button', { name: 'Sign in again' }))
    expect(sdk.logins).toEqual([{ prompt: 'login' }])
    const stored = JSON.parse(vi.mocked(window.sessionStorage.setItem).mock.calls.at(-1)![1] as string) as { returnTo: string }
    expect(stored.returnTo).toBe(`/approve?request=${REQUEST}&client_id=mate`)
  })

  it('says when the request was answered meanwhile, expired, or is not the user’s', async () => {
    server.use(http.post(`${API}/api/v1/users/approvals/:id/approve`, () => problem(409, 'approval_settled')))
    const { unmount } = render(<ApprovePage />)
    await user.click(await screen.findByRole('button', { name: 'Approve' }))
    expect(await screen.findByRole('status')).toHaveTextContent('This request was already answered.')
    unmount()

    status = 'expired'
    const expired = render(<ApprovePage />)
    expect(await screen.findByRole('status')).toHaveTextContent('This request has expired.')
    expired.unmount()

    mockSearchParams.set('request', 'someone-elses')
    render(<ApprovePage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('There is no such request for you.')
  })

  it('signs in first when there is no session', async () => {
    sdk.signedIn = false
    render(<ApprovePage />)
    await waitFor(() => expect(sdk.logins).toHaveLength(1))
  })

  it('needs the link the application gave', async () => {
    mockSearchParams.delete('request')
    render(<ApprovePage />)
    expect(await screen.findByText('Open this page from the link the application gave you.')).toBeInTheDocument()
  })
})
