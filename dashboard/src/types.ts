// Types mirroring the control plane's JSON API (see internal/registry and
// internal/propagation in the Go code).

export type ClusterHealth = "Healthy" | "Unhealthy" | "Unknown";

export interface Cluster {
  name: string;
  status: ClusterHealth;
  registeredAt: string;
  lastHealthCheck: string;
  nodeCount: number;
  readyNodes: number;
  unhealthySince?: string;
  endpoint: string;
}

export interface ClusterResult {
  cluster: string;
  action: "Created" | "Updated" | "Failed";
  error?: string;
}

export interface FailoverEvent {
  from: string;
  to: string;
  at: string;
  reason: string;
  cleanupError?: string;
}

export interface DeploymentRecord {
  namespace: string;
  name: string;
  clusters: string[];
  results: ClusterResult[];
  propagatedAt: string;
  failovers?: FailoverEvent[];
}

export interface ClusterStatus {
  cluster: string;
  desiredReplicas: number;
  readyReplicas: number;
  availableReplicas: number;
  updatedReplicas: number;
  ready: boolean;
  error?: string;
}

export interface DeploymentStatus extends DeploymentRecord {
  ready: boolean;
  statuses: ClusterStatus[];
}

/** A snapshot of everything the dashboard shows, fetched together. */
export interface Snapshot {
  clusters: Cluster[];
  deployments: DeploymentStatus[];
  fetchedAt: Date;
}
