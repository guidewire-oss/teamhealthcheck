---
name: deploy-kubevela
description: Deploy Team360 to Kubernetes using KubeVela and CloudNativePG. Use this when deploying or managing k8s environments.
---

# KubeVela + CNPG Deployment (Kubernetes)

Team360 supports deployment to Kubernetes via **KubeVela** (Open Application Model) with **CloudNativePG** for production-grade PostgreSQL. This is an alternative to the existing Helm charts in `helm/teams360/`.

## Prerequisites

- **Docker** (for building images)
- **k3d** (lightweight k3s-in-Docker for local clusters)
- **kubectl** (Kubernetes CLI)
- **Helm** (installed automatically for operator setup)
- **KubeVela CLI** (`vela`) — installed automatically by Makefile targets
- `/etc/hosts` entry: `127.0.0.1 teams360.local`

## Quick Start

```bash
# Full pipeline: create cluster, install operators, build images, deploy
make kubevela-deploy-all

# Monitor deployment (CNPG needs ~45s to bootstrap)
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

## Architecture Overview

| File | Purpose |
|------|---------|
| `kubevela/components/cnpg.cue` | CUE definition: `cloud-native-postgres` component type (renders CNPG Cluster CR) |
| `kubevela/components/gateway.cue` | CUE definition: `gateway` trait (renders Service + Ingress, k3d/Traefik only) |
| `kubevela/teams360-kubevela.yaml` | KubeVela Application manifest (3 components + ordered workflow) |
| `Makefile.kubevela` | All `kubevela-*` Make targets for automation |

**Deployment workflow** (defined in the YAML):
1. `deploy-database` — CNPG PostgreSQL cluster (1 instance, 0.5Gi)
2. `wait-for-infrastructure` — 45s suspend for CNPG to create cluster + secrets
3. `deploy-backend` — Go API (gets `DATABASE_URL` from CNPG secret via service-binding)
4. `deploy-frontend` — Next.js app (gets `BACKEND_URL=http://backend:8080`)

## Available Makefile Targets

| Target | Purpose |
|--------|---------|
| `kubevela-deploy-all` | Full pipeline: cluster + operators + build + deploy |
| `kubevela-deploy-quick` | Rebuild images and redeploy (assumes cluster exists) |
| `kubevela-k3d-create` | Create k3d cluster with port 8080:80 |
| `kubevela-k3d-delete` | Delete the k3d cluster |
| `kubevela-k3d-status` | Show cluster info and nodes |
| `kubevela-check-hosts` | Verify `/etc/hosts` has `teams360.local` |
| `kubevela-check-install-kubevela` | Install KubeVela CLI + operator |
| `kubevela-check-install-cnpg` | Install CNPG operator |
| `kubevela-build-and-load-images` | Docker build both images + k3d import |
| `kubevela-deploy` | Apply CUE defs + kubectl apply YAML |
| `kubevela-delete` | Remove the KubeVela application |
| `kubevela-status` | Show pods, services, ingress, CNPG clusters |

## Verification Checklist

After `make kubevela-deploy-all`, verify:

1. `make kubevela-deploy-all` completes from clean state
2. CNPG operator running: `kubectl get pods -n cnpg-system`
3. KubeVela operator running: `kubectl get pods -n vela-system`
4. PostgreSQL pod(s) running: `kubectl get pods -n teams360 -l cnpg.io/cluster=postgres`
5. Backend pod running and healthy: `kubectl get pods -n teams360 -l app.oam.dev/component=backend`
6. Frontend pod running and healthy: `kubectl get pods -n teams360 -l app.oam.dev/component=frontend`
7. Frontend loads: `curl -s http://teams360.local:8080/`
8. API returns data: `curl -s http://teams360.local:8080/api/v1/health-dimensions`
9. Login with `demo/demo` works through the browser
10. `make kubevela-delete` cleanly removes the app
11. `make kubevela-k3d-delete` removes the cluster

$ARGUMENTS
