import { afterEach, describe, expect, it, vi } from 'vitest'
import { getConfig } from './config'
import runtimeConfigSchema from '../../../../packages/contract/runtime-config.schema.json'

describe('getConfig', () => {
  afterEach(() => {
    delete window.__AUTHWAY_CONFIG__
    vi.unstubAllEnvs()
  })

  it('prefers values the deployment sets at run time', () => {
    window.__AUTHWAY_CONFIG__ = { apiUrl: 'https://auth.example.com/' }
    expect(getConfig().apiUrl).toBe('https://auth.example.com')
  })

  it('falls back to the build-time variable', () => {
    vi.stubEnv('VITE_API_URL', 'https://built.example.com')
    expect(getConfig().apiUrl).toBe('https://built.example.com')
  })

  it('treats an empty run-time value as "same origin", not as unset', () => {
    vi.stubEnv('VITE_API_URL', 'https://built.example.com')
    window.__AUTHWAY_CONFIG__ = { apiUrl: '' }
    expect(getConfig().apiUrl).toBe('')
  })
})

describe('runtime configuration contract', () => {
  it('reads only keys packages/contract/runtime-config.schema.json declares', () => {
    for (const key of Object.keys(getConfig())) {
      expect(Object.keys(runtimeConfigSchema.properties)).toContain(key)
    }
  })
})
