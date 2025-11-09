import { useEffect, useState } from 'react';
import { useSearchParams, Link } from 'react-router-dom';
import { CheckCircle2, XCircle, Loader2, Mail } from 'lucide-react';

export default function VerifyEmailPage() {
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading');
  const [message, setMessage] = useState('');
  const token = searchParams.get('token');

  useEffect(() => {
    const verifyEmail = async () => {
      if (!token) {
        setStatus('error');
        setMessage('인증 토큰이 없습니다.');
        return;
      }

      try {
        const response = await fetch(
          `${import.meta.env.VITE_API_URL}/api/email/verify?token=${token}`
        );

        const data = await response.json();

        if (response.ok) {
          setStatus('success');
          setMessage(data.message || '이메일이 성공적으로 인증되었습니다!');
        } else {
          setStatus('error');
          setMessage(data.error || '인증에 실패했습니다.');
        }
      } catch (error) {
        setStatus('error');
        setMessage('서버 연결에 실패했습니다.');
      }
    };

    verifyEmail();
  }, [token]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="w-full max-w-md">
        <div className="bg-white rounded-2xl shadow-xl p-8">
          {/* Header */}
          <div className="text-center mb-6">
            <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-indigo-100 mb-4">
              <Mail className="w-8 h-8 text-indigo-600" />
            </div>
            <h1 className="text-2xl font-bold text-gray-900">이메일 인증</h1>
          </div>

          {/* Status Display */}
          <div className="text-center">
            {status === 'loading' && (
              <div className="py-8">
                <Loader2 className="w-12 h-12 text-indigo-600 animate-spin mx-auto mb-4" />
                <p className="text-gray-600">이메일을 인증하고 있습니다...</p>
              </div>
            )}

            {status === 'success' && (
              <div className="py-8">
                <CheckCircle2 className="w-16 h-16 text-green-500 mx-auto mb-4" />
                <h2 className="text-xl font-semibold text-gray-900 mb-2">인증 완료!</h2>
                <p className="text-gray-600 mb-6">{message}</p>
                <Link
                  to="/login"
                  className="inline-block w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
                >
                  로그인하기
                </Link>
              </div>
            )}

            {status === 'error' && (
              <div className="py-8">
                <XCircle className="w-16 h-16 text-red-500 mx-auto mb-4" />
                <h2 className="text-xl font-semibold text-gray-900 mb-2">인증 실패</h2>
                <p className="text-gray-600 mb-6">{message}</p>
                <div className="space-y-3">
                  <Link
                    to="/resend-verification"
                    className="inline-block w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
                  >
                    인증 이메일 재발송
                  </Link>
                  <Link
                    to="/login"
                    className="inline-block w-full py-3 px-4 border border-gray-300 hover:bg-gray-50 text-gray-700 font-medium rounded-lg transition-colors"
                  >
                    로그인 페이지로
                  </Link>
                </div>
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="mt-6 pt-6 border-t border-gray-200 text-center">
            <p className="text-sm text-gray-500">
              문제가 계속되면{' '}
              <a href="mailto:support@authway.com" className="text-indigo-600 hover:underline">
                고객 지원
              </a>
              에 문의하세요.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
