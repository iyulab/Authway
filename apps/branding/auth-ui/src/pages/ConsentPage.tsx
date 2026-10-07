import React, { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { useTranslation, Trans } from 'react-i18next'
import { flowUrl, followRedirect, submitFlowStep, type FlowStep } from '../utils/loginFlow'

interface ConsentPageInfo {
  next: 'form'
  flow: string
  client_name: string
  requested_scope: string[]
  user: {
    email: string
    name: string
  }
}

const ConsentPage: React.FC = () => {
  const { t } = useTranslation(['consent', 'common'])
  const [searchParams] = useSearchParams()
  const [consentInfo, setConsentInfo] = useState<ConsentPageInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedScopes, setSelectedScopes] = useState<string[]>([])
  const [rememberConsent, setRememberConsent] = useState(true) // Default to true for better UX

  const flow = searchParams.get('flow')

  // Fetch what the consent flow asks
  useEffect(() => {
    if (!flow) {
      setError(t('consent:errors.missingChallenge'))
      setIsLoading(false)
      return
    }

    fetch(flowUrl('consent-flows', flow))
      .then(res => res.json())
      .then(data => {
        if (data.next === 'redirect' && data.redirect_to) {
          // Nothing to ask (the client skips consent, or single sign-on).
          // Keep the loading state while the browser leaves.
          followRedirect(data.redirect_to)
        } else if (data.error) {
          setError(data.error)
          setIsLoading(false)
        } else {
          setConsentInfo(data)
          // Every requested scope starts selected.
          setSelectedScopes(data.requested_scope || [])
          setIsLoading(false)
        }
      })
      .catch(err => {
        console.error('Consent flow fetch error:', err)
        setError(t('consent:errors.fetchFailed'))
        setIsLoading(false)
      })
  }, [flow, t])

  const handleAnswer = (data: FlowStep) => {
    if (data.next === 'redirect' && data.redirect_to) {
      followRedirect(data.redirect_to)
    } else if (data.error) {
      setError(data.error)
    }
  }

  // Accept consent mutation
  const acceptMutation = useMutation({
    mutationFn: (): Promise<FlowStep> =>
      submitFlowStep('consent-flows', flow ?? '', '/accept', {
        grant_scope: selectedScopes,
        remember: rememberConsent,
        remember_for: rememberConsent ? 3600 : 0, // 1 hour
      }),
    onSuccess: handleAnswer,
    onError: (error) => {
      console.error('Consent accept error:', error)
      setError(t('consent:errors.approveFailed'))
    },
  })

  // Reject consent mutation
  const rejectMutation = useMutation({
    mutationFn: (): Promise<FlowStep> => submitFlowStep('consent-flows', flow ?? '', '/reject'),
    onSuccess: handleAnswer,
    onError: (error) => {
      console.error('Consent reject error:', error)
      setError(t('consent:errors.denyFailed'))
    },
  })

  const handleScopeToggle = (scope: string) => {
    setSelectedScopes(prev =>
      prev.includes(scope)
        ? prev.filter(s => s !== scope)
        : [...prev, scope]
    )
  }

  const handleAccept = () => {
    setError(null)
    acceptMutation.mutate()
  }

  const handleReject = () => {
    setError(null)
    rejectMutation.mutate()
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600" data-testid="loading-spinner"></div>
      </div>
    )
  }

  if (error && !consentInfo) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('common:errorOccurred')}</h2>
            <p className="mt-2 text-sm text-red-600">{error}</p>
          </div>
        </div>
      </div>
    )
  }

  if (!consentInfo) {
    return null
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-lg w-full space-y-8">
        <div>
          <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
            <svg
              className="h-6 w-6 text-indigo-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            {t('consent:title')}
          </h2>
          <div className="mt-4 text-center">
            <p className="text-sm text-gray-600">
              {t('consent:greeting', { name: consentInfo.user.name || consentInfo.user.email })}
            </p>
            <p className="text-sm text-gray-600 mt-2">
              <Trans
                i18nKey="consent:subtitle"
                values={{ clientName: consentInfo.client_name }}
                components={{ strong: <span className="font-medium" /> }}
              />
            </p>
          </div>
        </div>

        <div className="mt-8 space-y-6">
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          {/* 권한 목록 */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium text-gray-900">{t('consent:requestedPermissions')}</h3>
            <div className="space-y-3">
              {consentInfo.requested_scope.map((scope) => {
                const scopeInfo = t(`consent:scopes.${scope}.name`, { defaultValue: '' })
                  ? {
                      name: t(`consent:scopes.${scope}.name`),
                      description: t(`consent:scopes.${scope}.description`),
                    }
                  : {
                      name: scope,
                      description: t('consent:genericPermission', { scope }),
                    }

                return (
                  <div key={scope} className="flex items-start">
                    <div className="flex items-center h-5">
                      <input
                        id={scope}
                        type="checkbox"
                        checked={selectedScopes.includes(scope)}
                        onChange={() => handleScopeToggle(scope)}
                        className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                      />
                    </div>
                    <div className="ml-3 text-sm">
                      <label htmlFor={scope} className="font-medium text-gray-700">
                        {scopeInfo.name}
                      </label>
                      <p className="text-gray-500">{scopeInfo.description}</p>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          {/* 동의 저장 옵션 */}
          <div className="flex items-center">
            <input
              id="remember"
              type="checkbox"
              checked={rememberConsent}
              onChange={(e) => setRememberConsent(e.target.checked)}
              className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
            />
            <label htmlFor="remember" className="ml-2 block text-sm text-gray-900">
              {t('consent:rememberChoice')}
            </label>
          </div>

          {/* 버튼들 */}
          <div className="flex space-x-4">
            <button
              onClick={handleReject}
              disabled={rejectMutation.isPending}
              className="flex-1 flex justify-center py-2 px-4 border border-gray-300 rounded-md shadow-xs text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {rejectMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600 mr-2"></div>
                  {t('consent:denying')}
                </div>
              ) : (
                t('consent:denyButton')
              )}
            </button>

            <button
              onClick={handleAccept}
              disabled={acceptMutation.isPending || selectedScopes.length === 0}
              className="flex-1 flex justify-center py-2 px-4 border border-transparent rounded-md shadow-xs text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {acceptMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  {t('consent:approving')}
                </div>
              ) : (
                t('consent:approveButtonWithCount', { count: selectedScopes.length })
              )}
            </button>
          </div>

          <div className="text-center">
            <p className="text-xs text-gray-500">
              {t('consent:approveInfo', { clientName: consentInfo.client_name })}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ConsentPage
