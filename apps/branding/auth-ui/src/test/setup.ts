import '@testing-library/jest-dom'
import { beforeAll, afterAll, afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'
import { server } from './mocks/server'

// Provide Vite env vars used by components (import.meta.env). vi.stubEnv is
// the reliable way to set these in vitest; per-file Object.defineProperty on
// import.meta.env does not take effect. Both point at the msw mock origin so
// LoginPage/ConsentPage fetch URLs resolve to intercepted handlers.
vi.stubEnv('VITE_API_URL', 'http://localhost:8080')
vi.stubEnv('VITE_AUTH_BACKEND_URL', 'http://localhost:8080')

// Mock server setup
beforeAll(() => server.listen())
afterEach(() => {
  server.resetHandlers()
  cleanup()
})
afterAll(() => server.close())

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // Deprecated
    removeListener: vi.fn(), // Deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
  length: 0,
  key: vi.fn(),
}
Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
})

// Mock sessionStorage
Object.defineProperty(window, 'sessionStorage', {
  value: localStorageMock,
})

// Mock window.location
delete (window as any).location
window.location = {
  ...window.location,
  href: 'http://localhost:3001',
  origin: 'http://localhost:3001',
  pathname: '/',
  search: '',
  hash: '',
  assign: vi.fn(),
  replace: vi.fn(),
  reload: vi.fn(),
}

// Mock alert, confirm, prompt
window.alert = vi.fn()
window.confirm = vi.fn()
window.prompt = vi.fn()