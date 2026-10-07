import { describe, it, expect, vi, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import MagicLinkPage from './MagicLinkPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams], useNavigate: () => vi.fn() }
})

describe('MagicLinkPage', () => {
  const user = userEvent.setup()

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
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

  it('explains a link that can no longer be used', async () => {
    mockSearchParams.set('token', 'spent')
    server.use(
      http.post('http://localhost:8080/api/v1/magic-links/inspect', () =>
        HttpResponse.json({ valid: false, error: 'magic link has already been used' })
      )
    )

    render(<MagicLinkPage />)

    expect(await screen.findByText('magic link has already been used')).toBeInTheDocument()
  })
})
