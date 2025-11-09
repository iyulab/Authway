import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'

/**
 * PopupCallbackPage
 *
 * Handles OAuth callback in popup mode by sending postMessage to the opener window.
 * This page is used when authentication happens in a popup window.
 *
 * Flow:
 * 1. OAuth provider redirects to this page with code/state parameters
 * 2. This page detects it's running in a popup (window.opener exists)
 * 3. Sends postMessage with auth data to the parent window
 * 4. Closes the popup automatically
 *
 * Used by @authway/client and @authway/react SDK popup login flows.
 */
const PopupCallbackPage = () => {
  const [searchParams] = useSearchParams()

  useEffect(() => {
    // Check if running in popup window
    if (!window.opener) {
      console.error('[PopupCallback] Not running in popup window - window.opener is null')
      return
    }

    // Extract OAuth callback parameters
    const code = searchParams.get('code')
    const state = searchParams.get('state')
    const error = searchParams.get('error')
    const errorDescription = searchParams.get('error_description')

    // Prepare message for parent window
    const message = {
      type: 'authway-callback',
      code,
      state,
      error,
      error_description: errorDescription
    }

    console.log('[PopupCallback] Sending postMessage to opener:', {
      type: message.type,
      hasCode: !!code,
      hasState: !!state,
      hasError: !!error,
      origin: window.opener.origin
    })

    // Send authentication result to parent window
    // Security: Using window.opener.origin ensures message only goes to the parent
    window.opener.postMessage(message, window.opener.origin)

    // Close popup after small delay to ensure message is sent
    const closeTimer = setTimeout(() => {
      console.log('[PopupCallback] Closing popup window')
      window.close()
    }, 500)

    // Cleanup
    return () => {
      clearTimeout(closeTimer)
    }
  }, [searchParams])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="text-center max-w-md p-8">
        <div className="inline-block animate-spin rounded-full h-16 w-16 border-b-4 border-indigo-600 mb-6"></div>
        <h2 className="text-2xl font-bold text-gray-900 mb-2">
          Authentication Successful
        </h2>
        <p className="text-gray-600 mb-4">
          This window will close automatically...
        </p>
        <p className="text-sm text-gray-500">
          If the window doesn't close, you can safely close it manually.
        </p>
      </div>
    </div>
  )
}

export default PopupCallbackPage
