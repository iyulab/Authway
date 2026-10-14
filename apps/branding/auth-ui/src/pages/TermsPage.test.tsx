import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '../test/utils'
import i18n from '../i18n'
import TermsPage from './TermsPage'
import { nextScreen } from '../utils/loginFlow'
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

const owed = [
  { kind: 'privacy_policy', version: 'a', url: 'https://example.test/privacy' },
  { kind: 'terms_of_service', version: '1', url: 'https://example.test/terms', title: 'Service Terms' },
]

function owing(terms = owed) {
  server.use(
    http.get('http://localhost:8080/api/v1/login-flows/:flow/terms', ({ request }) => {
      if (new URL(request.url).searchParams.get('terms_challenge') !== 'c-1') {
        return HttpResponse.json({ error: 'expired', code: 'invalid_terms_challenge' }, { status: 400 })
      }
      return HttpResponse.json({ next: 'terms', flow: 'f-1=', terms_challenge: 'c-1', terms })
    }),
  )
}

// After every factor, a user whose tenant asks them to accept documents does
// so here — or declines, and goes back to the application.
describe('TermsPage', () => {
  const user = userEvent.setup()

  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  afterEach(() => {
    Array.from(mockSearchParams.keys()).forEach((k) => mockSearchParams.delete(k))
    followed.length = 0
  })

  it('shows each document by its title, or by its kind, with a link to it', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing()
    render(<TermsPage />)
    expect(await screen.findByLabelText(/I agree to the Privacy Policy/)).toBeInTheDocument()
    expect(screen.getByLabelText(/I agree to the Service Terms/)).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: 'Read' }).map((a) => a.getAttribute('href'))).toEqual([
      'https://example.test/privacy',
      'https://example.test/terms',
    ])
  })

  it('names an unknown kind by itself', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing([{ kind: 'location_terms', version: '2', url: 'https://example.test/location' }])
    render(<TermsPage />)
    expect(await screen.findByLabelText(/I agree to the location_terms/)).toBeInTheDocument()
  })

  it('links only to an https address', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing([
      { kind: 'terms_of_service', version: '1', url: 'javascript:alert(1)' },
      { kind: 'privacy_policy', version: 'a', url: 'https://example.test/privacy' },
    ])
    render(<TermsPage />)
    expect(await screen.findByLabelText(/Terms of Service/)).toBeInTheDocument()
    expect(screen.getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual(['https://example.test/privacy'])
  })

  it('continues only once every document is ticked, sending exactly what was shown', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing()
    let sent: unknown = null
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/terms', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://localhost:4444/oauth2/auth?x=1' })
      }),
    )
    render(<TermsPage />)
    const accept = await screen.findByRole('button', { name: 'Agree and continue' })
    await user.click(screen.getByLabelText(/Privacy Policy/))
    expect(accept).toBeDisabled()
    await user.click(screen.getByLabelText(/Service Terms/))
    expect(accept).toBeEnabled()
    await user.click(accept)
    await waitFor(() => expect(followed).toEqual(['http://localhost:4444/oauth2/auth?x=1']))
    expect(sent).toEqual({
      terms_challenge: 'c-1',
      accepted: [
        { kind: 'privacy_policy', version: 'a' },
        { kind: 'terms_of_service', version: '1' },
      ],
    })
  })

  it('shows a document revised meanwhile, unticked, and says so', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing([owed[1]])
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/terms', () =>
        HttpResponse.json(
          { error: 'changed', code: 'terms_mismatch', terms: [{ kind: 'terms_of_service', version: '2', url: 'https://example.test/terms/2' }] },
          { status: 400 },
        ),
      ),
    )
    render(<TermsPage />)
    await user.click(await screen.findByLabelText(/Service Terms/))
    await user.click(screen.getByRole('button', { name: 'Agree and continue' }))
    expect(await screen.findByText(/was updated while you were reading/)).toBeInTheDocument()
    expect(screen.getByLabelText(/Terms of Service/)).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Agree and continue' })).toBeDisabled()
    expect(followed).toEqual([])
  })

  it('declines and goes back where the backend says', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing()
    let path = ''
    server.use(
      http.post('http://localhost:8080/api/v1/login-flows/:flow/terms/decline', ({ request }) => {
        path = new URL(request.url).pathname
        return HttpResponse.json({ next: 'redirect', redirect_to: 'http://localhost:9999/cb?error=access_denied' })
      }),
    )
    render(<TermsPage />)
    await user.click(await screen.findByRole('button', { name: 'I do not agree' }))
    await waitFor(() => expect(followed).toEqual(['http://localhost:9999/cb?error=access_denied']))
    expect(path).toBe('/api/v1/login-flows/f-1%3D/terms/decline')
  })

  it('says the step expired when the challenge is not known', async () => {
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'stale')
    owing()
    render(<TermsPage />)
    expect(await screen.findByText(/This step has expired/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Agree and continue' })).not.toBeInTheDocument()
  })

  it('speaks Korean', async () => {
    await i18n.changeLanguage('ko')
    mockSearchParams.set('flow', 'f-1=')
    mockSearchParams.set('terms_challenge', 'c-1')
    owing([owed[0]])
    render(<TermsPage />)
    expect(await screen.findByLabelText(/개인정보 처리방침에 동의합니다/)).toBeInTheDocument()
  })
})

describe('nextScreen', () => {
  it('sends a step that owes documents to the terms page, keeping the flow as given', () => {
    expect(nextScreen({ next: 'terms', flow: 'f-1=', terms_challenge: 'c 1' })).toBe('/terms?flow=f-1%3D&terms_challenge=c+1')
    expect(nextScreen({ next: 'terms', terms_challenge: 'c' }, 'f-2')).toBe('/terms?flow=f-2&terms_challenge=c')
    expect(nextScreen({ next: 'mfa', flow: 'f', mfa_challenge: 'm' })).toBe('/mfa/verify?flow=f&mfa_challenge=m')
    expect(nextScreen({ next: 'redirect', redirect_to: 'x' }, 'f')).toBeUndefined()
  })
})
