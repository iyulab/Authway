/**
 * Runtime configuration.
 *
 * A deployment can set values at run time by serving `/config.js` (loaded by
 * index.html before the app) that assigns `window.__AUTHWAY_CONFIG__`. The
 * same build then runs under any domain: an image or static bundle never has
 * to be rebuilt per installation. Values not set there fall back to the
 * build-time `VITE_*` variables, then to defaults.
 *
 * `apiUrl` defaults to "" in production builds — the API is reached on the
 * page's own origin, as in a single-domain deployment — and to the local API
 * in development.
 */
export interface RuntimeConfig {
  /** Base URL of the Authway API; "" means the page's own origin. */
  apiUrl: string
  /** Application Insights connection string; telemetry is off when empty. */
  appInsightsConnectionString: string
}

declare global {
  interface Window {
    __AUTHWAY_CONFIG__?: Partial<RuntimeConfig>
  }
}

const trimSlash = (url: string) => url.replace(/\/+$/, '')

export function getConfig(): RuntimeConfig {
  const runtime = (typeof window !== 'undefined' && window.__AUTHWAY_CONFIG__) || {}
  const env = import.meta.env
  const pick = (key: keyof RuntimeConfig, fromEnv: string | undefined, fallback: string) =>
    runtime[key] ?? fromEnv ?? fallback

  return {
    apiUrl: trimSlash(pick('apiUrl', env.VITE_API_URL, env.DEV ? 'http://localhost:8080' : '')),
    appInsightsConnectionString: pick(
      'appInsightsConnectionString',
      env.VITE_APPLICATIONINSIGHTS_CONNECTION_STRING,
      '',
    ),
  }
}
