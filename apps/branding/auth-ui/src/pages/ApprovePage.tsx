import React, { useCallback, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { useLoad } from '../utils/useLoad'
import { httpsUrl, refusal, useAccountSession } from './accountSession'

/** What GET /api/v1/users/approvals/{id} answers. */
interface Approval {
  id: string
  client_id: string
  client_name: string
  binding_message: string
  scopes: string[]
  authorization_details?: { type?: string }[]
  status: 'pending' | 'approved' | 'denied' | 'expired' | 'consumed'
  expires_at: string
}

type Outcome = 'approved' | 'denied' | 'settled' | 'notFound'

/**
 * The approval screen: an application acting for the user while they are
 * away asks them to approve one action. The application brings the user here
 * by its own means (approve_url); the user signs in to the tenant's account
 * application — a token the asking application holds cannot answer — checks
 * that the code matches the one the application shows, and approves or
 * denies. What the action is, the application shows itself: this screen shows
 * only the code, the kinds of action and the permissions.
 */
const ApprovePage: React.FC = () => {
  const { t, i18n } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get('request')
  const appClientId = searchParams.get('client_id')
  const returnTo = `/approve?${new URLSearchParams({ request: requestId ?? '', client_id: appClientId ?? '' })}`
  const { entry, email, client, call, fatal } = useAccountSession(requestId ? appClientId : null, returnTo)
  const [approval, setApproval] = useState<Approval | null>(null)
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [needsSignIn, setNeedsSignIn] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useLoad(
    useCallback(
      async (signal: AbortSignal) => {
        if (!client || !requestId) return
        const res = await call(`/api/v1/users/approvals/${encodeURIComponent(requestId)}`, { signal })
        if (res.status === 404) {
          if (!signal.aborted) setOutcome('notFound')
          return
        }
        if (!res.ok) throw new Error('approval')
        const answer = (await res.json()) as Approval
        if (!signal.aborted) setApproval(answer)
      },
      [client, call, requestId],
    ),
    () => setError(t('common:errorOccurred')),
  )

  const answer = async (decision: 'approve' | 'deny') => {
    if (!approval) return
    setBusy(true)
    setError(null)
    try {
      const res = await call(`/api/v1/users/approvals/${encodeURIComponent(approval.id)}/${decision}`, { method: 'POST' })
      if (res.ok) {
        setOutcome(decision === 'approve' ? 'approved' : 'denied')
        return
      }
      const problem = await refusal(res)
      if (problem.code === 'insufficient_user_authentication') setNeedsSignIn(true)
      else if (problem.code === 'approval_settled') setOutcome('settled')
      else if (res.status === 404) setOutcome('notFound')
      else setError(problem.detail ?? t('common:errorOccurred'))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setBusy(false)
    }
  }

  const appName = approval?.client_name || entry?.app.name || ''
  const backHref = httpsUrl(entry?.app.client_uri)

  const page = (body: React.ReactNode) => (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher />
      </div>
      <div className="max-w-md w-full space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 text-center">{t('auth:approve.title')}</h1>
          {email && <p className="mt-1 text-center text-sm text-gray-600">{email}</p>}
        </div>
        <section className="bg-white shadow rounded-lg p-6 space-y-4">{body}</section>
        {backHref && (
          <div className="text-center">
            <a href={backHref} className="text-sm font-medium text-indigo-600 hover:text-indigo-500">
              {t('auth:account.backTo', { app: entry!.app.name })}
            </a>
          </div>
        )}
      </div>
    </div>
  )
  const note = (text: string, role?: 'alert' | 'status') => (
    <p role={role} className={`text-sm text-center ${role === 'alert' ? 'text-red-600' : 'text-gray-700'}`}>
      {text}
    </p>
  )

  if (!requestId || !appClientId) return page(note(t('auth:approve.noRequest')))
  if (fatal) return page(note(fatal, 'alert'))
  if (outcome === 'approved') return page(note(t('auth:approve.approved', { app: appName }), 'status'))
  if (outcome === 'denied') return page(note(t('auth:approve.denied', { app: appName }), 'status'))
  if (outcome === 'settled') return page(note(t('auth:approve.settled'), 'status'))
  if (outcome === 'notFound') return page(note(t('auth:approve.notFound'), 'alert'))
  if (error && !approval) return page(note(error, 'alert'))
  if (!entry || !approval) return page(note(t('auth:account.signingIn')))

  if (approval.status !== 'pending') {
    return page(note(approval.status === 'expired' ? t('auth:approve.expired') : t('auth:approve.settled'), 'status'))
  }

  const kinds = [...new Set((approval.authorization_details ?? []).map((d) => d.type).filter((k): k is string => !!k))]
  const until = new Date(approval.expires_at).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })

  return page(
    <>
      <p className="text-sm text-gray-700">{t('auth:approve.asks', { app: appName })}</p>
      <div className="text-center">
        <p className="text-xs text-gray-500">{t('auth:approve.codeLabel', { app: appName })}</p>
        <p aria-label={t('auth:approve.code')} className="mt-1 font-mono text-2xl font-semibold tracking-wider text-gray-900 break-all">
          {approval.binding_message}
        </p>
      </div>
      {kinds.length > 0 && (
        <div>
          <p className="text-xs text-gray-500">{t('auth:approve.kinds')}</p>
          <ul aria-label={t('auth:approve.kinds')} className="mt-1 flex flex-wrap gap-1">
            {kinds.map((k) => (
              <li key={k} className="px-2 py-0.5 rounded bg-gray-100 text-xs font-mono text-gray-800">
                {k}
              </li>
            ))}
          </ul>
        </div>
      )}
      {approval.scopes.length > 0 && (
        <p className="text-xs text-gray-600">
          {t('auth:approve.scopes')} <span className="font-mono">{approval.scopes.join(' ')}</span>
        </p>
      )}
      <p className="text-xs text-gray-500">{t('auth:approve.until', { time: until })}</p>
      <p className="text-xs text-gray-500">{t('auth:approve.mismatch')}</p>
      {error && note(error, 'alert')}
      {needsSignIn ? (
        <>
          {note(t('auth:approve.signInAgain'))}
          <button
            type="button"
            onClick={() => void client!.loginWithRedirect({ prompt: 'login' })}
            className="w-full py-2 px-4 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 text-sm font-medium"
          >
            {t('auth:account.signInAgainButton')}
          </button>
        </>
      ) : (
        <div className="flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => void answer('deny')}
            className="flex-1 py-2 px-3 rounded-md border border-gray-300 text-sm disabled:opacity-50"
          >
            {t('auth:approve.deny')}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void answer('approve')}
            className="flex-1 py-2 px-3 rounded-md text-white bg-indigo-600 hover:bg-indigo-700 text-sm font-medium disabled:opacity-50"
          >
            {t('auth:approve.approve')}
          </button>
        </div>
      )}
    </>,
  )
}

export default ApprovePage
