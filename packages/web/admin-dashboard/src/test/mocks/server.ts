import { setupServer } from 'msw/node'
import { rest } from 'msw'

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
  rest.post('http://localhost:8080/auth/login', (req, res, ctx) => {
    return res(
      ctx.json({
        user: mockUser,
        tokens: {
          access_token: 'mock-admin-token'
        }
      })
    )
  }),

  rest.get('http://localhost:8080/profile/:id', (req, res, ctx) => {
    const { id } = req.params
    const user = mockUsers.find(u => u.id === id)
    if (user) {
      return res(ctx.json(user))
    }
    return res(ctx.status(404), ctx.json({ error: 'User not found' }))
  }),

  // Users API
  rest.get('http://localhost:8080/api/users', (req, res, ctx) => {
    const limit = parseInt(req.url.searchParams.get('limit') || '10')
    const offset = parseInt(req.url.searchParams.get('offset') || '0')

    const paginatedUsers = mockUsers.slice(offset, offset + limit)

    return res(
      ctx.json({
        users: paginatedUsers,
        total: mockUsers.length,
        limit,
        offset
      })
    )
  }),

  rest.get('http://localhost:8080/api/users/:id', (req, res, ctx) => {
    const { id } = req.params
    const user = mockUsers.find(u => u.id === id)
    if (user) {
      return res(ctx.json({ user }))
    }
    return res(ctx.status(404), ctx.json({ error: 'User not found' }))
  }),

  rest.put('http://localhost:8080/api/users/:id', (req, res, ctx) => {
    const { id } = req.params
    const userIndex = mockUsers.findIndex(u => u.id === id)
    if (userIndex !== -1) {
      mockUsers[userIndex] = { ...mockUsers[userIndex], ...req.body }
      return res(ctx.json({ message: 'User updated', user: mockUsers[userIndex] }))
    }
    return res(ctx.status(404), ctx.json({ error: 'User not found' }))
  }),

  rest.delete('http://localhost:8080/api/users/:id', (req, res, ctx) => {
    const { id } = req.params
    const userIndex = mockUsers.findIndex(u => u.id === id)
    if (userIndex !== -1) {
      mockUsers.splice(userIndex, 1)
      return res(ctx.json({ message: 'User deleted' }))
    }
    return res(ctx.status(404), ctx.json({ error: 'User not found' }))
  }),

  // Clients API
  rest.get('http://localhost:8080/api/clients', (req, res, ctx) => {
    const limit = parseInt(req.url.searchParams.get('limit') || '10')
    const offset = parseInt(req.url.searchParams.get('offset') || '0')

    const paginatedClients = mockClients.slice(offset, offset + limit)

    return res(
      ctx.json({
        clients: paginatedClients,
        total: mockClients.length,
        limit,
        offset
      })
    )
  }),

  rest.get('http://localhost:8080/api/clients/:id', (req, res, ctx) => {
    const { id } = req.params
    const client = mockClients.find(c => c.id === id)
    if (client) {
      return res(ctx.json({ client }))
    }
    return res(ctx.status(404), ctx.json({ error: 'Client not found' }))
  }),

  rest.post('http://localhost:8080/api/clients', (req, res, ctx) => {
    const newClient = {
      id: String(mockClients.length + 1),
      client_id: `client-${Date.now()}`,
      ...req.body,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    mockClients.push(newClient)
    return res(
      ctx.json({
        message: 'Client created',
        client: newClient,
        credentials: {
          client_id: newClient.client_id,
          client_secret: 'mock-secret'
        }
      })
    )
  }),

  rest.put('http://localhost:8080/api/clients/:id', (req, res, ctx) => {
    const { id } = req.params
    const clientIndex = mockClients.findIndex(c => c.id === id)
    if (clientIndex !== -1) {
      mockClients[clientIndex] = { ...mockClients[clientIndex], ...req.body }
      return res(ctx.json({ message: 'Client updated', client: mockClients[clientIndex] }))
    }
    return res(ctx.status(404), ctx.json({ error: 'Client not found' }))
  }),

  rest.delete('http://localhost:8080/api/clients/:id', (req, res, ctx) => {
    const { id } = req.params
    const clientIndex = mockClients.findIndex(c => c.id === id)
    if (clientIndex !== -1) {
      mockClients.splice(clientIndex, 1)
      return res(ctx.json({ message: 'Client deleted' }))
    }
    return res(ctx.status(404), ctx.json({ error: 'Client not found' }))
  }),

  rest.post('http://localhost:8080/api/clients/:id/regenerate-secret', (req, res, ctx) => {
    const { id } = req.params
    const client = mockClients.find(c => c.id === id)
    if (client) {
      return res(
        ctx.json({
          message: 'Client secret regenerated',
          credentials: {
            client_id: client.client_id,
            client_secret: `new-secret-${Date.now()}`
          }
        })
      )
    }
    return res(ctx.status(404), ctx.json({ error: 'Client not found' }))
  })
]

export const server = setupServer(...handlers)