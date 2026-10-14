import { describe, it, expect } from 'vitest'
import { screen, waitFor, render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '../i18n'
import AcceptInvitationPage, { continueTo } from './AcceptInvitationPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const API = 'http://localhost:8080'
const invitation = { id: 'i-1', email: 'user@example.com', tenant_name: 'Acme', inviter_name: 'Admin', role: 'member', expires_at: '2030-01-01T00:00:00Z' }

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <QueryClientProvider client={queryClient}>
          <Routes>
            <Route path="/invitation/accept" element={<AcceptInvitationPage />} />
            <Route path="/login" element={<p>sign-in screen</p>} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>
    </I18nextProvider>
  )
}

// An invitation is usually opened from its email, outside any sign-in an app
// started. The sign-in screen means nothing without the app's flow, so after
// accepting such an invitation the person is told to sign in from the app
// rather than sent to an empty sign-in screen.
describe('accepting an invitation', () => {
  const user = userEvent.setup()

  const accept = async (path: string, shown: Record<string, unknown> = invitation) => {
    server.use(
      http.get(`${API}/.well-known/authway-config`, () =>
        HttpResponse.json({ issuer: 'https://issuer.example', api_url: API, auth_ui: 'http://localhost:3001', version: 'x' }),
      ),
      http.get(`${API}/api/v1/invitations/token/:token`, () => HttpResponse.json({ invitation: shown, password_min_length: 8 })),
      http.post(`${API}/api/v1/invitations/accept`, () => HttpResponse.json({ user: { id: 'u-1' } })),
    )
    renderAt(path)
    await waitFor(() => expect(document.querySelector('input[name="password"]')).not.toBeNull())
    await user.type(document.querySelector('input[name="firstName"]')!, 'Kim')
    await user.type(document.querySelector('input[name="lastName"]')!, 'Lee')
    await user.type(document.querySelector('input[name="password"]')!, 'long-enough-1')
    await user.type(document.querySelector('input[name="confirmPassword"]')!, 'long-enough-1')
    await user.click(document.querySelector('button[type="submit"]')!)
    await screen.findByText(i18n.t('auth:invitation.success.title'))
  }

  it('from its email, ends on the account-created screen that says to sign in from the app', async () => {
    await accept('/invitation/accept?token=tok')

    expect(screen.getByText(i18n.t('auth:invitation.success.signInFromApp'))).toBeInTheDocument()
    expect(screen.queryByText(i18n.t('auth:invitation.success.redirect'))).toBeNull()
    // It stays: nothing takes the person on to the sign-in screen.
    await new Promise((r) => setTimeout(r, 3200))
    expect(screen.queryByText('sign-in screen')).toBeNull()
    expect(screen.getByText(i18n.t('auth:invitation.success.title'))).toBeInTheDocument()
  }, 15000)

  it('inside a sign-in an app started, goes on to that sign-in', async () => {
    await accept('/invitation/accept?token=tok&flow=f-1')

    expect(screen.getByText(i18n.t('auth:invitation.success.redirect'))).toBeInTheDocument()
    expect(await screen.findByText('sign-in screen', {}, { timeout: 5000 })).toBeInTheDocument()
  }, 15000)

  it('that cannot be used, offers the way back to sign-in only when there is a sign-in to go back to', async () => {
    server.use(http.get(`${API}/api/v1/invitations/token/:token`, () => HttpResponse.json({ error: 'gone' }, { status: 404 })))
    renderAt('/invitation/accept?token=used')
    await screen.findByText(i18n.t('auth:invitation.invalidTitle'))
    expect(screen.queryByText(i18n.t('auth:invitation.backToLogin'))).toBeNull()
  })

  it('for an application, goes on to where it starts a sign-in, telling it the issuer and the address', async () => {
    await accept('/invitation/accept?token=tok', {
      ...invitation,
      client: { client_id: 'app-1', name: 'Filer', initiate_login_uri: 'https://filer.example/login?from=invite', client_uri: 'https://filer.example' },
    })
    const link = await screen.findByRole('link', { name: i18n.t('auth:invitation.success.continueTo', { name: 'Filer' }) })
    const to = new URL(link.getAttribute('href')!)
    expect(to.origin + to.pathname).toBe('https://filer.example/login')
    expect(to.searchParams.get('from')).toBe('invite')
    expect(to.searchParams.get('iss')).toBe('https://issuer.example')
    expect(to.searchParams.get('login_hint')).toBe('user@example.com')
  })
})

describe('continueTo', () => {
  const app = { client_id: 'a', name: 'A' }
  it('falls back to the home page, and follows only https', () => {
    expect(continueTo({ ...app, client_uri: 'https://a.example/' }, 'https://iss', 'u@x')).toBe('https://a.example/')
    expect(continueTo({ ...app, initiate_login_uri: 'https://a.example/start' }, undefined, 'u@x')).toBeUndefined()
    expect(continueTo({ ...app, initiate_login_uri: 'javascript:alert(1)', client_uri: 'http://a.example' }, 'https://iss', 'u@x')).toBeUndefined()
  })
})
