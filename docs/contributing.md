# Contributing

The canonical, up-to-date contributing guide lives at the repository root:
[`CONTRIBUTING.md`](https://github.com/guidewire-oss/teams360/blob/main/CONTRIBUTING.md).
This page summarizes it for MkDocs navigation — edit the root file, not this page, when
the process changes.

## Getting started

- **Discussions vs. Issues** — open-ended questions, early-stage ideas, and "show and
  tell" belong in [GitHub Discussions](https://github.com/guidewire-oss/teams360/discussions);
  concrete bug reports and enhancement proposals belong in the
  [issue tracker](https://github.com/guidewire-oss/teams360/issues).
- **First contribution?** Look for the `good first issue` and `help wanted` labels.

## Development setup

```bash
make install       # backend (go mod) + frontend (npm) deps
make run           # start backend and frontend together
make test          # frontend Vitest + backend Ginkgo/Gomega + E2E
make test-e2e      # Playwright + Ginkgo end-to-end suite only
make lint          # lint both backend and frontend
make db-setup      # bring up local Postgres + run migrations
```

See [Testing](testing.md) for the full breakdown of what each test target actually runs,
and [Operations → Makefile](operations/makefile.md) for the complete target reference.

## Pull request process

1. Fork the repo, branch from `main`.
2. Add tests for anything you change — Ginkgo/Gomega (BDD style) on the backend, Vitest
   on the frontend, Playwright-via-Ginkgo for E2E flows in `tests/acceptance/`.
3. `make test` (and `make test-e2e` if relevant) must pass locally.
4. `make lint` must be clean.
5. Open the PR. You'll be asked to sign off commits (DCO) via `git commit -s`, certifying
   you have the right to submit the code under the Apache 2.0 license.

### Checklist

- [ ] Self-reviewed the diff.
- [ ] Tests added/updated where appropriate.
- [ ] Docs updated (`README.md`, `CLAUDE.md`, `docs/`) where appropriate.
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).
- [ ] Rebased onto latest `main`.

CI must be green before a maintainer merges.

## Style

- **Go** — idiomatic, `go fmt`/`go vet` clean, follows the existing DDD layout
  (`domain/`, `application/`, `infrastructure/`, `interfaces/` — see
  [Architecture](architecture/overview.md)).
- **TypeScript/React** — strict mode, ESLint clean, follows the existing Next.js App
  Router conventions in `frontend/app/`.
- **Commits** — [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
  (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`), present tense, subject under 72 chars.

## License

Contributions are licensed under [Apache License 2.0](https://github.com/guidewire-oss/teams360/blob/main/LICENSE).
By contributing you agree your work will be licensed under it.
