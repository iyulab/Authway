import { describe, it, expect, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useLoad } from './useLoad'

describe('useLoad', () => {
  it('reports a failure of a load that is still wanted', async () => {
    const onFail = vi.fn()
    const load = () => Promise.reject(new Error('down'))
    renderHook(() => useLoad(load, onFail))
    await waitFor(() => expect(onFail).toHaveBeenCalledTimes(1))
  })

  it('stops the load when the component goes away, and says nothing of it', async () => {
    const onFail = vi.fn()
    let seen: AbortSignal | undefined
    // A request that only ends by being aborted, as fetch does.
    const load = (signal: AbortSignal) =>
      new Promise<void>((_, reject) => {
        seen = signal
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
      })
    const { unmount } = renderHook(() => useLoad(load, onFail))
    unmount()
    expect(seen?.aborted).toBe(true)
    await Promise.resolve()
    expect(onFail).not.toHaveBeenCalled()
  })
})
