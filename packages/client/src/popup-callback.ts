/**
 * Auto-executing popup callback handler
 *
 * This module automatically handles OAuth popup callbacks when imported.
 * Use this in your callback page instead of manually creating callback.html:
 *
 * Option 1: Import in your callback page (React/Next.js/Vue)
 * ```tsx
 * // app/callback/page.tsx or pages/callback.tsx
 * import '@authway/client/popup-callback'
 * ```
 *
 * Option 2: Add as a script tag
 * ```html
 * <script src="https://unpkg.com/@authway/client/dist/popup-callback.js"></script>
 * ```
 *
 * This will:
 * 1. Detect if the page is running in a popup with OAuth params
 * 2. Send the auth result to the parent window via postMessage
 * 3. Close the popup automatically
 * 4. If not in popup mode, do nothing (let your app handle redirect flow)
 */

import { AuthwayClient } from './AuthwayClient'

// Auto-execute on import (side-effect)
if (typeof window !== 'undefined') {
  // Run after DOM is ready to ensure all params are available
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      AuthwayClient.handlePopupCallback()
    })
  } else {
    AuthwayClient.handlePopupCallback()
  }
}

/**
 * Manually handle popup callback
 * Returns true if this page is a popup callback and was handled
 */
export function handlePopupCallback(): boolean {
  return AuthwayClient.handlePopupCallback()
}

/**
 * Check if current page is a popup callback
 * Useful for conditional rendering in callback pages
 */
export function isPopupCallback(): boolean {
  if (typeof window === 'undefined') return false

  const params = new URLSearchParams(window.location.search)
  const hasOAuthParams = (params.has('code') && params.has('state')) || params.has('error')

  if (!hasOAuthParams) return false

  // Check if we're in a popup
  try {
    return !!(window.opener && !window.opener.closed)
  } catch {
    // COOP policy may block access - use alternative detection
    return window.name === 'authway-login' ||
           sessionStorage.getItem('authway_popup_context') === 'true'
  }
}
