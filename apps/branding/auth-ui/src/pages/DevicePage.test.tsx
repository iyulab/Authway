import { describe, it, expect, vi, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import DevicePage from './DevicePage'
import DeviceDonePage from './DeviceDonePage'
import { server } from '../test/mocks/server'
import { http, HttpResponse } from 'msw/http'

const mockSearchParams = new URLSearchParams()
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router')
  return { ...actual, useSearchParams: () => [mockSearchParams] }
})

const followed: string[] = []
vi.mock('../utils/loginFlow', async () => {
  const actual = await vi.importActual<typeof import('../utils/loginFlow')>('../utils/loginFlow')
  return { ...actual, followRedirect: (to: string) => followed.push(to) }
})

// Signing in a TV, a console or a command-line tool: the user types the code
// the device shows, and the sign-in goes on to the ordinary login screen.
describe('DevicePage', () => {
  const user = userEvent.setup()

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
    followed.length = 0
  })

  it('sends the code the device link carried and goes on where the backend says', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('user_code', 'ABCDEFGH')
    let sent: { path: string; body: unknown } | null = null
    server.use(
      http.post('http://localhost:8080/api/v1/device-flows/:flow', async ({ request }) => {
        sent = { path: new URL(request.url).pathname, body: await request.json() }
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://localhost:4444/oauth2/device/verify?x=1' })
      }),
    )

    render(<DevicePage />)
    expect(screen.getByRole('textbox')).toHaveValue('ABCDEFGH')
    await user.click(screen.getByRole('button', { name: i18n.t('auth:device.submit') }))

    await vi.waitFor(() => expect(followed).toEqual(['http://localhost:4444/oauth2/device/verify?x=1']))
    expect(sent).toEqual({ path: '/api/v1/device-flows/f-1%3D', body: { user_code: 'ABCDEFGH' } })
  })

  it('shows what is wrong with a code the backend does not accept', async () => {
    mockSearchParams.set('flow', 'f-1')
    server.use(
      http.post('http://localhost:8080/api/v1/device-flows/:flow', () =>
        HttpResponse.json({ error: 'That code is not valid.', code: 'user_code_invalid' }, { status: 400 })),
    )

    render(<DevicePage />)
    await user.type(screen.getByRole('textbox'), 'wrongcod')
    expect(screen.getByRole('textbox')).toHaveValue('WRONGCOD')
    await user.click(screen.getByRole('button', { name: i18n.t('auth:device.submit') }))

    expect(await screen.findByRole('alert')).toHaveTextContent(i18n.t('auth:device.errors.user_code_invalid'))
    expect(followed).toEqual([])
  })

  it('asks for a code before sending anything', async () => {
    mockSearchParams.set('flow', 'f-1')
    let calls = 0
    server.use(http.post('http://localhost:8080/api/v1/device-flows/:flow', () => { calls++; return HttpResponse.json({}) }))

    render(<DevicePage />)
    await user.click(screen.getByRole('button', { name: i18n.t('auth:device.submit') }))

    expect(await screen.findByRole('alert')).toHaveTextContent(i18n.t('auth:device.errors.user_code_required'))
    expect(calls).toBe(0)
  })

  it('says to start on the device when opened without a flow', () => {
    render(<DevicePage />)
    expect(screen.getByText(i18n.t('auth:device.noFlowTitle'))).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).toBeNull()
  })
})

describe('DeviceDonePage', () => {
  it('tells the user to go back to the device', () => {
    render(<DeviceDonePage />)
    expect(screen.getByText(i18n.t('auth:device.doneTitle'))).toBeInTheDocument()
    expect(screen.getByText(i18n.t('auth:device.doneMessage'))).toBeInTheDocument()
  })
})
