# The front of an on-premises installation: Caddy serving the login UI at the
# site root and the console under /console/, and routing the API's and the
# authorization server's paths on the same origin (Caddyfile).
#
# Build from the repository root:
#   docker build -f deploy/onprem/web.Dockerfile -t authway-web .

# The two screens as packages/ui builds them (packages/ui/build.mjs): the
# login UI for the site root, the console for /console/, both reaching the
# API on the page's own origin.
FROM node:24-alpine AS screens
WORKDIR /src
# Each app's dependencies first, in their own layer.
COPY apps/branding/auth-ui/package.json apps/branding/auth-ui/package-lock.json apps/branding/auth-ui/
COPY apps/central/admin/package.json apps/central/admin/package-lock.json apps/central/admin/
RUN cd apps/branding/auth-ui && npm ci --no-audit --no-fund
RUN cd apps/central/admin && npm ci --no-audit --no-fund
COPY apps/branding/auth-ui/ apps/branding/auth-ui/
COPY apps/central/admin/ apps/central/admin/
COPY packages/ui/ packages/ui/
# The screens are compiled against the contract's types.
COPY packages/contract/schema.d.ts packages/contract/schema.d.ts
RUN node packages/ui/build.mjs --no-install

FROM caddy:2.10-alpine
COPY --from=screens /src/packages/ui/login /srv/ui
COPY --from=screens /src/packages/ui/console /srv/console
COPY deploy/onprem/Caddyfile /etc/caddy/Caddyfile
