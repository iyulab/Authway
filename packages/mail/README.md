# @authway/mail

The messages Authway sends — email verification, password reset, invitation
and sign-in link — as files every Authway implementation renders the same
way. A message is a subject, an HTML body and a plain-text body.

```
messages/
  layout.html          the HTML shell every message shares
  <message>.html       the message's own HTML, the "content" partial
  <message>.txt        the message's plain-text body
  partials/*.html      shared HTML partials, named after the file
  styles.json          per message: { "color": … } (heading and button)
locales/
  <locale>.json        per message (and "common"): the strings
```

## Rendering a message

1. Take the message's values (below). A value that is an empty string counts
   as absent. A `link` must be an absolute `http` or `https` address — refuse
   to render otherwise.
2. Its strings are `common` with the message's own strings over it, from
   `locales/<locale>.json` (falling back to `ko`). Each string is itself a
   template: render it as **text** (no escaping) against the values.
3. Add the rendered strings to the values as `t`, and the message's entry in
   `styles.json` as `style`.
4. **Subject** = `t.subject`.
   **HTML body** = `layout.html` rendered with HTML escaping, with the
   message's `<message>.html` as the partial `content` and each file in
   `partials/` as a partial of its own name.
   **Text body** = `<message>.txt` rendered without escaping.

| message | values |
|---|---|
| `verification` | `link`, `organization` |
| `reset` | `link`, `organization` |
| `invitation` | `link`, `inviterName`, `tenantName`, `message` |
| `magiclink` | `link`, `organization`, `code`, `newUser` (boolean) |

`organization` names the tenant whose account a message is about, when the
address may have accounts in several.

## Template language

The files use this part of [Mustache](https://mustache.github.io/mustache.5.html):
variables (`{{name}}` escaped, `{{{name}}}` and `{{& name}}` as they are),
dotted names, sections, inverted sections, partials, comments and `{{.}}`.
They use no lambdas and no set delimiters, and keep section tags inline — so
an engine that removes lines holding only a section tag (as the Mustache
specification asks) renders the same text as one that does not.

## Changing a message

Change the files here: every implementation that renders them sends the
change. A new value a message needs is a new row in the table above, which
each implementation passes.
