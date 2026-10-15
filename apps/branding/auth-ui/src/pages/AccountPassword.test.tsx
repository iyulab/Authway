import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import AccountPassword from './AccountPassword'

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

// The account screen's password section: change it with the current one, or
// set a first one; the other sessions end.
describe('AccountPassword', () => {
  const user = userEvent.setup()
  let sent: unknown[]
  let signInAgain: ReturnType<typeof vi.fn>

  beforeEach(() => {
    i18n.changeLanguage('en')
    sent = []
    signInAgain = vi.fn()
  })

  const callWith = (hasPassword: boolean, answer: (body: any) => Response) =>
    vi.fn(async (path: string, init?: RequestInit) => {
      if (path === '/api/v1/profile/me') return json(200, { id: 'u', email: 'a@example.com', email_verified: true, has_password: hasPassword })
      const body = JSON.parse(String(init?.body))
      sent.push(body)
      return answer(body)
    })

  it('changes the password with the current one, and says the other sessions ended', async () => {
    const call = callWith(true, (body) =>
      body.current_password === 'old-password'
        ? json(200, { message: 'Password changed', sessions_ended: 'others' })
        : json(400, { code: 'invalid_current_password', detail: 'no' }))
    render(<AccountPassword call={call} signInAgain={signInAgain} />)

    await user.click(await screen.findByRole('button', { name: 'Change password' }))
    await user.type(screen.getByLabelText('Current password'), 'wrong')
    await user.type(screen.getByLabelText('New password'), 'new-password-1')
    await user.type(screen.getByLabelText('New password again'), 'new-password-1')
    await user.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByText('The current password is not right.')).toBeInTheDocument()

    await user.clear(screen.getByLabelText('Current password'))
    await user.type(screen.getByLabelText('Current password'), 'old-password')
    await user.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByRole('status')).toHaveTextContent('signed out everywhere else')
    expect(sent).toEqual([
      { current_password: 'wrong', new_password: 'new-password-1' },
      { current_password: 'old-password', new_password: 'new-password-1' },
    ])
  })

  it('checks the two new passwords match before sending', async () => {
    const call = callWith(true, () => json(200, { sessions_ended: 'others' }))
    render(<AccountPassword call={call} signInAgain={signInAgain} />)
    await user.click(await screen.findByRole('button', { name: 'Change password' }))
    await user.type(screen.getByLabelText('Current password'), 'old-password')
    await user.type(screen.getByLabelText('New password'), 'new-password-1')
    await user.type(screen.getByLabelText('New password again'), 'new-password-2')
    await user.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByText('The two new passwords are not the same.')).toBeInTheDocument()
    expect(sent).toEqual([])
  })

  it('sets a first password without a current one, signing in again first when it must', async () => {
    // The account screen's session is older than a recent sign-in.
    const call = callWith(false, () => json(401, { code: 'insufficient_user_authentication', max_age: 600 }))
    render(<AccountPassword call={call} signInAgain={signInAgain} />)

    await user.click(await screen.findByRole('button', { name: 'Set a password' }))
    expect(screen.queryByLabelText('Current password')).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('New password'), 'first-password-1')
    await user.type(screen.getByLabelText('New password again'), 'first-password-1')
    await user.click(screen.getByRole('button', { name: 'Set a password' }))

    await user.click(await screen.findByRole('button', { name: 'Sign in again' }))
    expect(signInAgain).toHaveBeenCalledWith(true)
    expect(sent).toEqual([{ new_password: 'first-password-1' }])
    await waitFor(() => expect(call).toHaveBeenCalledTimes(2))
  })
})
