# @authway/ui

Authway's two screens as static files, built for an installation that serves
everything on one origin:

| folder | served at | screen |
|---|---|---|
| `login/` | `/` | sign-in, consent, logout and account pages |
| `console/` | `/console/` | the administration console |

Both reach the Authway API on the page's own origin. Serve each folder as a
single-page app (an unknown path answers with that folder's `index.html`) and
the API's paths on the same origin. `config.js` in each folder is the
screen's runtime configuration — see `runtime-config.schema.json` in
`@authway/contract`; serve it uncached.

Built by `npm run build` (`build.mjs`) from the two apps in this repository;
the on-premises web image ships the same output.
