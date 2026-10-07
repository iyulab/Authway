import { describe, it, expect, vi, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { render } from '../test/utils'
import LogoutPage from './LogoutPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams] }
})

describe('LogoutPage', () => {
  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
  })

  it('completes the logout flow once and follows the redirect', async () => {
    mockSearchParams.set('flow', 'lf-1=')
    const requested: string[] = []
    server.use(
      http.post('http://localhost:8080/api/v1/logout-flows/:flow', ({ request }) => {
        requested.push(new URL(request.url).pathname)
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/signed-out' })
      })
    )
    delete (window as any).location
    window.location = { ...window.location, href: '' }

    render(<LogoutPage />)

    await waitFor(() => {
      expect(window.location.href).toBe('http://example.com/signed-out')
    })
    expect(requested).toEqual(['/api/v1/logout-flows/lf-1%3D'])
  })

  it('shows the backend error when the flow cannot be completed', async () => {
    mockSearchParams.set('flow', 'spent')
    server.use(
      http.post('http://localhost:8080/api/v1/logout-flows/:flow', () =>
        HttpResponse.json({ error: 'This sign-in has expired or was already completed.', code: 'flow_expired' }, { status: 410 })
      )
    )

    render(<LogoutPage />)

    expect(await screen.findByText('This sign-in has expired or was already completed.')).toBeInTheDocument()
  })
})
