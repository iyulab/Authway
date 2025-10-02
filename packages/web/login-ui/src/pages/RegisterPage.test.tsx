import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import RegisterPage from './RegisterPage'
import { server } from '../test/mocks/server'
import { rest } from 'msw'

// Mock useNavigate
const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

// Mock environment variables
Object.defineProperty(import.meta, 'env', {
  value: {
    VITE_API_URL: 'http://localhost:8080'
  },
  writable: true
})

describe('RegisterPage', () => {
  const user = userEvent.setup()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('Initial Render', () => {
    it('renders registration form', () => {
      render(<RegisterPage />)

      expect(screen.getByText('회원가입')).toBeInTheDocument()
      expect(screen.getByText('Authway 계정을 생성하세요')).toBeInTheDocument()

      expect(screen.getByLabelText('이름')).toBeInTheDocument()
      expect(screen.getByLabelText('성')).toBeInTheDocument()
      expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호 확인')).toBeInTheDocument()

      expect(screen.getByRole('button', { name: '회원가입' })).toBeInTheDocument()
      expect(screen.getByText('이미 계정이 있으신가요? 로그인')).toBeInTheDocument()
    })

    it('has proper form field placeholders', () => {
      render(<RegisterPage />)

      expect(screen.getByPlaceholderText('이름')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('성')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('이메일을 입력하세요')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('비밀번호를 입력하세요')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('비밀번호를 다시 입력하세요')).toBeInTheDocument()
    })
  })

  describe('Form Validation', () => {
    beforeEach(() => {
      render(<RegisterPage />)
    })

    it('validates required fields', async () => {
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('이름을 입력해주세요')).toBeInTheDocument()
        expect(screen.getByText('성을 입력해주세요')).toBeInTheDocument()
        expect(screen.getByText('올바른 이메일을 입력해주세요')).toBeInTheDocument()
        expect(screen.getByText('비밀번호는 최소 8자 이상이어야 합니다')).toBeInTheDocument()
        expect(screen.getByText('비밀번호 확인을 입력해주세요')).toBeInTheDocument()
      })
    })

    it('validates email format', async () => {
      const emailInput = screen.getByLabelText('이메일')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(emailInput, 'invalid-email')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('올바른 이메일을 입력해주세요')).toBeInTheDocument()
      })
    })

    it('validates password minimum length', async () => {
      const passwordInput = screen.getByLabelText('비밀번호')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(passwordInput, '123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('비밀번호는 최소 8자 이상이어야 합니다')).toBeInTheDocument()
      })
    })

    it('validates password confirmation match', async () => {
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(passwordInput, 'password123')
      await user.type(confirmPasswordInput, 'differentpassword')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('비밀번호가 일치하지 않습니다')).toBeInTheDocument()
      })
    })

    it('allows valid form submission', async () => {
      const firstNameInput = screen.getByLabelText('이름')
      const lastNameInput = screen.getByLabelText('성')
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(firstNameInput, 'Test')
      await user.type(lastNameInput, 'User')
      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.type(confirmPasswordInput, 'password123')
      await user.click(submitButton)

      // Should show loading state
      await waitFor(() => {
        expect(screen.getByText('회원가입 중...')).toBeInTheDocument()
        expect(submitButton).toBeDisabled()
      })
    })
  })

  describe('Registration Submission', () => {
    const validFormData = {
      firstName: 'Test',
      lastName: 'User',
      email: 'test@example.com',
      password: 'password123'
    }

    const fillValidForm = async () => {
      await user.type(screen.getByLabelText('이름'), validFormData.firstName)
      await user.type(screen.getByLabelText('성'), validFormData.lastName)
      await user.type(screen.getByLabelText('이메일'), validFormData.email)
      await user.type(screen.getByLabelText('비밀번호'), validFormData.password)
      await user.type(screen.getByLabelText('비밀번호 확인'), validFormData.password)
    }

    beforeEach(() => {
      render(<RegisterPage />)
    })

    it('handles successful registration', async () => {
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res(ctx.json({
            id: '1',
            email: 'test@example.com',
            name: 'Test User'
          }))
        })
      )

      await fillValidForm()
      const submitButton = screen.getByRole('button', { name: '회원가입' })
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('회원가입 완료!')).toBeInTheDocument()
        expect(screen.getByText('계정이 성공적으로 생성되었습니다.')).toBeInTheDocument()
        expect(screen.getByText('3초 후 로그인 페이지로 이동합니다...')).toBeInTheDocument()
      })

      // Fast forward 3 seconds
      vi.advanceTimersByTime(3000)

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/login')
      })
    })

    it('handles registration error from server', async () => {
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res(ctx.json({ error: 'Email already exists' }))
        })
      )

      await fillValidForm()
      const submitButton = screen.getByRole('button', { name: '회원가입' })
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('Email already exists')).toBeInTheDocument()
        expect(screen.queryByText('회원가입 완료!')).not.toBeInTheDocument()
      })
    })

    it('handles network error during registration', async () => {
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res.networkError('Network error')
        })
      )

      await fillValidForm()
      const submitButton = screen.getByRole('button', { name: '회원가입' })
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('회원가입 중 오류가 발생했습니다.')).toBeInTheDocument()
      })
    })

    it('sends correct request data', async () => {
      let requestBody: any

      server.use(
        rest.post('http://localhost:8080/register', async (req, res, ctx) => {
          requestBody = await req.json()
          return res(ctx.json({ id: '1' }))
        })
      )

      await fillValidForm()
      const submitButton = screen.getByRole('button', { name: '회원가입' })
      await user.click(submitButton)

      await waitFor(() => {
        expect(requestBody).toEqual({
          email: validFormData.email,
          password: validFormData.password,
          first_name: validFormData.firstName,
          last_name: validFormData.lastName,
        })
      })
    })

    it('clears error message on successful submission attempt', async () => {
      // First, cause an error
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res(ctx.json({ error: 'Initial error' }))
        })
      )

      await fillValidForm()
      const submitButton = screen.getByRole('button', { name: '회원가입' })
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('Initial error')).toBeInTheDocument()
      })

      // Now make it succeed
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res(ctx.json({ id: '1' }))
        })
      )

      await user.click(submitButton)

      // Error should be cleared before making the request
      await waitFor(() => {
        expect(screen.queryByText('Initial error')).not.toBeInTheDocument()
      })
    })
  })

  describe('Success State', () => {
    beforeEach(() => {
      server.use(
        rest.post('http://localhost:8080/register', (req, res, ctx) => {
          return res(ctx.json({ id: '1', email: 'test@example.com' }))
        })
      )

      render(<RegisterPage />)
    })

    it('shows success screen after successful registration', async () => {
      const firstNameInput = screen.getByLabelText('이름')
      const lastNameInput = screen.getByLabelText('성')
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(firstNameInput, 'Test')
      await user.type(lastNameInput, 'User')
      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.type(confirmPasswordInput, 'password123')
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('회원가입 완료!')).toBeInTheDocument()
        expect(screen.getByText('계정이 성공적으로 생성되었습니다.')).toBeInTheDocument()
        expect(screen.getByText('3초 후 로그인 페이지로 이동합니다...')).toBeInTheDocument()
      })

      // The form should not be visible anymore
      expect(screen.queryByLabelText('이름')).not.toBeInTheDocument()
      expect(screen.queryByLabelText('성')).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: '회원가입' })).not.toBeInTheDocument()
    })

    it('has success icon in success state', async () => {
      const firstNameInput = screen.getByLabelText('이름')
      const lastNameInput = screen.getByLabelText('성')
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(firstNameInput, 'Test')
      await user.type(lastNameInput, 'User')
      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.type(confirmPasswordInput, 'password123')
      await user.click(submitButton)

      await waitFor(() => {
        const successIcon = screen.getByRole('generic', { hidden: true }).parentElement
        expect(successIcon).toHaveClass('bg-green-100')
      })
    })
  })

  describe('Navigation', () => {
    it('navigates to login page when clicking login link', async () => {
      render(<RegisterPage />)

      const loginLink = screen.getByText('이미 계정이 있으신가요? 로그인')

      await user.click(loginLink)

      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
  })

  describe('Form Interaction', () => {
    beforeEach(() => {
      render(<RegisterPage />)
    })

    it('clears validation errors when typing in fields', async () => {
      const emailInput = screen.getByLabelText('이메일')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      // Trigger validation error
      await user.click(submitButton)

      await waitFor(() => {
        expect(screen.getByText('올바른 이메일을 입력해주세요')).toBeInTheDocument()
      })

      // Type valid email
      await user.type(emailInput, 'test@example.com')

      await waitFor(() => {
        expect(screen.queryByText('올바른 이메일을 입력해주세요')).not.toBeInTheDocument()
      })
    })

    it('disables submit button during form submission', async () => {
      const firstNameInput = screen.getByLabelText('이름')
      const lastNameInput = screen.getByLabelText('성')
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(firstNameInput, 'Test')
      await user.type(lastNameInput, 'User')
      await user.type(emailInput, 'test@example.com')
      await user.type(passwordInput, 'password123')
      await user.type(confirmPasswordInput, 'password123')
      await user.click(submitButton)

      // Button should be disabled during submission
      await waitFor(() => {
        expect(submitButton).toBeDisabled()
        expect(screen.getByText('회원가입 중...')).toBeInTheDocument()
      })
    })
  })

  describe('Accessibility', () => {
    beforeEach(() => {
      render(<RegisterPage />)
    })

    it('has proper form labels', () => {
      expect(screen.getByLabelText('이름')).toBeInTheDocument()
      expect(screen.getByLabelText('성')).toBeInTheDocument()
      expect(screen.getByLabelText('이메일')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호')).toBeInTheDocument()
      expect(screen.getByLabelText('비밀번호 확인')).toBeInTheDocument()
    })

    it('associates error messages with form fields', async () => {
      const emailInput = screen.getByLabelText('이메일')
      const submitButton = screen.getByRole('button', { name: '회원가입' })

      await user.type(emailInput, 'invalid-email')
      await user.click(submitButton)

      await waitFor(() => {
        const errorMessage = screen.getByText('올바른 이메일을 입력해주세요')
        expect(errorMessage).toBeInTheDocument()
      })
    })

    it('has proper input types for security', () => {
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')

      expect(emailInput).toHaveAttribute('type', 'email')
      expect(passwordInput).toHaveAttribute('type', 'password')
      expect(confirmPasswordInput).toHaveAttribute('type', 'password')
    })

    it('has proper autocomplete attributes', () => {
      const firstNameInput = screen.getByLabelText('이름')
      const lastNameInput = screen.getByLabelText('성')
      const emailInput = screen.getByLabelText('이메일')
      const passwordInput = screen.getByLabelText('비밀번호')
      const confirmPasswordInput = screen.getByLabelText('비밀번호 확인')

      expect(firstNameInput).toHaveAttribute('autoComplete', 'given-name')
      expect(lastNameInput).toHaveAttribute('autoComplete', 'family-name')
      expect(emailInput).toHaveAttribute('autoComplete', 'email')
      expect(passwordInput).toHaveAttribute('autoComplete', 'new-password')
      expect(confirmPasswordInput).toHaveAttribute('autoComplete', 'new-password')
    })
  })
})