/**
 * A minimal user agent: a cookie jar plus manual redirect handling.
 *
 * The authorization-code flow depends on cookies the authorization server sets
 * on the authorize request and checks again when the login and consent steps
 * redirect back to it, so every request in one login must share a jar.
 */
export class Browser {
  private readonly jar = new Map<string, Map<string, string>>()

  async fetch(url: string, init: RequestInit = {}): Promise<Response> {
    const target = new URL(url)
    const headers = new Headers(init.headers)
    const cookie = this.cookieHeader(target)
    if (cookie) headers.set('cookie', cookie)
    const res = await fetch(target, { ...init, headers, redirect: 'manual' })
    this.store(target, res)
    return res
  }

  /** Issues a GET and returns the Location of the redirect it must produce. */
  async redirectFrom(url: string): Promise<URL> {
    const res = await this.fetch(url)
    const location = res.headers.get('location')
    if (res.status < 300 || res.status >= 400 || !location) {
      const body = await res.text()
      throw new Error(`Expected a redirect from ${url}, got ${res.status}: ${body.slice(0, 300)}`)
    }
    return new URL(location, url)
  }

  private cookieHeader(url: URL): string {
    const cookies = this.jar.get(url.host)
    if (!cookies || cookies.size === 0) return ''
    return [...cookies].map(([k, v]) => `${k}=${v}`).join('; ')
  }

  private store(url: URL, res: Response): void {
    const setCookies = res.headers.getSetCookie()
    if (setCookies.length === 0) return
    let cookies = this.jar.get(url.host)
    if (!cookies) {
      cookies = new Map()
      this.jar.set(url.host, cookies)
    }
    for (const line of setCookies) {
      const [pair] = line.split(';')
      const eq = pair.indexOf('=')
      if (eq <= 0) continue
      cookies.set(pair.slice(0, eq).trim(), pair.slice(eq + 1).trim())
    }
  }
}
