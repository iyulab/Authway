import { useNavigate } from 'react-router-dom'

const HomePage = () => {
  const navigate = useNavigate()

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="max-w-4xl w-full space-y-8 p-8">
        <div className="text-center">
          <h1 className="text-4xl font-bold text-gray-900 mb-4">
            Authway Login UI
          </h1>
          <p className="text-lg text-gray-600 mb-8">
            OAuth 2.0 인증 서버 로그인 인터페이스
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* OAuth Flow */}
          <div className="bg-white rounded-lg shadow-md p-6">
            <h2 className="text-xl font-semibold text-gray-900 mb-4">
              🔐 OAuth 2.0 Flow
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              정상적인 OAuth 인증 흐름을 통한 로그인
            </p>
            <div className="space-y-2 text-sm text-gray-500">
              <p>• login_challenge 파라미터 필요</p>
              <p>• Ory Hydra와 연동</p>
              <p>• 클라이언트 애플리케이션에서 시작</p>
            </div>
            <div className="mt-4 p-3 bg-yellow-50 rounded-md">
              <p className="text-xs text-yellow-800">
                ⚠️ OAuth 클라이언트 애플리케이션을 통해 접근하세요
              </p>
            </div>
          </div>

          {/* Direct Access */}
          <div className="bg-white rounded-lg shadow-md p-6">
            <h2 className="text-xl font-semibold text-gray-900 mb-4">
              ✨ 직접 테스트
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              개발/테스트용 페이지
            </p>
            <div className="space-y-3">
              <button
                onClick={() => navigate('/register')}
                className="w-full py-2 px-4 bg-indigo-600 hover:bg-indigo-700 text-white rounded-md transition-colors"
              >
                회원가입
              </button>
              <button
                onClick={() => navigate('/verify-email')}
                className="w-full py-2 px-4 bg-gray-200 hover:bg-gray-300 text-gray-800 rounded-md transition-colors"
              >
                이메일 인증
              </button>
              <button
                onClick={() => navigate('/forgot-password')}
                className="w-full py-2 px-4 bg-gray-200 hover:bg-gray-300 text-gray-800 rounded-md transition-colors"
              >
                비밀번호 재설정
              </button>
            </div>
          </div>
        </div>

        {/* API Info */}
        <div className="bg-white rounded-lg shadow-md p-6">
          <h2 className="text-xl font-semibold text-gray-900 mb-4">
            🚀 서비스 정보
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
            <div>
              <p className="font-medium text-gray-700">Login UI</p>
              <p className="text-gray-500">http://localhost:3001</p>
            </div>
            <div>
              <p className="font-medium text-gray-700">Backend API</p>
              <p className="text-gray-500">http://localhost:8080</p>
            </div>
            <div>
              <p className="font-medium text-gray-700">MailHog</p>
              <p className="text-gray-500">http://localhost:8025</p>
            </div>
          </div>
        </div>

        {/* Documentation Links */}
        <div className="text-center text-sm text-gray-500">
          <p>
            문서:{' '}
            <a href="/docs/START-HERE.md" className="text-indigo-600 hover:text-indigo-500">
              빠른 시작
            </a>
            {' | '}
            <a href="/docs/DOCKER-GUIDE.md" className="text-indigo-600 hover:text-indigo-500">
              Docker 가이드
            </a>
            {' | '}
            <a href="/docs/TESTING-GUIDE.md" className="text-indigo-600 hover:text-indigo-500">
              테스트 가이드
            </a>
          </p>
        </div>
      </div>
    </div>
  )
}

export default HomePage
