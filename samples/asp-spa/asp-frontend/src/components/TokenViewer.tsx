import { useState } from 'react'
import { useAccessToken } from '@authway/react'

export function TokenViewer() {
  const { token, getToken } = useAccessToken()
  const [tokenInfo, setTokenInfo] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  const handleGetToken = async () => {
    setLoading(true)
    try {
      const accessToken = await getToken()

      // Decode JWT payload (for display only, not for verification!)
      const parts = accessToken.split('.')
      if (parts.length === 3) {
        const payload = JSON.parse(atob(parts[1]))
        setTokenInfo(payload)
      }
    } catch (error) {
      console.error('Failed to get token:', error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="token-section">
      <h3>🎫 Access Token</h3>
      <p className="section-description">
        View and decode your JWT access token. Use it to make authenticated API calls.
      </p>

      <div className="token-actions">
        <button
          className="btn btn-primary"
          onClick={handleGetToken}
          disabled={loading}
        >
          {loading ? 'Getting Token...' : '🔑 Get Access Token'}
        </button>
      </div>

      {token && (
        <div className="token-display-card">
          <h4>Access Token (JWT)</h4>
          <div className="token-textarea-wrapper">
            <textarea
              readOnly
              value={token}
              rows={6}
              className="token-textarea"
              onClick={(e) => e.currentTarget.select()}
            />
            <button
              className="copy-button"
              onClick={() => {
                navigator.clipboard.writeText(token)
                alert('Token copied to clipboard!')
              }}
            >
              📋 Copy
            </button>
          </div>
        </div>
      )}

      {tokenInfo && (
        <div className="token-payload-card">
          <h4>🔍 Decoded Token Payload</h4>

          <div className="token-info-grid">
            <div className="info-item">
              <label>Issuer (iss)</label>
              <code className="value-code">{tokenInfo.iss}</code>
            </div>

            <div className="info-item">
              <label>Audience (aud)</label>
              <code className="value-code">{tokenInfo.aud}</code>
            </div>

            <div className="info-item">
              <label>Subject (sub)</label>
              <code className="value-code">{tokenInfo.sub}</code>
            </div>

            <div className="info-item">
              <label>Issued At (iat)</label>
              <span className="value">
                {new Date(tokenInfo.iat * 1000).toLocaleString()}
              </span>
            </div>

            <div className="info-item">
              <label>Expires (exp)</label>
              <span className="value">
                {new Date(tokenInfo.exp * 1000).toLocaleString()}
              </span>
            </div>

            <div className="info-item">
              <label>Time Remaining</label>
              <span className={`badge ${tokenInfo.exp * 1000 > Date.now() ? 'valid' : 'expired'}`}>
                {tokenInfo.exp * 1000 > Date.now()
                  ? `${Math.floor((tokenInfo.exp * 1000 - Date.now()) / 1000 / 60)} minutes`
                  : 'Expired'}
              </span>
            </div>
          </div>

          <details className="details-section">
            <summary>View Full Payload</summary>
            <pre className="claims-json">
              {JSON.stringify(tokenInfo, null, 2)}
            </pre>
          </details>
        </div>
      )}

      <div className="info-box">
        <h4>🔐 About JWT Tokens</h4>
        <p>
          JSON Web Tokens (JWT) are used for secure authentication and authorization.
          Each token contains three parts:
        </p>
        <ul>
          <li><strong>Header</strong>: Algorithm and token type</li>
          <li><strong>Payload</strong>: Claims about the user (shown above)</li>
          <li><strong>Signature</strong>: Cryptographic signature for verification</li>
        </ul>

        <h4 className="use-cases-title">🎯 Using Access Tokens</h4>
        <div className="code-example">
          <pre>{`// Making authenticated API calls
const { getAccessToken } = useAuth()

const token = await getAccessToken()

const response = await fetch('/api/protected', {
  headers: {
    'Authorization': \`Bearer \${token}\`
  }
})

const data = await response.json()`}</pre>
        </div>

        <div className="token-features">
          <h5>⚡ SDK Features</h5>
          <ul>
            <li>✅ Automatic token refresh before expiration</li>
            <li>✅ Secure token storage in memory</li>
            <li>✅ Token caching for performance</li>
            <li>✅ Silent refresh with refresh tokens</li>
          </ul>
        </div>
      </div>
    </div>
  )
}
