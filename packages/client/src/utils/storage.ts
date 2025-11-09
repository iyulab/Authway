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
 * Memory storage (default, most secure)
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
  private prefix = 'authway.'

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
 * SessionStorage wrapper for OAuth state
 */
export class SessionStorageCache implements IStorage {
  private prefix = 'authway.'

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
 * Create storage based on cache location
 */
export function createStorage(cacheLocation: 'memory' | 'localstorage'): IStorage {
  if (cacheLocation === 'localstorage') {
    return new LocalStorageCache()
  }
  return new MemoryStorage()
}

/**
 * Create session storage for OAuth state (PKCE, appState)
 */
export function createSessionStorage(): IStorage {
  return new SessionStorageCache()
}
