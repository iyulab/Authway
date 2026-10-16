import { useEffect, useRef } from 'react'

/**
 * Runs load when the component appears, and again when load changes; stops it
 * when the component goes away or load changes. The request is aborted through
 * the signal load is given, and a failure of a load that was stopped is not
 * reported: nothing of it reaches a component that is gone.
 *
 * load should not set state once its signal is aborted.
 */
export function useLoad(load: (signal: AbortSignal) => Promise<void>, onFail: () => void): void {
  const fail = useRef(onFail)
  fail.current = onFail

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal).catch(() => {
      if (!controller.signal.aborted) fail.current()
    })
    return () => controller.abort()
  }, [load])
}
