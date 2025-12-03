'use client'

import { useState } from 'react'
import { useAuth } from '@authway/react'
import ProfileTab from './tabs/ProfileTab'
import TokenTab from './tabs/TokenTab'
import ApiTestTab from './tabs/ApiTestTab'

type TabType = 'profile' | 'token' | 'api'

export default function Dashboard() {
  const { user } = useAuth()
  const [activeTab, setActiveTab] = useState<TabType>('profile')

  return (
    <div className="dashboard">
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'profile' ? 'active' : ''}`}
          onClick={() => setActiveTab('profile')}
        >
          👤 Profile
        </button>
        <button
          className={`tab ${activeTab === 'token' ? 'active' : ''}`}
          onClick={() => setActiveTab('token')}
        >
          🎫 Token
        </button>
        <button
          className={`tab ${activeTab === 'api' ? 'active' : ''}`}
          onClick={() => setActiveTab('api')}
        >
          🔌 API Test
        </button>
      </div>

      <div className="tab-content">
        {activeTab === 'profile' && <ProfileTab user={user} />}
        {activeTab === 'token' && <TokenTab />}
        {activeTab === 'api' && <ApiTestTab />}
      </div>
    </div>
  )
}
