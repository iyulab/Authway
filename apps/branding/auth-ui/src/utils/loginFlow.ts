import { getConfig } from '../config'

export type SocialProvider = 'google' | 'github' | 'microsoft' | 'apple'

/** What a login-flow endpoint tells the screen to do next. */
export interface FlowStep {
  next?: 'form' | 'redirect' | 'mfa' | 'email_sent'
  redirect_to?: string
  mfa_challenge?: string
  error?: string
  code?: string
}

export type FlowKind = 'login-flows' | 'consent-flows' | 'logout-flows'

/** URL of a flow endpoint. The flow id is opaque; it is only ever passed back. */
export function flowUrl(kind: FlowKind, flow: string, path = ''): string {
  return `${getConfig().apiUrl}/api/v1/${kind}/${encodeURIComponent(flow)}${path}`
}

export function loginFlowUrl(flow: string, path = ''): string {
  return flowUrl('login-flows', flow, path)
}

/** POSTs a step of a flow and returns the backend's answer. */
export async function submitFlowStep(kind: FlowKind, flow: string, path: string, body?: unknown): Promise<FlowStep> {
  const response = await fetch(flowUrl(kind, flow, path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  return response.json()
}

export function submitLoginStep(flow: string, path: string, body: unknown): Promise<FlowStep> {
  return submitFlowStep('login-flows', flow, path, body)
}

/**
 * Follows a flow's redirect_to. A popup navigates in place as well — the
 * client's callback page in the popup reports back to the opener — except
 * for response_mode=form_post, which cannot run in a popup, so the popup
 * flag is dropped first.
 */
export function followRedirect(redirectTo: string): void {
  try {
    if (new URL(redirectTo).searchParams.get('response_mode') === 'form_post') {
      sessionStorage.removeItem('authway_popup_mode')
    }
  } catch {
    // not an absolute URL; navigate as given
  }
  window.location.href = redirectTo
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
