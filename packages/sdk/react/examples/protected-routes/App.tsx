/**
 * Protected routes example with React Router
 *
 * This example demonstrates:
 * - Using withAuth HOC for route protection
 * - Custom protected route component
 * - Integration with React Router
 * - Multiple protected pages
 */

import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Link, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth, withAuth } from '@authway/react';

// Protected Route Component
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return <div style={{ padding: '20px' }}>Loading...</div>;
  }

  if (!isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

// Public Home Page
function HomePage() {
  const { isAuthenticated, login } = useAuth();

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <div style={{ padding: '40px', textAlign: 'center' }}>
      <h1>Welcome to Authway Protected Routes Example</h1>
      <p>This example demonstrates protected routes with @authway/react</p>
      <button
        onClick={login}
        style={{
          padding: '12px 24px',
          fontSize: '16px',
          backgroundColor: '#4F46E5',
          color: 'white',
          border: 'none',
          borderRadius: '8px',
          cursor: 'pointer',
          marginTop: '20px',
        }}
      >
        Login to Continue
      </button>
    </div>
  );
}

// Dashboard Page (Protected)
function DashboardPage() {
  const { user } = useAuth();

  return (
    <div style={{ padding: '20px' }}>
      <h1>Dashboard</h1>
      <p>Welcome back, {user?.name}!</p>
      <p>This is a protected page that requires authentication.</p>
    </div>
  );
}

// Profile Page (Protected)
function ProfilePage() {
  const { user } = useAuth();

  return (
    <div style={{ padding: '20px' }}>
      <h1>Profile</h1>
      <div style={{ backgroundColor: '#F3F4F6', padding: '20px', borderRadius: '8px' }}>
        <h2>User Information</h2>
        <p><strong>ID:</strong> {user?.sub}</p>
        <p><strong>Name:</strong> {user?.name}</p>
        <p><strong>Email:</strong> {user?.email}</p>
        <p><strong>Email Verified:</strong> {user?.email_verified ? 'Yes' : 'No'}</p>
      </div>
    </div>
  );
}

// Settings Page (Protected with HOC)
const SettingsPage = withAuth(
  function Settings() {
    return (
      <div style={{ padding: '20px' }}>
        <h1>Settings</h1>
        <p>This page is protected using the withAuth HOC.</p>
      </div>
    );
  },
  {
    redirectToLogin: true,
  }
);

// Navigation Component
function Navigation() {
  const { isAuthenticated, user, logout } = useAuth();

  if (!isAuthenticated) {
    return null;
  }

  return (
    <nav
      style={{
        backgroundColor: '#1F2937',
        padding: '16px',
        color: 'white',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}
    >
      <div style={{ display: 'flex', gap: '20px' }}>
        <Link to="/dashboard" style={{ color: 'white', textDecoration: 'none' }}>
          Dashboard
        </Link>
        <Link to="/profile" style={{ color: 'white', textDecoration: 'none' }}>
          Profile
        </Link>
        <Link to="/settings" style={{ color: 'white', textDecoration: 'none' }}>
          Settings
        </Link>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        <span>{user?.name}</span>
        <button
          onClick={logout}
          style={{
            padding: '8px 16px',
            backgroundColor: '#EF4444',
            color: 'white',
            border: 'none',
            borderRadius: '6px',
            cursor: 'pointer',
          }}
        >
          Logout
        </button>
      </div>
    </nav>
  );
}

// Main App Component
function App() {
  return (
    <BrowserRouter>
      <Navigation />
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route
          path="/dashboard"
          element={
            <ProtectedRoute>
              <DashboardPage />
            </ProtectedRoute>
          }
        />
        <Route
          path="/profile"
          element={
            <ProtectedRoute>
              <ProfilePage />
            </ProtectedRoute>
          }
        />
        <Route path="/settings" element={<SettingsPage />} />
      </Routes>
    </BrowserRouter>
  );
}

// Root component with AuthProvider
function Root() {
  return (
    <AuthProvider
      config={{
        authwayUrl: import.meta.env.VITE_AUTHWAY_URL || 'http://localhost:4444',
        clientId: import.meta.env.VITE_CLIENT_ID || 'your-client-id',
        redirectUri: import.meta.env.VITE_REDIRECT_URI || 'http://localhost:3000',
        scope: ['openid', 'profile', 'email'],
      }}
    >
      <App />
    </AuthProvider>
  );
}

// Render app
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <Root />
  </React.StrictMode>
);

export default Root;
