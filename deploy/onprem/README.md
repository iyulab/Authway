# Authway on your own host

One host with Docker (Compose v2) runs the whole of Authway from images — the
same images the hosted service runs — with no access to the internet needed.

| Container | Role |
|---|---|
| `web` | The one address users and applications reach (TLS): the login UI at `/`, the console at `/console/`, and the routes to the two below |
| `api` | The Authway API — sign-in flows, administration, token endpoint |
| `hydra` | The OAuth 2.0 / OpenID Connect authorization server (Ory Hydra) |
| `postgres` | Storage. Hydra and the API share one database |
| `hydra-migrate` | Runs once at start: the authorization server's schema |

Only `web` publishes ports (80, 443).

Sign-in state that lasts minutes — a challenge between two sign-in steps, a
state value between a redirect and its callback, rate-limit counters — is
kept in the API's process, which suits the one API container here; a
restart drops only the sign-ins in progress. With `COMPOSE_PROFILES=redis`
and `AUTHWAY_REDIS_ENABLED=true` in `.env` it is kept in a `redis` container
instead. Running more than one API container needs Redis.

## Install

From a bundle made with `deploy/onprem/pack.sh` (or this directory, after
running it with `--no-images` and building the images yourself):

```sh
docker load -i images.tar        # skip where the host can pull the images
sh setup.sh                      # creates .env with every secret generated
vi .env                          # AUTHWAY_DOMAIN, AUTHWAY_TLS, mail
docker compose up -d
```

The first start takes a minute: the API applies its database migrations.
`docker compose ps` shows `api` as healthy when it is ready.

- **`AUTHWAY_DOMAIN`** is the issuer every token names. Choose it once.
- **Certificates** — `AUTHWAY_TLS=internal` has Caddy issue the certificate from
  its own authority. Browsers and applications must trust that authority's
  root: copy it out with
  `docker compose cp web:/data/caddy/pki/authorities/local/root.crt .` and
  distribute it as your organization distributes roots. With a certificate of
  your own, place it in `./certs` and set
  `AUTHWAY_TLS=/certs/site.crt /certs/site.key`.
- **Mail** — invitations, sign-in links, address checks and password resets go
  through the SMTP server in `.env`.

## First steps

- The console is at `https://<AUTHWAY_DOMAIN>/console/`. Sign in with the
  administrator password from `.env` (`AUTHWAY_ADMIN_PASSWORD`); scripts use
  `AUTHWAY_ADMIN_API_KEY` as a bearer token on `/api/v1/…`.
- Create a tenant and register each application as a client — the steps are
  in [docs/api/client-management.md](../../docs/api/client-management.md).
  Applications read everything else from
  `https://<AUTHWAY_DOMAIN>/.well-known/openid-configuration`.

## Upgrade and back up

- **Upgrade**: load (or pull) the new images and `docker compose up -d`.
  Migrations run on start and only move forward — back up first.
- **Back up** the database and `.env` together — stored second factors and
  provider credentials are encrypted with keys in `.env`:
  `docker compose exec postgres pg_dump -U authway authway > authway.sql`
