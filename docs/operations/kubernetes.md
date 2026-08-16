# Kubernetes (KubeVela)

Team Health Check can be deployed to Kubernetes via **KubeVela** (Open Application Model) with
**CloudNativePG (CNPG)** providing production-grade PostgreSQL. This is defined entirely
in `kubevela/` and `Makefile.kubevela`.

!!! warning "No Helm chart exists in this repository"
    `CLAUDE.md` describes KubeVela as "an alternative to the existing Helm charts in
    `helm/teams360/`" — that directory does not exist in this repository. Treat KubeVela
    as the only Kubernetes deployment path currently present.

## Architecture

| File | Purpose |
|---|---|
| `kubevela/components/cnpg.cue` | Component type `cloud-native-postgres` — renders a CNPG `Cluster` custom resource |
| `kubevela/components/gateway.cue` | Trait `gateway` — renders a Service + Ingress, targeted at k3d's Traefik ingress; production clusters should define their own ingress trait instead |
| `kubevela/teams360-kubevela.yaml` | The KubeVela `Application`: two components (`postgres`, `app`) plus an ordered workflow |
| `Makefile.kubevela` | All `kubevela-*` Make targets |

!!! warning "The application is one unified component, not separate frontend/backend components"
    `CLAUDE.md`'s deployment workflow table describes four steps including
    `deploy-backend` and `deploy-frontend` as distinct components. The actual
    `kubevela/teams360-kubevela.yaml` workflow only has two `apply-component` steps:
    `deploy-database` (component `postgres`) and `deploy-app` (component `app`, type
    `webservice`) — consistent with the single unified container image described in
    [Docker](docker.md) (the Go binary serves the statically-exported Next.js app itself,
    so there is only one application component to deploy).

## Deployment workflow

Defined under `workflow.steps` in `kubevela/teams360-kubevela.yaml`:

1. **`deploy-database`** (`apply-component`, component `postgres`) — creates the CNPG
   PostgreSQL cluster.
2. **`wait-for-infrastructure`** (`suspend`, 90s) — pauses for CNPG to finish bootstrapping
   the cluster and its secrets before the app tries to connect.
3. **`deploy-app`** (`apply-component`, component `app`, depends on step 2) — deploys the
   unified Team Health Check `webservice`, wired to the CNPG-generated database secret via a
   `service-binding` trait and exposed via the `expose`/`gateway` traits.

## Make targets

Run `make -f Makefile.kubevela help`-style discovery via the target comments below, or
just use these directly (see `Makefile.kubevela` for exact commands):

| Target | Purpose |
|---|---|
| `kubevela-deploy-all` | Full pipeline: create k3d cluster, install CNPG + KubeVela operators, build/import the image, deploy |
| `kubevela-deploy-quick` | Rebuild the image and redeploy, assuming the cluster and operators already exist |
| `kubevela-k3d-create` / `kubevela-k3d-delete` / `kubevela-k3d-status` | Manage the local k3d cluster |
| `kubevela-check-hosts` | Verify `/etc/hosts` has the `teams360.local` entry the k3d Ingress expects |
| `kubevela-check-install-kubevela` / `kubevela-check-install-cnpg` | Install the KubeVela and CNPG operators if missing |
| `kubevela-build-and-load-images` | Build the unified Docker image and import it into k3d |
| `kubevela-deploy` | Apply the CUE component/trait definitions and the Application manifest |
| `kubevela-delete` | Remove the KubeVela Application |
| `kubevela-status` | Show pods, services, ingress, and the CNPG cluster status |

Prerequisites and the verification checklist are documented in `CLAUDE.md`'s KubeVela
section — that content is accurate against the current `Makefile.kubevela` targets and is
not duplicated here.
