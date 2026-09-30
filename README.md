# Orkestra

A Karmada-inspired multi-cluster orchestration platform for managing and monitoring workloads across multiple Kubernetes clusters from a single control plane.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-orchestration-326CE5?logo=kubernetes)](https://kubernetes.io/)
[![Status](https://img.shields.io/badge/status-in%20development-yellow)]()

## Overview

Running workloads across multiple Kubernetes clusters introduces real operational pain: clusters need to be registered and tracked individually, deployments must be manually propagated to each one, and there's no single place to see overall health. **Orkestra** solves this by acting as a lightweight, Karmada-style control plane that sits above your existing clusters - registering them, propagating deployments, monitoring health, and automatically moving workloads off clusters that fail.

This project was built to explore the internals of multi-cluster Kubernetes management: how propagation, health aggregation, and failover actually work under the hood, rather than just consuming an existing tool.

## Features

- **Cluster Registration** - Onboard clusters by kubeconfig. Connectivity is verified at registration, and the API server endpoint is recorded.
- **Deployment Propagation** - Apply one Deployment manifest to any set of member clusters in parallel. Targets are validated up front, so a typo never causes a partial rollout; per-cluster results are reported individually.
- **Live Rollout Status** - Query ready / up-to-date / available replicas for a deployment on every cluster it runs on, with an overall Ready verdict.
- **Health Monitoring** - Clusters are polled continuously for node readiness. Each check is bounded by a timeout so one unresponsive cluster can't stall the rest.
- **Automatic Failover** - When a cluster stays Unhealthy past a grace period, its deployments are moved to the healthy cluster with the most ready nodes. The new copy is applied *before* the old one is removed, and every move is recorded.
- **Persistent State** - Registered clusters, propagation records and failover history are saved atomically to disk and restored on restart.
- **CLI + REST API** - Everything is available over a JSON API, and the `orkestra` CLI is a thin client for it.
- **Direct Kubernetes API Integration** - Built on client-go, not by shelling out to `kubectl`.

## Demo

Run the whole story locally on three [kind](https://kind.sigs.k8s.io/) clusters: register them, propagate nginx to two, kill one, and watch Orkestra fail over.

```bash
make demo-up     # create kind clusters orkestra-a, -b, -c (~1 min)
make demo        # run the end-to-end walkthrough
make demo-down   # delete everything
```

Condensed output of `make demo`:

```
==> Propagating examples/nginx-deployment.yaml to orkestra-a and orkestra-b
Deployment default/web:
  ✅ orkestra-a   Created
  ✅ orkestra-b   Created

==> Simulating an outage: stopping orkestra-a
NAME         STATUS      NODES   ENDPOINT                  LAST CHECK
orkestra-a   Unhealthy   0/0     https://127.0.0.1:40307   2026-10-01 01:06:04
orkestra-b   Healthy     1/1     https://127.0.0.1:34075   2026-10-01 01:06:04
orkestra-c   Healthy     1/1     https://127.0.0.1:34861   2026-10-01 01:06:04

==> Waiting out the 15s grace period for failover
    ✔ Orkestra moved web off orkestra-a

Deployment default/web: Ready

CLUSTER      READY   UP-TO-DATE   AVAILABLE   STATUS
orkestra-c   2/2     2            2           Ready
orkestra-b   2/2     2            2           Ready

Failovers:
  2026-10-01 01:06:19  orkestra-a -> orkestra-c (cluster unhealthy since 2026-10-01T01:06:04+05:30)
```

The demo clusters' kubeconfigs are written to `.demo/`; your `~/.kube/config` is not modified.

## Architecture

```
                    ┌──────────────────────────┐
     orkestra CLI ─▶│   Orkestra Control Plane │
       REST API     │  ┌─────────────────────┐ │
                    │  │  Cluster Registry   │ │
                    │  ├─────────────────────┤ │
                    │  │  Propagation Engine │ │
                    │  ├─────────────────────┤ │
                    │  │  Health Aggregator  │ │
                    │  ├─────────────────────┤ │
                    │  │ Failover Controller │ │
                    │  ├─────────────────────┤ │
                    │  │  State Store (JSON) │ │
                    │  └─────────────────────┘ │
                    └────────────┬─────────────┘
                                 │ Kubernetes API (client-go)
              ┌──────────────────┼──────────────────┐
              ▼                  ▼                  ▼
       ┌────────────┐     ┌────────────┐     ┌────────────┐
       │ Cluster A  │     │ Cluster B  │     │ Cluster C  │
       └────────────┘     └────────────┘     └────────────┘
```

| Component | Package | Responsibility |
|---|---|---|
| Cluster Registry | [`internal/registry`](internal/registry) | Thread-safe store of member clusters and their health |
| Propagation Engine | [`internal/propagation`](internal/propagation) | Parses manifests, applies them to clusters in parallel (create-or-update), tracks rollout status |
| Health Aggregator | [`internal/health`](internal/health) | Polls each cluster's nodes and marks it Healthy / Unhealthy |
| Failover Controller | [`internal/propagation`](internal/propagation/failover.go) | Moves deployments off clusters that stay Unhealthy past the grace period |
| State Store | [`internal/store`](internal/store) | Atomic JSON persistence and restore on startup |
| REST API | [`internal/api`](internal/api) | HTTP interface over all of the above |
| CLI | [`cmd/orkestra`](cmd/orkestra), [`internal/client`](internal/client) | `orkestra` command; a thin client of the REST API |

**How failover decides:** a cluster becomes eligible once it has been Unhealthy for `gracePeriodSeconds` (so brief blips don't cause flapping). The replacement is the Healthy cluster with the most ready nodes that isn't already running the deployment. If applying to the replacement fails, nothing changes and it is retried on the next cycle. Removing the old copy is best effort, since the failed cluster is usually unreachable; failures are recorded on the failover event.

## Tech Stack

| Layer | Technology |
|---|---|
| Control Plane | Go, client-go |
| Multi-cluster orchestration model | Karmada-inspired design |
| Container orchestration | Kubernetes |
| Local environment | kind, Docker |

## Getting Started

### Prerequisites
- Go 1.21+
- One or more Kubernetes clusters (or Docker + kind for the demo)

### Build and run

```bash
git clone https://github.com/<your-username>/orkestra.git
cd orkestra

make build                                    # -> bin/orkestra
./bin/orkestra serve --config config.yaml     # start the control plane
```

### Using the CLI

The CLI talks to the control plane at `http://localhost:8080` by default; override with `--server` or `ORKESTRA_SERVER`.

```bash
# Register member clusters
./bin/orkestra cluster register --name cluster-a --kubeconfig ~/.kube/config-a
./bin/orkestra cluster register --name cluster-b --kubeconfig ~/.kube/config-b
./bin/orkestra cluster list

# Propagate a deployment and watch it roll out
./bin/orkestra deploy --file examples/nginx-deployment.yaml --clusters cluster-a,cluster-b
./bin/orkestra deployment list
./bin/orkestra deployment status web --namespace default
```

The kubeconfig path is read by the server, so it must exist on the machine running `orkestra serve`.

### Configuration

```yaml
server:
  port: 8080

health:
  pollIntervalSeconds: 30     # how often clusters are health-checked

failover:
  enabled: true
  gracePeriodSeconds: 60      # how long a cluster must stay Unhealthy before failover

storage:
  path: data/orkestra.json    # leave empty to keep state in memory only

log:
  level: info
```

### REST API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Control plane liveness |
| `POST` | `/api/v1/clusters` | Register a cluster: `{"name", "kubeconfigPath"}` |
| `GET` | `/api/v1/clusters` | List clusters with health |
| `GET` | `/api/v1/clusters/{name}` | Get one cluster |
| `DELETE` | `/api/v1/clusters/{name}` | Deregister a cluster |
| `POST` | `/api/v1/clusters/{name}/healthcheck` | Run a health check now |
| `POST` | `/api/v1/deployments` | Propagate: `{"manifest": "<yaml>", "clusters": [...]}` |
| `GET` | `/api/v1/deployments` | List propagation records (including failover history) |
| `GET` | `/api/v1/deployments/{namespace}/{name}` | Live rollout status per cluster |

### Development

```bash
make test    # go test -race ./...
make lint    # go vet ./...
```

## Limitations

These are known and deliberate for the current scope:

- Only `apps/v1` Deployments are propagated.
- Deployments do not move back automatically when a failed cluster recovers.
- If a failed cluster comes back before its old copy could be removed, that copy keeps running until removed manually (the failover record notes this).
- Re-propagating a deployment to a different set of clusters does not remove it from clusters dropped from the list.
- There is no authentication on the API; run it on a trusted network.

## Roadmap

- [x] Deployment propagation with per-cluster status
- [x] Failover and re-propagation on cluster health degradation
- [x] Persistent control plane state
- [ ] React dashboard with live cluster and deployment topology
- [ ] Policy-based scheduling (resource-aware placement across clusters)
- [ ] Cleanup of orphaned copies when a failed cluster recovers
- [ ] Metrics export (Prometheus integration)
- [ ] Multi-tenancy support
- [ ] Helm chart for Orkestra control-plane deployment

## Why This Project

Multi-cluster management tools like Karmada and Rancher exist, but their internals are large and dense. Orkestra was built as a focused re-implementation of the core ideas - cluster registration, propagation, health aggregation, and failover - to deeply understand the distributed-systems and Kubernetes API mechanics involved, rather than treating multi-cluster orchestration as a black box.
