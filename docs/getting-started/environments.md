# Environment & Access

> For local development setup, see [Local Setup Guide](./local-setup.md).

---

## What Actually Exists Today

Team Health Check ships as an open-source app with exactly one first-party environment: **local development** (this repo, run on your machine via `make run` / `make dev`). There is no hosted Dev, Int, or Prod environment, no auto-deploy-on-merge pipeline, and no shared Slack/Okta/VPN/secrets-manager setup for this project. The workflows in `.github/workflows/` run CI and publish release images to GHCR, but do not deploy a hosted application environment.

| Environment | Purpose | Frontend URL | Backend URL | DB Host |
|-------------|---------|-------------|-------------|---------|
| **Local** | Individual dev; full stack on your laptop | http://localhost:3000 | http://localhost:8080 | localhost:5432 |

For self-hosted deployment, the repo ships a KubeVela + CloudNativePG setup — see [CI/CD & Deployment](../operations/deployment.md) for the real, verified `make kubevela-*` workflow (runs against a local k3d cluster by default; adapt the ingress/ CUE definitions for a real cluster).

### Local environment variables

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable
PORT=8080
GIN_MODE=debug
NEXT_PUBLIC_API_URL=http://localhost:8080
```

See [Local Setup Guide](./local-setup.md) for the full variable list and how these are actually loaded (they are **not** auto-sourced by the Makefile or the Go backend).

---

## Template: Adding Your Own Shared Environments

If your organization deploys Team Health Check to shared Dev/Int/Prod environments, none of that configuration exists in this repo — it's specific to your infrastructure. The table below is a **starting template**, not documentation of anything that currently exists. Replace every placeholder before relying on it, or delete this section if it doesn't apply to your deployment.

| Environment | Purpose | Frontend URL | Backend URL | DB Host |
|-------------|---------|-------------|-------------|---------|
| Dev | Shared integration testing | `<your-dev-frontend-url>` | `<your-dev-backend-url>` | `<your-dev-db-host>` |
| Int | Pre-prod validation | `<your-int-frontend-url>` | `<your-int-backend-url>` | `<your-int-db-host>` |
| Prod | Live | `<your-prod-frontend-url>` | `<your-prod-backend-url>` | `<your-prod-db-host>` |

Things you'll need to define yourself for each shared environment:

- How engineers request DB read/write access, and who approves it
- Deployment trigger (this repo has no deploy workflow — you'll need to add one, or use the `kubevela-*` Make targets against your own cluster)
- Secrets management for `DATABASE_URL`, `OAUTH_*`, `AWS_SES_*` / `SMTP_*` per environment (see [Authentication & Authorization](./auth.md) for what the `OAUTH_*` backend variables do)
- SSO redirect URIs per environment, registered with your OIDC provider
- On-call / escalation contacts and channels for incidents in each environment

---

## SSO Redirect URIs

Whatever environments you add, each one needs its own redirect URI registered with your OIDC provider:

| Setting | Value |
|---------|-------|
| Application type | Single Page App (SPA) / Public client |
| Client secret | Not required (PKCE) |
| Allowed redirect URI (Local) | `http://localhost:3000/auth/callback` |
| Allowed redirect URI (other environments) | `https://<your-frontend-host>/auth/callback` |
| Required scopes | `openid email profile` |
| Token claim needed | `email` in ID token |

See [Authentication & Authorization](./auth.md#sso-oidc-flow-authorization-code-pkce) for the full SSO flow and backend `OAUTH_*` variables.
