import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import LoginPage from './LoginPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw'

// Mock useSearchParams and useNavigate
const mockNavigate = vi.fn()
const mockSearchParams = new URLSearchParams()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
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

// Mock environment variables
Object.defineProperty(import.meta, 'env', {
  value: {
    VITE_API_URL: 'http://localhost:8080'
  },
  writable: true
})

describe('LoginPage', () => {
  const user = userEvent.setup()

  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.set('login_challenge', 'test-challenge')
  })

  afterEach(() => {
    mockSearchParams.clear()
  })

  describe('Initial Loading and Error States', () => {
    it('shows loading spinner initially', () => {
      render(<LoginPage />)

      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
    })

    it('shows error when login_challenge is missing', async () => {
      mockSearchParams.delete('login_challenge')

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('Login challenge가 누락되었습니다.')).toBeInTheDocument()
      })
    })

    it('shows error when login challenge fetch fails', async () => {
      server.use(
        http.get('http://localhost:8080/login', () => {
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
        http.get('http://localhost:8080/login', () => {
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
        expect(screen.getByText('로그인')).toBeInTheDocument()
        expect(screen.getByText('Test App에 로그인하시겠습니까?')).toBeInTheDocument()
        expect(screen.getByText('요청된 권한: openid, email')).toBeInTheDocument()
      })

      expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호')).toBeInTheDocument()
      expect(screen.getByLabelText('로그인 상태 유지')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: '로그인' })).toBeInTheDocument()
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
        http.get('http://localhost:8080/login', () => {
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
        http.get('http://localhost:8080/login', () => {
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
        http.post('http://localhost:8080/login', () => {
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
        http.post('http://localhost:8080/login', () => {
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

    it('sends correct request data', async () => {
      let requestBody: any

      server.use(
        http.post('http://localhost:8080/login', async ({ request }) => {
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

  describe('Navigation', () => {
    beforeEach(async () => {
      server.use(
        http.get('http://localhost:8080/login', () => {
          return HttpResponse.json({
            challenge: 'test-challenge',
            client_name: 'Test App',
            requested_scope: ['openid', 'email']
          })
        })
      )

      render(<LoginPage />)

      await waitFor(() => {
        expect(screen.getByText('계정이 없으신가요? 회원가입')).toBeInTheDocument()
      })
    })

    it('navigates to register page', async () => {
      const registerLink = screen.getByText('계정이 없으신가요? 회원가입')

      await user.click(registerLink)

      expect(mockNavigate).toHaveBeenCalledWith('/register')
    })
  })
})