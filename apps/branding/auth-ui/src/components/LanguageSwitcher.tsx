import React from 'react'
import { useTranslation } from 'react-i18next'

const languages = [
  { code: 'ko', name: '한국어', flag: '🇰🇷' },
  { code: 'en', name: 'English', flag: '🇺🇸' },
]

interface LanguageSwitcherProps {
  className?: string
  variant?: 'dropdown' | 'buttons' | 'minimal'
}

const LanguageSwitcher: React.FC<LanguageSwitcherProps> = ({
  className = '',
  variant = 'minimal'
}) => {
  const { i18n } = useTranslation()

  const currentLanguage = languages.find(lang => lang.code === i18n.language) || languages[0]

  const handleLanguageChange = (langCode: string) => {
    i18n.changeLanguage(langCode)
    localStorage.setItem('authway-language', langCode)
  }

  if (variant === 'buttons') {
    return (
      <div className={`flex gap-2 ${className}`}>
        {languages.map((lang) => (
          <button
            key={lang.code}
            onClick={() => handleLanguageChange(lang.code)}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              i18n.language === lang.code
                ? 'bg-indigo-600 text-white'
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            {lang.flag} {lang.name}
          </button>
        ))}
      </div>
    )
  }

  if (variant === 'dropdown') {
    return (
      <div className={`relative inline-block ${className}`}>
        <select
          value={i18n.language}
          onChange={(e) => handleLanguageChange(e.target.value)}
          className="appearance-none bg-white border border-gray-300 rounded-md px-3 py-2 pr-8 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 cursor-pointer"
        >
          {languages.map((lang) => (
            <option key={lang.code} value={lang.code}>
              {lang.flag} {lang.name}
            </option>
          ))}
        </select>
        <div className="absolute inset-y-0 right-0 flex items-center pr-2 pointer-events-none">
          <svg className="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </div>
    )
  }

  // Minimal variant (default) - just toggles between languages
  const nextLanguage = languages.find(lang => lang.code !== i18n.language) || languages[1]

  return (
    <button
      onClick={() => handleLanguageChange(nextLanguage.code)}
      className={`inline-flex items-center gap-1 px-2 py-1 text-sm text-gray-600 hover:text-gray-900 transition-colors ${className}`}
      title={`Switch to ${nextLanguage.name}`}
    >
      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
      </svg>
      <span>{currentLanguage.flag}</span>
    </button>
  )
}

export default LanguageSwitcher
