import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import LoginPage from './LoginPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw'

// Mock useSearchParams and useNavigate
const mockNavigate = vi.fn()
const mockSearchParams = new URLSearchParams()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return {
    ...actual,
    useSearchParams: () => [mockSearchParams],
    useNavigate: () => mockNavigate,
  }
})

// Mock GoogleLoginButton
vi.mock('../components/GoogleLoginButton', () => ({
  default: ({ onError, disabled, clientId }: any) => (
    <button
      data-testid="google-login-button"
      disabled={disabled}
      onClick={() => onError('Google login error')}
    >
      Google로 로그인 {clientId ? `(${clientId})` : ''}
    </button>
  )
}))

// Vite env vars (VITE_API_URL / VITE_AUTH_BACKEND_URL) are provided globally
// via vi.stubEnv in src/test/setup.ts.

describe('LoginPage', () => {
  const user = userEvent.setup()

  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.set('login_challenge', 'test-challenge')
  })

  afterEach(() => {
    // URLSearchParams has no .clear() in this runtime; delete each key.
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
  })

  describe('Initial Loading and Error States', () => {
    it('shows loading spinner initially', () => {
      render(<LoginPage />)

      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
    })

    it('shows the Authway home fallback when login_challenge is missing', async () => {
      // Direct access without an OAuth login_challenge is not part of the
      // normal flow; the page renders a plain Authway landing instead of a form.
      mockSearchParams.delete('login_challenge')

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('Authway')).toBeInTheDocument()
      })
    })

    it('shows error when login challenge fetch fails', async () => {
      server.use(
        http.get('http://localhost:8080/auth/google/login', () => {
          return HttpResponse.json({ error: 'Server error' }, { status: 500 })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('Server error')).toBeInTheDocument()
      })
    })
  })

  describe('Successful Login Challenge Fetch', () => {
    beforeEach(() => {
      server.use(
        http.get('http://localhost:8080/auth/google/login', () => {
          return HttpResponse.json({
            challenge: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email'],
            client: { client_id: 'test-client-id' }
          })
        })
      )
    })

    it('renders login form with client info', async () => {
      render(<LoginPage />)

      await waitFor(() => {
        // Title and submit button share the text '로그인'; disambiguate by role.
        expect(screen.getByRole('heading', { name: '로그인' })).toBeInTheDocument()
        // The subtitle must read as one full sentence with the client name emphasized.
        const subtitle = screen.getByText(
          (_, el) => el?.tagName === 'P' && el.textContent === 'Test App에 로그인하시겠습니까?'
        )
        expect(within(subtitle).getByText('Test App')).toHaveClass('font-medium')
        expect(screen.getByText('요청된 권한: openid, email')).toBeInTheDocument()
      })

      expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호')).toBeInTheDocument()
      expect(screen.getByLabelText('로그인 상태 유지')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '로그인' })).toBeInTheDocument()
    })

    it('renders the subtitle in English word order (client name mid-sentence)', async () => {
      // Regression: en places {{clientName}} mid-sentence; prepending the name
      // to a name-stripped template rendered "All.ModelsSign in to ?" in prod.
      await i18n.changeLanguage('en')
      try {
        render(<LoginPage />)

        await waitFor(() => {
          expect(
            screen.getByText(
              (_, el) => el?.tagName === 'P' && el.textContent === 'Sign in to Test App?'
            )
          ).toBeInTheDocument()
        })
      } finally {
        await i18n.changeLanguage('ko')
      }
    })

    it('falls back to a generic subtitle when client_name is missing', async () => {
      server.use(
        http.get('http://localhost:8080/auth/google/login', () => {
          return HttpResponse.json({
            challenge: 'test-challenge',
            client_name: '',
            requested_scope: ['openid'],
            client: { client_id: 'test-client-id' }
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('계속하려면 로그인하세요')).toBeInTheDocument()
      })
    })

    it('shows Google login button with client ID', async () => {
      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByTestId('google-login-button')).toBeInTheDocument()
        expect(screen.getByText('Google로 로그인 (test-client-id)')).toBeInTheDocument()
      })
    })
  })

  describe('Form Validation', () => {
    beforeEach(async () => {
      server.use(
        http.get('http://localhost:8080/auth/google/login', () => {
          return HttpResponse.json({
            challenge: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email']
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: '로그인' })).toBeInTheDocument()
      })
    })

    it('validates email field', async () => {
      const emailInput = screen.getByLabelText('이메일')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'invalid-email')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('올바른 이메일을 입력해주세요')).toBeInTheDocument()
      })
    })

    it('validates password field', async () => {
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, '123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('비밀번호는 최소 6자 이상이어야 합니다')).toBeInTheDocument()
      })
    })
  })

  describe('Login Submission', () => {
    beforeEach(async () => {
      server.use(
        http.get('http://localhost:8080/auth/google/login', () => {
          return HttpResponse.json({
            challenge: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email']
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: '로그인' })).toBeInTheDocument()
      })
    })

    it('handles successful login with redirect', async () => {
      server.use(
        http.post('http://localhost:8080/authenticate', () => {
          return HttpResponse.json({ redirect_to: 'http://example.com/callback' })
        })
      )

      // Mock window.location.href
      delete (window as any).location
      window.location = { ...window.location, href: '' }

      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(window.location.href).toBe('http://example.com/callback')
      })
    })

    it('handles login error from server', async () => {
      server.use(
        http.post('http://localhost:8080/authenticate', () => {
          return HttpResponse.json({ error: 'Invalid credentials' })
        })
      )

      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'wrongpassword')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('Invalid credentials')).toBeInTheDocument()
      })
    })

    it('navigates to the MFA verify page when the server requires a second factor', async () => {
      server.use(
        http.post('http://localhost:8080/authenticate', () => {
          return HttpResponse.json({ mfa_required: true, mfa_challenge: 'chal-123' })
        })
      )

      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/mfa/verify?mfa_challenge=chal-123')
      })
    })

    it('sends correct request data', async () => {
      let requestBody: any

      server.use(
        http.post('http://localhost:8080/authenticate', async ({ request }) => {
          requestBody = await request.json()
          return HttpResponse.json({ redirect_to: 'http://example.com/callback' })
        })
      )

      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const rememberCheckbox = screen.getByLabelText('로그인 상태 유지')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.click(rememberCheckbox)
      await user.click(submitButton)

      await waitFor(() => {
        expect(requestBody).toEqual({
          challenge: 'test-challenge',
          email: 'test@example.com',
          password: 'password123',
          remember: true
        })
      })
    })
  })

  // Navigation-to-register tests removed: onboarding is invitation-only, so the
  // login page no longer renders a public sign-up CTA.
})