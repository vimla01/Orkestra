import type { ClusterStatus } from "../types";

/** "name 2/2" chip colored by rollout state on one cluster. */
export function ReplicaChip({ label, status }: { label: string; status: ClusterStatus }) {
  const tone = status.error ? "bad" : status.ready ? "ok" : "warn";
  const value = status.error ? "error" : `${status.readyReplicas}/${status.desiredReplicas}`;
  return (
    <span className={`chip chip-${tone}`} title={status.error ?? `${status.readyReplicas} of ${status.desiredReplicas} replicas ready`}>
      <span className="chip-label">{label}</span>
      <span className="chip-value">{value}</span>
    </span>
  );
}
