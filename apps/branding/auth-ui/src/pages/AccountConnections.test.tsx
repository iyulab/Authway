import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import AccountConnections from './AccountConnections'

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('AccountConnections', () => {
  const user = userEvent.setup()
  beforeEach(() => {
    i18n.changeLanguage('en')
  })

  it('lists the linked providers and removes one after confirming', async () => {
    let linked = ['google']
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        linked = linked.filter((p) => !path.endsWith(`/${p}`))
        return json(200, { message: 'Connection removed' })
      }
      return json(200, { connections: linked.map((provider) => ({ provider })) })
    })
    render(<AccountConnections call={call} signInAgain={vi.fn()} />)

    expect(await screen.findByText('Google')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove' }))
    await user.click(screen.getByRole('button', { name: 'Remove Google' }))
    await waitFor(() => expect(screen.queryByText('Google')).not.toBeInTheDocument())
    expect(call).toHaveBeenCalledWith('/api/v1/users/connections/google', { method: 'DELETE' })
  })

  it('explains the last way in, and asks for a fresh sign-in when it must', async () => {
    let answer = json(409, { code: 'last_sign_in_method' })
    const signInAgain = vi.fn()
    const call = vi.fn(async (_path: string, init?: RequestInit) =>
      init?.method === 'DELETE' ? answer : json(200, { connections: [{ provider: 'google' }] }))
    render(<AccountConnections call={call} signInAgain={signInAgain} />)

    await user.click(await screen.findByRole('button', { name: 'Remove' }))
    await user.click(screen.getByRole('button', { name: 'Remove Google' }))
    expect(await screen.findByText('This is the only way you sign in. Set a password first.')).toBeInTheDocument()

    answer = json(401, { code: 'insufficient_user_authentication', max_age: 600 })
    await user.click(screen.getByRole('button', { name: 'Remove Google' }))
    await user.click(await screen.findByRole('button', { name: 'Sign in again' }))
    expect(signInAgain).toHaveBeenCalledWith(true)
  })

  it('shows nothing for an account without links', async () => {
    const call = vi.fn(async () => json(200, { connections: [] }))
    const { container } = render(<AccountConnections call={call} signInAgain={vi.fn()} />)
    await waitFor(() => expect(call).toHaveBeenCalled())
    expect(container.querySelector('section')).toBeNull()
  })
})
