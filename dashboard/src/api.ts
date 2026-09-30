import type { Cluster, ClusterResult, DeploymentRecord, DeploymentStatus, Snapshot } from "./types";

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      Accept: "application/json",
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const data = (await res.json()) as { error?: string };
      if (data.error) message = data.error;
    } catch {
      // Non-JSON error body; keep the status line.
    }
    throw new Error(message);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/**
 * Fetches clusters, deployments, and live per-cluster status for every
 * deployment. Status requests run in parallel; a deployment whose status
 * can't be read falls back to its record with every cluster marked errored,
 * so one failure doesn't blank the whole dashboard.
 */
export async function fetchSnapshot(): Promise<Snapshot> {
  const [clusters, records] = await Promise.all([
    request<Cluster[]>("GET", "/api/v1/clusters"),
    request<DeploymentRecord[]>("GET", "/api/v1/deployments"),
  ]);

  const deployments = await Promise.all(
    records.map(async (record): Promise<DeploymentStatus> => {
      try {
        return await request<DeploymentStatus>("GET", deploymentPath(record.namespace, record.name));
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err);
        return {
          ...record,
          ready: false,
          statuses: record.clusters.map((cluster) => ({
            cluster,
            desiredReplicas: 0,
            readyReplicas: 0,
            availableReplicas: 0,
            updatedReplicas: 0,
            ready: false,
            error,
          })),
        };
      }
    }),
  );

  return { clusters, deployments, fetchedAt: new Date() };
}

function deploymentPath(namespace: string, name: string): string {
  return `/api/v1/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;
}

function clusterPath(name: string): string {
  return `/api/v1/clusters/${encodeURIComponent(name)}`;
}

export function registerCluster(name: string, kubeconfigPath: string): Promise<Cluster> {
  return request<Cluster>("POST", "/api/v1/clusters", { name, kubeconfigPath });
}

export function deregisterCluster(name: string): Promise<void> {
  return request<void>("DELETE", clusterPath(name));
}

export function checkClusterHealth(name: string): Promise<Cluster> {
  return request<Cluster>("POST", `${clusterPath(name)}/healthcheck`);
}

export interface PropagateResponse {
  namespace: string;
  name: string;
  results: ClusterResult[];
}

export function propagate(manifest: string, clusters: string[]): Promise<PropagateResponse> {
  return request<PropagateResponse>("POST", "/api/v1/deployments", { manifest, clusters });
}
