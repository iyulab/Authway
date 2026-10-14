import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { followRedirect, loginFlowUrl, submitLoginStep, type FlowStep, type TermsDocument } from '../utils/loginFlow'

const versionKey = (d: TermsDocument) => `${d.kind}\u0000${d.version}`

/**
 * The document's address, if it is one to link to. The backend only stores
 * https addresses; a screen does not rely on that to keep a javascript: or
 * data: address out of an href.
 */
function documentLink(url: string): string | undefined {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'https:' ? parsed.href : undefined
  } catch {
    return undefined
  }
}

/**
 * The documents the user's tenant asks them to accept before the sign-in
 * completes — after every factor, or when a session is reused and a document
 * was revised. Each opens where the application publishes it; the user ticks
 * each one and continues, or declines and goes back to the application.
 */
const TermsPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const flow = searchParams.get('flow')
  const challenge = searchParams.get('terms_challenge')

  const [terms, setTerms] = useState<TermsDocument[] | null>(null)
  const [ticked, setTicked] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [revised, setRevised] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!flow || !challenge) return
    fetch(`${loginFlowUrl(flow, '/terms')}?${new URLSearchParams({ terms_challenge: challenge })}`)
      .then((res) => res.json() as Promise<FlowStep>)
      .then((step) => {
        if (step.next === 'terms' && step.terms) {
          setTerms(step.terms)
        } else {
          setError(t('auth:terms.expired'))
        }
      })
      .catch(() => setError(t('auth:terms.expired')))
  }, [flow, challenge, t])

  const name = (d: TermsDocument) => d.title || t(`auth:terms.kinds.${d.kind}`, { defaultValue: d.kind })

  const toggle = (d: TermsDocument) => {
    setTicked((prev) => {
      const next = new Set(prev)
      if (next.has(versionKey(d))) {
        next.delete(versionKey(d))
      } else {
        next.add(versionKey(d))
      }
      return next
    })
  }

  const accept = async () => {
    if (!flow || !terms) return
    setBusy(true)
    setError(null)
    try {
      const step = await submitLoginStep(flow, '/terms', {
        terms_challenge: challenge,
        accepted: terms.map(({ kind, version }) => ({ kind, version })),
      })
      if (step.next === 'redirect' && step.redirect_to) {
        followRedirect(step.redirect_to)
        return
      }
      if (step.code === 'terms_mismatch' && step.terms) {
        // Revised while it was on screen: show what is owed now, unticked.
        setTerms(step.terms)
        setTicked(new Set())
        setRevised(true)
      } else {
        setError(step.code === 'invalid_terms_challenge' ? t('auth:terms.expired') : (step.error ?? t('auth:terms.failed')))
      }
    } catch {
      setError(t('auth:terms.failed'))
    }
    setBusy(false)
  }

  const decline = async () => {
    if (!flow) return
    setBusy(true)
    setError(null)
    try {
      const step = await submitLoginStep(flow, '/terms/decline', { terms_challenge: challenge })
      if (step.next === 'redirect' && step.redirect_to) {
        followRedirect(step.redirect_to)
        return
      }
      setError(step.code === 'invalid_terms_challenge' ? t('auth:terms.expired') : (step.error ?? t('auth:terms.failed')))
    } catch {
      setError(t('auth:terms.failed'))
    }
    setBusy(false)
  }

  const everyTicked = terms !== null && terms.every((d) => ticked.has(versionKey(d)))

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>
      <div className="max-w-md w-full space-y-6 bg-white p-8 rounded-lg shadow">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">{t('auth:terms.title')}</h2>
          <p className="mt-2 text-sm text-gray-600">{t('auth:terms.subtitle')}</p>
        </div>

        {(!flow || !challenge) && <div className="rounded-md bg-red-50 p-4 text-sm text-red-700">{t('auth:terms.expired')}</div>}
        {revised && <div className="rounded-md bg-yellow-50 p-4 text-sm text-yellow-800">{t('auth:terms.revised')}</div>}
        {error && <div className="rounded-md bg-red-50 p-4 text-sm text-red-700">{error}</div>}

        {terms && (
          <ul className="space-y-3">
            {terms.map((d) => (
              <li key={versionKey(d)} className="flex items-start gap-3">
                <input
                  id={`terms-${d.kind}`}
                  type="checkbox"
                  className="mt-1 h-4 w-4 rounded border-gray-300 text-indigo-600"
                  checked={ticked.has(versionKey(d))}
                  onChange={() => toggle(d)}
                />
                <label htmlFor={`terms-${d.kind}`} className="text-sm text-gray-800">
                  {t('auth:terms.agree', { name: name(d) })}
                  {documentLink(d.url) && (
                    <>
                      {' '}
                      <a href={documentLink(d.url)} target="_blank" rel="noopener noreferrer" className="text-indigo-600 hover:text-indigo-500 underline">
                        {t('auth:terms.read')}
                      </a>
                    </>
                  )}
                </label>
              </li>
            ))}
          </ul>
        )}

        {terms && (
          <div className="flex flex-col gap-3">
            <button
              type="button"
              onClick={accept}
              disabled={busy || !everyTicked}
              className="w-full py-3 px-4 rounded-md text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {t('auth:terms.accept')}
            </button>
            <button
              type="button"
              onClick={decline}
              disabled={busy}
              className="w-full py-3 px-4 rounded-md text-sm font-medium text-gray-700 bg-white border border-gray-300 hover:bg-gray-50 disabled:opacity-50"
            >
              {t('auth:terms.decline')}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export default TermsPage
