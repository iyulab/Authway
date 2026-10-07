# @authway/contract

The OpenAPI 3.1 description (`openapi.yaml`) of the API Authway's login
screens use: login, consent and logout flows, sign-in links and capabilities.

Every Authway implementation answers these endpoints the same way, so the
bundled screens — or a custom one — work with any of them. The conformance
suite (`packages/conformance`) validates each answer a provider gives against
this document; a change to the contract is a change to that suite's
expectations.

For a walk-through of the flows, see `docs/api/login-flows.md`.
