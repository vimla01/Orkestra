import type { Snapshot } from "../types";
import { timeAgo } from "../format";
import { href } from "../router";
import { SummaryTiles } from "../components/SummaryTiles";
import { PageHeader } from "../components/PageHeader";
import { ReplicaChip } from "../components/ReplicaChip";
import { StatusBadge, healthTone } from "../components/StatusBadge";

interface Activity {
  at: string;
  kind: "deploy" | "failover";
  title: string;
  detail: string;
}

function recentActivity(snapshot: Snapshot, limit: number): Activity[] {
  const items: Activity[] = [];
  for (const d of snapshot.deployments) {
    const id = `${d.namespace}/${d.name}`;
    items.push({
      at: d.propagatedAt,
      kind: "deploy",
      title: `${id} propagated`,
      detail: `to ${d.results.map((r) => r.cluster).join(", ")}`,
    });
    for (const f of d.failovers ?? []) {
      items.push({ at: f.at, kind: "failover", title: `${id} failed over`, detail: `${f.from} → ${f.to}` });
    }
  }
  return items.sort((a, b) => b.at.localeCompare(a.at)).slice(0, limit);
}

export function Overview({ snapshot, now }: { snapshot: Snapshot; now: Date }) {
  const { clusters, deployments } = snapshot;
  const activity = recentActivity(snapshot, 8);

  return (
    <>
      <PageHeader title="Overview" subtitle="Fleet health and where your workloads run, updated live." />
      <SummaryTiles snapshot={snapshot} />

      <section className="panel">
        <div className="panel-head">
          <h2>Fleet map</h2>
          <a className="link" href={href("clusters")}>
            Manage clusters
          </a>
        </div>
        {clusters.length === 0 ? (
          <p className="empty">No clusters registered yet. Add one from the Clusters page.</p>
        ) : (
          <div className="fleet">
            {clusters.map((c) => {
              const workloads = deployments.flatMap((d) => {
                const s = d.statuses.find((x) => x.cluster === c.name);
                return s ? [{ id: `${d.namespace}/${d.name}`, name: d.name, status: s }] : [];
              });
              return (
                <div key={c.name} className={`lane lane-${c.status.toLowerCase()}`}>
                  <div className="lane-head">
                    <span className="lane-name">{c.name}</span>
                    <StatusBadge tone={healthTone(c.status)} label={c.status} />
                  </div>
                  <div className="lane-meta">
                    {c.readyNodes}/{c.nodeCount} nodes ready · checked {timeAgo(c.lastHealthCheck, now)}
                  </div>
                  <div className="lane-body">
                    {workloads.length === 0 ? (
                      <span className="lane-empty">No workloads</span>
                    ) : (
                      workloads.map((w) => <ReplicaChip key={w.id} label={w.name} status={w.status} />)
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <div className="split">
        <section className="panel">
          <div className="panel-head">
            <h2>Deployment health</h2>
            <a className="link" href={href("deployments")}>
              View all
            </a>
          </div>
          {deployments.length === 0 ? (
            <p className="empty">Nothing propagated yet.</p>
          ) : (
            <ul className="health-list">
              {deployments.map((d) => {
                const desired = d.statuses.reduce((n, s) => n + s.desiredReplicas, 0);
                const ready = d.statuses.reduce((n, s) => n + s.readyReplicas, 0);
                const pct = desired > 0 ? Math.round((ready / desired) * 100) : 0;
                return (
                  <li key={`${d.namespace}/${d.name}`}>
                    <div className="health-row">
                      <span>
                        <span className="strong">{d.name}</span> <span className="muted">· {d.namespace}</span>
                      </span>
                      <span className="muted small">
                        {ready}/{desired} replicas on {d.clusters.length} cluster{d.clusters.length === 1 ? "" : "s"}
                      </span>
                    </div>
                    <div className="bar">
                      <div className={`bar-fill ${d.ready ? "" : "bar-warn"}`} style={{ width: `${pct}%` }} />
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        <section className="panel">
          <div className="panel-head">
            <h2>Recent activity</h2>
            <a className="link" href={href("failovers")}>
              Failover history
            </a>
          </div>
          {activity.length === 0 ? (
            <p className="empty">No activity yet.</p>
          ) : (
            <ul className="activity">
              {activity.map((a, i) => (
                <li key={`${a.at}-${i}`}>
                  <span className={`activity-dot activity-${a.kind}`} aria-hidden="true" />
                  <div className="activity-text">
                    <div>{a.title}</div>
                    <div className="muted small">{a.detail}</div>
                  </div>
                  <span className="muted small nowrap">{timeAgo(a.at, now)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </>
  );
}
