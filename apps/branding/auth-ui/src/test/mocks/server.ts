import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

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
  // Login endpoint
  http.post('http://localhost:8080/auth/login', () => {
    return HttpResponse.json({
      user: mockUser,
      tokens: {
        access_token: 'mock-access-token'
      }
    })
  }),

  // Register endpoint
  http.post('http://localhost:8080/register', () => {
    return HttpResponse.json({
      message: 'User registered successfully',
      user: mockUser
    })
  }),

  // Google OAuth endpoint
  http.get('http://localhost:8080/auth/google', () => {
    return new Response(null, {
      status: 302,
      headers: {
        Location: 'https://accounts.google.com/oauth/authorize?client_id=mock'
      }
    })
  }),

  // Login challenge endpoint
  http.get('http://localhost:8080/login', ({ request }) => {
    const url = new URL(request.url)
    const challenge = url.searchParams.get('login_challenge')
    return HttpResponse.json({
      challenge,
      client_name: 'Test Application',
      requested_scope: ['openid', 'email'],
      client: {
        client_id: 'test-client'
      }
    })
  }),

  // Login endpoint
  http.post('http://localhost:8080/login', () => {
    return HttpResponse.json({
      redirect_to: 'http://localhost:3000/callback?code=mock-auth-code'
    })
  }),

  // Consent endpoint
  http.get('http://localhost:8080/consent', ({ request }) => {
    const url = new URL(request.url)
    const challenge = url.searchParams.get('consent_challenge')
    return HttpResponse.json({
      challenge,
      client_name: 'Test Application',
      requested_scope: ['openid', 'email', 'profile'],
      user: mockUser
    })
  }),

  // Accept consent
  http.post('http://localhost:8080/consent', () => {
    return HttpResponse.json({
      redirect_to: 'http://localhost:3000/callback?code=mock-auth-code'
    })
  }),

  // Reject consent
  http.post('http://localhost:8080/consent/reject', () => {
    return HttpResponse.json({
      redirect_to: 'http://localhost:3000/error?error=access_denied'
    })
  })
]

export const server = setupServer(...handlers)