import { useCallback, useEffect, useRef, useState } from 'react'
import { beginPasskeySignIn, finishPasskeySignIn, type FlowStep } from './loginFlow'
import { conditionalMediationAvailable, getPasskey, PasskeyCancelled, type RequestOptionsJSON } from './passkey'

/** What went wrong on this side of a passkey sign-in; the server's refusals arrive as steps. */
export type PasskeyProblem = 'cancelled' | 'failed' | string

interface Options {
  /** The login flow being completed. */
  flow: string | null
  /** The screen offers a passkey here and the browser can make one. */
  enabled: boolean
  /** Also offer passkeys in the autofill of the address field. */
  autofill: boolean
  /** The user's choice to stay signed in, read when the answer is sent. */
  remember: () => boolean
  /** The server's answer to a finished attempt: signed in, a second factor owed, or a refusal. */
  onStep: (step: FlowStep) => void
  /** A failure that is not the server's answer. */
  onProblem: (problem: PasskeyProblem) => void
}

type Outcome = 'done' | 'expired' | 'ended'

/**
 * Signing in with a passkey on the login screen.
 *
 * Two ways in. The button (signIn) opens the browser's own prompt. The autofill
 * way, when the browser has it, waits behind the address field: the browser
 * lists the user's passkeys with the saved addresses, and choosing one signs
 * in. Only one request to the browser is open at a time, so pressing the
 * button ends the waiting one, and the autofill way starts again afterwards.
 */
export function usePasskeySignIn(options: Options): { signIn: () => void; busy: boolean } {
  const latest = useRef(options)
  latest.current = options
  const current = useRef<AbortController | null>(null)
  const [busy, setBusy] = useState(false)
  const [again, setAgain] = useState(0)

  /** One ceremony: ask the server, ask the device, tell the server. */
  const attempt = useCallback(async (conditional: boolean, signal: AbortSignal): Promise<Outcome> => {
    const { flow, remember, onStep, onProblem } = latest.current
    if (!flow) return 'ended'
    const begun = await beginPasskeySignIn(flow)
    if (!begun.ceremony_id || !begun.options) {
      // The server said why it will not (a client that offers no passkey, a flow
      // that ended); that is the answer to show.
      onStep(begun)
      return 'ended'
    }
    let credential: Record<string, unknown>
    try {
      credential = await getPasskey(begun.options as RequestOptionsJSON, { conditional, signal })
    } catch (error) {
      if (error instanceof PasskeyCancelled) {
        if (!signal.aborted && !conditional) onProblem('cancelled')
        return 'ended'
      }
      throw error
    }
    if (signal.aborted) return 'ended'
    setBusy(true)
    const step = await finishPasskeySignIn(flow, begun.ceremony_id, credential, remember())
    if (step.code === 'ceremony_expired') {
      setBusy(false)
      return 'expired'
    }
    onStep(step)
    return 'done'
  }, [])

  const signIn = useCallback(() => {
    current.current?.abort()
    const controller = new AbortController()
    current.current = controller
    setBusy(true)
    attempt(false, controller.signal)
      .then((outcome) => {
        // The ceremony ran out while the prompt was open: say so, and let the
        // user press the button again.
        if (outcome === 'expired') latest.current.onProblem('expired')
      })
      .catch(() => latest.current.onProblem('failed'))
      .finally(() => {
        if (current.current === controller) current.current = null
        setBusy(false)
        setAgain((n) => n + 1)
      })
  }, [attempt])

  const { enabled, autofill, flow } = options
  useEffect(() => {
    if (!enabled || !autofill || !flow) return
    const controller = new AbortController()
    current.current = controller
    let stopped = false
    void (async () => {
      if (!(await conditionalMediationAvailable()) || stopped) return
      try {
        // A ceremony lasts minutes; a field can wait for longer. When the user
        // finally chooses and the server has forgotten the ceremony, start a
        // new one and keep waiting.
        let outcome: Outcome = 'expired'
        while (outcome === 'expired' && !stopped) {
          outcome = await attempt(true, controller.signal)
        }
      } catch {
        // Nothing the user asked for failed; the button is still there.
      } finally {
        if (!stopped) setBusy(false)
      }
    })()
    return () => {
      stopped = true
      controller.abort()
      if (current.current === controller) current.current = null
    }
  }, [enabled, autofill, flow, again, attempt])

  return { signIn, busy }
}
