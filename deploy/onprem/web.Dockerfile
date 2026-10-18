# The front of an on-premises installation: Caddy serving the login UI at the
# site root and the console under /console/, and routing the API's and the
# authorization server's paths on the same origin (Caddyfile).
#
# Build from the repository root:
#   docker build -f deploy/onprem/web.Dockerfile -t authway-web .

FROM node:24-alpine AS login-ui
WORKDIR /src
COPY apps/branding/auth-ui/package.json apps/branding/auth-ui/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY apps/branding/auth-ui/ ./
RUN npm run build

FROM node:24-alpine AS console
WORKDIR /src
COPY apps/central/admin/package.json apps/central/admin/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY apps/central/admin/ ./
RUN ADMIN_BASE_PATH=/console/ npm run build

FROM caddy:2.10-alpine
COPY --from=login-ui /src/dist /srv/ui
COPY --from=console /src/dist /srv/console
# Hosting files for other platforms; nothing here reads them.
RUN rm -f /srv/ui/_headers /srv/ui/_redirects /srv/ui/staticwebapp.config.json \
          /srv/console/_headers /srv/console/_redirects /srv/console/staticwebapp.config.json
COPY deploy/onprem/Caddyfile /etc/caddy/Caddyfile
