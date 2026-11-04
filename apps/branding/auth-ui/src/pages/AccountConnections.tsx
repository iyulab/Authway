import { problemMessage } from '../utils/problem'
import React, { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLoad } from '../utils/useLoad'
import { apiBase } from './accountSession'

interface Props {
  /** Calls the account API with the user's token. */
  call: (path: string, init?: RequestInit) => Promise<Response>
  /** Sends the user through sign-in again — removing a link needs a recent sign-in. */
  signInAgain: (fresh: boolean) => void
  /** Goes to an address — the provider's round trip is a page navigation. */
  goTo?: (url: string) => void
}

export const PROVIDER_NAMES: Record<string, string> = { google: 'Google', github: 'GitHub', microsoft: 'Microsoft', apple: 'Apple' }
const NAMES = PROVIDER_NAMES

/**
 * The account screen's connections: the provider accounts that sign in to
 * this one — one entry for each, with the address it was linked under — and
 * removing a link; nothing to show when there is none.
 */
interface Connection {
  id: string
  provider: string
  email: string
}

const AccountConnections: React.FC<Props> = ({ call, signInAgain, goTo = (url) => window.location.assign(url) }) => {
  const { t } = useTranslation(['auth', 'common'])
  const [connections, setConnections] = useState<Connection[] | null>(null)
  const [available, setAvailable] = useState<string[]>([])
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [needsSignIn, setNeedsSignIn] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      const res = await call('/api/v1/users/connections', { signal })
      if (!res.ok) throw new Error('connections')
      const answer = ((await res.json()) as { connections: Connection[] }).connections
      const offered = await call('/api/v1/users/connections/available', { signal })
      const providers = offered.ok ? (((await offered.json()) as { providers?: string[] }).providers ?? []) : []
      if (!signal?.aborted) {
        setConnections(answer)
        setAvailable(providers)
      }
    },
    [call],
  )

  useLoad(load, () => setConnections([]))

  /** Starts adding an account at a provider: the answer is where the browser goes next. */
  const add = async (provider: string) => {
    setBusy(true)
    setError(null)
    try {
      const res = await call(`/api/v1/users/connections/${encodeURIComponent(provider)}/link`, { method: 'POST' })
      if (res.ok) {
        goTo(`${apiBase()}${((await res.json()) as { url: string }).url}`)
        return
      }
      const body = await res.json().catch(() => ({}))
      if (body.code === 'insufficient_user_authentication') setNeedsSignIn(true)
      else setError(problemMessage(body, t('common:errorOccurred')))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (id: string) => {
    setBusy(true)
    setError(null)
    try {
      const res = await call(`/api/v1/users/connections/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (res.ok) {
        setConfirming(null)
        await load()
        return
      }
      const body = await res.json().catch(() => ({}))
      if (body.code === 'insufficient_user_authentication') setNeedsSignIn(true)
      else if (body.code === 'last_sign_in_method') setError(t('auth:account.connections.lastWay'))
      else setError(problemMessage(body, t('common:errorOccurred')))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  if (!connections || (connections.length === 0 && available.length === 0)) return null

  return (
    <section className="bg-white shadow rounded-lg p-6 space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">{t('auth:account.connections.title')}</h2>
      <p className="text-sm text-gray-700">{t('auth:account.connections.intro')}</p>
      {connections.length > 0 && (
      <ul className="divide-y divide-gray-200">
        {connections.map((c) => (
          <li key={c.id} className="py-2 flex items-center justify-between gap-2">
            <span>
              <span className="text-sm font-medium text-gray-900">{NAMES[c.provider] ?? c.provider}</span>
              {c.email && <span className="ml-2 text-sm text-gray-600">{c.email}</span>}
            </span>
            {confirming === c.id ? (
              <span className="flex gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void remove(c.id)}
                  className="py-1 px-3 rounded-md border border-red-300 text-red-700 text-sm disabled:opacity-50"
                >
                  {t('auth:account.connections.confirm', { provider: NAMES[c.provider] ?? c.provider })}
                </button>
                <button type="button" onClick={() => setConfirming(null)} className="py-1 px-3 text-sm text-gray-600">
                  {t('common:cancel')}
                </button>
              </span>
            ) : (
              <button type="button" onClick={() => setConfirming(c.id)} className="py-1 px-3 rounded-md border border-gray-300 text-sm">
                {t('auth:account.connections.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
      )}
      {available.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {available.map((p) => (
            <button
              key={p}
              type="button"
              disabled={busy}
              onClick={() => void add(p)}
              className="py-1 px-3 rounded-md border border-gray-300 text-sm disabled:opacity-50"
            >
              {t('auth:account.connections.add', { provider: NAMES[p] ?? p })}
            </button>
          ))}
        </div>
      )}
      {error && (
        <p role="alert" className="text-sm text-red-600">
          {error}
        </p>
      )}
      {needsSignIn && (
        <div className="space-y-3">
          <p className="text-sm text-amber-700">{t('auth:account.connections.signInFirst')}</p>
          <button
            type="button"
            onClick={() => signInAgain(true)}
            className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 text-sm font-medium"
          >
            {t('auth:account.signInAgainButton')}
          </button>
        </div>
      )}
    </section>
  )
}

export default AccountConnections
