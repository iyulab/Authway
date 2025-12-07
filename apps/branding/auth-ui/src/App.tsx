import { Routes, Route } from 'react-router-dom'
import HomePage from './pages/HomePage'
import LoginPage from './pages/LoginPage'
import LogoutPage from './pages/LogoutPage'
import ConsentPage from './pages/ConsentPage'
import RegisterPage from './pages/RegisterPage'
import VerifyEmailPage from './pages/VerifyEmailPage'
import ResendVerificationPage from './pages/ResendVerificationPage'
import ForgotPasswordPage from './pages/ForgotPasswordPage'
import ResetPasswordPage from './pages/ResetPasswordPage'
import ErrorPage from './pages/ErrorPage'
import PopupCallbackPage from './pages/PopupCallbackPage'
import MFASetupPage from './pages/MFASetupPage'
import MFAVerifyPage from './pages/MFAVerifyPage'
import MagicLinkPage from './pages/MagicLinkPage'

function App() {
  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100">
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/logout" element={<LogoutPage />} />
        <Route path="/consent" element={<ConsentPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/resend-verification" element={<ResendVerificationPage />} />
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
        <Route path="/reset-password" element={<ResetPasswordPage />} />
        <Route path="/error" element={<ErrorPage />} />
        <Route path="/popup-callback" element={<PopupCallbackPage />} />
        <Route path="/mfa/setup" element={<MFASetupPage />} />
        <Route path="/mfa/verify" element={<MFAVerifyPage />} />
        <Route path="/magic-link" element={<MagicLinkPage />} />
        <Route path="*" element={<HomePage />} />
      </Routes>
    </div>
  )
}

export default App