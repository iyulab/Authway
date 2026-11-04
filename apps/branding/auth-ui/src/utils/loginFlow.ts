import { getConfig } from '../config'
import { contract } from './contract'

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

/**
 * What a flow endpoint answers, whether it went well or not: the next step, or
 * the problem that stopped it. Both are read the same way by the screens.
 */
export type FlowAnswer = FlowStep & Record<string, any>

/** Reads either answer of a call to the contract's client; an empty body is an empty answer. */
async function answer(call: Promise<{ data?: unknown; error?: unknown }>): Promise<FlowAnswer> {
  const { data, error } = await call
  return (data ?? error ?? {}) as FlowAnswer
}

/** The flow id is opaque to the screens; they only pass it back. */
const flowParams = (flow: string) => ({ params: { path: { flow } } })

/** What a login flow shows: the form, or a way on without one. */
export function getLoginFlow(flow: string): Promise<FlowAnswer> {
  return answer(contract().GET('/api/v1/login-flows/{flow}', flowParams(flow)))
}

export function submitPassword(flow: string, email: string, password: string, remember: boolean): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/password', { ...flowParams(flow), body: { email, password, remember } }))
}

/** The second factor: a code from the authenticator, or a recovery code. */
export function submitSecondFactor(flow: string, mfaChallenge: string, code: string, recovery: boolean): Promise<FlowAnswer> {
  const body = { mfa_challenge: mfaChallenge, code }
  return answer(
    recovery
      ? contract().POST('/api/v1/login-flows/{flow}/mfa/recovery', { ...flowParams(flow), body })
      : contract().POST('/api/v1/login-flows/{flow}/mfa', { ...flowParams(flow), body }),
  )
}

export function requestMagicLink(flow: string, email: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/magic-link', { ...flowParams(flow), body: { email } }))
}

export function submitMagicLinkCode(flow: string, code: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/magic-link/code', { ...flowParams(flow), body: { code } }))
}

/** The documents the tenant asks the user to accept, for the challenge a sign-in answered with. */
export function getTerms(flow: string, termsChallenge: string): Promise<FlowAnswer> {
  return answer(contract().GET('/api/v1/login-flows/{flow}/terms', { ...flowParams(flow), params: { path: { flow }, query: { terms_challenge: termsChallenge } } }))
}

export function acceptTerms(flow: string, termsChallenge: string, accepted: { kind: string; version: string }[]): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/terms', { ...flowParams(flow), body: { terms_challenge: termsChallenge, accepted } }))
}

export function declineTerms(flow: string, termsChallenge: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/terms/decline', { ...flowParams(flow), body: { terms_challenge: termsChallenge } }))
}

/** What a consent flow asks, or a way on without asking. */
export function getConsentFlow(flow: string): Promise<FlowAnswer> {
  return answer(contract().GET('/api/v1/consent-flows/{flow}', flowParams(flow)))
}

export function acceptConsent(flow: string, grantScope: string[], remember: boolean): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/consent-flows/{flow}/accept', { ...flowParams(flow), body: { grant_scope: grantScope, remember } }))
}

export function rejectConsent(flow: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/consent-flows/{flow}/reject', flowParams(flow)))
}

export function completeLogout(flow: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/logout-flows/{flow}', flowParams(flow)))
}

/** The code a user typed on the device page. */
export function submitDeviceUserCode(flow: string, userCode: string): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/device-flows/{flow}', { ...flowParams(flow), body: { user_code: userCode } }))
}

/** What starting a passkey sign-in answers: the browser's input, or why not. */
export interface PasskeyBegin extends FlowStep {
  ceremony_id?: string
  /** PublicKeyCredentialRequestOptionsJSON, passed to the browser as it is. */
  options?: unknown
}

export function beginPasskeySignIn(flow: string): Promise<PasskeyBegin> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/passkey/begin', flowParams(flow))) as Promise<PasskeyBegin>
}

/** Sends the device's answer (PublicKeyCredential.toJSON()) and returns the flow's next step. */
export function finishPasskeySignIn(flow: string, ceremonyId: string, credential: Record<string, unknown>, remember: boolean): Promise<FlowAnswer> {
  return answer(contract().POST('/api/v1/login-flows/{flow}/passkey/finish', { ...flowParams(flow), body: { ceremony_id: ceremonyId, credential, remember } }))
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
  window.location.assign(`${getConfig().apiUrl}/api/v1/login-flows/${encodeURIComponent(flow)}/social/${provider}`)
}
