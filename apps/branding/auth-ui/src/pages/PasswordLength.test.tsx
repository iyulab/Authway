import { describe, it, expect } from 'vitest'
import { screen, waitFor, render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '../i18n'
import AcceptInvitationPage from './AcceptInvitationPage'
import ResetPasswordPage from './ResetPasswordPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

i18n.changeLanguage('ko')

const API = 'http://localhost:8080'

const renderAt = (path: string, ui: React.ReactElement) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
      </MemoryRouter>
    </I18nextProvider>
  )
}

const invitation = { id: 'i-1', email: 'user@example.com', tenant_name: 'Acme', inviter_name: 'Admin', role: 'member', expires_at: '2030-01-01T00:00:00Z' }

// The length a password has to have is the tenant's, and the screens that ask
// for one learn it from the lookup they already make.
describe('the password length a tenant asks for', () => {
  const user = userEvent.setup()

  describe('accepting an invitation', () => {
    const open = async (shown: number) => {
      server.use(
        http.get(`${API}/api/v1/invitations/token/:token`, () =>
          HttpResponse.json({ invitation, password_min_length: shown })),
      )
      renderAt('/invitation/accept?token=tok', <AcceptInvitationPage />)
      await waitFor(() => expect(document.querySelector('input[name="password"]')).not.toBeNull())
    }
    const fill = async (password: string) => {
      const [first, last] = [document.querySelector('input[name="firstName"]')!, document.querySelector('input[name="lastName"]')!]
      await user.type(first, 'Kim')
      await user.type(last, 'Lee')
      await user.type(document.querySelector('input[name="password"]')!, password)
      await user.type(document.querySelector('input[name="confirmPassword"]')!, password)
      await user.click(document.querySelector('button[type="submit"]')!)
    }

    it('holds the password to it before anything is sent', async () => {
      let sent = 0
      server.use(http.post(`${API}/api/v1/invitations/accept`, () => { sent++; return HttpResponse.json({ user: { id: 'u-1' } }) }))
      await open(12)
      await fill('nine-char')

      expect(await screen.findByText(/12자/)).toBeInTheDocument()
      expect(sent).toBe(0)
    })

    it('says a refusal for length in the screen\'s language, with the length in force', async () => {
      server.use(http.post(`${API}/api/v1/invitations/accept`, () =>
        HttpResponse.json({ detail: 'password must be at least 12 characters', code: 'password_too_short', min_length: 12 }, { status: 400 })))
      await open(8)
      await fill('nine-char')

      expect(await screen.findByText(/12자/)).toBeInTheDocument()
      expect(screen.queryByText(/must be at least/)).not.toBeInTheDocument()
    })
  })

  describe('resetting a password', () => {
    const open = async (shown: number) => {
      server.use(
        http.get(`${API}/api/v1/email/verify-reset-token`, () =>
          HttpResponse.json({ valid: true, message: 'Reset token is valid', password_min_length: shown })),
      )
      renderAt('/reset-password?token=tok', <ResetPasswordPage />)
      await waitFor(() => expect(document.querySelector('#newPassword')).not.toBeNull())
    }
    const fill = async (password: string) => {
      await user.type(document.querySelector('#newPassword')!, password)
      await user.type(document.querySelector('#confirmPassword')!, password)
      await user.click(document.querySelector('button[type="submit"]')!)
    }

    it('states it, and holds the password to it before anything is sent', async () => {
      let sent = 0
      server.use(http.post(`${API}/api/v1/email/reset-password`, () => { sent++; return HttpResponse.json({ message: 'ok' }) }))
      await open(12)

      expect(screen.getByText(/최소 12자 이상$/)).toBeInTheDocument()
      expect(document.querySelector('#newPassword')).toHaveAttribute('minlength', '12')
      await fill('nine-char')
      // The browser's own check stops a short value; the page's check is what a script-driven submit meets.
      expect(sent).toBe(0)
    })

    it('says a refusal for length in the screen\'s language, with the length in force', async () => {
      server.use(http.post(`${API}/api/v1/email/reset-password`, () =>
        HttpResponse.json({ detail: 'password must be at least 12 characters', code: 'password_too_short', min_length: 12 }, { status: 400 })))
      await open(8)
      await fill('nine-char')

      expect(await screen.findByText(/최소 12자 이상이어야/)).toBeInTheDocument()
      expect(screen.queryByText(/must be at least/)).not.toBeInTheDocument()
    })
  })
})
