# @authway/contract

The OpenAPI 3.1 description (`openapi.yaml`) of the API Authway's login
screens use: login, consent and logout flows, sign-in links and capabilities.

Every Authway implementation answers these endpoints the same way, so the
bundled screens — or a custom one — work with any of them. The conformance
suite (`packages/conformance`) validates each answer a provider gives against
this document; a change to the contract is a change to that suite's
expectations.

For a walk-through of the flows, see `docs/api/login-flows.md`.

## Runtime configuration of the bundled screens

`runtime-config.schema.json` describes `config.js`, the one file a deployment
changes to point the bundled login UI and console at its own addresses. Each
screen loads `config.js` from the place it is served (the login UI from its
root, the console from its base path) before it starts, and reads the object
that file assigns:

```js
window.__AUTHWAY_CONFIG__ = { apiUrl: 'https://auth.example.com' }
```

An implementation that serves the screens itself serves this file, uncached.
Absent keys fall back to what the screen was built with; an empty `apiUrl`
means the API is on the screen's own origin.
