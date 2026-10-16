import React, { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  /** Calls the account API with the user's token. */
  call: (path: string, init?: RequestInit) => Promise<Response>
}

interface Delegation {
  id: string
  client_id: string
  client_name: string
  scopes: string[]
  created_at: string
}

/**
 * The account screen's delegations: the applications the user lets act for
 * them while they are away, and taking that back — nothing to show when there
 * is none.
 */
const AccountDelegations: React.FC<Props> = ({ call }) => {
  const { t, i18n } = useTranslation(['auth', 'common'])
  const [delegations, setDelegations] = useState<Delegation[] | null>(null)
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    const res = await call('/api/v1/users/delegations')
    if (!res.ok) throw new Error('delegations')
    setDelegations(((await res.json()) as { delegations: Delegation[] }).delegations)
  }, [call])

  useEffect(() => {
    load().catch(() => setDelegations([]))
  }, [load])

  const revoke = async (id: string) => {
    setBusy(true)
    setError(null)
    try {
      const res = await call(`/api/v1/users/delegations/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (res.ok || res.status === 404) {
        setConfirming(null)
        await load()
        return
      }
      const body = await res.json().catch(() => ({}))
      setError(body.detail ?? t('common:errorOccurred'))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  if (!delegations || delegations.length === 0) return null

  const since = (iso: string) => new Date(iso).toLocaleDateString(i18n.language)

  return (
    <section className="bg-white shadow rounded-lg p-6 space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">{t('auth:account.delegations.title')}</h2>
      <p className="text-sm text-gray-700">{t('auth:account.delegations.intro')}</p>
      <ul className="divide-y divide-gray-200">
        {delegations.map((d) => (
          <li key={d.id} className="py-2 flex items-center justify-between gap-2">
            <span className="text-sm">
              <span className="font-medium text-gray-900">{d.client_name || d.client_id}</span>
              <span className="block text-gray-500">
                {t('auth:account.delegations.detail', { scopes: d.scopes.join(', '), since: since(d.created_at) })}
              </span>
            </span>
            {confirming === d.id ? (
              <span className="flex gap-2 shrink-0">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void revoke(d.id)}
                  className="py-1 px-3 rounded-md border border-red-300 text-red-700 text-sm disabled:opacity-50"
                >
                  {t('auth:account.delegations.confirm', { app: d.client_name || d.client_id })}
                </button>
                <button type="button" onClick={() => setConfirming(null)} className="py-1 px-3 text-sm text-gray-600">
                  {t('common:cancel')}
                </button>
              </span>
            ) : (
              <button type="button" onClick={() => setConfirming(d.id)} className="py-1 px-3 rounded-md border border-gray-300 text-sm shrink-0">
                {t('auth:account.delegations.revoke')}
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
    </section>
  )
}

export default AccountDelegations
