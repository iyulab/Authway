import React from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import {
  HomeIcon,
  UsersIcon,
  CogIcon,
  KeyIcon,
  ArrowRightOnRectangleIcon,
} from '@heroicons/react/24/outline'

interface LayoutProps {
  children: React.ReactNode
}

const navigation = [
  { name: '대시보드', href: '/dashboard', icon: HomeIcon },
  { name: '사용자 관리', href: '/users', icon: UsersIcon },
  { name: 'OAuth 클라이언트', href: '/clients', icon: KeyIcon },
  { name: '설정', href: '/settings', icon: CogIcon },
]

const Layout: React.FC<LayoutProps> = ({ children }) => {
  const location = useLocation()
  const navigate = useNavigate()
  const { user, logout } = useAuthStore()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex h-screen bg-gray-100">
      {/* 사이드바 */}
      <div className="flex flex-col w-64 bg-white shadow-lg">
        {/* 로고 */}
        <div className="flex items-center justify-center h-16 px-4 bg-indigo-600">
          <h1 className="text-xl font-bold text-white">Authway Admin</h1>
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
              >
                <item.icon className="w-5 h-5 mr-3" />
                {item.name}
              </Link>
            )
          })}
        </nav>

        {/* 사용자 정보 및 로그아웃 */}
        <div className="px-4 py-4 border-t border-gray-200">
          <div className="flex items-center">
            <div className="flex-1">
              <p className="text-sm font-medium text-gray-900">{user?.first_name} {user?.last_name}</p>
              <p className="text-xs text-gray-500">{user?.email}</p>
            </div>
            <button
              onClick={handleLogout}
              className="p-2 text-gray-400 hover:text-gray-600 transition-colors"
              title="로그아웃"
            >
              <ArrowRightOnRectangleIcon className="w-5 h-5" />
            </button>
          </div>
        </div>
      </div>

      {/* 메인 콘텐츠 */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* 헤더 */}
        <header className="bg-white shadow-sm border-b border-gray-200">
          <div className="px-6 py-4">
            <h2 className="text-2xl font-semibold text-gray-900">
              {navigation.find(item => item.href === location.pathname)?.name || '대시보드'}
            </h2>
          </div>
        </header>

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