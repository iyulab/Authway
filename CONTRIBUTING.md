# Contributing to Authway

Thanks for your interest in improving Authway.

## Getting set up

Follow the [README Quick Start](./README.md#quick-start) to get the backing
services (PostgreSQL, Redis, MailHog, Hydra) and every app running locally.
Each Go service reads its own `.env`, and the two Vite apps run with
`npm run dev` for working HMR.

## Before opening a pull request

- Run the relevant test suite for what you changed:
  - Go: `go test ./...` from the repo root, or from `apps/branding/auth-api`
    (its own module).
  - TypeScript apps/packages: `pnpm test` from the repo root, or
    `npx vitest run` inside `apps/central/admin` / `apps/branding/auth-ui`.
- `go build ./... && go vet ./...` for any Go change.
- Add or update tests for the behavior you changed — the CI workflow
  (`.github/workflows/ci.yml`) runs the full suite on every pull request.
- Update [CHANGELOG.md](./CHANGELOG.md) under `[Unreleased]` for any
  user-visible change.

## Reporting issues

Open a [GitHub issue](https://github.com/iyulab/authway/issues) with
reproduction steps. For security-sensitive reports, please avoid filing a
public issue with exploit details — describe the concern generally and a
maintainer will follow up.

## License

By contributing, you agree that your contributions will be licensed under
the project's [Apache License 2.0](./LICENSE).
