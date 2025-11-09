import { useState } from 'react'
import { useAuth, useClaims } from '@authway/react'

export function DynamicClaims() {
  const { claims, isLoading, error, refreshClaims } = useClaims()
  const { client } = useAuth()

  const [claimKey, setClaimKey] = useState('')
  const [claimValue, setClaimValue] = useState('')
  const [updating, setUpdating] = useState(false)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // Predefined claim examples
  const exampleClaims = [
    { key: 'role', value: 'admin', description: 'User role' },
    { key: 'department', value: 'engineering', description: 'Department name' },
    { key: 'workspace_id', value: 'ws_12345', description: 'Workspace identifier' },
    { key: 'permissions', value: '["read", "write", "delete"]', description: 'JSON array of permissions' },
  ]

  const handleUpdateClaim = async () => {
    if (!claimKey.trim()) {
      setUpdateError('Key is required')
      return
    }

    setUpdating(true)
    setUpdateError(null)
    setSuccessMessage(null)

    try {
      // Parse value as JSON if it looks like JSON
      let parsedValue: any = claimValue.trim()
      if (parsedValue.startsWith('{') || parsedValue.startsWith('[')) {
        try {
          parsedValue = JSON.parse(claimValue)
        } catch {
          // Keep as string if JSON parsing fails
        }
      }

      // Update user claims without re-authentication
      await client.updateUserClaims({ [claimKey]: parsedValue })

      setClaimKey('')
      setClaimValue('')
      setSuccessMessage('✅ Claim updated successfully! Token automatically refreshed.')

      // Refresh claims to show the new value
      await refreshClaims()
    } catch (err: any) {
      setUpdateError(err.message)
    } finally {
      setUpdating(false)
    }
  }

  const handleUseExample = (key: string, value: string) => {
    setClaimKey(key)
    setClaimValue(value)
    setUpdateError(null)
    setSuccessMessage(null)
  }

  if (isLoading) {
    return (
      <div className="loading-section">
        <div className="spinner"></div>
        <p>Loading claims...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="error-section">
        <h3>⚠️ Error Loading Claims</h3>
        <p>{error.message}</p>
      </div>
    )
  }

  return (
    <div className="claims-section">
      <h3>🎭 Dynamic Claims Management</h3>
      <p className="section-description">
        Update user claims in real-time without re-authentication.
        Perfect for workspace switching, role changes, or custom metadata.
      </p>

      {/* Update Claims Form */}
      <div className="claims-update-card">
        <h4>✏️ Update Claims</h4>
        <p className="card-description">
          Add or update custom claims. The token will be automatically refreshed.
        </p>

        <div className="form-group">
          <label htmlFor="claim-key">Claim Key</label>
          <input
            id="claim-key"
            type="text"
            placeholder="e.g., role, department, workspace_id"
            value={claimKey}
            onChange={(e) => setClaimKey(e.target.value)}
            className="form-input"
          />
        </div>

        <div className="form-group">
          <label htmlFor="claim-value">Claim Value</label>
          <input
            id="claim-value"
            type="text"
            placeholder='e.g., admin, engineering, ["read", "write"]'
            value={claimValue}
            onChange={(e) => setClaimValue(e.target.value)}
            className="form-input"
          />
          <small className="form-hint">
            💡 Tip: JSON values are supported (arrays, objects)
          </small>
        </div>

        <button
          className="btn btn-primary"
          onClick={handleUpdateClaim}
          disabled={updating || !claimKey.trim()}
        >
          {updating ? 'Updating...' : '🔄 Update Claim'}
        </button>

        {updateError && (
          <div className="error-box">
            <strong>Error:</strong> {updateError}
          </div>
        )}

        {successMessage && (
          <div className="success-box">
            {successMessage}
          </div>
        )}
      </div>

      {/* Example Claims */}
      <div className="examples-card">
        <h4>💡 Example Claims</h4>
        <p className="card-description">Click to use these example claims</p>

        <div className="examples-grid">
          {exampleClaims.map((example) => (
            <button
              key={example.key}
              className="example-item"
              onClick={() => handleUseExample(example.key, example.value)}
            >
              <div className="example-header">
                <code className="example-key">{example.key}</code>
              </div>
              <div className="example-value">{example.value}</div>
              <div className="example-description">{example.description}</div>
            </button>
          ))}
        </div>
      </div>

      {/* Current Claims Display */}
      <div className="current-claims-card">
        <div className="card-header">
          <h4>🎭 Current Claims</h4>
          <button className="btn btn-sm btn-secondary" onClick={refreshClaims}>
            🔄 Refresh
          </button>
        </div>

        <pre className="claims-json">
          {JSON.stringify(claims, null, 2)}
        </pre>
      </div>

      {/* How It Works */}
      <div className="info-box">
        <h4>⚙️ How Dynamic Claims Work</h4>
        <ol>
          <li>
            <strong>Update Without Re-auth</strong>: Claims are updated server-side without requiring the user to log in again
          </li>
          <li>
            <strong>Automatic Token Refresh</strong>: The SDK automatically refreshes the ID token to include new claims
          </li>
          <li>
            <strong>Instant Availability</strong>: Updated claims are immediately available in your app via <code>useClaims()</code>
          </li>
          <li>
            <strong>Flexible Data</strong>: Store any JSON-serializable data (strings, numbers, arrays, objects)
          </li>
        </ol>

        <h4 className="use-cases-title">🎯 Use Cases</h4>
        <ul>
          <li><strong>Workspace Switching</strong>: Update <code>workspace_id</code> when user switches workspaces</li>
          <li><strong>Role Changes</strong>: Update <code>role</code> or <code>permissions</code> when user role changes</li>
          <li><strong>Feature Flags</strong>: Add <code>features: ["beta", "advanced"]</code> for gradual rollouts</li>
          <li><strong>Custom Metadata</strong>: Store any app-specific user data (preferences, settings, etc.)</li>
        </ul>

        <div className="code-example">
          <h5>SDK Usage Example</h5>
          <pre>{`import { useClaims, useAuth } from '@authway/react'

function MyComponent() {
  const { claims, refreshClaims } = useClaims()
  const { client } = useAuth()

  // Update claim
  await client.updateUserClaims({
    workspace_id: 'ws_new_workspace',
    role: 'admin'
  })

  // Refresh to get updated claims
  await refreshClaims()

  // Use claims in your app
  console.log(claims.workspace_id) // 'ws_new_workspace'
}`}</pre>
        </div>
      </div>
    </div>
  )
}
