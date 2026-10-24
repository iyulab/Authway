# What Authway keeps about a user

The facts a privacy policy needs about the accounts Authway holds: what is
stored, for how long, what leaves it, and what deleting an account removes.
They describe the software. Where it runs, who hosts it and how long the
host keeps its own logs are the deployment's to state.

## Stored for each account

| What | Notes |
|---|---|
| Email address, name, picture address | The picture is an address, not an image Authway stores |
| Whether the address is verified, whether the account is suspended | |
| Password | Only as a bcrypt hash, and only for accounts that set one |
| A provider's account id | For each of Google, GitHub, Microsoft and Apple the user signed in with. The provider's own tokens are not kept |
| Two-step sign-in | The authenticator secret, encrypted (AES-256-GCM); recovery codes as hashes |
| When the account was created, changed and last signed in | |
| Custom claims | What the application's backend writes for the user ([claims](api/claims.md)) |
| Accepted terms | Which document and version, through which application, and when. No IP address or browser |
| Delegations and approvals | Which application may act for the user, with which scopes, until when; the approval requests the user answered ([delegation](api/delegation.md)) |
| Console memberships | For administrators only: which tenants their console account administers |

Sign-in sessions and consents are held by the authorization server and end
with the session, or when the user signs out everywhere.

## Held for a short time

| What | How long |
|---|---|
| Sign-in links and codes by email | 15 minutes, then removed within the hour. Each records the IP address and browser that asked for it |
| Email verification links | 6 hours, then removed within the hour (only a hash of the link is stored) |
| Password reset links | 1 hour, then removed within the hour (hash only) |
| Invitations | Valid for 7 days. Once accepted, revoked or expired, the invitation — the invited address, who invited, the message — is removed after 90 days |
| Webhook delivery records | 30 days — each delivery's payload (ids only) and the receiver's answer |

## The audit log

Each tenant has an audit log of sign-ins (successful and failed), account and
administration changes, and webhook activity. An entry records who acted (an
account id and email address, or the admin key), what was done to what, the
IP address and browser, and the outcome. A failed sign-in records the address
that was tried; an administrator's deletion of an account records the deleted
account's address.

Entries are kept for the tenant's `audit_retention_days` — 365 days unless
the tenant sets another, at least 30 — and removed daily once older. A tenant
owner can also remove older entries at once: `DELETE
/api/v1/audit/logs/purge?retention_days=N` — without `retention_days`, those
past the tenant's own retention.

## What leaves Authway

- **Email** to the user's own address: verification and reset links, sign-in
  links and codes, and invitations (with the inviter's name, the tenant's name
  and the inviter's message). The deployment chooses the mail service.
- **Webhooks** to the addresses a tenant registers. A delivery carries ids —
  the event, the tenant, the resource and who acted — and no profile data
  ([webhooks](api/webhooks.md)). Authway keeps each delivery and the
  receiver's answer for 30 days.
- **Sign-in providers** are asked for the user's basic profile and email
  address (Google and Microsoft `openid email profile`, GitHub `user:email
  read:user`, Apple `name email`). Authway keeps the provider's account id, the
  address and whether the provider verified it, the name, and the picture
  address when the account has none.
- **Tokens** issued to applications carry the account id (`sub`) and the
  claims Authway asserts — `tenant_id`, `email`, `name`, `email_verified` —
  with the application's custom claims ([claims](api/claims.md)).

The service's own logs name email addresses in some lines (an invitation
sent, an account linked to a provider); how long those are kept is the
deployment's log retention.

## When an account is deleted

By the user ([`DELETE /api/v1/profile/me`](api/account.md#2-deleting-the-account))
or by an administrator:

- the account's sessions, consents and tokens are ended at the authorization
  server;
- the account is removed, with its custom claims, accepted terms,
  delegations, approval requests, console memberships and unused email links;
- the [`user.deleted`](api/webhooks.md#events) webhook tells the tenant's
  services, which remove what they keep about the user.

What stays, for as long as above: the tenant's audit-log entries about the
account, invitations sent to its address (the invitation's link to the
account is cleared), webhook delivery records (ids only), and the service's
logs.
