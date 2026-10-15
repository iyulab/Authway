import { describe, it, expect, vi, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import MagicLinkPage from './MagicLinkPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()
const mockNavigate = vi.fn()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams], useNavigate: () => mockNavigate }
})

describe('MagicLinkPage', () => {
  const user = userEvent.setup()

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
    mockNavigate.mockReset()
    vi.mocked(window.localStorage.getItem).mockReset()
    vi.mocked(window.localStorage.setItem).mockClear()
  })

  it('sends a sign-in link for the login flow', async () => {
    mockSearchParams.set('flow', 'flow-1=')
    let sent: { path: string; body: unknown } | null = null
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/magic-link', async ({ request }) => {
        sent = { path: new URL(request.url).pathname, body: await request.json() }
        return HttpResponse.json({ next: 'email_sent' })
      })
    )

    render(<MagicLinkPage />)
    await user.type(screen.getByRole('textbox'), 'user@example.com')
    await user.click(screen.getByRole('button', { name: /link|링크/i }))

    await screen.findByText('user@example.com')
    expect(sent).toEqual({ path: '/api/v1/login-flows/flow-1%3D/magic-link', body: { email: 'user@example.com' } })
    // Remembered, so that the link opened in this browser can finish the sign-in.
    expect(window.localStorage.setItem).toHaveBeenCalledWith('authway.magicLinkFlows', JSON.stringify(['flow-1=']))
  })

  it('does not use the link up until the user continues, then completes the login', async () => {
    mockSearchParams.set('token', 'tok-1')
    const calls: string[] = []
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () => {
        calls.push('inspect')
        return HttpResponse.json({ valid: true, email: 'user@example.com' })
      }),
      http.post('http://localhost:8080/api/v1/magic-links/redeem', () => {
        calls.push('redeem')
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/after-login' })
      })
    )
    delete (window as any).location
    window.location = { ...window.location, href: '' }

    render(<MagicLinkPage />)

    expect(await screen.findByText('user@example.com')).toBeInTheDocument()
    expect(calls).toEqual(['inspect'])

    await user.click(screen.getByRole('button', { name: /continue|계속/i }))
    await waitFor(() => expect(window.location.href).toBe('http://example.com/after-login'))
    expect(calls).toEqual(['inspect', 'redeem'])
  })

  // The link is one factor. A user who also has an authenticator is taken to
  // the code step — not signed in, and not shown an error.
  it('goes on to the second factor step when the user has one', async () => {
    mockSearchParams.set('token', 'tok-1')
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () =>
        HttpResponse.json({ valid: true, email: 'user@example.com' })
      ),
      http.post('http://localhost:8080/api/v1/magic-links/redeem', () =>
        HttpResponse.json({ next: 'mfa', mfa_challenge: 'chal-1', flow: 'flow-1=' })
      )
    )
    delete (window as any).location
    window.location = { ...window.location, href: '' }

    render(<MagicLinkPage />)
    await screen.findByText('user@example.com')
    await user.click(screen.getByRole('button', { name: /continue|계속/i }))

    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/mfa/verify?flow=flow-1%3D&mfa_challenge=chal-1'))
    expect(window.location.href).toBe('')
  })

  // The email also carries a code: read on a phone, it is typed here, in the
  // browser that asked for it, and signs in like the link.
  it('takes the code from the email in the browser that asked for it', async () => {
    mockSearchParams.set('flow', 'flow-1=')
    const codes: unknown[] = []
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/magic-link', () => HttpResponse.json({ next: 'email_sent' })),
      http.post('http://localhost:8080/api/v1/login-flows/:flow/magic-link/code', async ({ request }) => {
        const body = (await request.json()) as { code: string }
        codes.push(body)
        if (body.code !== 'BCDF-GHJK') {
          return HttpResponse.json({ status: 400, code: 'invalid_code', detail: 'this code is not valid' }, { status: 400 })
        }
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/after-code' })
      })
    )
    delete (window as any).location
    window.location = { ...window.location, href: '' }

    render(<MagicLinkPage />)
    await user.type(screen.getByRole('textbox'), 'user@example.com')
    await user.click(screen.getByRole('button', { name: /link|링크/i }))
    const input = await screen.findByLabelText(/code from the email|메일의 코드/i)

    await user.type(input, 'bcdf-ghjx')
    await user.click(screen.getByRole('button', { name: /continue|계속/i }))
    expect(await screen.findByText('this code is not valid')).toBeInTheDocument()

    await user.clear(input)
    await user.type(input, 'bcdf-ghjk')
    await user.click(screen.getByRole('button', { name: /continue|계속/i }))
    await waitFor(() => expect(window.location.href).toBe('http://example.com/after-code'))
    expect(codes).toEqual([{ code: 'BCDF-GHJX' }, { code: 'BCDF-GHJK' }])
  })

  // Opened in another browser — the email read on a phone — the link cannot
  // finish the sign-in, so the page shows its code and uses nothing up.
  it('shows the code where the link cannot finish the sign-in', async () => {
    vi.mocked(window.localStorage.getItem).mockReturnValue(JSON.stringify(['flow-mine']))
    mockSearchParams.set('token', 'tok-1')
    mockSearchParams.set('code', 'BCDF-GHJK')
    const calls: string[] = []
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () => {
        calls.push('inspect')
        return HttpResponse.json({ valid: true, email: 'user@example.com', flow: 'flow-elsewhere' })
      }),
      http.post('http://localhost:8080/api/v1/magic-links/redeem', () => {
        calls.push('redeem')
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/x' })
      })
    )

    render(<MagicLinkPage />)

    expect(await screen.findByTestId('link-code')).toHaveTextContent('BCDF-GHJK')
    expect(screen.queryByRole('button', { name: /continue|계속/i })).not.toBeInTheDocument()
    expect(calls).toEqual(['inspect'])
  })

  it('offers to continue in the browser that asked for the link', async () => {
    vi.mocked(window.localStorage.getItem).mockImplementation((key) =>
      key === 'authway.magicLinkFlows' ? JSON.stringify(['flow-here']) : null
    )
    mockSearchParams.set('token', 'tok-1')
    mockSearchParams.set('code', 'BCDF-GHJK')
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () =>
        HttpResponse.json({ valid: true, email: 'user@example.com', flow: 'flow-here' })
      )
    )

    render(<MagicLinkPage />)

    expect(await screen.findByRole('button', { name: /continue|계속/i })).toBeInTheDocument()
    expect(screen.queryByTestId('link-code')).not.toBeInTheDocument()
  })

  it('explains a link that can no longer be used', async () => {
    mockSearchParams.set('token', 'spent')
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () =>
        HttpResponse.json({ valid: false, detail: 'magic link has already been used' })
      )
    )

    render(<MagicLinkPage />)

    expect(await screen.findByText('magic link has already been used')).toBeInTheDocument()
  })
})
