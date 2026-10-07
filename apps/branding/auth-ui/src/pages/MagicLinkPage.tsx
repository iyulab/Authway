import React, { useEffect, useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { getConfig } from '../config'
import { followRedirect, submitLoginStep } from '../utils/loginFlow'

interface LinkInfo {
  valid: boolean
  email?: string
  error?: string
}

// Full class names, so the stylesheet build sees them.
const TONES = {
  indigo: { circle: 'bg-indigo-100', icon: 'text-indigo-600' },
  green: { circle: 'bg-green-100', icon: 'text-green-600' },
  red: { circle: 'bg-red-100', icon: 'text-red-600' },
}

const cardIcon = (tone: keyof typeof TONES, path: string) => (
  <div className={`mx-auto h-12 w-12 flex items-center justify-center rounded-full ${TONES[tone].circle}`}>
    <svg className={`h-6 w-6 ${TONES[tone].icon}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={path} />
    </svg>
  </div>
)
const MAIL_ICON = 'M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z'
const LINK_ICON = 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1'
const ERROR_ICON = 'M6 18L18 6M6 6l12 12'

/**
 * Sign-in by emailed link, in two halves:
 *  - `?flow=` — part of a login: ask for the address and send the link.
 *  - `?token=` — the link's landing page: confirm, then complete the login.
 *    It must be the browser that started the sign-in.
 * The landing page never redeems on load: mail scanners open links too.
 */
const MagicLinkPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const flow = searchParams.get('flow')
  const token = searchParams.get('token')

  const [email, setEmail] = useState('')
  const [isSent, setIsSent] = useState(false)
  const [isBusy, setIsBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [linkInfo, setLinkInfo] = useState<LinkInfo | null>(null)

  const post = async (path: string, body: unknown) => {
    const res = await fetch(`${getConfig().apiUrl}/api/v1${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    return res.json()
  }

  // Landing: say whose link this is before anything is used up.
  useEffect(() => {
    if (!token) return
    post('/magic-links/inspect', { token })
      .then((data: LinkInfo) => setLinkInfo(data))
      .catch(() => setLinkInfo({ valid: false }))
  }, [token])

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!flow || !email) return
    setIsBusy(true)
    setError(null)
    try {
      const data = await submitLoginStep(flow, '/magic-link', { email })
      if (data.next === 'email_sent') {
        setIsSent(true)
      } else {
        setError(data.error || t('auth:magicLink.sendFailed', 'The sign-in link could not be sent.'))
      }
    } catch {
      setError(t('auth:magicLink.sendFailed', 'The sign-in link could not be sent.'))
    } finally {
      setIsBusy(false)
    }
  }

  const handleRedeem = async () => {
    setIsBusy(true)
    setError(null)
    try {
      const data = await post('/magic-links/redeem', { token })
      if (data.next === 'redirect' && data.redirect_to) {
        followRedirect(data.redirect_to)
        return
      }
      setError(data.error || t('auth:magicLink.invalidLink', 'Invalid sign-in link'))
    } catch {
      setError(t('auth:magicLink.invalidLink', 'Invalid sign-in link'))
    }
    setIsBusy(false)
  }

  const page = (children: React.ReactNode) => (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>
      <div className="max-w-md w-full space-y-8">{children}</div>
    </div>
  )

  // ---- landing page of an emailed link ----
  if (token) {
    if (!linkInfo) {
      return page(
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto"></div>
        </div>
      )
    }
    if (!linkInfo.valid || error) {
      return page(
        <div className="text-center">
          {cardIcon('red', ERROR_ICON)}
          <h2 className="mt-6 text-2xl font-bold text-gray-900">{t('auth:magicLink.invalidLink', 'Invalid sign-in link')}</h2>
          <p className="mt-2 text-sm text-red-600">{error || linkInfo.error}</p>
          <p className="mt-4 text-sm text-gray-600">{t('auth:magicLink.startAgain', 'Start again from the application.')}</p>
        </div>
      )
    }
    return page(
      <div className="text-center">
        {cardIcon('indigo', LINK_ICON)}
        <h2 className="mt-6 text-2xl font-bold text-gray-900">{t('auth:magicLink.confirmTitle', 'Sign in')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:magicLink.confirmAs', 'Continue as')}</p>
        <p className="font-medium text-gray-900">{linkInfo.email}</p>
        <button
          type="button"
          onClick={handleRedeem}
          disabled={isBusy}
          className="mt-6 w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
        >
          {t('auth:magicLink.continue', 'Continue')}
        </button>
      </div>
    )
  }

  // ---- part of a login: send the link ----
  if (!flow) {
    return page(
      <div className="text-center">
        {cardIcon('red', ERROR_ICON)}
        <h2 className="mt-6 text-2xl font-bold text-gray-900">{t('auth:magicLink.invalidLink', 'Invalid sign-in link')}</h2>
        <p className="mt-4 text-sm text-gray-600">{t('auth:magicLink.startAgain', 'Start again from the application.')}</p>
      </div>
    )
  }

  if (isSent) {
    return page(
      <div className="text-center">
        {cardIcon('green', MAIL_ICON)}
        <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:magicLink.checkEmail', 'Check your email')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:magicLink.sentTo', 'If this address can sign in, we sent a link to')}</p>
        <p className="font-medium text-gray-900">{email}</p>
        <p className="mt-4 text-sm text-gray-500">
          {t('auth:magicLink.sameBrowser', 'Open the link in this browser within 15 minutes to sign in.')}
        </p>
      </div>
    )
  }

  return page(
    <>
      <div className="text-center">
        {cardIcon('indigo', LINK_ICON)}
        <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:magicLink.title', 'Sign in with an email link')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:magicLink.description', "We'll email you a link to sign in without a password")}</p>
      </div>
      <form onSubmit={handleSend} className="mt-8 space-y-6">
        {error && (
          <div className="rounded-md bg-red-50 p-4">
            <div className="text-sm text-red-700">{error}</div>
          </div>
        )}
        <div>
          <label htmlFor="email" className="block text-sm font-medium text-gray-700">
            {t('auth:login.emailLabel', 'Email address')}
          </label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
            placeholder={t('auth:login.emailPlaceholder', 'you@example.com')}
            autoComplete="email"
            autoFocus
          />
        </div>
        <button
          type="submit"
          disabled={isBusy || !email}
          className="w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {t('auth:magicLink.sendLink', 'Email me a sign-in link')}
        </button>
        <div className="text-center">
          <button
            type="button"
            onClick={() => navigate(`/login?flow=${encodeURIComponent(flow)}`)}
            className="text-sm text-indigo-600 hover:text-indigo-500"
          >
            {t('auth:magicLink.backToLogin', 'Back to sign in')}
          </button>
        </div>
      </form>
    </>
  )
}

export default MagicLinkPage
