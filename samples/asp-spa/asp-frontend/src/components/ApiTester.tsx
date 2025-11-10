import { useState } from 'react'
import { useAuth } from '@authway/react'
import { API_BASE_URL } from '../config'

export function ApiTester() {
  const { getAccessToken } = useAuth()
  const [apiResponse, setApiResponse] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedEndpoint, setSelectedEndpoint] = useState('/api/protected')

  const endpoints = [
    { path: '/health', method: 'GET', description: 'Health check endpoint (public)' },
    { path: '/api/public', method: 'GET', description: 'Public API endpoint (no auth required)' },
    { path: '/api/protected', method: 'GET', description: 'Protected API endpoint (auth required)' },
    { path: '/api/me', method: 'GET', description: 'Get user profile (auth required)' },
    { path: '/api/weather', method: 'GET', description: 'Get weather forecast (auth required)' },
  ]

  const testApi = async (endpoint: string, method: string = 'GET') => {
    setLoading(true)
    setError(null)
    setApiResponse(null)

    try {
      const token = await getAccessToken()

      const response = await fetch(`${API_BASE_URL}${endpoint}`, {
        method,
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        }
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }

      const contentType = response.headers.get('content-type')
      const data = contentType?.includes('application/json')
        ? await response.json()
        : await response.text()

      setApiResponse({
        status: response.status,
        statusText: response.statusText,
        data
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="api-tester-section">
      <h3>🔌 API Tester</h3>
      <p className="section-description">
        Test protected API endpoints with automatic Bearer token authentication.
      </p>

      <div className="api-endpoints-card">
        <h4>Available Endpoints</h4>

        <div className="endpoints-list">
          {endpoints.map((endpoint) => (
            <button
              key={endpoint.path}
              className={`endpoint-item ${selectedEndpoint === endpoint.path ? 'active' : ''}`}
              onClick={() => {
                setSelectedEndpoint(endpoint.path)
                setApiResponse(null)
                setError(null)
              }}
            >
              <div className="endpoint-header">
                <span className={`method-badge ${endpoint.method.toLowerCase()}`}>
                  {endpoint.method}
                </span>
                <code className="endpoint-path">{endpoint.path}</code>
              </div>
              <div className="endpoint-description">{endpoint.description}</div>
            </button>
          ))}
        </div>

        <div className="api-actions">
          <button
            className="btn btn-primary btn-large"
            onClick={() => {
              const endpoint = endpoints.find(e => e.path === selectedEndpoint)
              if (endpoint) {
                testApi(endpoint.path, endpoint.method)
              }
            }}
            disabled={loading}
          >
            {loading ? '⏳ Sending Request...' : '🚀 Send Request'}
          </button>
        </div>
      </div>

      {loading && (
        <div className="loading-section">
          <div className="spinner"></div>
          <p>Making API request...</p>
        </div>
      )}

      {error && (
        <div className="error-box">
          <h4>❌ Request Failed</h4>
          <p>{error}</p>
        </div>
      )}

      {apiResponse && (
        <div className="api-response-card">
          <div className="response-header">
            <h4>✅ Response</h4>
            <div className="response-status">
              <span className={`status-badge ${apiResponse.status < 400 ? 'success' : 'error'}`}>
                {apiResponse.status} {apiResponse.statusText}
              </span>
            </div>
          </div>

          <details className="details-section" open>
            <summary>Response Data</summary>
            <pre className="response-json">
              {typeof apiResponse.data === 'string'
                ? apiResponse.data
                : JSON.stringify(apiResponse.data, null, 2)}
            </pre>
          </details>
        </div>
      )}

      <div className="info-box">
        <h4>🔐 How API Authentication Works</h4>
        <ol>
          <li>
            <strong>Get Token</strong>: SDK automatically retrieves a valid access token
          </li>
          <li>
            <strong>Add Header</strong>: Token is added to the <code>Authorization</code> header
          </li>
          <li>
            <strong>Make Request</strong>: API validates the token and processes the request
          </li>
          <li>
            <strong>Auto Refresh</strong>: SDK automatically refreshes expired tokens
          </li>
        </ol>

        <h4 className="use-cases-title">💡 Example Code</h4>
        <div className="code-example">
          <pre>{`import { useAuth } from '@authway/react'

function MyComponent() {
  const { getAccessToken } = useAuth()

  const callApi = async () => {
    // Get token (auto-refreshed if expired)
    const token = await getAccessToken()

    // Make authenticated request
    const response = await fetch('/api/protected', {
      headers: {
        'Authorization': \`Bearer \${token}\`
      }
    })

    const data = await response.json()
    return data
  }

  return (
    <button onClick={callApi}>
      Call Protected API
    </button>
  )
}`}</pre>
        </div>

        <div className="api-features">
          <h5>⚡ SDK Features</h5>
          <ul>
            <li>✅ Automatic token refresh before expiration</li>
            <li>✅ Token caching to minimize API calls</li>
            <li>✅ Error handling for expired/invalid tokens</li>
            <li>✅ TypeScript support with type inference</li>
          </ul>
        </div>
      </div>
    </div>
  )
}
