import { describe, it, expect, vi, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import MFAVerifyPage from './MFAVerifyPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return {
    ...actual,
    useSearchParams: () => [mockSearchParams],
    useNavigate: () => vi.fn(),
  }
})

describe('MFAVerifyPage', () => {
  const user = userEvent.setup()

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
  })

  it('sends the code with the mfa_challenge to the flow it belongs to', async () => {
    mockSearchParams.set('flow', 'flow-1=')
    mockSearchParams.set('mfa_challenge', 'chal-123')
    let requestPath = ''
    let requestBody: unknown
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/mfa', async ({ request }) => {
        requestPath = new URL(request.url).pathname
        requestBody = await request.json()
        return HttpResponse.json({ error: 'invalid verification code', code: 'invalid_code' }, { status: 401 })
      })
    )

    render(<MFAVerifyPage />)
    await user.type(screen.getByPlaceholderText('000000'), '123456')
    await user.click(screen.getByRole('button', { name: /verify|확인|인증/i }))

    await waitFor(() => {
      expect(requestPath).toBe('/api/v1/login-flows/flow-1%3D/mfa')
      expect(requestBody).toEqual({ mfa_challenge: 'chal-123', code: '123456' })
    })
    expect(await screen.findByText('invalid verification code')).toBeInTheDocument()
  })

  it('explains an expired challenge when the flow or mfa_challenge is missing', () => {
    mockSearchParams.set('mfa_challenge', 'chal-123')

    render(<MFAVerifyPage />)

    expect(screen.queryByPlaceholderText('000000')).not.toBeInTheDocument()
  })
})
