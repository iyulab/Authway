import React, { useState, useEffect } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/lib/api'
import {
  HomeIcon,
  BuildingOfficeIcon,
  CogIcon,
  KeyIcon,
  BookOpenIcon,
  ArrowRightOnRectangleIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from '@heroicons/react/24/outline'

interface LayoutProps {
  children: React.ReactNode
}

const navigation = [
  { name: '대시보드', href: '/dashboard', icon: HomeIcon },
  { name: '테넌트 관리', href: '/tenants', icon: BuildingOfficeIcon },
  { name: '앱(클라이언트) 관리', href: '/clients', icon: KeyIcon },
  { name: '개발자 가이드', href: '/docs', icon: BookOpenIcon },
  { name: '설정', href: '/settings', icon: CogIcon },
]

const Layout: React.FC<LayoutProps> = ({ children }) => {
  const location = useLocation()
  const navigate = useNavigate()
  const logout = useAuthStore((state) => state.logout)

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
    try {
      await authApi.logout()
    } catch (error) {
      console.error('Logout error:', error)
    } finally {
      logout()
      navigate('/login')
    }
  }

  return (
    <div className="flex h-screen bg-gray-100">
      {/* 사이드바 */}
      <div
        className={`flex flex-col bg-white shadow-lg transition-all duration-300 ease-in-out ${
          isCollapsed ? 'w-16' : 'w-64'
        }`}
      >
        {/* 로고 & 토글 버튼 */}
        <div className="flex items-center justify-between h-16 px-4 bg-indigo-600">
          {isCollapsed ? (
            <button
              onClick={toggleSidebar}
              className="w-full flex items-center justify-center text-white hover:bg-indigo-700 rounded-md transition-colors p-2"
              title="사이드바 펼치기"
            >
              <ChevronRightIcon className="w-6 h-6" />
            </button>
          ) : (
            <>
              <h1 className="text-xl font-bold text-white">
                Authway Admin
              </h1>
              <button
                onClick={toggleSidebar}
                className="text-white hover:bg-indigo-700 rounded-md transition-colors p-1"
                title="사이드바 접기"
              >
                <ChevronLeftIcon className="w-5 h-5" />
              </button>
            </>
          )}
        </div>

        {/* 네비게이션 */}
        <nav className="flex-1 px-4 py-6 space-y-2">
          {navigation.map((item) => {
            const isActive = location.pathname === item.href
            return (
              <Link
                key={item.name}
                to={item.href}
                className={`
                  flex items-center px-4 py-2 text-sm font-medium rounded-md transition-colors
                  ${
                    isActive
                      ? 'bg-indigo-100 text-indigo-700'
                      : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'
                  }
                `}
                title={isCollapsed ? item.name : undefined}
              >
                <item.icon className={`w-5 h-5 flex-shrink-0 ${isCollapsed ? '' : 'mr-3'}`} />
                <span className={`transition-opacity duration-200 whitespace-nowrap ${
                  isCollapsed ? 'opacity-0 w-0 overflow-hidden' : 'opacity-100'
                }`}>
                  {item.name}
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
            title={isCollapsed ? '로그아웃' : undefined}
          >
            <ArrowRightOnRectangleIcon className={`w-5 h-5 flex-shrink-0 ${isCollapsed ? '' : 'mr-3'}`} />
            <span className={`transition-opacity duration-200 whitespace-nowrap ${
              isCollapsed ? 'opacity-0 w-0 overflow-hidden' : 'opacity-100'
            }`}>
              로그아웃
            </span>
          </button>
        </div>
      </div>

      {/* 메인 콘텐츠 */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* 헤더 - Hide for /docs page */}
        {location.pathname !== '/docs' && (
          <header className="bg-white shadow-sm border-b border-gray-200">
            <div className="px-6 py-4">
              <h2 className="text-2xl font-semibold text-gray-900">
                {navigation.find(item => item.href === location.pathname)?.name || '대시보드'}
              </h2>
            </div>
          </header>
        )}

        {/* 페이지 콘텐츠 */}
        <main className="flex-1 overflow-x-hidden overflow-y-auto bg-gray-50">
          {location.pathname === '/docs' ? (
            // DocsPage gets full space without padding
            children
          ) : (
            // Other pages get padding
            <div className="px-6 py-8">
              {children}
            </div>
          )}
        </main>
      </div>
    </div>
  )
}

export default Layout