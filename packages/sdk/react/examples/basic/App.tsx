/**
 * Basic example of using @authway/react
 *
 * This example demonstrates:
 * - Setting up AuthProvider
 * - Using useAuth hook
 * - Login/logout functionality
 * - Displaying user information
 */

import React from 'react';
import ReactDOM from 'react-dom/client';
import { AuthProvider, useAuth } from '@authway/react';

// Main App component
function App() {
  const { isAuthenticated, user, login, logout, isLoading, error } = useAuth();

  if (isLoading) {
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <h2>Loading...</h2>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ padding: '20px', textAlign: 'center', color: 'red' }}>
        <h2>Error</h2>
        <p>{error}</p>
        <button onClick={login}>Try Again</button>
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div style={{ padding: '40px', textAlign: 'center', maxWidth: '600px', margin: '0 auto' }}>
        <h1>Welcome to Authway Example</h1>
        <p>Click the button below to login with Authway</p>
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
          Login with Authway
        </button>
      </div>
    );
  }

  return (
    <div style={{ padding: '40px', maxWidth: '800px', margin: '0 auto' }}>
      <h1>Welcome, {user?.name || 'User'}!</h1>

      <div
        style={{
          backgroundColor: '#F3F4F6',
          padding: '20px',
          borderRadius: '8px',
          marginTop: '20px',
        }}
      >
        <h2>User Information</h2>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <tbody>
            <tr style={{ borderBottom: '1px solid #E5E7EB' }}>
              <td style={{ padding: '8px', fontWeight: 'bold' }}>User ID</td>
              <td style={{ padding: '8px' }}>{user?.sub}</td>
            </tr>
            <tr style={{ borderBottom: '1px solid #E5E7EB' }}>
              <td style={{ padding: '8px', fontWeight: 'bold' }}>Name</td>
              <td style={{ padding: '8px' }}>{user?.name || 'N/A'}</td>
            </tr>
            <tr style={{ borderBottom: '1px solid #E5E7EB' }}>
              <td style={{ padding: '8px', fontWeight: 'bold' }}>Email</td>
              <td style={{ padding: '8px' }}>{user?.email || 'N/A'}</td>
            </tr>
            <tr style={{ borderBottom: '1px solid #E5E7EB' }}>
              <td style={{ padding: '8px', fontWeight: 'bold' }}>Email Verified</td>
              <td style={{ padding: '8px' }}>
                {user?.email_verified ? '✅ Yes' : '❌ No'}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <button
        onClick={logout}
        style={{
          padding: '12px 24px',
          fontSize: '16px',
          backgroundColor: '#EF4444',
          color: 'white',
          border: 'none',
          borderRadius: '8px',
          cursor: 'pointer',
          marginTop: '20px',
        }}
      >
        Logout
      </button>
    </div>
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
        autoRefresh: true,
        refreshInterval: 60000, // Refresh every minute
      }}
      onRedirectCallback={() => {
        console.log('Login successful, redirected back to app');
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
