import { getConfig } from '../config'

export type SocialProvider = 'google' | 'github' | 'microsoft' | 'apple'

/** What a login-flow endpoint tells the screen to do next. */
export interface FlowStep {
  next?: 'form' | 'redirect' | 'mfa'
  redirect_to?: string
  mfa_challenge?: string
  error?: string
  code?: string
}

/** URL of a login-flow endpoint. The flow id is opaque; it is only ever passed back. */
export function loginFlowUrl(flow: string, path = ''): string {
  return `${getConfig().apiUrl}/api/v1/login-flows/${encodeURIComponent(flow)}${path}`
}

/** POSTs a step of a login flow and returns the backend's answer. */
export async function submitLoginStep(flow: string, path: string, body: unknown): Promise<FlowStep> {
  const response = await fetch(loginFlowUrl(flow, path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return response.json()
}

/**
 * Leaves for a social provider. This is a page navigation, not a fetch: the
 * backend binds the sign-in to this browser with a cookie it can only set
 * first-party, then redirects to the provider.
 */
export function startSocialSignIn(flow: string, provider: SocialProvider): void {
  // A popup must keep window.opener across the provider round trip; the flag
  // survives the cross-origin redirects that window.opener checks cannot.
  const isPopupMode = window.opener !== null && window.opener !== window
  if (isPopupMode) {
    sessionStorage.setItem('authway_popup_mode', 'true')
  } else {
    sessionStorage.removeItem('authway_popup_mode')
  }
  window.location.assign(loginFlowUrl(flow, `/social/${provider}`))
}
