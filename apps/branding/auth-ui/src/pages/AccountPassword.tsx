import React, { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  /** Calls the account API with the user's token. */
  call: (path: string, init?: RequestInit) => Promise<Response>
  /** Sends the user through sign-in again — when every session ended, or setting a first password needs a recent sign-in. */
  signInAgain: (fresh: boolean) => void
}

/**
 * The account screen's password section: change the password — or, for an
 * account that signs in by link or another provider, set a first one.
 */
const AccountPassword: React.FC<Props> = ({ call, signInAgain }) => {
  const { t } = useTranslation(['auth', 'common'])
  const [hasPassword, setHasPassword] = useState<boolean | null>(null)
  const [open, setOpen] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [needsSignIn, setNeedsSignIn] = useState(false)
  const [done, setDone] = useState<'others' | 'all' | 'none' | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    call('/api/v1/profile/me')
      .then(async (res) => (res.ok ? setHasPassword(Boolean((await res.json()).has_password)) : setHasPassword(true)))
      .catch(() => setHasPassword(true))
  }, [call])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (next !== confirm) {
      setError(t('auth:account.password.mismatch'))
      return
    }
    setBusy(true)
    try {
      const res = await call('/api/v1/users/password', {
        method: 'POST',
        body: JSON.stringify(hasPassword ? { current_password: current, new_password: next } : { new_password: next }),
      })
      const body = await res.json().catch(() => ({}))
      if (res.ok) {
        setDone(body.sessions_ended)
        setHasPassword(true)
        setOpen(false)
        setCurrent('')
        setNext('')
        setConfirm('')
        return
      }
      if (body.code === 'insufficient_user_authentication') setNeedsSignIn(true)
      else if (body.code === 'invalid_current_password') setError(t('auth:account.password.wrongCurrent'))
      else if (body.code === 'password_too_short') setError(t('auth:account.password.tooShort', { count: body.min_length }))
      else setError(body.detail ?? t('common:errorOccurred'))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  if (hasPassword === null) return null
  const field = (id: string, label: string, value: string, set: (v: string) => void, autoComplete: string) => (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-gray-700">
        {label}
      </label>
      <input
        id={id}
        type="password"
        autoComplete={autoComplete}
        value={value}
        onChange={(e) => set(e.target.value)}
        className="mt-1 appearance-none block w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
      />
    </div>
  )

  return (
    <section className="bg-white shadow rounded-lg p-6 space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">{t('auth:account.password.title')}</h2>
      {done && (
        <p role="status" className="text-sm text-green-700">
          {t(done === 'all' ? 'auth:account.password.changedSignInAgain' : 'auth:account.password.changed')}
        </p>
      )}
      {done === 'all' || needsSignIn ? (
        <div className="space-y-3">
          {needsSignIn && <p className="text-sm text-amber-700">{t('auth:account.password.signInFirst')}</p>}
          <button
            type="button"
            onClick={() => signInAgain(needsSignIn)}
            className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 text-sm font-medium"
          >
            {t('auth:account.signInAgainButton')}
          </button>
        </div>
      ) : open ? (
        <form onSubmit={submit} className="space-y-3">
          {hasPassword && field('current-password', t('auth:account.password.current'), current, setCurrent, 'current-password')}
          {field('new-password', t('auth:account.password.new'), next, setNext, 'new-password')}
          {field('confirm-password', t('auth:account.password.confirm'), confirm, setConfirm, 'new-password')}
          {error && (
            <p role="alert" className="text-sm text-red-600">
              {error}
            </p>
          )}
          <button
            type="submit"
            disabled={busy || !next || (hasPassword && !current)}
            className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-sm font-medium"
          >
            {t(hasPassword ? 'auth:account.password.change' : 'auth:account.password.set')}
          </button>
          <button type="button" onClick={() => setOpen(false)} className="w-full text-sm text-gray-600 hover:text-gray-800">
            {t('common:cancel')}
          </button>
        </form>
      ) : (
        <div className="space-y-3">
          <p className="text-sm text-gray-700">{t(hasPassword ? 'auth:account.password.has' : 'auth:account.password.none')}</p>
          <button type="button" onClick={() => setOpen(true)} className="w-full py-2 px-3 rounded-md border border-gray-300 text-sm">
            {t(hasPassword ? 'auth:account.password.change' : 'auth:account.password.set')}
          </button>
        </div>
      )}
    </section>
  )
}

export default AccountPassword
