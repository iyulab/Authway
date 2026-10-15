# Errors

Every refusal from `/api/v1/*` is an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)
problem details document, sent as `application/problem+json`:

```http
HTTP/1.1 409 Conflict
Content-Type: application/problem+json

{
  "type": "about:blank",
  "title": "Conflict",
  "status": 409,
  "detail": "A tenant with that slug already exists.",
  "code": "tenant_slug_taken"
}
```

| Member | |
|--------|---|
| `type` | Always `about:blank` |
| `title` | The HTTP status's reason phrase |
| `status` | The HTTP status |
| `detail` | A message to show a person |
| `code` | A stable identifier to branch on — the contract lists them ([`openapi.yaml`](../../packages/contract/openapi.yaml), schema `Error`) |

Some refusals carry further members, named where they occur: `field` (the
request member at fault — a member inside an object by its path, such as
`settings.allowed_domains`, and a list by its own name, not an element's
index), `hint` (what to do about it), `retry_after`
(seconds), `max_age`, `min_length`, `terms`, `sync_status`. A `401` carries
`WWW-Authenticate`; a `429` carries `Retry-After`.

Branch on `code`, show `detail`. Any problem details library reads these
documents — in ASP.NET Core, `ProblemDetails` with `code` in `Extensions`.

The endpoints that OAuth 2.0 and OpenID Connect define keep the error form
those specifications give them (`{"error": "invalid_client", …}`, RFC 6749
§5.2): token, introspection and the redirects back to an application.

## From the previous form

Before this form, a refusal was `{"error": "<message>", "code": "<code>"}`
with `application/json`. `code` keeps its values and its place; the message
moved from `error` to `detail`.
