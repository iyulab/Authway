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
    let linked = [{ id: 'id-1', provider: 'google', email: 'work@corp.example' }]
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        linked = linked.filter((c) => !path.endsWith(`/${c.id}`))
        return json(200, { message: 'Connection removed' })
      }
      return json(200, { connections: linked })
    })
    render(<AccountConnections call={call} signInAgain={vi.fn()} />)

    expect(await screen.findByText('Google')).toBeInTheDocument()
    expect(screen.getByText('work@corp.example')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove' }))
    await user.click(screen.getByRole('button', { name: 'Remove Google' }))
    await waitFor(() => expect(screen.queryByText('Google')).not.toBeInTheDocument())
    expect(call).toHaveBeenCalledWith('/api/v1/users/connections/id-1', { method: 'DELETE' })
  })

  it('lists two accounts at one provider and removes the one chosen', async () => {
    let linked = [
      { id: 'id-1', provider: 'google', email: 'work@corp.example' },
      { id: 'id-2', provider: 'google', email: 'home@gmail.example' },
    ]
    const call = vi.fn(async (path: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        linked = linked.filter((c) => !path.endsWith(`/${c.id}`))
        return json(200, { message: 'Connection removed' })
      }
      return json(200, { connections: linked })
    })
    render(<AccountConnections call={call} signInAgain={vi.fn()} />)

    expect(await screen.findByText('work@corp.example')).toBeInTheDocument()
    expect(screen.getByText('home@gmail.example')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'Remove' })[1])
    await user.click(screen.getByRole('button', { name: 'Remove Google' }))
    await waitFor(() => expect(screen.queryByText('home@gmail.example')).not.toBeInTheDocument())
    expect(screen.getByText('work@corp.example')).toBeInTheDocument()
    expect(call).toHaveBeenCalledWith('/api/v1/users/connections/id-2', { method: 'DELETE' })
  })

  it('explains the last way in, and asks for a fresh sign-in when it must', async () => {
    let answer = json(409, { code: 'last_sign_in_method' })
    const signInAgain = vi.fn()
    const call = vi.fn(async (_path: string, init?: RequestInit) =>
      init?.method === 'DELETE' ? answer : json(200, { connections: [{ id: 'id-1', provider: 'google', email: '' }] }))
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
