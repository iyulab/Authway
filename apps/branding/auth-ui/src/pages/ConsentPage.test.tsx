import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import ConsentPage from './ConsentPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse, delay } from 'msw'

// Mock useSearchParams
const mockSearchParams = new URLSearchParams()

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return {
    ...actual,
    useSearchParams: () => [mockSearchParams],
  }
})

// Vite env vars (VITE_API_URL) are provided globally via vi.stubEnv in
// src/test/setup.ts.

describe('ConsentPage', () => {
  const user = userEvent.setup()

  const mockConsentInfo = {
    challenge: 'test-consent-challenge',
    client_name: 'Test Application',
    requested_scope: ['openid', 'profile', 'email', 'offline_access'],
    user: {
      // ConsentPage renders `user.name || user.email`; the backend supplies
      // a composed `name`. first/last kept for shape parity.
      name: 'Test User',
      email: 'test@example.com',
      first_name: 'Test',
      last_name: 'User'
    }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams.set('consent_challenge', 'test-consent-challenge')

    server.use(
      http.post('http://localhost:8080/consent', () => {
        return HttpResponse.json(mockConsentInfo)
      })
    )
  })

  afterEach(() => {
    // URLSearchParams has no .clear() in this runtime; delete each key.
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
  })

  describe('Initial Loading and Error States', () => {
    it('shows loading spinner initially', () => {
      render(<ConsentPage />)

      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
    })

    it('shows error when consent_challenge is missing', async () => {
      mockSearchParams.delete('consent_challenge')

      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('Consent challenge가 누락되었습니다.')).toBeInTheDocument()
      })
    })

    it('shows error when consent challenge fetch fails', async () => {
      server.use(
        http.post('http://localhost:8080/consent', () => {
          return HttpResponse.json({ error: 'Server error' }, { status: 500 })
        })
      )

      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('Server error')).toBeInTheDocument()
      })
    })

    it('shows error when consent challenge fetch throws', async () => {
      server.use(
        http.post('http://localhost:8080/consent', () => {
          return HttpResponse.error()
        })
      )

      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('오류 발생')).toBeInTheDocument()
        expect(screen.getByText('동의 정보를 가져오는데 실패했습니다.')).toBeInTheDocument()
      })
    })
  })

  describe('Successful Consent Challenge Fetch', () => {
    it('renders consent page with user and client info', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('앱 권한 승인')).toBeInTheDocument()
        expect(screen.getByText('안녕하세요, Test User님')).toBeInTheDocument()
        // Full sentence with the client name emphasized, matching LoginPage.
        const subtitle = screen.getByText(
          (_, el) =>
            el?.tagName === 'P' &&
            el.textContent === 'Test Application에서 다음 권한을 요청하고 있습니다.'
        )
        expect(within(subtitle).getByText('Test Application')).toHaveClass('font-medium')
        expect(screen.getByText('요청된 권한')).toBeInTheDocument()
      })
    })

    it('renders all requested scopes with descriptions', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('OpenID 인증')).toBeInTheDocument()
        expect(screen.getByText('기본 사용자 인증 정보에 접근합니다.')).toBeInTheDocument()

        expect(screen.getByText('프로필 정보')).toBeInTheDocument()
        expect(screen.getByText('사용자 이름, 프로필 사진 등 기본 프로필 정보에 접근합니다.')).toBeInTheDocument()

        expect(screen.getByText('이메일 주소')).toBeInTheDocument()
        expect(screen.getByText('사용자의 이메일 주소에 접근합니다.')).toBeInTheDocument()

        expect(screen.getByText('오프라인 접근')).toBeInTheDocument()
        expect(screen.getByText('사용자가 오프라인일 때도 정보에 접근할 수 있습니다.')).toBeInTheDocument()
      })
    })

    it('shows fallback description for unknown scopes', async () => {
      server.use(
        http.post('http://localhost:8080/consent', () => {
          return HttpResponse.json({
            ...mockConsentInfo,
            requested_scope: ['unknown_scope']
          })
        })
      )

      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('unknown_scope')).toBeInTheDocument()
        expect(screen.getByText('unknown_scope 권한에 접근합니다.')).toBeInTheDocument()
      })
    })

    it('all scopes are selected by default', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        const checkboxes = screen.getAllByRole('checkbox')
        // All scope checkboxes should be checked (excluding remember checkbox)
        const scopeCheckboxes = checkboxes.filter(cb => cb.getAttribute('id') !== 'remember')
        scopeCheckboxes.forEach(checkbox => {
          expect(checkbox).toBeChecked()
        })
      })
    })

    it('shows action buttons', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: '거부' })).toBeInTheDocument()
        expect(screen.getByRole('button', { name: /승인 \(4개 권한\)/ })).toBeInTheDocument()
      })
    })

    it('shows remember consent option', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByLabelText('이 선택을 기억하기 (1시간)')).toBeInTheDocument()
      })
    })
  })

  describe('Scope Selection', () => {
    beforeEach(async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('요청된 권한')).toBeInTheDocument()
      })
    })

    it('allows toggling individual scopes', async () => {
      const openidCheckbox = screen.getByLabelText('OpenID 인증')
      const approveButton = screen.getByRole('button', { name: /승인 \(4개 권한\)/ })

      expect(openidCheckbox).toBeChecked()
      expect(approveButton).toHaveTextContent('승인 (4개 권한)')

      await user.click(openidCheckbox)

      expect(openidCheckbox).not.toBeChecked()
      expect(screen.getByRole('button', { name: /승인 \(3개 권한\)/ })).toBeInTheDocument()
    })

    it('disables approve button when no scopes are selected', async () => {
      const checkboxes = screen.getAllByRole('checkbox')
      const scopeCheckboxes = checkboxes.filter(cb => cb.getAttribute('id') !== 'remember')

      // Uncheck all scopes
      for (const checkbox of scopeCheckboxes) {
        await user.click(checkbox)
      }

      const approveButton = screen.getByRole('button', { name: /승인 \(0개 권한\)/ })
      expect(approveButton).toBeDisabled()
    })

    it('can toggle remember consent option', async () => {
      const rememberCheckbox = screen.getByLabelText('이 선택을 기억하기 (1시간)')

      // rememberConsent defaults to true (better-UX default in ConsentPage).
      expect(rememberCheckbox).toBeChecked()

      await user.click(rememberCheckbox)

      expect(rememberCheckbox).not.toBeChecked()
    })
  })

  describe('Consent Approval', () => {
    beforeEach(async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /승인/ })).toBeInTheDocument()
      })
    })

    it('handles successful consent approval with redirect', async () => {
      server.use(
        http.post('http://localhost:8080/consent/accept', async () => {
          await delay(50) // keep isPending long enough to observe the loading label
          return HttpResponse.json({ redirect_to: 'http://example.com/callback?code=auth-code' })
        })
      )

      // Mock window.location.href
      delete (window as any).location
      window.location = { ...window.location, href: '' }

      const approveButton = screen.getByRole('button', { name: /승인/ })
      await user.click(approveButton)

      await waitFor(() => {
        expect(screen.getByText('승인 중...')).toBeInTheDocument()
      })

      await waitFor(() => {
        expect(window.location.href).toBe('http://example.com/callback?code=auth-code')
      })
    })

    it('handles consent approval error from server', async () => {
      server.use(
        http.post('http://localhost:8080/consent/accept', () => {
          return HttpResponse.json({ error: 'Consent processing failed' })
        })
      )

      const approveButton = screen.getByRole('button', { name: /승인/ })
      await user.click(approveButton)

      await waitFor(() => {
        expect(screen.getByText('Consent processing failed')).toBeInTheDocument()
      })
    })

    it('handles network error during consent approval', async () => {
      server.use(
        http.post('http://localhost:8080/consent/accept', () => {
          return HttpResponse.error()
        })
      )

      const approveButton = screen.getByRole('button', { name: /승인/ })
      await user.click(approveButton)

      await waitFor(() => {
        expect(screen.getByText('동의 처리 중 오류가 발생했습니다.')).toBeInTheDocument()
      })
    })

    it('sends correct request data for approval', async () => {
      let requestBody: any

      server.use(
        http.post('http://localhost:8080/consent/accept', async ({ request }) => {
          requestBody = await request.json()
          return HttpResponse.json({ redirect_to: 'http://example.com/callback' })
        })
      )

      // Toggle off one scope. remember stays at its default (true).
      const profileCheckbox = screen.getByLabelText('프로필 정보')
      await user.click(profileCheckbox)

      const approveButton = screen.getByRole('button', { name: /승인/ })
      await user.click(approveButton)

      await waitFor(() => {
        expect(requestBody).toEqual({
          challenge: 'test-consent-challenge',
          grant_scope: ['openid', 'email', 'offline_access'], // profile removed
          remember: true,
          remember_for: 3600
        })
      })
    })
  })

  describe('Consent Rejection', () => {
    beforeEach(async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: '거부' })).toBeInTheDocument()
      })
    })

    it('handles successful consent rejection with redirect', async () => {
      server.use(
        http.post('http://localhost:8080/consent/reject', async () => {
          await delay(50) // keep isPending long enough to observe the loading label
          return HttpResponse.json({ redirect_to: 'http://example.com/error?error=access_denied' })
        })
      )

      // Mock window.location.href
      delete (window as any).location
      window.location = { ...window.location, href: '' }

      const rejectButton = screen.getByRole('button', { name: '거부' })
      await user.click(rejectButton)

      await waitFor(() => {
        expect(screen.getByText('거부 중...')).toBeInTheDocument()
      })

      await waitFor(() => {
        expect(window.location.href).toBe('http://example.com/error?error=access_denied')
      })
    })

    it('handles consent rejection error from server', async () => {
      server.use(
        http.post('http://localhost:8080/consent/reject', () => {
          return HttpResponse.json({ error: 'Rejection processing failed' })
        })
      )

      const rejectButton = screen.getByRole('button', { name: '거부' })
      await user.click(rejectButton)

      await waitFor(() => {
        expect(screen.getByText('Rejection processing failed')).toBeInTheDocument()
      })
    })

    it('handles network error during consent rejection', async () => {
      server.use(
        http.post('http://localhost:8080/consent/reject', () => {
          return HttpResponse.error()
        })
      )

      const rejectButton = screen.getByRole('button', { name: '거부' })
      await user.click(rejectButton)

      await waitFor(() => {
        expect(screen.getByText('거부 처리 중 오류가 발생했습니다.')).toBeInTheDocument()
      })
    })
  })

  describe('User Display', () => {
    it('displays user email when names are not available', async () => {
      server.use(
        http.post('http://localhost:8080/consent', () => {
          return HttpResponse.json({
            ...mockConsentInfo,
            user: {
              email: 'test@example.com',
              first_name: '',
              last_name: ''
            }
          })
        })
      )

      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('안녕하세요, test@example.com님')).toBeInTheDocument()
      })
    })

    it('displays full name when available', async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('안녕하세요, Test User님')).toBeInTheDocument()
      })
    })
  })

  describe('Loading States', () => {
    beforeEach(async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /승인/ })).toBeInTheDocument()
      })
    })

    it('disables buttons during approval', async () => {
      server.use(
        http.post('http://localhost:8080/consent/accept', async () => {
          await delay(50)
          return HttpResponse.json({ redirect_to: 'http://example.com/callback' })
        })
      )

      const approveButton = screen.getByRole('button', { name: /승인/ })
      const rejectButton = screen.getByRole('button', { name: '거부' })

      await user.click(approveButton)

      await waitFor(() => {
        expect(approveButton).toBeDisabled()
        expect(rejectButton).not.toBeDisabled() // Only the clicked button should be disabled
      })
    })

    it('disables buttons during rejection', async () => {
      server.use(
        http.post('http://localhost:8080/consent/reject', async () => {
          await delay(50)
          return HttpResponse.json({ redirect_to: 'http://example.com/error' })
        })
      )

      const approveButton = screen.getByRole('button', { name: /승인/ })
      const rejectButton = screen.getByRole('button', { name: '거부' })

      await user.click(rejectButton)

      await waitFor(() => {
        expect(rejectButton).toBeDisabled()
        expect(approveButton).not.toBeDisabled() // Only the clicked button should be disabled
      })
    })
  })

  describe('Accessibility', () => {
    beforeEach(async () => {
      render(<ConsentPage />)

      await waitFor(() => {
        expect(screen.getByText('요청된 권한')).toBeInTheDocument()
      })
    })

    it('has proper form labels for all checkboxes', () => {
      expect(screen.getByLabelText('OpenID 인증')).toBeInTheDocument()
      expect(screen.getByLabelText('프로필 정보')).toBeInTheDocument()
      expect(screen.getByLabelText('이메일 주소')).toBeInTheDocument()
      expect(screen.getByLabelText('오프라인 접근')).toBeInTheDocument()
      expect(screen.getByLabelText('이 선택을 기억하기 (1시간)')).toBeInTheDocument()
    })

    it('has proper button roles and names', () => {
      expect(screen.getByRole('button', { name: '거부' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /승인 \(4개 권한\)/ })).toBeInTheDocument()
    })

    it('updates button text based on selected scopes count', async () => {
      const openidCheckbox = screen.getByLabelText('OpenID 인증')

      await user.click(openidCheckbox)

      expect(screen.getByRole('button', { name: /승인 \(3개 권한\)/ })).toBeInTheDocument()
    })
  })
})
