import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

// Mock data
const mockUser = {
  id: '1',
  email: 'admin@example.com',
  first_name: 'Admin',
  last_name: 'User',
  email_verified: true,
  active: true,
  last_login_at: '2023-12-01T10:00:00Z',
  created_at: '2023-01-01T00:00:00Z',
  updated_at: '2023-01-01T00:00:00Z',
}

const mockUsers = [
  mockUser,
  {
    id: '2',
    email: 'user1@example.com',
    first_name: 'John',
    last_name: 'Doe',
    email_verified: true,
    active: true,
    last_login_at: '2023-11-30T15:30:00Z',
    created_at: '2023-01-15T00:00:00Z',
    updated_at: '2023-01-15T00:00:00Z',
  },
  {
    id: '3',
    email: 'user2@example.com',
    first_name: 'Jane',
    last_name: 'Smith',
    email_verified: false,
    active: false,
    created_at: '2023-02-01T00:00:00Z',
    updated_at: '2023-02-01T00:00:00Z',
  }
]

const mockClients = [
  {
    id: '1',
    client_id: 'test-client-1',
    name: 'Test Application 1',
    description: 'A test application for OAuth',
    website: 'https://example.com',
    redirect_uris: ['http://localhost:3000/callback'],
    grant_types: ['authorization_code'],
    scopes: ['openid', 'email', 'profile'],
    public: false,
    active: true,
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
  }
]

export const handlers = [
  // Auth endpoints
  http.post('http://localhost:8080/auth/login', () => {
    return HttpResponse.json({
      user: mockUser,
      tokens: {
        access_token: 'mock-admin-token'
      }
    })
  }),

  http.get('http://localhost:8080/profile/:id', ({ params }) => {
    const { id } = params
    const user = mockUsers.find(u => u.id === id)
    if (user) {
      return HttpResponse.json(user)
    }
    return HttpResponse.json({ error: 'User not found' }, { status: 404 })
  }),

  // Users API
  http.get('http://localhost:8080/api/users', ({ request }) => {
    const url = new URL(request.url)
    const limit = parseInt(url.searchParams.get('limit') || '10')
    const offset = parseInt(url.searchParams.get('offset') || '0')

    const paginatedUsers = mockUsers.slice(offset, offset + limit)

    return HttpResponse.json({
      users: paginatedUsers,
      total: mockUsers.length,
      limit,
      offset
    })
  }),

  http.get('http://localhost:8080/api/users/:id', ({ params }) => {
    const { id } = params
    const user = mockUsers.find(u => u.id === id)
    if (user) {
      return HttpResponse.json({ user })
    }
    return HttpResponse.json({ error: 'User not found' }, { status: 404 })
  }),

  http.put('http://localhost:8080/api/users/:id', async ({ params, request }) => {
    const { id } = params
    const body = await request.json()
    const userIndex = mockUsers.findIndex(u => u.id === id)
    if (userIndex !== -1) {
      mockUsers[userIndex] = { ...mockUsers[userIndex], ...body }
      return HttpResponse.json({ message: 'User updated', user: mockUsers[userIndex] })
    }
    return HttpResponse.json({ error: 'User not found' }, { status: 404 })
  }),

  http.delete('http://localhost:8080/api/users/:id', ({ params }) => {
    const { id } = params
    const userIndex = mockUsers.findIndex(u => u.id === id)
    if (userIndex !== -1) {
      mockUsers.splice(userIndex, 1)
      return HttpResponse.json({ message: 'User deleted' })
    }
    return HttpResponse.json({ error: 'User not found' }, { status: 404 })
  }),

  // Clients API
  http.get('http://localhost:8080/api/clients', ({ request }) => {
    const url = new URL(request.url)
    const limit = parseInt(url.searchParams.get('limit') || '10')
    const offset = parseInt(url.searchParams.get('offset') || '0')

    const paginatedClients = mockClients.slice(offset, offset + limit)

    return HttpResponse.json({
      clients: paginatedClients,
      total: mockClients.length,
      limit,
      offset
    })
  }),

  http.get('http://localhost:8080/api/clients/:id', ({ params }) => {
    const { id } = params
    const client = mockClients.find(c => c.id === id)
    if (client) {
      return HttpResponse.json({ client })
    }
    return HttpResponse.json({ error: 'Client not found' }, { status: 404 })
  }),

  http.post('http://localhost:8080/api/clients', async ({ request }) => {
    const body = await request.json()
    const newClient = {
      id: String(mockClients.length + 1),
      client_id: `client-${Date.now()}`,
      ...body,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockClients.push(newClient)
    return HttpResponse.json({
      message: 'Client created',
      client: newClient,
      credentials: {
        client_id: newClient.client_id,
        client_secret: 'mock-secret'
      }
    })
  }),

  http.put('http://localhost:8080/api/clients/:id', async ({ params, request }) => {
    const { id } = params
    const body = await request.json()
    const clientIndex = mockClients.findIndex(c => c.id === id)
    if (clientIndex !== -1) {
      mockClients[clientIndex] = { ...mockClients[clientIndex], ...body }
      return HttpResponse.json({ message: 'Client updated', client: mockClients[clientIndex] })
    }
    return HttpResponse.json({ error: 'Client not found' }, { status: 404 })
  }),

  http.delete('http://localhost:8080/api/clients/:id', ({ params }) => {
    const { id } = params
    const clientIndex = mockClients.findIndex(c => c.id === id)
    if (clientIndex !== -1) {
      mockClients.splice(clientIndex, 1)
      return HttpResponse.json({ message: 'Client deleted' })
    }
    return HttpResponse.json({ error: 'Client not found' }, { status: 404 })
  }),

  http.post('http://localhost:8080/api/clients/:id/regenerate-secret', ({ params }) => {
    const { id } = params
    const client = mockClients.find(c => c.id === id)
    if (client) {
      return HttpResponse.json({
        message: 'Client secret regenerated',
        credentials: {
          client_id: client.client_id,
          client_secret: `new-secret-${Date.now()}`
        }
      })
    }
    return HttpResponse.json({ error: 'Client not found' }, { status: 404 })
  })
]

export const server = setupServer(...handlers)