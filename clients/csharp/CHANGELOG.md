# Changelog

All notable changes to `Authway.Client` are recorded here. The package follows its own version line, apart from the
Authway service.

## Unreleased

### Added

- `AuthwayClient` for the administration API, calling with the admin key or a tenant service credential's token:
  applications (`Clients`), invitations (`Invitations`, optionally for one application), users and their claims and
  terms acceptances (`Users`), webhooks (`Webhooks`) and a tenant's terms (`Terms`).
- `AuthwayException` with the refusal as `AuthwayProblem` — RFC 9457 problem details: `Code`, `Detail`, `Field`,
  `Hint`, `RetryAfter` and the remaining members.
- `AuthwayWebhook` — verifies Standard Webhooks deliveries and reads their `WebhookEvent`; `WebhookEventTypes`.
