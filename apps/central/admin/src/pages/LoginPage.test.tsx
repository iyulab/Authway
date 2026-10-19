import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw/http'
import { server } from '@/test/mocks/server'
import LoginPage from './LoginPage'

const { signInWithAccount } = vi.hoisted(() => ({ signInWithAccount: vi.fn() }))
vi.mock('../lib/consoleSignIn', async (actual) => ({
  ...(await actual<typeof import('../lib/consoleSignIn')>()),
  signInWithAccount,
}))

const entryAt = 'http://localhost:8080/api/v1/console/entry'
const entry = {
  client_id: 'console-client',
  redirect_uri: 'http://localhost:3000/callback',
  scope: 'openid email profile',
  audience: 'urn:authway:console',
}

const show = () =>
  render(
    <MemoryRouter>
      <LoginPage />
    </MemoryRouter>,
  )

beforeEach(() => signInWithAccount.mockReset())

describe('LoginPage', () => {
  it('offers the console account sign-in when the deployment has one', async () => {
    server.use(http.get(entryAt, () => HttpResponse.json(entry)))
    show()
    const button = await screen.findByRole('button', { name: '콘솔 계정으로 로그인' })
    await userEvent.click(button)
    expect(signInWithAccount).toHaveBeenCalledWith(entry)
    // The admin password stays as the other way in.
    expect(screen.getByLabelText('관리자 비밀번호')).toBeInTheDocument()
  })

  it('offers only the admin password when the deployment has no console sign-in', async () => {
    let asked = false
    server.use(
      http.get(entryAt, () => {
        asked = true
        return HttpResponse.json({ code: 'console_sign_in_unavailable' }, { status: 404 })
      }),
    )
    show()
    await waitFor(() => expect(asked).toBe(true))
    expect(screen.queryByRole('button', { name: '콘솔 계정으로 로그인' })).not.toBeInTheDocument()
    expect(screen.getByText('관리자 비밀번호로 로그인하세요')).toBeInTheDocument()
  })

  it('no longer links to a login page on this machine', async () => {
    server.use(http.get(entryAt, () => HttpResponse.json({}, { status: 404 })))
    show()
    expect(screen.queryByText('사용자 로그인 페이지')).not.toBeInTheDocument()
  })
})
