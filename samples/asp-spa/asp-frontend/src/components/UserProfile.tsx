import { useAuth } from '@authway/react'

export function UserProfile() {
  const { user } = useAuth()

  if (!user) {
    return (
      <div className="profile-empty">
        <p>사용자 정보를 불러올 수 없습니다.</p>
      </div>
    )
  }

  return (
    <div className="profile-section">
      <h3>👤 User Profile</h3>

      <div className="profile-card">
        {user.picture && (
          <div className="profile-avatar-large">
            <img src={user.picture} alt={user.name || 'User'} />
          </div>
        )}

        <div className="profile-info-grid">
          <div className="info-item">
            <label>Name</label>
            <span className="value">{user.name || 'N/A'}</span>
          </div>

          <div className="info-item">
            <label>Email</label>
            <span className="value">{user.email}</span>
          </div>

          <div className="info-item">
            <label>Email Verified</label>
            <span className={`badge ${user.email_verified ? 'verified' : 'unverified'}`}>
              {user.email_verified ? '✅ Verified' : '❌ Not Verified'}
            </span>
          </div>

          <div className="info-item">
            <label>Subject (User ID)</label>
            <code className="value-code">{user.sub}</code>
          </div>
        </div>
      </div>

      <details className="details-section">
        <summary>🔍 View All Claims</summary>
        <pre className="claims-json">
          {JSON.stringify(user, null, 2)}
        </pre>
      </details>

      <div className="info-box">
        <h4>💡 What are Claims?</h4>
        <p>
          Claims are key-value pairs that contain information about the user.
          They are included in the ID token and can be used to make authorization decisions.
        </p>
        <ul>
          <li><strong>sub</strong>: Unique user identifier (Subject)</li>
          <li><strong>email</strong>: User's email address</li>
          <li><strong>name</strong>: User's display name</li>
          <li><strong>picture</strong>: User's profile picture URL</li>
          <li><strong>email_verified</strong>: Email verification status</li>
        </ul>
      </div>
    </div>
  )
}
