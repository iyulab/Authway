// Authway Configuration - Local Development
export const AUTHWAY_CONFIG = {
  domain: 'http://localhost:8080',  // Authway API URL (discovers the OIDC issuer)
  clientId: 'authway_spa_sample_local',
  redirectUri: window.location.origin,
  scope: 'openid profile email',

  // Backend API
  apiBaseUrl: 'http://localhost:5222',
} as const;
