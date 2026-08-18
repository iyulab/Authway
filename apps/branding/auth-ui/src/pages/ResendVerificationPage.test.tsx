import { describe, it, expect } from 'vitest'
import { screen, waitFor, render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '../i18n'
import ResendVerificationPage from './ResendVerificationPage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw'

// ../test/utils.tsx pins this at module load for its own render(); this file
// renders standalone (MemoryRouter, not BrowserRouter) so it must pin it too.
i18n.changeLanguage('ko')

// Same provider stack as ../test/utils.tsx's render(), but with MemoryRouter
// in place of BrowserRouter so the route (and its client_id query param) is
// set directly rather than via jsdom's window.history.
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

// Regression coverage: same tenant-scoping contract as ForgotPasswordPage,
// for the resend-verification endpoint.
describe('ResendVerificationPage', () => {
  const user = userEvent.setup()

  it('includes client_id in the request when present in the URL', async () => {
    let requestBody: any
    server.use(
      http.post('http://localhost:8080/api/email/send-verification', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ message: 'ok' })
      })
    )

    renderAt('/resend-verification?client_id=test-client-id', <ResendVerificationPage />)

    await user.type(screen.getByLabelText('이메일 주소'), 'test@example.com')
    await user.click(screen.getByRole('button', { name: '인증 이메일 보내기' }))

    await waitFor(() => {
      expect(requestBody).toEqual({ email: 'test@example.com', client_id: 'test-client-id' })
    })
  })

  it('omits client_id when the page was reached without one', async () => {
    let requestBody: any
    server.use(
      http.post('http://localhost:8080/api/email/send-verification', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ message: 'ok' })
      })
    )

    renderAt('/resend-verification', <ResendVerificationPage />)

    await user.type(screen.getByLabelText('이메일 주소'), 'test@example.com')
    await user.click(screen.getByRole('button', { name: '인증 이메일 보내기' }))

    await waitFor(() => {
      expect(requestBody).toEqual({ email: 'test@example.com' })
    })
  })
})
