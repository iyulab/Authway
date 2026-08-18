import { useEffect, useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'

interface ErrorInfo {
  title: string
  description: string
  icon: string
  color: string
}

const ErrorPage = () => {
  const { t } = useTranslation(['errors', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [errorInfo, setErrorInfo] = useState<ErrorInfo>({
    title: t('errors:title'),
    description: t('errors:codes.unknown.description'),
    icon: '⚠️',
    color: 'red',
  })

  useEffect(() => {
    const error = searchParams.get('error')
    const errorDescription = searchParams.get('error_description')

    // Log developer-friendly guidance to console
    const logDeveloperGuidance = (errorCode: string | null, description: string | null) => {
      const decodedDesc = description ? decodeURIComponent(description) : ''

      console.group('%c🔧 Authway Developer Guide', 'color: #2563eb; font-weight: bold; font-size: 14px')
      console.log('%cError:', 'font-weight: bold', errorCode)
      console.log('%cDescription:', 'font-weight: bold', decodedDesc)

      // Specific guidance based on error type
      if (decodedDesc.includes('post_logout_redirect_uri') && decodedDesc.includes('not a whitelisted')) {
        console.log('%c\n📋 How to fix:', 'color: #059669; font-weight: bold')
        console.log('1. Register post_logout_redirect_uri in Hydra:')
        console.log('%c   curl -X PUT http://localhost:4445/admin/clients/YOUR_CLIENT_ID \\', 'color: #6b7280; font-family: monospace')
        console.log('%c     -H "Content-Type: application/json" \\', 'color: #6b7280; font-family: monospace')
        console.log('%c     -d \'{"post_logout_redirect_uris": ["http://localhost:YOUR_PORT"]}\'', 'color: #6b7280; font-family: monospace')
        console.log('\n2. Or update via Authway Central API:')
        console.log('%c   POST /api/v1/clients/YOUR_CLIENT_ID', 'color: #6b7280; font-family: monospace')
        console.log('\n📚 Docs: https://github.com/authway/authway/docs/SETUP.md')
      } else if (errorCode === 'invalid_client') {
        console.log('%c\n📋 How to fix:', 'color: #059669; font-weight: bold')
        console.log('1. Verify client_id is registered')
        console.log('2. Check client_secret matches')
        console.log('3. Register client: POST /api/v1/clients')
      } else if (errorCode === 'consent_required') {
        console.log('%c\n📋 How to fix:', 'color: #059669; font-weight: bold')
        console.log('1. Set skip_consent: true in client config')
        console.log('2. Or implement consent flow in your app')
      } else if (decodedDesc.includes('redirect_uri')) {
        console.log('%c\n📋 How to fix:', 'color: #059669; font-weight: bold')
        console.log('1. Register redirect_uri in client config')
        console.log('2. Ensure exact match (including trailing slash)')
      }

      console.groupEnd()
    }

    logDeveloperGuidance(error, errorDescription)

    const getErrorInfo = (errorCode: string | null, description: string | null): ErrorInfo => {
      // URL 디코딩
      const decodedDescription = description ? decodeURIComponent(description) : ''

      const errorKey = errorCode as keyof typeof errorCodeMap
      const errorCodeMap = {
        invalid_client: { icon: '🔒', color: 'red' },
        access_denied: { icon: '🚫', color: 'orange' },
        invalid_request: { icon: '❌', color: 'red' },
        unauthorized_client: { icon: '🔐', color: 'red' },
        unsupported_response_type: { icon: '🚫', color: 'orange' },
        invalid_scope: { icon: '⚠️', color: 'orange' },
        server_error: { icon: '💥', color: 'red' },
        temporarily_unavailable: { icon: '⏸️', color: 'yellow' },
        consent_required: { icon: '✋', color: 'blue' },
        login_required: { icon: '🔑', color: 'blue' },
        invalid_token: { icon: '⚠️', color: 'red' },
        insufficient_scope: { icon: '⚠️', color: 'orange' },
        request_not_supported: { icon: '🚫', color: 'orange' },
        request_uri_not_supported: { icon: '🚫', color: 'orange' },
        registration_not_supported: { icon: '🚫', color: 'orange' },
        invalid_request_uri: { icon: '❌', color: 'red' },
        invalid_request_object: { icon: '❌', color: 'red' },
      }

      if (errorCode && errorCodeMap[errorKey]) {
        return {
          title: t(`errors:codes.${errorCode}.title`, { defaultValue: t('errors:codes.unknown.title') }),
          description: decodedDescription || t(`errors:codes.${errorCode}.description`, { defaultValue: t('errors:codes.unknown.description') }),
          icon: errorCodeMap[errorKey].icon,
          color: errorCodeMap[errorKey].color,
        }
      }

      return {
        title: t('errors:codes.unknown.title'),
        description: decodedDescription || t('errors:codes.unknown.description'),
        icon: '⚠️',
        color: 'red',
      }
    }

    setErrorInfo(getErrorInfo(error, errorDescription))
  }, [searchParams, t])

  const getColorClasses = (color: string) => {
    switch (color) {
      case 'red':
        return {
          bg: 'bg-red-50',
          border: 'border-red-200',
          text: 'text-red-800',
          icon: 'text-red-500',
          button: 'bg-red-600 hover:bg-red-700',
        }
      case 'orange':
        return {
          bg: 'bg-orange-50',
          border: 'border-orange-200',
          text: 'text-orange-800',
          icon: 'text-orange-500',
          button: 'bg-orange-600 hover:bg-orange-700',
        }
      case 'yellow':
        return {
          bg: 'bg-yellow-50',
          border: 'border-yellow-200',
          text: 'text-yellow-800',
          icon: 'text-yellow-500',
          button: 'bg-yellow-600 hover:bg-yellow-700',
        }
      case 'blue':
        return {
          bg: 'bg-blue-50',
          border: 'border-blue-200',
          text: 'text-blue-800',
          icon: 'text-blue-500',
          button: 'bg-blue-600 hover:bg-blue-700',
        }
      default:
        return {
          bg: 'bg-gray-50',
          border: 'border-gray-200',
          text: 'text-gray-800',
          icon: 'text-gray-500',
          button: 'bg-gray-600 hover:bg-gray-700',
        }
    }
  }

  const colors = getColorClasses(errorInfo.color)

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100 px-4">
      <div className="max-w-2xl w-full">
        <div className={`${colors.bg} border-2 ${colors.border} rounded-lg p-8 shadow-lg`}>
          {/* Icon */}
          <div className="flex justify-center mb-6">
            <span className="text-6xl" role="img" aria-label="error icon">
              {errorInfo.icon}
            </span>
          </div>

          {/* Title */}
          <h1 className={`text-3xl font-bold ${colors.text} text-center mb-4`}>
            {errorInfo.title}
          </h1>

          {/* Description */}
          <div className="bg-white rounded-md p-4 mb-6">
            <p className="text-gray-700 text-center leading-relaxed">
              {errorInfo.description}
            </p>
          </div>

          {/* Error Code (if available) */}
          {searchParams.get('error') && (
            <div className="bg-gray-100 rounded-md p-3 mb-6">
              <p className="text-xs text-gray-600 font-mono text-center">
                {t('errors:errorCode')} {searchParams.get('error')}
              </p>
            </div>
          )}

          {/* Actions */}
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            <button
              onClick={() => window.history.back()}
              className="px-6 py-3 bg-white border-2 border-gray-300 text-gray-700 rounded-md font-medium hover:bg-gray-50 transition-colors"
            >
              {t('common:goBack')}
            </button>
            <button
              onClick={() => navigate('/')}
              className={`px-6 py-3 ${colors.button} text-white rounded-md font-medium transition-colors`}
            >
              {t('common:goHome')}
            </button>
          </div>

          {/* Support Info */}
          <div className="mt-8 pt-6 border-t border-gray-300">
            <p className="text-sm text-gray-600 text-center">
              {t('common:contactSupport')}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ErrorPage
