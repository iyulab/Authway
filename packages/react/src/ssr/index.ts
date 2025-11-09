/**
 * SSR utilities for Next.js and other server-side rendering frameworks
 */

import { AuthwayConfig } from '@authway/client'

/**
 * Check if we're running in a browser environment
 */
export function isBrowser(): boolean {
  return typeof window !== 'undefined'
}

/**
 * Create a safe config for SSR that won't break on the server
 * Auto-generates redirectUri on the client side only
 */
export function createSSRConfig(config: Omit<AuthwayConfig, 'redirectUri'> & { redirectUri?: string }): AuthwayConfig {
  return {
    ...config,
    redirectUri: config.redirectUri || (isBrowser() ? window.location.origin : 'https://placeholder.local')
  }
}

/**
 * Wrapper for browser-only code
 * Prevents hydration mismatches in SSR frameworks
 *
 * @example
 * ```tsx
 * import { withBrowserOnly } from '@authway/react/ssr'
 *
 * function MyComponent() {
 *   const content = withBrowserOnly(
 *     () => <UserProfile />,
 *     () => <div>Loading...</div>
 *   )
 *
 *   return content
 * }
 * ```
 */
export function withBrowserOnly<T>(
  browserComponent: () => T,
  serverFallback?: () => T
): T | null {
  if (isBrowser()) {
    return browserComponent()
  }
  return serverFallback ? serverFallback() : null
}

/**
 * Hook for detecting hydration completion
 * Useful for preventing hydration mismatches with authentication state
 *
 * Note: This should be imported from 'react' in your component
 *
 * @example
 * ```tsx
 * import { useState, useEffect } from 'react'
 *
 * function MyComponent() {
 *   const [hydrated, setHydrated] = useState(false)
 *
 *   useEffect(() => {
 *     setHydrated(true)
 *   }, [])
 *
 *   if (!hydrated) {
 *     return <div>Loading...</div>
 *   }
 *
 *   return <UserProfile />
 * }
 * ```
 */
export function createHydrationHook() {
  return function useHydrated(): boolean {
    // This will be implemented by the consumer using their React instance
    // to avoid bundling React twice
    throw new Error(
      'useHydrated must be implemented in your app. ' +
      'Use: const [hydrated, setHydrated] = useState(false); useEffect(() => setHydrated(true), [])'
    )
  }
}

/**
 * Type guard to check if we're in a server environment
 */
export function isServer(): boolean {
  return !isBrowser()
}

/**
 * Safe wrapper for localStorage that works in SSR
 * Returns null on server, actual value on client
 */
export function getLocalStorageItem(key: string): string | null {
  if (!isBrowser()) {
    return null
  }

  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/**
 * Safe wrapper for sessionStorage that works in SSR
 * Returns null on server, actual value on client
 */
export function getSessionStorageItem(key: string): string | null {
  if (!isBrowser()) {
    return null
  }

  try {
    return sessionStorage.getItem(key)
  } catch {
    return null
  }
}
