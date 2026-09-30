## KubeVela + CNPG Deployment (Kubernetes)

Team Health Check supports deployment to Kubernetes via **KubeVela** (Open Application Model) with **CloudNativePG** for production-grade PostgreSQL.

### Prerequisites

- **Docker** (for building images)
- **k3d** (lightweight k3s-in-Docker for local clusters)
- **kubectl** (Kubernetes CLI)
- **Helm** (installed automatically for operator setup)
- **KubeVela CLI** (`vela`) — installed automatically by Makefile targets
- `/etc/hosts` entry: `127.0.0.1 teams360.local`

### Quick Start

```bash
# Full pipeline: create cluster, install operators, build images, deploy
make kubevela-deploy-all

# Monitor deployment (CNPG needs ~90s to bootstrap)
make kubevela-status

# Access the app
open http://teams360.local:8080/      # Frontend
curl http://teams360.local:8080/api/v1/health-dimensions  # API

# Quick rebuild after code changes (skip cluster/operator setup)
make kubevela-deploy-quick

# Tear down
make kubevela-delete        # Remove application
make kubevela-k3d-delete    # Remove cluster
```

### Architecture Overview

| File | Purpose |
|------|---------|
| `kubevela/components/cnpg.cue` | CUE definition: `cloud-native-postgres` component type (renders CNPG Cluster CR) |
| `kubevela/components/gateway.cue` | CUE definition: `gateway` trait (renders Service + Ingress, k3d/Traefik only — production deployments should define their own ingress) |
| `kubevela/teams360-kubevela.yaml` | KubeVela Application manifest (2 components + ordered workflow) |
| `Makefile.kubevela` | All `kubevela-*` Make targets for automation |

**Deployment workflow** (defined in the YAML):
1. `deploy-database` — CNPG PostgreSQL cluster (1 instance, 0.5Gi)
2. `wait-for-infrastructure` — 90s suspend for CNPG to create cluster + secrets
3. `deploy-app` — Unified app container (Go API serving static frontend; gets `DATABASE_URL` from CNPG secret via service-binding)

### Verification Checklist

After `make kubevela-deploy-all`, verify:

1. `make kubevela-deploy-all` completes from clean state
2. CNPG operator running: `kubectl get pods -n cnpg-system`
3. KubeVela operator running: `kubectl get pods -n vela-system`
4. PostgreSQL pod(s) running: `kubectl get pods -n teams360 -l cnpg.io/cluster=postgres`
5. App pod running and healthy: `kubectl get pods -n teams360 -l app.oam.dev/component=app`
6. Frontend loads: `curl -s http://teams360.local:8080/`
7. API returns data: `curl -s http://teams360.local:8080/api/v1/health-dimensions`
8. Login with `demo/demo` works through the browser
9. `make kubevela-delete` cleanly removes the app
10. `make kubevela-k3d-delete` removes the cluster
