// Configuration
export const CONFIG = {
  // Local development configuration
  domain: 'http://localhost:8081',  // Auth Backend (proxies to Central API on 8080)
  issuer: 'http://localhost:4444',  // Hydra OAuth Server (for discovery and token validation)
  clientId: 'authway_spa_sample_local',
  redirectUri: window.location.origin,
  popupRedirectUri: window.location.origin + '/callback.html',
  scope: 'openid profile email offline_access',
  apiBaseUrl: 'http://localhost:5222',  // ASP.NET Backend

  // Authway endpoints (via Auth Backend)
  centralApiUrl: 'http://localhost:8081'  // Auth Backend proxies to Central API
};
