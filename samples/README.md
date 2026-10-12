# Samples

Three small applications, one for each part an application can play with
Authway. They are meant to be read and copied: each is a few files, and every
choice in them is one a real application should make too.

| Sample | The part it plays | Stack |
|---|---|---|
| [`asp-sample`](asp-sample) | A **server-side application** that signs users in and keeps its own session (a confidential client) | ASP.NET Core |
| [`react-sdk-sample`](react-sdk-sample) | A **browser application** that signs users in and calls an API with their access token (a public client) | React, `@authway/react` |
| [`asp-api`](asp-api) | An **API** that accepts those access tokens (a resource server) | ASP.NET Core |

All three talk to Authway through OpenID Connect discovery only, so the same
code works against any Authway deployment.

## Running them

You need a running Authway — see [`docs/SETUP.md`](../docs/SETUP.md). The
READMEs assume the Authway API at `http://localhost:8080` and its OpenID
Connect issuer at `http://localhost:4444`, and register their clients with
the admin API key of that deployment.

- `asp-sample` runs by itself.
- `react-sdk-sample` and `asp-api` go together: the browser application signs
  the user in, and calls the API with the token it got.

Run `asp-sample` and `react-sdk-sample` in the same browser and tenant to see
single sign-on: signing in to one signs the user in to the other, and signing
out of either ends the session for both.
