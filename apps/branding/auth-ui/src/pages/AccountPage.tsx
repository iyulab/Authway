import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import AccountPassword from './AccountPassword'
import AccountConnections from './AccountConnections'
import AccountDelegations from './AccountDelegations'
import { useLoad } from '../utils/useLoad'
import { httpsUrl, readStored, refusal, returnPath, signIn, useAccountSession } from './accountSession'

export type { AccountEntry } from './accountSession'

interface Status {
  enabled: boolean
  enabled_at?: string
  recovery_codes_left: number
}

interface Setup {
  secret: string
  otpauth_uri: string
  qr_image: string
  account_name: string
}

// The code exchange of each comeback, run once: the code works once, so a
// second run of the effect (React runs effects twice in development) waits for
// the first instead of failing on a used-up code.
const exchanges = new Map<string, Promise<unknown>>()

/**
 * The comeback from sign-in: finishes the authorization code exchange, then
 * returns to the screen that sent the user — the account screen of the
 * application they came from, or the approval screen.
 */
export const AccountCallbackPage: React.FC = () => {
  const { t } = useTranslation(['auth'])
  const navigate = useNavigate()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const stored = readStored()
    if (!stored) {
      setFailed(true)
      return
    }
    const key = window.location.search
    if (!exchanges.has(key)) exchanges.set(key, signIn(stored.entry).handleRedirectCallback())
    exchanges
      .get(key)!
      .then(() => navigate(returnPath(stored), { replace: true }))
      .catch(() => setFailed(true))
  }, [navigate])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <p className="text-sm text-gray-600">{failed ? t('auth:account.signInFailed') : t('auth:account.signingIn')}</p>
    </div>
  )
}

/**
 * The account screen: a signed-in user manages their own sign-in — for now
 * the second factor and its recovery codes. An application links here with
 * its client_id; the screen signs the user in with the tenant's account
 * client (no screen at all when their session is still there) and offers the
 * way back to the application.
 */
const AccountPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const appClientId = searchParams.get('client_id')
  const { entry, email, client, call, fatal, setFatal } = useAccountSession(
    appClientId,
    `/account?client_id=${encodeURIComponent(appClientId ?? '')}`,
  )
  const [status, setStatus] = useState<Status | null>(null)
  const [setup, setSetup] = useState<Setup | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [signInAgain, setSignInAgain] = useState(false)
  const [code, setCode] = useState('')
  const [action, setAction] = useState<'disable' | 'regenerate' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const loadStatus = useCallback(
    async (signal?: AbortSignal) => {
      const res = await call('/api/v1/users/mfa/status', { signal })
      if (!res.ok) throw new Error('status')
      const answer = (await res.json()) as Status
      if (!signal?.aborted) setStatus(answer)
    },
    [call],
  )

  useLoad(
    useCallback(async (signal: AbortSignal) => {
      if (client) await loadStatus(signal)
    }, [client, loadStatus]),
    () => setFatal(t('common:errorOccurred')),
  )

  const run = async (work: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await work()
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  const startSetup = () =>
    run(async () => {
      const res = await call('/api/v1/users/mfa/setup', { method: 'POST' })
      if (!res.ok) throw new Error('setup')
      setSetup((await res.json()) as Setup)
      setCode('')
    })

  /** Sends the code with a call that needs one; true when it was accepted. */
  const withCode = async (path: string, method: string): Promise<Response | null> => {
    const res = await call(path, { method, body: JSON.stringify({ code: code.trim() }) })
    if (res.ok) return res
    const problem = await refusal(res)
    setError(problem.code === 'invalid_code' || problem.code === 'mfa_code_required' ? t('auth:account.wrongCode') : (problem.detail ?? t('common:errorOccurred')))
    return null
  }

  const confirmSetup = (e: React.FormEvent) => {
    e.preventDefault()
    void run(async () => {
      const res = await withCode('/api/v1/users/mfa/verify', 'POST')
      if (!res) return
      const body = (await res.json()) as { recovery_codes: string[]; sessions_ended: 'others' | 'all' | 'none' }
      setSetup(null)
      setCodes(body.recovery_codes)
      setCode('')
      if (body.sessions_ended === 'all') setSignInAgain(true)
      else await loadStatus()
    })
  }

  const confirmAction = (e: React.FormEvent) => {
    e.preventDefault()
    void run(async () => {
      if (action === 'disable') {
        if (!(await withCode('/api/v1/users/mfa', 'DELETE'))) return
        setCodes(null)
      } else {
        const res = await withCode('/api/v1/users/mfa/recovery/regenerate', 'POST')
        if (!res) return
        setCodes(((await res.json()) as { recovery_codes: string[] }).recovery_codes)
      }
      setAction(null)
      setCode('')
      await loadStatus()
    })
  }

  const app = entry?.app ?? readStored()?.entry.app
  const backHref = httpsUrl(app?.client_uri)
  const back = backHref ? (
    <a href={backHref} className="text-sm font-medium text-indigo-600 hover:text-indigo-500">
      {t('auth:account.backTo', { app: app!.name })}
    </a>
  ) : null

  const page = (body: React.ReactNode) => (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher />
      </div>
      <div className="max-w-md w-full space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 text-center">{t('auth:account.title')}</h1>
          {email && <p className="mt-1 text-center text-sm text-gray-600">{email}</p>}
        </div>
        {body}
        {back && <div className="text-center">{back}</div>}
      </div>
    </div>
  )

  if (!appClientId) return page(<p className="text-sm text-gray-600 text-center">{t('auth:account.noApp')}</p>)
  if (fatal) return page(<p role="alert" className="text-sm text-red-600 text-center">{fatal}</p>)
  if (!entry || !status) return page(<p className="text-sm text-gray-600 text-center">{t('auth:account.signingIn')}</p>)

  const codeInput = (
    <input
      type="text"
      inputMode="numeric"
      autoComplete="one-time-code"
      aria-label={t('auth:account.code')}
      placeholder={t('auth:account.code')}
      value={code}
      onChange={(e) => setCode(e.target.value)}
      className="appearance-none block w-full px-3 py-2 border border-gray-300 rounded-md text-center tracking-widest focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
    />
  )
  const errorLine = error && (
    <p role="alert" className="text-sm text-red-600">
      {error}
    </p>
  )

  return page(
    <>
    <section className="bg-white shadow rounded-lg p-6 space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">{t('auth:account.twoFactor')}</h2>

      {codes && (
        <div className="space-y-2">
          <p className="text-sm text-gray-700">{t('auth:account.recoveryCodesIntro')}</p>
          <ul aria-label={t('auth:account.recoveryCodes')} className="grid grid-cols-2 gap-1 font-mono text-sm bg-gray-50 p-3 rounded">
            {codes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
          {signInAgain && <p className="text-sm text-amber-700">{t('auth:account.signInAgain')}</p>}
        </div>
      )}

      {signInAgain ? (
        <button
          type="button"
          onClick={() => void client!.loginWithRedirect()}
          className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 text-sm font-medium"
        >
          {t('auth:account.signInAgainButton')}
        </button>
      ) : setup ? (
        <form onSubmit={confirmSetup} className="space-y-3">
          <p className="text-sm text-gray-700">{t('auth:account.scan')}</p>
          <img src={setup.qr_image} alt={t('auth:account.qrAlt')} width={200} height={200} className="mx-auto" />
          <p className="text-xs text-gray-600">
            {t('auth:account.manualKey')} <code className="font-mono break-all">{setup.secret}</code>
          </p>
          {codeInput}
          {errorLine}
          <button type="submit" disabled={busy} className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-sm font-medium">
            {t('auth:account.turnOn')}
          </button>
          <button type="button" onClick={() => setSetup(null)} className="w-full text-sm text-gray-600 hover:text-gray-800">
            {t('common:cancel')}
          </button>
        </form>
      ) : status.enabled ? (
        <div className="space-y-3">
          <p className="text-sm text-green-700">{t('auth:account.on', { count: status.recovery_codes_left })}</p>
          {action ? (
            <form onSubmit={confirmAction} className="space-y-3">
              <p className="text-sm text-gray-700">{t(action === 'disable' ? 'auth:account.codeToTurnOff' : 'auth:account.codeToRegenerate')}</p>
              {codeInput}
              {errorLine}
              <button type="submit" disabled={busy} className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-sm font-medium">
                {t(action === 'disable' ? 'auth:account.turnOff' : 'auth:account.regenerate')}
              </button>
              <button type="button" onClick={() => setAction(null)} className="w-full text-sm text-gray-600 hover:text-gray-800">
                {t('common:cancel')}
              </button>
            </form>
          ) : (
            <div className="flex gap-2">
              <button type="button" onClick={() => setAction('regenerate')} className="flex-1 py-2 px-3 rounded-md border border-gray-300 text-sm">
                {t('auth:account.regenerate')}
              </button>
              <button type="button" onClick={() => setAction('disable')} className="flex-1 py-2 px-3 rounded-md border border-red-300 text-red-700 text-sm">
                {t('auth:account.turnOff')}
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          <p className="text-sm text-gray-700">{t('auth:account.off')}</p>
          {errorLine}
          <button type="button" onClick={startSetup} disabled={busy} className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-sm font-medium">
            {t('auth:account.setUp')}
          </button>
        </div>
      )}
    </section>
    <AccountPassword
      call={call}
      signInAgain={(fresh) => void client!.loginWithRedirect(fresh ? { prompt: 'login' } : {})}
    />
    <AccountConnections
      call={call}
      signInAgain={(fresh) => void client!.loginWithRedirect(fresh ? { prompt: 'login' } : {})}
    />
    <AccountDelegations call={call} />
    </>,
  )
}

export default AccountPage
