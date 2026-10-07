import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw/http'

// Mock data
const mockUser = {
  id: '1',
  email: 'test@example.com',
  first_name: 'Test',
  last_name: 'User',
  email_verified: true,
  active: true,
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
}

export const handlers = [
  // Login flow info (LoginPage asks what to show for the flow)
  http.get('http://localhost:8080/api/v1/login-flows/:flow', ({ params }) => {
    return HttpResponse.json({
      next: 'form',
      flow: params.flow,
      client_name: 'Test Application',
      requested_scope: ['openid', 'email'],
      client: {
        client_id: 'test-client',
        sign_in_methods: ['email', 'google']
      }
    })
  }),

  // Password step of a login flow
  http.post('http://localhost:8080/api/v1/login-flows/:flow/password', () => {
    return HttpResponse.json({
      next: 'redirect',
      redirect_to: 'http://localhost:3000/callback?code=mock-auth-code'
    })
  }),

  // Consent flow: what to ask, and the answers
  http.get('http://localhost:8080/api/v1/consent-flows/:flow', ({ params }) => {
    return HttpResponse.json({
      next: 'form',
      flow: params.flow,
      client_name: 'Test Application',
      requested_scope: ['openid', 'email', 'profile'],
      user: mockUser
    })
  }),

  http.post('http://localhost:8080/api/v1/consent-flows/:flow/accept', () => {
    return HttpResponse.json({
      next: 'redirect',
      redirect_to: 'http://localhost:3000/callback?code=mock-auth-code'
    })
  }),

  http.post('http://localhost:8080/api/v1/consent-flows/:flow/reject', () => {
    return HttpResponse.json({
      next: 'redirect',
      redirect_to: 'http://localhost:3000/error?error=access_denied'
    })
  })
]

export const server = setupServer(...handlers)
