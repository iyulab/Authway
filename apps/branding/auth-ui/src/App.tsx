import { Routes, Route, Navigate } from 'react-router'
import HomePage from './pages/HomePage'
import LoginPage from './pages/LoginPage'
import LogoutPage from './pages/LogoutPage'
import ConsentPage from './pages/ConsentPage'
import AcceptInvitationPage from './pages/AcceptInvitationPage'
import VerifyEmailPage from './pages/VerifyEmailPage'
import ResendVerificationPage from './pages/ResendVerificationPage'
import ForgotPasswordPage from './pages/ForgotPasswordPage'
import ResetPasswordPage from './pages/ResetPasswordPage'
import ErrorPage from './pages/ErrorPage'
import PopupCallbackPage from './pages/PopupCallbackPage'
import MFAVerifyPage from './pages/MFAVerifyPage'
import TermsPage from './pages/TermsPage'
import MagicLinkPage from './pages/MagicLinkPage'
import DevicePage from './pages/DevicePage'
import DeviceDonePage from './pages/DeviceDonePage'

function App() {
  return (
    <div className="min-h-screen bg-linear-to-br from-blue-50 to-indigo-100">
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/logout" element={<LogoutPage />} />
        <Route path="/consent" element={<ConsentPage />} />
        {/* Onboarding is invitation-only: public self-registration is removed.
            The invitation email links to /invitation/accept?token=... */}
        <Route path="/invitation/accept" element={<AcceptInvitationPage />} />
        <Route path="/register" element={<Navigate to="/login" replace />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/resend-verification" element={<ResendVerificationPage />} />
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
        <Route path="/reset-password" element={<ResetPasswordPage />} />
        <Route path="/error" element={<ErrorPage />} />
        <Route path="/popup-callback" element={<PopupCallbackPage />} />
        <Route path="/mfa/verify" element={<MFAVerifyPage />} />
        <Route path="/terms" element={<TermsPage />} />
        <Route path="/magic-link" element={<MagicLinkPage />} />
        <Route path="/device" element={<DevicePage />} />
        <Route path="/device/done" element={<DeviceDonePage />} />
        <Route path="*" element={<HomePage />} />
      </Routes>
    </div>
  )
}

export default App