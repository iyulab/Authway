'use client'

import Image from 'next/image'
import { User } from '@authway/react'

interface ProfileTabProps {
  user: User | null
}

export default function ProfileTab({ user }: ProfileTabProps) {
  if (!user) {
    return <div>No user data available</div>
  }

  return (
    <div className="card">
      <h2>User Profile</h2>

      {user.picture && (
        <Image
          src={user.picture}
          alt={user.name || 'User'}
          width={80}
          height={80}
          className="profile-avatar"
        />
      )}

      <div className="info-grid">
        <div className="info-item">
          <label>Name</label>
          <span>{user.name || 'N/A'}</span>
        </div>
        <div className="info-item">
          <label>Email</label>
          <span>{user.email}</span>
        </div>
        <div className="info-item">
          <label>Email Verified</label>
          <span className={user.email_verified ? 'verified' : 'unverified'}>
            {user.email_verified ? '✅ Verified' : '❌ Not Verified'}
          </span>
        </div>
        <div className="info-item">
          <label>User ID</label>
          <code>{user.sub}</code>
        </div>
      </div>

      <details style={{ marginTop: '1.5rem' }}>
        <summary>View All Claims</summary>
        <pre>{JSON.stringify(user, null, 2)}</pre>
      </details>
    </div>
  )
}
