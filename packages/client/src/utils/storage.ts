/**
 * Storage interface for token cache
 */
export interface IStorage {
  get(key: string): string | null
  set(key: string, value: string): void
  remove(key: string): void
  clear(): void
}

/**
 * Memory storage: the tokens live in the page and nowhere else
 */
export class MemoryStorage implements IStorage {
  private store: Map<string, string> = new Map()

  get(key: string): string | null {
    return this.store.get(key) || null
  }

  set(key: string, value: string): void {
    this.store.set(key, value)
  }

  remove(key: string): void {
    this.store.delete(key)
  }

  clear(): void {
    this.store.clear()
  }
}

/**
 * LocalStorage wrapper
 */
export class LocalStorageCache implements IStorage {
  constructor(private prefix: string) {}

  get(key: string): string | null {
    if (typeof window === 'undefined') return null
    try {
      return window.localStorage.getItem(this.prefix + key)
    } catch {
      return null
    }
  }

  set(key: string, value: string): void {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(this.prefix + key, value)
    } catch (error) {
      console.warn('Failed to write to localStorage:', error)
    }
  }

  remove(key: string): void {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.removeItem(this.prefix + key)
    } catch {
      // Ignore errors
    }
  }

  clear(): void {
    if (typeof window === 'undefined') return
    try {
      const keys = Object.keys(window.localStorage)
      keys.forEach(key => {
        if (key.startsWith(this.prefix)) {
          window.localStorage.removeItem(key)
        }
      })
    } catch {
      // Ignore errors
    }
  }
}

/**
 * SessionStorage wrapper: the state of an authorization request in flight,
 * and — under its own prefix — the token cache of `cacheLocation:
 * 'sessionstorage'`.
 */
export class SessionStorageCache implements IStorage {
  constructor(private prefix = 'authway.') {}

  get(key: string): string | null {
    if (typeof window === 'undefined') return null
    try {
      return window.sessionStorage.getItem(this.prefix + key)
    } catch {
      return null
    }
  }

  set(key: string, value: string): void {
    if (typeof window === 'undefined') return
    try {
      window.sessionStorage.setItem(this.prefix + key, value)
    } catch (error) {
      console.warn('Failed to write to sessionStorage:', error)
    }
  }

  remove(key: string): void {
    if (typeof window === 'undefined') return
    try {
      window.sessionStorage.removeItem(this.prefix + key)
    } catch {
      // Ignore errors
    }
  }

  clear(): void {
    if (typeof window === 'undefined') return
    try {
      const keys = Object.keys(window.sessionStorage)
      keys.forEach(key => {
        if (key.startsWith(this.prefix)) {
          window.sessionStorage.removeItem(key)
        }
      })
    } catch {
      // Ignore errors
    }
  }
}

/**
 * Create the token cache of one client.
 *
 * The keys carry the client id: applications served from one origin share
 * its storage, and one must neither read nor clear what another keeps there.
 * The prefix is also apart from that of a request in flight, so that clearing
 * the tokens leaves the request alone.
 */
export function createStorage(cacheLocation: 'memory' | 'sessionstorage' | 'localstorage', clientId: string): IStorage {
  const prefix = `authway.cache.${clientId}.`
  if (cacheLocation === 'localstorage') {
    return new LocalStorageCache(prefix)
  }
  if (cacheLocation === 'sessionstorage') {
    return new SessionStorageCache(prefix)
  }
  return new MemoryStorage()
}

/**
 * Versions before 0.5.0 kept tokens in localStorage under keys that named no
 * client. This removes them and returns the ID token that was among them, if
 * any — nothing else about them is of use: whose they were is not known
 * until that token is read.
 *
 * TODO: remove in 0.7.0, by when a browser that last ran 0.4.x has little
 * left to clean up — what remains then has expired.
 */
export function removeUnscopedTokens(): string | null {
  if (typeof window === 'undefined') return null
  try {
    const idToken = window.localStorage.getItem('authway.idToken')
    for (const key of ['accessToken', 'idToken', 'refreshToken', 'expiresAt', 'authway_dpop_key']) {
      window.localStorage.removeItem(`authway.${key}`)
    }
    return idToken
  } catch {
    return null
  }
}

/**
 * Remembers, on this browser, that a user has signed in to a client — and
 * nothing else about them. A page that holds no tokens (a reload with tokens
 * kept in memory, a new tab) uses it to decide whether asking the provider
 * for the session is worth a round trip: without it every visitor, signed in
 * or not, would be sent there on every page load.
 */
export interface SessionHint {
  has(): boolean
  set(): void
  clear(): void
}

export function createSessionHint(clientId: string): SessionHint {
  const key = `authway.signed-in.${clientId}`
  const store = (): Storage | null => {
    try {
      return typeof window === 'undefined' ? null : window.localStorage
    } catch {
      return null
    }
  }
  return {
    has: () => {
      try {
        return store()?.getItem(key) === '1'
      } catch {
        return false
      }
    },
    set: () => {
      try {
        store()?.setItem(key, '1')
      } catch {
        // Without it the session is simply not looked for.
      }
    },
    clear: () => {
      try {
        store()?.removeItem(key)
      } catch {
        // Nothing to clear.
      }
    }
  }
}

/**
 * Create session storage for OAuth state (PKCE, appState)
 */
export function createSessionStorage(): IStorage {
  return new SessionStorageCache()
}
