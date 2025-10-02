import { Routes, Route } from 'react-router-dom'
import LoginPage from './pages/LoginPage'
import ConsentPage from './pages/ConsentPage'
import RegisterPage from './pages/RegisterPage'

function App() {
  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100">
      <Routes>
        <Route path="/" element={<LoginPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/consent" element={<ConsentPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="*" element={<LoginPage />} />
      </Routes>
    </div>
  )
}

export default App