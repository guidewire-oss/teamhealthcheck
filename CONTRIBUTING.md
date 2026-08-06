# Contributing to Teams360

First off, thank you for considering contributing to Teams360! It's people like
you who make Teams360 a useful tool for teams everywhere. Any contribution you
make — code, docs, bug reports, enhancement ideas — is greatly appreciated.

## Code of Conduct

While we are not a CNCF project, we fully support and endorse its general
principles. As an extension, Teams360 follows their
[Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md)
that we expect project participants to adhere to. Please read the full text so
you understand what actions will and will not be tolerated.

## How Can I Contribute?

### Starting a Discussion

Have a question, an idea that isn't fully formed, or want to share how your
team is using Teams360? Use
[GitHub Discussions](https://github.com/guidewire-oss/teams360/discussions)
rather than opening an issue:

- **Q&A** — usage questions, or "is this a bug or expected?" before filing
- **Ideas** — early-stage enhancement thinking that hasn't crystallised into a
  concrete proposal yet
- **Show and tell** — screenshots, custom dimension configs, or workflows your
  team has built around Teams360

Issues are best for *actionable* items (bug reports, concrete enhancement
proposals); Discussions are best for *open-ended* conversations.

### Reporting Bugs

Before opening a new issue, please check the
[issue tracker](https://github.com/guidewire-oss/teams360/issues) to see if
it's already reported. When opening a bug report, include as many details as
possible:

1. A clear and descriptive title.
2. The exact steps to reproduce the problem.
3. Specific examples (commands, payloads, screenshots) demonstrating the steps.
4. The behavior you observed, and what you expected instead.
5. Your environment — OS, browser, Go/Node version, and how you deployed
   (local, Docker, KubeVela, Helm).

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. A good suggestion
includes:

1. A clear and descriptive title.
2. A step-by-step description of the suggested behavior.
3. The use case — *which persona* (team member, team lead, manager, admin)
   benefits and *why*.
4. The current behavior, the proposed behavior, and the gap between them.
5. Optional: prior art from other team-health tools.

### Your First Code Contribution

Looking for a good starting point? Check the
[`good first issue`](https://github.com/guidewire-oss/teams360/labels/good%20first%20issue)
and
[`help wanted`](https://github.com/guidewire-oss/teams360/labels/help%20wanted)
labels.

## Development Setup

Teams360 is a Go (Gin) backend + Next.js frontend with a PostgreSQL database.
The Makefile orchestrates everything; the most useful targets:

```bash
make install       # install backend (go mod) + frontend (npm) deps
make run           # start backend and frontend together
make test          # run all tests (Go unit/integration + frontend Vitest)
make test-e2e      # run Playwright + Ginkgo end-to-end suite
make lint          # lint both backend and frontend
make db-setup      # bring up local Postgres + run migrations
```

Run `make help` to see the full list. For deployment manifests
(KubeVela + CloudNativePG), see `Makefile.kubevela` and `kubevela/`.

## Pull Request Process

1. **Fork** the repo and create your branch from `main`.
2. **If you've added code that should be tested, add tests.** Backend uses
   Ginkgo + Gomega (BDD style). Frontend unit tests use Vitest. End-to-end
   flows use Playwright via Ginkgo in `tests/acceptance/`.
3. **Ensure the test suite passes** locally — `make test` and (if relevant)
   `make test-e2e`.
4. **Make sure the code lints** — `make lint`.
5. **Open the pull request.**

### Pull Request Checklist

- [ ] I have read the contributing guidelines.
- [ ] I have performed a self-review of my own changes.
- [ ] I have added or updated tests where appropriate.
- [ ] I have updated documentation (README, CLAUDE.md, `docs/`) where
      appropriate.
- [ ] My commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).
- [ ] I have rebased onto the latest `main` and resolved any conflicts.

### Pull Request Process

You will be asked to sign a
[Developer Certificate of Origin (DCO)](https://developercertificate.org/),
certifying that you wrote or otherwise have the right to submit the code under
the project's Apache 2.0 license. This is signified by adding a `Signed-off-by`
line to your commits (`git commit -s`).

Once the pull request is opened, a Teams360 maintainer will review your
changes. CI must be green before merge. If everything is good, your PR will be
merged into `main`.

## Styleguides

### Git Commit Messages

- Follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
  (e.g., `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`).
- Use the present tense ("Add feature" not "Added feature").
- Keep the subject line under 72 characters.
- Reference related issues in the body when applicable (e.g., `Refs #42`).

### Code Style

- **Go**: idiomatic Go; `go fmt` and `go vet` clean. Follow the existing DDD
  layout (`domain/`, `application/`, `infrastructure/`, `interfaces/`).
- **TypeScript / React**: TypeScript strict mode, ESLint clean. Follow the
  existing Next.js App Router conventions in `frontend/app/`.
- **Tests**: prefer behaviour-driven names (Ginkgo `Describe/Context/It`,
  Vitest `describe/it`). Cover the happy path and at least one error/edge case.

## License

By contributing to Teams360, you agree that your contributions will be licensed
under the [Apache License 2.0](LICENSE).
