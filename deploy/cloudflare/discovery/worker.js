// Serves the issuer's two discovery paths from the API, which describes the
// deployment as it is offered (see docs/DEPLOYMENT.md, "Discovery document").
// Routed only at those two exact paths of the issuer's host; every other
// request to the issuer never reaches this Worker.
//
// When the API does not answer with a document for this issuer, the request
// goes on to the issuer's origin — the authorization server's own document,
// which clients read before this Worker existed.

const PATHS = new Set([
  '/.well-known/openid-configuration',
  '/.well-known/oauth-authorization-server',
]);

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!PATHS.has(url.pathname) || request.method !== 'GET') {
      return fetch(request);
    }

    // The API answers CORS from the request's Origin; a page reading
    // discovery from another origin needs it passed on.
    const headers = { Accept: 'application/json', 'User-Agent': 'authway-discovery' };
    const origin = request.headers.get('Origin');
    if (origin) {
      headers.Origin = origin;
    }

    try {
      const response = await fetch(new URL(url.pathname, env.API_ORIGIN), { headers });
      if (response.ok) {
        const body = await response.text();
        // A client rejects a document whose issuer is not the address it
        // read it from (OpenID Connect Discovery 1.0 §4.3).
        if (JSON.parse(body).issuer === url.origin) {
          return new Response(body, response);
        }
      }
    } catch {
      // fall through to the issuer's origin
    }
    return fetch(request);
  },
};
