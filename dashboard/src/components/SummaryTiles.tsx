import type { Snapshot } from "../types";

export function SummaryTiles({ snapshot }: { snapshot: Snapshot }) {
  const { clusters, deployments } = snapshot;
  const healthy = clusters.filter((c) => c.status === "Healthy").length;
  const unhealthy = clusters.filter((c) => c.status === "Unhealthy").length;
  const unchecked = clusters.length - healthy - unhealthy;
  const readyNodes = clusters.reduce((n, c) => n + c.readyNodes, 0);
  const totalNodes = clusters.reduce((n, c) => n + c.nodeCount, 0);
  const readyDeployments = deployments.filter((d) => d.ready).length;
  const failovers = deployments.reduce((n, d) => n + (d.failovers?.length ?? 0), 0);

  const tiles = [
    {
      label: "Healthy clusters",
      value: `${healthy}/${clusters.length}`,
      note:
        unhealthy > 0
          ? `${unhealthy} unhealthy`
          : unchecked > 0
            ? `${unchecked} not yet checked`
            : clusters.length > 0
              ? "all reachable"
              : "none registered",
      tone: unhealthy > 0 ? "bad" : unchecked > 0 || clusters.length === 0 ? "muted" : "ok",
    },
    {
      label: "Ready nodes",
      value: `${readyNodes}/${totalNodes}`,
      note: "across the fleet",
      tone: totalNodes === 0 ? "muted" : readyNodes < totalNodes ? "warn" : "ok",
    },
    {
      label: "Deployments ready",
      value: `${readyDeployments}/${deployments.length}`,
      note:
        deployments.length === 0
          ? "nothing propagated"
          : readyDeployments < deployments.length
            ? "rolling out or degraded"
            : "fully rolled out",
      tone: deployments.length === 0 ? "muted" : readyDeployments < deployments.length ? "warn" : "ok",
    },
    {
      label: "Failovers",
      value: String(failovers),
      note: failovers > 0 ? "workloads moved" : "none so far",
      tone: failovers > 0 ? "warn" : "muted",
    },
  ] as const;

  return (
    <div className="tiles">
      {tiles.map((t) => (
        <div key={t.label} className={`tile tile-${t.tone}`}>
          <div className="tile-label">{t.label}</div>
          <div className="tile-value">{t.value}</div>
          <div className="tile-note">{t.note}</div>
        </div>
      ))}
    </div>
  );
}
