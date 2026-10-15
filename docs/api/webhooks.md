# Webhooks

A **webhook** sends a tenant's events — a user signing up, a client being
changed — as signed HTTP `POST` requests to an endpoint you run. The admin API
registers and manages them; [`packages/contract/openapi.yaml`](../../packages/contract/openapi.yaml)
describes every operation.

---

## 1. Registering a webhook

```http
POST /api/v1/webhooks
Authorization: Bearer <AUTHWAY_ADMIN_API_KEY or admin session token>
X-Tenant-ID: <tenant id>
Content-Type: application/json

{
  "name": "user sync",
  "url": "https://app.example.com/hooks/authway",
  "events": ["user.created", "user.deleted"],
  "retry_count": 3,
  "timeout_secs": 10
}
```

| Field | Default | Range |
|-------|---------|-------|
| `events` | — | at least one of `GET /api/v1/webhooks/events`; `*` is every event |
| `enabled` | `true` | |
| `retry_count` | `3` | `0`–`10` — attempts after the first |
| `timeout_secs` | `30` | `1`–`60` — how long each attempt waits for an answer |

A value outside its range is refused with `400 invalid_request`; nothing is
silently replaced.

Deliveries go to public addresses only: a URL naming `localhost` or a
loopback, private, link-local, shared or reserved address is refused, and a hostname
that resolves to one fails when the delivery connects. Redirects are not
followed — a `3xx` answer is a failed attempt. A deployment whose receivers
run on its own network — or a developer testing against a local receiver —
sets `AUTHWAY_ALLOW_PRIVATE_TARGETS=true`. That opens this host and private
networks; link-local addresses (cloud metadata endpoints among them),
multicast and reserved ranges stay refused. The same setting governs the
addresses clients register for
[back-channel logout](client-management.md#applications-that-keep-their-own-session-backchannel_logout_uri).

The `201` answer carries the webhook **and its signing secret**:

```json
{
  "webhook": { "id": "…", "name": "user sync", "enabled": true, "…": "…" },
  "secret": "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw…"
}
```

The secret is in the [Standard Webhooks](https://www.standardwebhooks.com)
form — `whsec_` followed by the base64 of the key bytes — so a Standard
Webhooks library takes it as it is.

This is the only time the secret is shown. Store it where your receiver can
read it. If it is lost or exposed, issue a new one:

```http
POST /api/v1/webhooks/{id}/rotate-secret
```

which answers `{ "secret": "…" }`. The previous secret stops signing
immediately.

---

## 2. What a delivery looks like

```http
POST /hooks/authway
Content-Type: application/json
webhook-id: b2c6…
webhook-timestamp: 1791518472
webhook-signature: v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=

{
  "id": "b2c6…",
  "type": "user.created",
  "timestamp": "2026-10-09T04:01:12Z",
  "tenant_id": "…",
  "data": { "…": "…" }
}
```

`id` is unique per event, and `webhook-id` carries the same value. Retries of
the same event carry the same id, so use it to ignore a delivery you have
already processed. The event's type is the body's `type`.

### Events

`GET /api/v1/webhooks/events` lists what a webhook can subscribe to. Every
event on the list is sent; an event recorded as failed in the audit log (a
refused sign-in, for one) sends nothing.

| Event | Sent when |
|-------|-----------|
| `user.created` | An account is created — an invitation accepted, or a first sign-in by link or social provider |
| `user.updated` | An administrator changes an account |
| `user.deleted` | An account is deleted |
| `user.login` | A user signs in |
| `user.logout` | A user signs out |
| `user.password_changed` | A user sets a new password |
| `user.mfa_enabled`, `user.mfa_disabled` | A user turns a second factor on or off |
| `client.created`, `client.updated`, `client.deleted` | An OAuth client is registered, changed or deleted |
| `test` | `POST /api/v1/webhooks/{id}/test` |

`data` names what the event is about and who caused it:

```json
{
  "resource": { "type": "user", "id": "0b6f…" },
  "actor": { "type": "admin_session", "id": "5c1e…" }
}
```

`resource.id` is the id the admin API uses (`GET /api/v1/users/{id}`,
`GET /api/v1/clients/{id}`) — for a user it is also the `sub` of their tokens.
`actor.type` is what the audit log records as `actor_type` — `user`,
`api_key` (the deployment's admin key), `admin_session`, `service_client` or
`system` — and `actor.id` is absent when the actor has none. No profile data is included: ask the admin API for
what you need, and remember that a deleted resource can no longer be read.

Deliveries are queued, and a deployment under more events than it can deliver
drops the excess rather than falling behind without limit — treat webhooks as
a prompt to synchronize, not as the only record. The audit log
(`GET /api/v1/audit/logs`) keeps every event.

---

## 3. Verifying the signature

Deliveries are signed as [Standard Webhooks](https://www.standardwebhooks.com)
specifies, so any of its libraries verifies them — give it the secret, the
three `webhook-*` headers and the raw body:

```js
import { Webhook } from 'standardwebhooks'

// throws if the delivery is not genuine or is more than five minutes old
const event = new Webhook(secret).verify(rawBody, headers)
```

Without a library:

1. Take the key: the base64 after `whsec_`, decoded to bytes.
2. Compute `HMAC-SHA256(key, "<webhook-id>.<webhook-timestamp>.<body>")` over
   the **raw bytes** of the body, before any JSON parsing, and base64-encode it.
3. `webhook-signature` lists space-separated signatures, each `v1,<base64>`.
   Accept the delivery if one of them equals yours, compared in constant time.
4. Reject it if `webhook-timestamp` is more than five minutes from your clock —
   a captured delivery cannot be replayed later.

```js
import { createHmac, timingSafeEqual } from 'node:crypto'

export function verify(rawBody, headers, secret, toleranceSecs = 300) {
  const id = headers['webhook-id']
  const t = Number(headers['webhook-timestamp'])
  if (!id || !Number.isInteger(t) || Math.abs(Date.now() / 1000 - t) > toleranceSecs) return false
  const key = Buffer.from(secret.replace(/^whsec_/, ''), 'base64')
  const expected = createHmac('sha256', key).update(`${id}.${t}.`).update(rawBody).digest()
  return (headers['webhook-signature'] ?? '').split(' ').some((sig) => {
    const [version, value] = sig.split(',')
    const given = Buffer.from(value ?? '', 'base64')
    return version === 'v1' && given.length === expected.length && timingSafeEqual(given, expected)
  })
}
```

### From the previous signature

Before this form, deliveries carried `X-Webhook-Signature: t=<time>,v1=<hex>`
(with `X-Webhook-ID` naming the webhook and `X-Webhook-Event` the type) and
secrets were 64 hex characters. Those headers are no longer sent. A secret
issued then still works: the server now holds it as
`whsec_` + base64(<the 64 characters>) — the same key bytes — so write it that
way in your receiver, or rotate to get a new one.

---

## 4. Answering

Answer any `2xx` within `timeout_secs` to acknowledge. Anything else — another
status, no answer, a refused connection — counts as a failed attempt, and the
delivery is tried again up to `retry_count` more times, waiting 1, 4, 9, …
seconds between attempts. Do the work after answering if it may take long.

Every attempt is recorded:

```http
GET /api/v1/webhooks/{id}/deliveries?limit=20
```

answers `{ "deliveries": [ … ] }`, newest first, each with `success`,
`status_code` (`0` when no answer arrived), `error_message` and `attempt`.

To check an endpoint without waiting for a real event:

```http
POST /api/v1/webhooks/{id}/test
```

sends one `test` event to that webhook — whether or not it is enabled or
subscribed to `test` — makes a single attempt, and answers `{ "delivery": … }`
with the outcome.
