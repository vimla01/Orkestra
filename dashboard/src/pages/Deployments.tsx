import { Fragment, useState } from "react";
import type { Snapshot } from "../types";
import { formatTime, timeAgo } from "../format";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { ReplicaChip } from "../components/ReplicaChip";
import { ChevronIcon, PlusIcon } from "../components/Icons";
import { DeployDialog } from "../components/DeployDialog";

interface Props {
  snapshot: Snapshot;
  now: Date;
  refresh: () => void;
}

export function Deployments({ snapshot, now, refresh }: Props) {
  const [deploying, setDeploying] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  return (
    <>
      <PageHeader
        title="Deployments"
        subtitle="Workloads propagated from the control plane and their rollout on each cluster."
        actions={
          <button className="btn btn-primary" onClick={() => setDeploying(true)}>
            <PlusIcon size={16} /> New deployment
          </button>
        }
      />

      <section className="panel panel-flush">
        {snapshot.deployments.length === 0 ? (
          <p className="empty">Nothing propagated yet. Create a deployment to push it to your clusters.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th aria-label="Expand" />
                  <th>Name</th>
                  <th>Namespace</th>
                  <th>Placement</th>
                  <th>Status</th>
                  <th>Failovers</th>
                  <th>Propagated</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.deployments.map((d) => {
                  const id = `${d.namespace}/${d.name}`;
                  const isOpen = open === id;
                  return (
                    <Fragment key={id}>
                      <tr className="clickable" onClick={() => setOpen(isOpen ? null : id)} aria-expanded={isOpen}>
                        <td className="expand-cell">
                          <ChevronIcon size={16} className={`chevron ${isOpen ? "chevron-open" : ""}`} />
                        </td>
                        <td className="strong">{d.name}</td>
                        <td className="muted">{d.namespace}</td>
                        <td>
                          <div className="chips">
                            {d.statuses.map((s) => (
                              <ReplicaChip key={s.cluster} label={s.cluster} status={s} />
                            ))}
                          </div>
                        </td>
                        <td>
                          <StatusBadge tone={d.ready ? "ok" : "warn"} label={d.ready ? "Ready" : "Progressing"} />
                        </td>
                        <td>{d.failovers?.length ?? 0}</td>
                        <td className="muted">{timeAgo(d.propagatedAt, now)}</td>
                      </tr>
                      {isOpen && (
                        <tr className="detail-row">
                          <td />
                          <td colSpan={6}>
                            <div className="detail">
                              <table className="table table-inner">
                                <thead>
                                  <tr>
                                    <th>Cluster</th>
                                    <th>Ready</th>
                                    <th>Up-to-date</th>
                                    <th>Available</th>
                                    <th>State</th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {d.statuses.map((s) => (
                                    <tr key={s.cluster}>
                                      <td className="mono">{s.cluster}</td>
                                      <td>
                                        {s.readyReplicas}/{s.desiredReplicas}
                                      </td>
                                      <td>{s.updatedReplicas}</td>
                                      <td>{s.availableReplicas}</td>
                                      <td>
                                        {s.error ? (
                                          <span className="text-bad small">{s.error}</span>
                                        ) : (
                                          <StatusBadge tone={s.ready ? "ok" : "warn"} label={s.ready ? "Ready" : "Progressing"} />
                                        )}
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                              <p className="muted small">
                                Propagated {formatTime(d.propagatedAt)}
                                {d.failovers?.length
                                  ? ` · last failover ${d.failovers[d.failovers.length - 1].from} → ${d.failovers[d.failovers.length - 1].to}`
                                  : ""}
                              </p>
                            </div>
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {deploying && <DeployDialog clusters={snapshot.clusters} onClose={() => setDeploying(false)} onDone={refresh} />}
    </>
  );
}
