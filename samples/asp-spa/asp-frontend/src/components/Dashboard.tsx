import { useState } from 'react'
import { UserProfile } from './UserProfile'
import { DynamicClaims } from './DynamicClaims'
import { TokenViewer } from './TokenViewer'
import { ApiTester } from './ApiTester'

export function Dashboard() {
  const [activeTab, setActiveTab] = useState<'profile' | 'claims' | 'token' | 'api'>('profile')

  return (
    <div className="dashboard">
      <div className="dashboard-header">
        <h2>🎯 Dashboard</h2>
        <p>Explore Authway SDK features and capabilities</p>
      </div>

      <div className="tabs">
        <button
          className={`tab ${activeTab === 'profile' ? 'active' : ''}`}
          onClick={() => setActiveTab('profile')}
        >
          <span className="tab-icon">👤</span>
          <span className="tab-label">Profile</span>
        </button>
        <button
          className={`tab ${activeTab === 'claims' ? 'active' : ''}`}
          onClick={() => setActiveTab('claims')}
        >
          <span className="tab-icon">🎭</span>
          <span className="tab-label">Dynamic Claims</span>
        </button>
        <button
          className={`tab ${activeTab === 'token' ? 'active' : ''}`}
          onClick={() => setActiveTab('token')}
        >
          <span className="tab-icon">🎫</span>
          <span className="tab-label">Token</span>
        </button>
        <button
          className={`tab ${activeTab === 'api' ? 'active' : ''}`}
          onClick={() => setActiveTab('api')}
        >
          <span className="tab-icon">🔌</span>
          <span className="tab-label">API Test</span>
        </button>
      </div>

      <div className="tab-content">
        {activeTab === 'profile' && <UserProfile />}
        {activeTab === 'claims' && <DynamicClaims />}
        {activeTab === 'token' && <TokenViewer />}
        {activeTab === 'api' && <ApiTester />}
      </div>
    </div>
  )
}
