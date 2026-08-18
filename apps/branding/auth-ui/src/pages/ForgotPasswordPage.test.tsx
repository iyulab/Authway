import { describe, it, expect } from 'vitest'
import { screen, waitFor, render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '../i18n'
import ForgotPasswordPage from './ForgotPasswordPage'
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

// Regression coverage for ISSUE-Authway-20260817-115815 (HD-10): the
// forgot-password request must carry client_id through to the API when the
// page was reached with one, so the backend can scope the lookup to the
// right tenant instead of matching the email globally.
describe('ForgotPasswordPage', () => {
  const user = userEvent.setup()

  it('includes client_id in the request when present in the URL', async () => {
    let requestBody: any
    server.use(
      http.post('http://localhost:8080/api/email/forgot-password', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ message: 'ok' })
      })
    )

    renderAt('/forgot-password?client_id=test-client-id', <ForgotPasswordPage />)

    await user.type(screen.getByLabelText('이메일 주소'), 'test@example.com')
    await user.click(screen.getByRole('button', { name: '재설정 링크 보내기' }))

    await waitFor(() => {
      expect(requestBody).toEqual({ email: 'test@example.com', client_id: 'test-client-id' })
    })
  })

  it('omits client_id when the page was reached without one', async () => {
    let requestBody: any
    server.use(
      http.post('http://localhost:8080/api/email/forgot-password', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ message: 'ok' })
      })
    )

    renderAt('/forgot-password', <ForgotPasswordPage />)

    await user.type(screen.getByLabelText('이메일 주소'), 'test@example.com')
    await user.click(screen.getByRole('button', { name: '재설정 링크 보내기' }))

    await waitFor(() => {
      expect(requestBody).toEqual({ email: 'test@example.com' })
    })
  })
})
