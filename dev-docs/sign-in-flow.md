# Sign-in flow

**Question:** in what order do an application, the browser, the API, the login
screens and the authorization server cooperate until the application holds a
token — and where does a sign-in stop?
**Level:** L2 · **Baseline:** `a5a7056` (`apps/central/api/internal/handler`) ·
**Source:** the flow contract (`docs/api/login-flows.md`) and `packages/contract/openapi.yaml`;
this page only shows the order

The authorization server runs the protocol and asks the API, three times, for
what the protocol leaves open: who the person is (login), whether they agree
(consent), and what ends a session (logout). Each time it sends the browser to the
API with a challenge; the API either settles it at once or opens a screen with an
opaque **flow id** and answers the screen's questions until it can hand the
result back.

```mermaid
sequenceDiagram
  autonumber
  actor P as Person
  participant B as Browser
  participant A as Application
  participant S as Authorization server
  participant API as Authway API
  participant UI as Login screen

  A->>B: redirect to the authorization endpoint (PKCE)
  B->>S: authorization request
  S->>B: redirect to the API with a login challenge
  B->>API: open the login flow
  alt the browser already has a session the client can use
    API->>S: accept the login
    S->>B: continue to consent
  else a person has to answer
    API->>B: redirect to a login screen with a flow id
    B->>UI: load the screen
    UI->>API: ask what to show
    API-->>UI: next = form (methods the client offers)
    P->>UI: password, passkey, emailed link or a social provider
    UI->>API: answer
    opt the account has a second factor or terms are owed
      API-->>UI: next = mfa or terms
      P->>UI: code or acceptance
      UI->>API: answer
    end
    API->>S: accept the login
    API-->>UI: next = redirect
    UI->>B: navigate on
    B->>S: back to the authorization server
  end
  Note over S,API: consent: the same pattern, skipped when nothing is to be asked
  S->>B: redirect to the application with a code
  B->>A: code
  A->>API: token request (the API fronts the authorization server's token endpoint)
  API-->>A: tokens
```

## What the screen is told

Every flow endpoint answers `next`, and the screen does exactly that:
`form` shows a form, `redirect` navigates, `mfa` asks for the second factor,
`terms` asks the person to accept the tenant's documents, `email_sent` says a
link was mailed. The screen holds no state of its own: the flow id is the only
thing it passes back, so a custom screen can replace the bundled one.

## Where a sign-in stops

```mermaid
flowchart LR
  start(["Sign-in starts"]) --> m{"Method allowed<br/>for this client?"}
  m -->|no| r1["Refused:<br/>sign_in_method_not_allowed"]:::fail
  m -->|yes| c{"Credential right?"}
  c -->|no| r2["One answer for every reason,<br/>failures counted"]:::fail
  c -->|yes| s{"Account active, tenant<br/>and client on?"}
  s -->|no| r3["Refused"]:::fail
  s -->|yes| f{"Second factor?"}
  f -->|"has one"| m2["Code or recovery code"] --> t
  f -->|none| t{"Terms owed?"}
  t -->|yes| t2["Accept the current versions"] --> ok
  t -->|no| ok(["Login accepted"]):::ok

  classDef fail fill:#fee2e2,stroke:#dc2626,color:#450a0a
  classDef ok fill:#dcfce7,stroke:#16a34a,color:#052e16
```

The second factor is asked after **any** first factor, a passkey included: a
passkey is a first factor, not a way around the second. A failed credential gets
one answer whatever the reason, so the answer does not tell who has an account.

## Invariants

- The flow id is opaque. A flow is signed in once: an unknown id is
  `400 invalid_flow`, one that expired or was already completed is
  `410 flow_expired`. The sign-in screen stays up while the browser follows the
  redirect, so a flow that was signed in answers the user it signed in once
  more, for ten minutes, when they prove who they are again: the same redirect,
  and no second `user.login` entry. Submits that overlap are taken one after the
  other. A password for anyone else's account is not checked and is refused as a
  wrong one is, so the answer says nothing of whether it was right; a wrong
  credential is a failure like any other.
- A sign-in only opens the tenant the client belongs to; a person with accounts in
  two tenants has two accounts.
- A method is offered only where the client allows it: passkeys wherever the
  password form is offered (or where `passkey` is named), a social provider only
  where the client enables it and the deployment has credentials for it.
