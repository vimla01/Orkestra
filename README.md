# Orkestra

A Karmada-inspired multi-cluster orchestration platform for managing and monitoring workloads across multiple Kubernetes clusters from a single control plane.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-orchestration-326CE5?logo=kubernetes)](https://kubernetes.io/)
[![Status](https://img.shields.io/badge/status-in%20development-yellow)]()

## Overview

Running workloads across multiple Kubernetes clusters introduces real operational pain: clusters need to be registered and tracked individually, deployments must be manually propagated to each one, and there's no single place to see overall health and topology. **Orkestra** solves this by acting as a lightweight, Karmada-style control plane that sits above your existing clusters - registering them, propagating deployments, monitoring health, and visualizing the entire fleet in real time.

This project was built to explore the internals of multi-cluster Kubernetes management: how propagation, scheduling, and health aggregation actually work under the hood, rather than just consuming an existing tool.

## Features

- **Cluster Registration** - Onboard and manage multiple Kubernetes clusters under one control plane using kubeconfig-based registration.
- **Deployment Propagation** - Push a single deployment spec from the control plane and propagate it across selected member clusters automatically.
- **Health Monitoring** - Continuously poll cluster and workload health, surfacing node status, pod readiness, and resource availability.
- **Live Visualization Dashboard** - A React-based UI showing real-time cluster status, deployment topology, and workload distribution across the fleet.
- **Kubernetes API Integration** - Direct integration with the Kubernetes API server for resource monitoring and automated deployment management (not screen-scraping `kubectl` output).

## Architecture

```
                    ┌───────────────────────── ┐
                    │      Orkestra Control    │
                    │           Plane          │ 
                    │  ┌─────────────────────┐ │
                    │  │  Cluster Registry   | │ 
                    │  ├─────────────────────┤ │
                    │  │  Propagation Engine | │ 
                    │  ├─────────────────────┤ │
                    │  │  Health Aggregator  | │ 
                    │  └─────────────────────┘ │
                    └────────────┬─────────────┘
                                 │ Kubernetes API
              ┌──────────────────┼──────────────────┐
              ▼                  ▼                  ▼
       ┌────────────┐     ┌────────────┐     ┌────────────┐
       │ Cluster A  │     │ Cluster B  │     │ Cluster C  │
       │ (K8s)      │     │ (K8s)      │     │ (K8s)      │
       └────────────┘     └────────────┘     └────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │   React Dashboard (UI)  │
                    │  Live topology + status │
                    └─────────────────────────┘
```

**Control Plane (Go)** - Handles cluster registration, deployment propagation logic, and health-check polling against each member cluster's API server.

**Dashboard (React)** - Consumes control-plane APIs to render real-time cluster topology, deployment status, and workload distribution.

**Containerization (Docker)** - Both control plane and dashboard are containerized for consistent local development and deployment.

## Tech Stack

| Layer | Technology |
|---|---|
| Control Plane | Go |
| Multi-cluster orchestration model | Karmada-inspired design |
| Container orchestration | Kubernetes |
| Dashboard | React |
| Containerization | Docker |

## Getting Started

### Prerequisites
- Go 1.21+
- Docker
- Access to one or more Kubernetes clusters (kind/minikube work for local testing)
- `kubectl` configured with valid kubeconfig(s)

### Setup

```bash
# Clone the repository
git clone https://github.com/<your-username>/orkestra.git
cd orkestra

# Build the control plane
go build -o bin/orkestra ./cmd/orkestra

# Run the control plane
./bin/orkestra --config config.yaml

# In a separate terminal, start the dashboard
cd dashboard
npm install
npm start
```

### Registering a Cluster

```bash
./bin/orkestra cluster register --name cluster-a --kubeconfig ~/.kube/config-a
```

### Propagating a Deployment

```bash
./bin/orkestra deploy --file deployment.yaml --clusters cluster-a,cluster-b
```

## Roadmap

- [ ] Policy-based scheduling (resource-aware placement across clusters)
- [ ] Failover and re-propagation on cluster health degradation
- [ ] Multi-tenancy support
- [ ] Metrics export (Prometheus integration)
- [ ] Helm chart for Orkestra control-plane deployment

## Why This Project

Multi-cluster management tools like Karmada and Rancher exist, but their internals are large and dense. Orkestra was built as a focused re-implementation of the core ideas - cluster registration, propagation, and health aggregation - to deeply understand the distributed-systems and Kubernetes API mechanics involved, rather than treating multi-cluster orchestration as a black box.
