import React, { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLoad } from '../utils/useLoad'
import { createPasskey, PasskeyCancelled, PasskeyExists, passkeysSupported, type CreationOptionsJSON } from '../utils/passkey'

interface Props {
  /** Calls the account API with the user's token. */
  call: (path: string, init?: RequestInit) => Promise<Response>
  /** Sends the user through sign-in again - changing how they sign in needs a recent sign-in. */
  signInAgain: (fresh: boolean) => void
}

interface Passkey {
  id: string
  name: string
  created_at: string
  last_used_at: string | null
  synced: boolean
}

/**
 * The account screen's passkeys: the ones the user has, adding one on this
 * device, and removing one. Nothing is shown where the server has no passkeys
 * (it answers 404), and nothing can be added in a browser that cannot make one.
 */
const AccountPasskeys: React.FC<Props> = ({ call, signInAgain }) => {
  const { t, i18n } = useTranslation(['auth', 'common'])
  const [passkeys, setPasskeys] = useState<Passkey[] | null>(null)
  const [name, setName] = useState('')
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [needsSignIn, setNeedsSignIn] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      const res = await call('/api/v1/users/passkeys', { signal })
      // Not set up on this server: nothing to show.
      if (!res.ok) {
        if (!signal?.aborted) setPasskeys(null)
        return
      }
      const list = ((await res.json()) as { passkeys: Passkey[] }).passkeys
      if (!signal?.aborted) setPasskeys(list)
    },
    [call],
  )

  useLoad(load, () => setPasskeys(null))

  const date = (iso: string) => new Date(iso).toLocaleDateString(i18n.language)

  /** What a refusal of a change means for the user. */
  const explain = (body: { code?: string; detail?: string }) => {
    switch (body.code) {
      case 'insufficient_user_authentication':
        setNeedsSignIn(true)
        break
      case 'last_sign_in_method':
        setError(t('auth:account.passkeys.lastWay'))
        break
      case 'passkey_already_registered':
        setError(t('auth:account.passkeys.alreadyRegistered'))
        break
      case 'ceremony_expired':
        setError(t('auth:account.passkeys.expired'))
        break
      default:
        setError(t('auth:account.passkeys.failed'))
    }
  }

  const add = async () => {
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const begun = await call('/api/v1/users/passkeys/register/begin', { method: 'POST' })
      const options = await begun.json().catch(() => ({}))
      if (!begun.ok) {
        explain(options)
        return
      }
      const credential = await createPasskey(options.options as CreationOptionsJSON)
      const finished = await call('/api/v1/users/passkeys/register/finish', {
        method: 'POST',
        body: JSON.stringify({ ceremony_id: options.ceremony_id, name: name.trim(), credential }),
      })
      if (!finished.ok) {
        explain(await finished.json().catch(() => ({})))
        return
      }
      setName('')
      setNotice(t('auth:account.passkeys.added'))
      await load()
    } catch (e) {
      setError(
        e instanceof PasskeyCancelled
          ? t('auth:account.passkeys.cancelled')
          : e instanceof PasskeyExists
            ? t('auth:account.passkeys.alreadyRegistered')
            : t('auth:account.passkeys.failed'),
      )
    } finally {
      setBusy(false)
    }
  }

  const remove = async (id: string) => {
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const res = await call(`/api/v1/users/passkeys/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (res.ok) {
        setConfirming(null)
        await load()
        return
      }
      explain(await res.json().catch(() => ({})))
    } catch {
      setError(t('auth:account.passkeys.failed'))
    } finally {
      setBusy(false)
    }
  }

  if (!passkeys) return null
  const canAdd = passkeysSupported()
  if (passkeys.length === 0 && !canAdd) return null

  return (
    <section className="bg-white shadow rounded-lg p-6 space-y-4" aria-labelledby="passkeys-title">
      <h2 id="passkeys-title" className="text-lg font-semibold text-gray-900">
        {t('auth:account.passkeys.title')}
      </h2>
      <p className="text-sm text-gray-700">{t('auth:account.passkeys.intro')}</p>
      {passkeys.length > 0 && (
        <ul className="divide-y divide-gray-200">
          {passkeys.map((p) => (
            <li key={p.id} className="py-2 flex items-center justify-between gap-2">
              <span>
                <span className="block text-sm font-medium text-gray-900">{p.name}</span>
                <span className="block text-xs text-gray-600">
                  {t('auth:account.passkeys.created', { date: date(p.created_at) })}
                  {' · '}
                  {p.last_used_at ? t('auth:account.passkeys.lastUsed', { date: date(p.last_used_at) }) : t('auth:account.passkeys.neverUsed')}
                  {p.synced && ` · ${t('auth:account.passkeys.synced')}`}
                </span>
              </span>
              {confirming === p.id ? (
                <span className="flex gap-2">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void remove(p.id)}
                    className="py-1 px-3 rounded-md border border-red-300 text-red-700 text-sm disabled:opacity-50"
                  >
                    {t('auth:account.passkeys.confirm', { name: p.name })}
                  </button>
                  <button type="button" onClick={() => setConfirming(null)} className="py-1 px-3 text-sm text-gray-600">
                    {t('common:cancel')}
                  </button>
                </span>
              ) : (
                <button type="button" onClick={() => setConfirming(p.id)} className="py-1 px-3 rounded-md border border-gray-300 text-sm">
                  {t('auth:account.passkeys.remove')}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canAdd && (
        <div className="space-y-2">
          <label htmlFor="passkey-name" className="block text-sm font-medium text-gray-700">
            {t('auth:account.passkeys.nameLabel')}
          </label>
          <div className="flex gap-2">
            <input
              id="passkey-name"
              value={name}
              maxLength={100}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('auth:account.passkeys.namePlaceholder')}
              className="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm"
            />
            <button
              type="button"
              disabled={busy}
              onClick={() => void add()}
              className="py-1 px-3 rounded-md border border-gray-300 text-sm disabled:opacity-50"
            >
              {busy ? t('auth:account.passkeys.adding') : t('auth:account.passkeys.add')}
            </button>
          </div>
        </div>
      )}
      {notice && (
        <p role="status" className="text-sm text-green-700">
          {notice}
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-red-600">
          {error}
        </p>
      )}
      {needsSignIn && (
        <div className="space-y-3">
          <p className="text-sm text-amber-700">{t('auth:account.passkeys.signInFirst')}</p>
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

export default AccountPasskeys
