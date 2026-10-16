import { describe, it, expect } from 'vitest'
import { returnPath, type Stored } from './accountSession'

const entry = { client_id: 'authway_account', redirect_uri: 'http://localhost:3001/account/callback', scope: 'openid', app: { name: 'Notes' } }

// After sign-in the user comes back to the screen that sent them — only ever
// a page of this site.
describe('returnPath', () => {
  const at = (returnTo?: string): Stored => ({ appClientId: 'notes app', entry, returnTo })

  it('goes back to the screen that sent the user', () => {
    expect(returnPath(at('/approve?request=r&client_id=notes'))).toBe('/approve?request=r&client_id=notes')
  })

  it('goes to the account screen when none is named, or one that is not a page of this site', () => {
    for (const returnTo of [undefined, '', 'https://elsewhere.example/', '//elsewhere.example/x', '/\\elsewhere.example/x', 'javascript:alert(1)']) {
      expect(returnPath(at(returnTo)), String(returnTo)).toBe('/account?client_id=notes%20app')
    }
  })
})
