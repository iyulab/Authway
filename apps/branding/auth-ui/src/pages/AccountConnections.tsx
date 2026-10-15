import React, { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  /** Calls the account API with the user's token. */
  call: (path: string, init?: RequestInit) => Promise<Response>
  /** Sends the user through sign-in again — removing a link needs a recent sign-in. */
  signInAgain: (fresh: boolean) => void
}

const NAMES: Record<string, string> = { google: 'Google', github: 'GitHub', microsoft: 'Microsoft', apple: 'Apple' }

/**
 * The account screen's connections: the provider accounts that sign in to
 * this one, and removing a link — nothing to show when there is none.
 */
const AccountConnections: React.FC<Props> = ({ call, signInAgain }) => {
  const { t } = useTranslation(['auth', 'common'])
  const [providers, setProviders] = useState<string[] | null>(null)
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [needsSignIn, setNeedsSignIn] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    const res = await call('/api/v1/users/connections')
    if (!res.ok) throw new Error('connections')
    setProviders(((await res.json()) as { connections: { provider: string }[] }).connections.map((c) => c.provider))
  }, [call])

  useEffect(() => {
    load().catch(() => setProviders([]))
  }, [load])

  const remove = async (provider: string) => {
    setBusy(true)
    setError(null)
    try {
      const res = await call(`/api/v1/users/connections/${encodeURIComponent(provider)}`, { method: 'DELETE' })
      if (res.ok) {
        setConfirming(null)
        await load()
        return
      }
      const body = await res.json().catch(() => ({}))
      if (body.code === 'insufficient_user_authentication') setNeedsSignIn(true)
      else if (body.code === 'last_sign_in_method') setError(t('auth:account.connections.lastWay'))
      else setError(body.detail ?? t('common:errorOccurred'))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  if (!providers || providers.length === 0) return null

  return (
    <section className="bg-white shadow rounded-lg p-6 space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">{t('auth:account.connections.title')}</h2>
      <p className="text-sm text-gray-700">{t('auth:account.connections.intro')}</p>
      <ul className="divide-y divide-gray-200">
        {providers.map((p) => (
          <li key={p} className="py-2 flex items-center justify-between gap-2">
            <span className="text-sm font-medium text-gray-900">{NAMES[p] ?? p}</span>
            {confirming === p ? (
              <span className="flex gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void remove(p)}
                  className="py-1 px-3 rounded-md border border-red-300 text-red-700 text-sm disabled:opacity-50"
                >
                  {t('auth:account.connections.confirm', { provider: NAMES[p] ?? p })}
                </button>
                <button type="button" onClick={() => setConfirming(null)} className="py-1 px-3 text-sm text-gray-600">
                  {t('common:cancel')}
                </button>
              </span>
            ) : (
              <button type="button" onClick={() => setConfirming(p)} className="py-1 px-3 rounded-md border border-gray-300 text-sm">
                {t('auth:account.connections.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
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
