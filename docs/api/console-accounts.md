# Console accounts

The administration API takes three kinds of credential:

| Credential | Administers |
|---|---|
| The deployment's admin key (`AUTHWAY_ADMIN_API_KEY`) | every tenant |
| An admin console session (`POST /admin/login`, the shared admin password) | every tenant |
| A **console account's** access token | what its memberships say |

A tenant's service credential (see [Service clients](service-clients.md)) is a
fourth, for the few routes that take one, and administers its own tenant.

## What a console account is

A console account is an ordinary user of one tenant, the **system tenant**.
It signs in the way any user does — password, second factor, providers,
invitation, suspension all apply — through an application registered in the
system tenant with the audience `urn:authway:console`
(`access_token_strategy: jwt` is recommended). The administration API takes
the access token that application receives:

```
Authorization: Bearer <the console account's access token>
```

A token is taken only when all of these hold; otherwise it is answered as any
token the API does not know (`401`):

- the authorization server says it is active, and it was issued to a user,
  not to a client for itself;
- its audience includes `urn:authway:console` — a token the same account holds
  for another application of the system tenant administers nothing;
- its `tenant_id` is the system tenant.

A token a client holds to act for the user (it names the client in `act`) is
refused with `403 delegated_token`. An account with no membership is refused
with `403 no_console_access`.

## Memberships

| Role | Tenant | May |
|---|---|---|
| `operator` | none | everything the admin key may |
| `owner` | one | everything in the tenant, its settings (`PUT /api/v1/tenants/{id}` — except turning it off or on), its service credentials, its audit-log purge, and who administers it |
| `admin` | one | the tenant's applications, users, invitations, webhooks, delegations, documents, claims and audit records |

One role per account and tenant; giving another replaces it. The system
tenant itself takes no `owner` or `admin`: its users are the console
accounts, and whoever could change them could take an operator's account —
only operators administer it.

```
PUT /api/v1/console/memberships
{ "user_id": "<a user of the system tenant>", "tenant_id": "<tenant>", "role": "admin" }
```

`tenant_id` is left out for the operator role. An operator gives any role; an
owner gives roles in its tenant. `GET /api/v1/console/memberships` lists the
memberships of the tenants the caller administers (`?tenant_id=` narrows it);
`DELETE /api/v1/console/memberships/{id}` removes one. `GET
/api/v1/console/me` answers who the caller is and its memberships — what a
console offers to pick from.

## Which tenant a call is about

The server decides, not the caller:

- A tenant outside what the caller administers is answered as one that does
  not exist — `404 not_found` — wherever the request names it: the
  `X-Tenant-ID` header, the `tenant_id` query parameter, a path, or a request
  body.
- A caller that administers one tenant and names none acts on that tenant.
  A caller that administers several names one on a call about a tenant (a
  list, a creation), or the call is refused with `400 tenant_required`; a
  call about one resource (`/api/v1/clients/{id}`, …) needs no tenant named —
  the resource's own tenant must be one the caller administers. Only a caller
  that administers every tenant may name none on a list call and see every
  tenant's rows.
- Creating or deleting a tenant, turning one off or on, choosing the system
  tenant and synchronising every client with the authorization server are an
  operator's: anyone else gets `403 insufficient_scope`. So are an owner's
  operations for an `admin`.
- `GET /api/v1/tenants` lists the tenants the caller administers.

## Setting it up

With the admin key:

1. Create the tenant for console accounts (`POST /api/v1/tenants`) and choose
   it: `PUT /api/v1/console/system-tenant` with `{ "tenant_id": "…" }`. It can
   be replaced only while no account of it holds a membership
   (`409 system_tenant_in_use`), and it cannot be turned off or deleted
   (`409 system_tenant`).
2. Give the deployment the console's address, `AUTHWAY_APP_CONSOLE_URL`
   (for example `https://admin.example.com`, or `https://auth.example.com/console`
   where the console is served under a path). The console's application is
   then Authway's own: the first time the console asks
   (`GET /api/v1/console/entry`), Authway registers a public client in the
   system tenant — sign-in returns to `<console>/callback`, no consent screen,
   access tokens for the audience `urn:authway:console`, no refresh token —
   and keeps its addresses at the configured one. Like the account screen's
   client it cannot be changed or deleted through the administration API
   (`409 system_client`). Without the address, `GET /api/v1/console/entry`
   answers `404 console_sign_in_unavailable` and the console signs in with the
   admin password only.
3. Invite the people who administer (`POST /api/v1/invitations` with the
   system tenant in `X-Tenant-ID`), and once they have accounts give each a
   membership.

Changes to memberships and the system tenant are recorded in the audit log
(`console.membership_set`, `console.membership_removed`,
`console.system_tenant_set`), with the console account as the actor of what
it does.
