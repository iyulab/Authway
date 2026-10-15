import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import LoginPage from './LoginPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

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

// Social sign-in leaves the page; record where it would go instead.
const mockStartSocialSignIn = vi.fn()
vi.mock('../utils/loginFlow', async () => {
  const actual = await vi.importActual<typeof import('../utils/loginFlow')>('../utils/loginFlow')
  return { ...actual, startSocialSignIn: (...args: unknown[]) => mockStartSocialSignIn(...args) }
})

// Vite env vars (VITE_API_URL) is provided globally
// via vi.stubEnv in src/test/setup.ts.

describe('LoginPage', () => {
  const user = userEvent.setup()

  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.set('flow', 'test-challenge')
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

    it('tells someone who opened it without a flow to sign in from their app', async () => {
      // A bookmark, a reload after the flow ended, or a link from outside an
      // app: there is no sign-in to do here, so the page says where to go
      // instead of showing a bare title.
      mockSearchParams.delete('flow')

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('Authway')).toBeInTheDocument()
      })
      expect(screen.getByText(i18n.t('auth:login.noFlow.title'))).toBeInTheDocument()
      expect(screen.getByText(i18n.t('auth:login.noFlow.message'))).toBeInTheDocument()
      expect(document.querySelector('form')).toBeNull()
    })

    it('shows error when login challenge fetch fails', async () => {
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({ detail: 'Server error' }, { status: 500 })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('Server error')).toBeInTheDocument()
      })
    })
  })

  it('asks the backend about the flow with the id escaped in the path', async () => {
    // Hydra challenges are padded base64 and end in "=".
    mockSearchParams.set('flow', 'abc+/==')
    let requested = ''
    server.use(
      http.get('http://localhost:8080/api/v1/login-flows/:flow', ({ request }) => {
        requested = new URL(request.url).pathname
        return HttpResponse.json({ next: 'form', flow: 'abc+/==', client_name: 'Test App', requested_scope: [] })
      })
    )

    render(<LoginPage />)

    await waitFor(() => {
      expect(requested).toBe('/api/v1/login-flows/abc%2B%2F%3D%3D')
    })
  })

  describe('Successful Login Challenge Fetch', () => {
    beforeEach(() => {
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email'],
            client: { client_id: 'test-client-id', sign_in_methods: ['email', 'google'] }
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
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: '',
            requested_scope: ['openid'],
            client: { client_id: 'test-client-id', sign_in_methods: ['email', 'google'] }
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('계속하려면 로그인하세요')).toBeInTheDocument()
      })
    })

    it('starts social sign-in for the flow when a provider button is pressed', async () => {
      render(<LoginPage />)

      const google = await screen.findByRole('button', { name: /Google/ })
      await user.click(google)

      expect(mockStartSocialSignIn).toHaveBeenCalledWith('test-challenge', 'google')
    })

    it('links "forgot password" to the forgot-password page carrying the client_id', async () => {
      // Regression: this link did not exist at all before, so /forgot-password
      // had no way to learn which tenant's user to scope the reset to.
      render(<LoginPage />)

      await waitFor(() => {
        const link = screen.getByRole('link', { name: '비밀번호를 잊으셨나요?' })
        expect(link).toHaveAttribute('href', '/forgot-password?client_id=test-client-id')
      })
    })

    it('links "forgot password" without a query string when no client_id is known', async () => {
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid'],
            client: { sign_in_methods: ['email'] },
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        const link = screen.getByRole('link', { name: '비밀번호를 잊으셨나요?' })
        expect(link).toHaveAttribute('href', '/forgot-password')
      })
    })
  })

  describe('What the application asked of the screen', () => {
    const flowWith = (hints: Record<string, unknown>) =>
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () =>
          HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: 'Test Client',
            requested_scope: ['openid'],
            client: { client_id: 'c', sign_in_methods: ['email', 'google'] },
            hints,
          })
        )
      )

    afterEach(async () => {
      await i18n.changeLanguage('ko')
    })

    it('fills in the address the application expects', async () => {
      flowWith({ login_hint: 'someone@example.com' })
      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByLabelText('이메일')).toHaveValue('someone@example.com')
      })
      expect(mockStartSocialSignIn).not.toHaveBeenCalled()
    })

    it('goes straight on to the provider the application named, once', async () => {
      flowWith({ sign_in_method: 'google' })
      render(<LoginPage />)

      await waitFor(() => {
        expect(mockStartSocialSignIn).toHaveBeenCalledWith('test-challenge', 'google')
      })
      expect(mockStartSocialSignIn).toHaveBeenCalledTimes(1)
    })

    it('does not leave for a way of signing in that is not a provider', async () => {
      flowWith({ sign_in_method: 'email' })
      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      })
      expect(mockStartSocialSignIn).not.toHaveBeenCalled()
    })

    it('speaks the first of the preferred languages it has', async () => {
      flowWith({ ui_locales: ['fr', 'en-GB', 'ko'] })
      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByLabelText('Email')).toBeInTheDocument()
      })
    })

    it('lets the user switch the language away from the one the application asked for', async () => {
      flowWith({ ui_locales: ['en'] })
      render(<LoginPage />)
      await waitFor(() => {
        expect(screen.getByLabelText('Email')).toBeInTheDocument()
      })

      // Switching re-asks about the flow, and the answer carries the same hint.
      await i18n.changeLanguage('ko')
      await waitFor(() => {
        expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      })
      await new Promise((r) => setTimeout(r, 300))
      expect(screen.getByLabelText('이메일')).toBeInTheDocument()
    })

    it('keeps the language the address asks for over the application\'s', async () => {
      mockSearchParams.set('lang', 'ko')
      flowWith({ ui_locales: ['en'] })
      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      })
    })
  })

  describe('Form Validation', () => {
    beforeEach(async () => {
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email'],
            client: { client_id: 'test-client-id', sign_in_methods: ['email'] },
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

    it('asks for a password, and leaves judging it to the server', async () => {
      // How long a password must be is the tenant's rule, and an account's
      // password may predate it — the sign-in form only needs one to send.
      const emailInput = screen.getByLabelText('이메일')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('비밀번호를 입력해주세요')).toBeInTheDocument()
      })
      expect(screen.queryByText(/최소 \d+자/)).toBeNull()
    })
  })

  describe('Login Submission', () => {
    beforeEach(async () => {
      server.use(
        http.get('http://localhost:8080/api/v1/login-flows/:flow', () => {
          return HttpResponse.json({
            next: 'form',
            flow: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email'],
            client: { client_id: 'test-client-id', sign_in_methods: ['email'] },
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
        http.post('http://localhost:8080/api/v1/login-flows/:flow/password', () => {
          return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/callback' })
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
        http.post('http://localhost:8080/api/v1/login-flows/:flow/password', () => {
          return HttpResponse.json({ detail: 'Invalid credentials' })
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

    it('keeps the form open with a localized message on a wrong password', async () => {
      server.use(
        http.post('http://localhost:8080/api/v1/login-flows/:flow/password', () => {
          return HttpResponse.json(
            { detail: 'Invalid email or password', code: 'invalid_credentials' },
            { status: 401 }
          )
        })
      )

      await user.type(screen.getByLabelText('이메일'), 'test@example.com')
      await user.type(screen.getByLabelText('비밀번호'), 'wrongpassword')
      await user.click(screen.getByRole('button', { name: '로그인' }))

      await waitFor(() => {
        expect(screen.getByText('이메일 또는 비밀번호가 올바르지 않습니다.')).toBeInTheDocument()
      })
      // The user can correct the password and submit again on the same screen.
      expect(screen.getByRole('button', { name: '로그인' })).toBeEnabled()
      expect(mockNavigate).not.toHaveBeenCalled()
    })

    it('navigates to the MFA verify page when the server requires a second factor', async () => {
      server.use(
        http.post('http://localhost:8080/api/v1/login-flows/:flow/password', () => {
          return HttpResponse.json({ next: 'mfa', mfa_challenge: 'chal-123' })
        })
      )

      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '로그인' })

      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/mfa/verify?flow=test-challenge&mfa_challenge=chal-123')
      })
    })

    it('sends correct request data', async () => {
      let requestBody: any
      let requestFlow = ''

      server.use(
        http.post('http://localhost:8080/api/v1/login-flows/:flow/password', async ({ request, params }) => {
          requestFlow = params.flow as string
          requestBody = await request.json()
          return HttpResponse.json({ next: 'redirect', redirect_to: 'http://example.com/callback' })
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
        expect(requestFlow).toBe('test-challenge')
        expect(requestBody).toEqual({
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