import React from 'react'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

/**
 * Where a device sign-in ends in the browser: the device receives its tokens
 * on its own, so the user only needs to know to go back to it.
 */
const DeviceDonePage: React.FC = () => {
  const { t } = useTranslation(['auth'])
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>
      <div className="max-w-md w-full text-center">
        <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-green-100">
          <svg className="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:device.doneTitle')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:device.doneMessage')}</p>
      </div>
    </div>
  )
}

export default DeviceDonePage
