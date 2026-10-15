import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { AuthwayClient } from '@authway/client'
import LanguageSwitcher from '../components/LanguageSwitcher'
import AccountPassword from './AccountPassword'
import { getConfig } from '../config'

/** What GET /api/v1/account-entry answers: the client this screen signs in with, and the application to go back to. */
export interface AccountEntry {
  client_id: string
  redirect_uri: string
  scope: string
  app: { name: string; client_uri?: string }
}

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

/** Where the entry is kept between leaving for sign-in and coming back to /account/callback. */
const ENTRY_KEY = 'authway.account.entry'

const apiBase = () => getConfig().apiUrl || window.location.origin

/** The application's address when it is an https URL — the only kind it registers, and the only kind followed. */
const httpsUrl = (raw?: string): string | undefined => {
  try {
    const u = raw ? new URL(raw) : undefined
    return u && u.protocol === 'https:' ? u.toString() : undefined
  } catch {
    return undefined
  }
}

const signIn = (entry: AccountEntry) =>
  new AuthwayClient({
    domain: apiBase(),
    clientId: entry.client_id,
    redirectUri: entry.redirect_uri,
    scope: entry.scope,
    cacheLocation: 'sessionstorage',
  })

interface Stored {
  appClientId: string
  entry: AccountEntry
}

const readStored = (): Stored | null => {
  try {
    const raw = sessionStorage.getItem(ENTRY_KEY)
    return raw ? (JSON.parse(raw) as Stored) : null
  } catch {
    return null
  }
}

/** A refusal's message, or null when the answer is not one. */
const refusal = async (res: Response): Promise<{ code?: string; detail?: string }> => {
  try {
    return (await res.json()) as { code?: string; detail?: string }
  } catch {
    return {}
  }
}

// The code exchange of each comeback, run once: the code works once, so a
// second run of the effect (React runs effects twice in development) waits for
// the first instead of failing on a used-up code.
const exchanges = new Map<string, Promise<unknown>>()

/**
 * The comeback from sign-in: finishes the authorization code exchange, then
 * returns to the account screen of the application the user came from.
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
      .then(() => navigate(`/account?client_id=${encodeURIComponent(stored.appClientId)}`, { replace: true }))
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
  const [entry, setEntry] = useState<AccountEntry | null>(null)
  const [email, setEmail] = useState<string | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const [setup, setSetup] = useState<Setup | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [signInAgain, setSignInAgain] = useState(false)
  const [code, setCode] = useState('')
  const [action, setAction] = useState<'disable' | 'regenerate' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [fatal, setFatal] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const client = useMemo(() => (entry ? signIn(entry) : null), [entry])

  /** Calls the account API with the user's token. */
  const call = useCallback(
    async (path: string, init: RequestInit = {}) => {
      const token = await client!.getAccessToken()
      return fetch(`${apiBase()}${path}`, {
        ...init,
        headers: { Authorization: `Bearer ${token}`, ...(init.body ? { 'Content-Type': 'application/json' } : {}) },
      })
    },
    [client],
  )

  const loadStatus = useCallback(async () => {
    const res = await call('/api/v1/users/mfa/status')
    if (!res.ok) throw new Error('status')
    setStatus((await res.json()) as Status)
  }, [call])

  // Find the account client for the application, then make sure the user is signed in to it.
  useEffect(() => {
    if (!appClientId) return
    let cancelled = false
    ;(async () => {
      const res = await fetch(`${apiBase()}/api/v1/account-entry?client_id=${encodeURIComponent(appClientId)}`)
      if (!res.ok) {
        if (!cancelled) setFatal(res.status === 404 ? t('auth:account.unknownApp') : t('common:errorOccurred'))
        return
      }
      const answer = (await res.json()) as AccountEntry
      sessionStorage.setItem(ENTRY_KEY, JSON.stringify({ appClientId, entry: answer } satisfies Stored))
      const sdk = signIn(answer)
      if (!(await sdk.isAuthenticated()) && !(await sdk.checkSession().catch(() => false))) {
        // A run that was cleaned up must not start a sign-in: two would each
        // store their own state, and the one that comes back would not match.
        if (!cancelled) await sdk.loginWithRedirect()
        return
      }
      if (cancelled) return
      setEntry(answer)
      setEmail((await sdk.getUser())?.email ?? null)
    })().catch(() => !cancelled && setFatal(t('common:errorOccurred')))
    return () => {
      cancelled = true
    }
  }, [appClientId, t])

  useEffect(() => {
    if (client) loadStatus().catch(() => setFatal(t('common:errorOccurred')))
  }, [client, loadStatus, t])

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
    </>,
  )
}

export default AccountPage
