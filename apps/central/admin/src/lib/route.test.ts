import { describe, expect, it } from 'vitest'
import { route } from './route'

describe('route', () => {
  it('is the contract path for a call, with its parameters put in', () => {
    expect(route('get', '/api/v1/clients')).toBe('/api/v1/clients')
    expect(route('get', '/api/v1/clients/{id}', { id: 'abc' })).toBe('/api/v1/clients/abc')
    expect(route('post', '/api/v1/webhooks/{id}/rotate-secret', { id: 'w1' })).toBe('/api/v1/webhooks/w1/rotate-secret')
  })

  it('writes a parameter safely into the path', () => {
    expect(route('get', '/api/v1/clients/{id}', { id: 'a/b c' })).toBe('/api/v1/clients/a%2Fb%20c')
  })

  it('does not compile a path the contract does not have, a method it does not answer, or a missing parameter', () => {
    // @ts-expect-error — not a path of the contract
    route('get', '/api/v1/clientz')
    // @ts-expect-error — the path has no such method
    route('patch', '/api/v1/capabilities')
    // @ts-expect-error — the path names a parameter
    route('get', '/api/v1/clients/{id}')
    // @ts-expect-error — and only the ones it names
    route('get', '/api/v1/clients/{id}', { id: 'a', other: 'b' })
    expect(true).toBe(true)
  })
})
