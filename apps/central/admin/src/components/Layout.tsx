import React, { useState, useEffect } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useTenantStore } from '@/stores/tenant'
import { signOut } from '@/lib/signOut'
import SupportAccessBand from '@/components/SupportAccessBand'
import LanguageSwitcher from '@/components/LanguageSwitcher'
import {
  HomeIcon,
  CogIcon,
  KeyIcon,
  UsersIcon,
  ArrowRightOnRectangleIcon,
  ArrowPathIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  BellAlertIcon,
  ClipboardDocumentListIcon,
  EnvelopeIcon,
} from '@heroicons/react/24/outline'

interface LayoutProps {
  children: React.ReactNode
}

// `label` is a key of the `common` namespace's `nav` group.
const navigation = [
  { label: 'dashboard', href: '/dashboard', icon: HomeIcon },
  { label: 'clients', href: '/clients', icon: KeyIcon },
  { label: 'users', href: '/users', icon: UsersIcon },
  { label: 'invitations', href: '/invitations', icon: EnvelopeIcon },
  { label: 'webhooks', href: '/webhooks', icon: BellAlertIcon },
  { label: 'auditLogs', href: '/audit-logs', icon: ClipboardDocumentListIcon },
  { label: 'settings', href: '/settings', icon: CogIcon },
] as const

const GITHUB_URL = 'https://github.com/iyulab/Authway'

const Layout: React.FC<LayoutProps> = ({ children }) => {
  const { t } = useTranslation('common')
  const location = useLocation()
  const navigate = useNavigate()
  const { selectedTenant, clearSelectedTenant } = useTenantStore()

  // Sidebar collapse state with localStorage persistence
  const [isCollapsed, setIsCollapsed] = useState(() => {
    const saved = localStorage.getItem('sidebar-collapsed')
    return saved ? JSON.parse(saved) : false
  })

  useEffect(() => {
    localStorage.setItem('sidebar-collapsed', JSON.stringify(isCollapsed))
  }, [isCollapsed])

  const toggleSidebar = () => {
    setIsCollapsed(!isCollapsed)
  }

  const handleLogout = async () => {
    if (!(await signOut())) navigate('/login')
  }

  return (
    <div className="flex h-screen bg-gray-100">
      {/* 사이드바 */}
      <div
        className={`flex flex-col bg-white shadow-lg transition-all duration-300 ease-in-out ${
          isCollapsed ? 'w-16' : 'w-64'
        }`}
      >
        {/* 로고 & 테넌트 정보 */}
        <div className="bg-indigo-600">
          {/* 로고 & 토글 */}
          <div className="flex items-center justify-between h-14 px-4">
            {isCollapsed ? (
              <button
                onClick={toggleSidebar}
                className="w-full flex items-center justify-center text-white hover:bg-indigo-700 rounded-md transition-colors p-2"
                title={t('layout.expandSidebar')}
              >
                <ChevronRightIcon className="w-6 h-6" />
              </button>
            ) : (
              <>
                <h1 className="text-lg font-bold text-white">
                  Authway
                </h1>
                <button
                  onClick={toggleSidebar}
                  className="text-white hover:bg-indigo-700 rounded-md transition-colors p-1"
                  title={t('layout.collapseSidebar')}
                >
                  <ChevronLeftIcon className="w-5 h-5" />
                </button>
              </>
            )}
          </div>
          {/* 현재 테넌트 */}
          {!isCollapsed && selectedTenant && (
            <div className="px-4 pb-3">
              <div className="bg-indigo-500/50 rounded-lg p-3">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-white rounded-lg flex items-center justify-center shrink-0">
                    <span className="text-lg font-bold text-indigo-600">
                      {selectedTenant.name.charAt(0).toUpperCase()}
                    </span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold text-white truncate">
                      {selectedTenant.name}
                    </p>
                    <p className="text-xs text-indigo-200 truncate">
                      {selectedTenant.slug}
                    </p>
                  </div>
                  <button
                    onClick={clearSelectedTenant}
                    className="p-1.5 text-indigo-200 hover:text-white hover:bg-indigo-400/50 rounded-md transition-colors"
                    title={t('layout.switchTenant')}
                  >
                    <ArrowPathIcon className="w-4 h-4" />
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* 네비게이션 */}
        <nav className="flex-1 px-4 py-6 space-y-2">
          {navigation.map((item) => {
            const isActive = location.pathname === item.href
            return (
              <Link
                key={item.label}
                to={item.href}
                className={`
                  flex items-center px-4 py-2 text-sm font-medium rounded-md transition-colors
                  ${
                    isActive
                      ? 'bg-indigo-100 text-indigo-700'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
                  }
                `}
                title={isCollapsed ? t(`nav.${item.label}`) : undefined}
              >
                <item.icon className={`w-5 h-5 shrink-0 ${isCollapsed ? '' : 'mr-3'}`} />
                <span className={`transition-opacity duration-200 whitespace-nowrap ${
                  isCollapsed ? 'opacity-0 w-0 overflow-hidden' : 'opacity-100'
                }`}>
                  {t(`nav.${item.label}`)}
                </span>
              </Link>
            )
          })}
        </nav>

        {/* 로그아웃 */}
        <div className="px-4 py-4 border-t border-gray-200">
          <button
            onClick={handleLogout}
            className="flex items-center w-full px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50 hover:text-gray-900 rounded-md transition-colors"
            title={isCollapsed ? t('layout.signOut') : undefined}
          >
            <ArrowRightOnRectangleIcon className={`w-5 h-5 shrink-0 ${isCollapsed ? '' : 'mr-3'}`} />
            <span className={`transition-opacity duration-200 whitespace-nowrap ${
              isCollapsed ? 'opacity-0 w-0 overflow-hidden' : 'opacity-100'
            }`}>
              {t('layout.signOut')}
            </span>
          </button>
        </div>
      </div>

      {/* 메인 콘텐츠 */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* 헤더 */}
        <header className="bg-white shadow-xs border-b border-gray-200">
          <div className="px-6 py-4 flex items-center justify-between">
            <div className="flex items-center gap-4">
              <h2 className="text-2xl font-semibold text-gray-900">
                {t(`nav.${navigation.find(item => item.href === location.pathname)?.label ?? 'dashboard'}`)}
              </h2>
            </div>
            <div className="flex items-center gap-3">
              <LanguageSwitcher />
              {/* GitHub Link */}
              <a
                href={GITHUB_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-2 px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors"
                title="GitHub"
              >
                <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path fillRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" clipRule="evenodd" />
                </svg>
                <span>GitHub</span>
              </a>
            </div>
          </div>
        </header>
        <SupportAccessBand />

        {/* 페이지 콘텐츠 */}
        <main className="flex-1 overflow-x-hidden overflow-y-auto bg-gray-50">
          <div className="px-6 py-8">
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}

export default Layout