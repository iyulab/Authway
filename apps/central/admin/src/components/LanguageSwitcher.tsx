import React from 'react'
import { useTranslation } from 'react-i18next'
import { LanguageIcon } from '@heroicons/react/24/outline'
import { SUPPORTED_LANGUAGES, currentLanguage } from '@/i18n'
import { cn } from '@/lib/utils'

interface LanguageSwitcherProps {
  className?: string
}

/** Chooses the console's language; the choice is remembered in this browser. */
const LanguageSwitcher: React.FC<LanguageSwitcherProps> = ({ className }) => {
  const { t, i18n } = useTranslation('common')
  const active = currentLanguage()

  return (
    <div
      role="group"
      aria-label={t('language.label')}
      className={cn('inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-white p-0.5 text-sm', className)}
    >
      <LanguageIcon className="ml-1.5 h-4 w-4 text-gray-400" aria-hidden="true" />
      {SUPPORTED_LANGUAGES.map((language) => (
        <button
          key={language.code}
          type="button"
          lang={language.code}
          aria-pressed={active === language.code}
          onClick={() => i18n.changeLanguage(language.code)}
          className={cn(
            'rounded-md px-2 py-1 transition-colors',
            active === language.code
              ? 'bg-indigo-100 font-medium text-indigo-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
          )}
        >
          {language.name}
        </button>
      ))}
    </div>
  )
}

export default LanguageSwitcher
