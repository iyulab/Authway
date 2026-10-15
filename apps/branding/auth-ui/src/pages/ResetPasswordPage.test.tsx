import { describe, it, expect } from 'vitest'
import { screen, render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '../i18n'
import ResetPasswordPage from './ResetPasswordPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

i18n.changeLanguage('ko')

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[path]}>
        <QueryClientProvider client={queryClient}>
          <ResetPasswordPage />
        </QueryClientProvider>
      </MemoryRouter>
    </I18nextProvider>
  )
}

// A reset link sent before an administrator suspended the account is refused
// with account_suspended; the screen says the account is suspended rather than
// that the link expired.
describe('ResetPasswordPage', () => {
  it('says the account is suspended when the link is for a suspended account', async () => {
    server.use(
      http.get('http://localhost:8080/api/v1/email/verify-reset-token', () =>
        HttpResponse.json(
          { type: 'about:blank', title: 'Forbidden', status: 403, code: 'account_suspended', detail: 'This account is suspended' },
          { status: 403 }
        )
      )
    )

    renderAt('/reset-password?token=sent-before-suspension')

    expect(await screen.findByText(i18n.t('auth:errors.accountSuspended'))).toBeInTheDocument()
    expect(screen.queryByText('This account is suspended')).not.toBeInTheDocument()
  })
})
