# Authway constitution

> What Authway is for, and the rules it keeps whatever is built next. A change
> to this document is a design decision of the maintainers and is made as one,
> in writing, with what it gives up. What is in and out of scope is in
> [scope.md](scope.md).

## 1. What Authway is

Authway is a standards-based identity provider — OAuth 2.0 and OpenID Connect —
for applications that want sign-in handled once and handled well. An
application registers a client, sends its users to Authway, and gets back
standard tokens. Authway owns the part that is easy to get wrong: how a person
proves who they are, what an account may do to its own sign-in, and what is
recorded about it.

It is multi-tenant: a tenant is a set of accounts, clients and settings that no
other tenant can read or reach. It runs as one build wherever it runs — hosted
for many tenants, or installed for one — and the installed form is not a
different product.

## 2. Principles

1. **Standards before inventions.** Applications and SDKs find endpoints
   through OpenID Connect discovery and speak standard flows. Authway-specific
   paths exist only where no standard does the job, live under `/api/v1/`, and
   are in the published contract.
2. **The contract is the product.** `packages/contract/openapi.yaml` and the
   conformance suite define what an Authway answers. Anything an application can
   see — a path, a field, a refusal, a claim — is in the contract first, with a
   conformance scenario. A second implementation passes the same suite; where it
   cannot offer something, it says so in `capabilities`, never by behaving
   differently.
3. **A tenant is a wall.** An account, a client, a credential, a passkey belongs
   to one tenant. Accounts of different tenants are never merged, and a
   credential of one tenant never opens another's sign-in — even where the
   sign-in screen is shared.
4. **Sign-up is invited unless a tenant opens it.** Every account starts from an
   invitation by default. Magic links, social sign-in and passkeys are gated by
   the same policy and never become a side door.
5. **A token an application holds is not the user.** An application that holds
   a token for a user can read what its scopes allow. It cannot add a way to
   sign in to the user's account, choose their password, or answer a question
   meant for them: those take the account screen and a recent sign-in. Acting for
   a user while they are away is a grant the user made, bounded, and revocable.
6. **No account is left without a way in, and none is made weaker by being
   shared.** The last way to sign in cannot be removed, and removals racing for it
   are serialized. A second factor a user has is still asked, whatever the first
   factor was.
7. **Refuse; don't guess, and don't explain to strangers.** Ambiguity is
   refused with a stable code. A failed credential gets one answer whatever the
   reason, so it does not tell who has an account.
8. **Secrets are sealed, and logs hold none.** Keys, secrets and tokens are
   sealed at rest and never written to logs, audit records, or error bodies.
9. **Every change of consequence leaves a record.** Sign-ins, changes to how an
   account signs in, administrative actions and their outcomes are audited per
   tenant, and tenants can be told by webhook.
10. **One build, one set of defaults.** What is hosted and what is installed run
    the same code with the same defaults; a deployment differs by configuration,
    not by fork.

## 3. How it changes

- Before 1.0 the contract may change incompatibly in a minor release when that
  gives the better design. Every such change is in the changelog with how to
  migrate, and the contract and the conformance suite change first, so that what
  an application can rely on is always written down. At 1.0 and after, an
  incompatible change needs a major version, which the maintainers decide by
  hand.
- A new way to sign in, a new boundary between tenants, or a new place secrets
  live is a design decision, written down before it is built.
- Security fixes ship when ready; they are not held for a release.
