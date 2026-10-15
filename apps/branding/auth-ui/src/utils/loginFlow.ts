import { getConfig } from '../config'

export type SocialProvider = 'google' | 'github' | 'microsoft' | 'apple'

/** A document the user's tenant asks them to accept at sign-in. */
export interface TermsDocument {
  kind: string
  version: string
  url: string
  title?: string
}

/** What a login-flow endpoint tells the screen to do next. */
export interface FlowStep {
  next?: 'form' | 'redirect' | 'mfa' | 'terms' | 'email_sent'
  redirect_to?: string
  mfa_challenge?: string
  terms_challenge?: string
  terms?: TermsDocument[]
  /** The flow a second factor or terms step belongs to. */
  flow?: string
  detail?: string
  code?: string
}

/**
 * The screen a step that is not finished goes on to — the second factor, or
 * the documents the user's tenant asks them to accept — or undefined when the
 * step is something else. `flow` is used when the step does not name its own.
 */
export function nextScreen(step: FlowStep, flow?: string | null): string | undefined {
  const owner = step.flow ?? flow
  if (!owner) return undefined
  if (step.next === 'mfa' && step.mfa_challenge) {
    return `/mfa/verify?${new URLSearchParams({ flow: owner, mfa_challenge: step.mfa_challenge })}`
  }
  if (step.next === 'terms' && step.terms_challenge) {
    return `/terms?${new URLSearchParams({ flow: owner, terms_challenge: step.terms_challenge })}`
  }
  return undefined
}

export type FlowKind = 'login-flows' | 'consent-flows' | 'logout-flows' | 'device-flows'

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
