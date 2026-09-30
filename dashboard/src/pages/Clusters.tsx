import { useState } from "react";
import type { Snapshot } from "../types";
import { timeAgo, formatTime } from "../format";
import { checkClusterHealth, deregisterCluster } from "../api";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge, healthTone } from "../components/StatusBadge";
import { PlusIcon, RefreshIcon, TrashIcon } from "../components/Icons";
import { RegisterClusterDialog } from "../components/RegisterClusterDialog";
import { Modal } from "../components/Modal";

interface Props {
  snapshot: Snapshot;
  now: Date;
  refresh: () => void;
  notify: (tone: "ok" | "bad", message: string) => void;
}

export function Clusters({ snapshot, now, refresh, notify }: Props) {
  const [registering, setRegistering] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const workloadCount = (cluster: string) =>
    snapshot.deployments.filter((d) => d.clusters.includes(cluster)).length;

  const runCheck = async (name: string) => {
    setBusy(name);
    try {
      const c = await checkClusterHealth(name);
      notify(c.status === "Healthy" ? "ok" : "bad", `${name} is ${c.status}`);
      refresh();
    } catch (err) {
      notify("bad", err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const remove = async (name: string) => {
    try {
      await deregisterCluster(name);
      notify("ok", `Cluster ${name} removed`);
      refresh();
    } catch (err) {
      notify("bad", err instanceof Error ? err.message : String(err));
    } finally {
      setRemoving(null);
    }
  };

  return (
    <>
      <PageHeader
        title="Clusters"
        subtitle="Member clusters under this control plane and their health."
        actions={
          <button className="btn btn-primary" onClick={() => setRegistering(true)}>
            <PlusIcon size={16} /> Register cluster
          </button>
        }
      />

      <section className="panel panel-flush">
        {snapshot.clusters.length === 0 ? (
          <p className="empty">No clusters registered yet.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Nodes</th>
                  <th>Workloads</th>
                  <th>Endpoint</th>
                  <th>Last check</th>
                  <th>Registered</th>
                  <th aria-label="Actions" />
                </tr>
              </thead>
              <tbody>
                {snapshot.clusters.map((c) => (
                  <tr key={c.name}>
                    <td className="strong">{c.name}</td>
                    <td>
                      <StatusBadge tone={healthTone(c.status)} label={c.status} />
                      {c.status === "Unhealthy" && (
                        <div className="small text-bad">down {timeAgo(c.unhealthySince, now).replace(" ago", "")}</div>
                      )}
                    </td>
                    <td>
                      <div className="nodes">
                        <div className="bar bar-sm">
                          <div
                            className={`bar-fill ${c.status === "Unhealthy" ? "bar-bad" : ""}`}
                            style={{ width: c.nodeCount ? `${(c.readyNodes / c.nodeCount) * 100}%` : "0%" }}
                          />
                        </div>
                        <span>
                          {c.readyNodes}/{c.nodeCount}
                        </span>
                      </div>
                    </td>
                    <td>{workloadCount(c.name)}</td>
                    <td className="mono muted">{c.endpoint}</td>
                    <td className="muted">{timeAgo(c.lastHealthCheck, now)}</td>
                    <td className="muted">{formatTime(c.registeredAt)}</td>
                    <td>
                      <div className="row-actions">
                        <button className="btn btn-sm" onClick={() => runCheck(c.name)} disabled={busy === c.name}>
                          <RefreshIcon size={14} className={busy === c.name ? "spin" : ""} /> Check
                        </button>
                        <button className="icon-btn icon-btn-danger" onClick={() => setRemoving(c.name)} aria-label={`Remove ${c.name}`}>
                          <TrashIcon size={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {registering && (
        <RegisterClusterDialog
          onClose={() => setRegistering(false)}
          onDone={(message) => {
            setRegistering(false);
            notify("ok", message);
            refresh();
          }}
        />
      )}

      {removing && (
        <Modal
          title={`Remove ${removing}?`}
          onClose={() => setRemoving(null)}
          footer={
            <>
              <button className="btn" onClick={() => setRemoving(null)}>
                Cancel
              </button>
              <button className="btn btn-danger" onClick={() => remove(removing)}>
                Remove cluster
              </button>
            </>
          }
        >
          <p>
            Orkestra will stop monitoring <span className="mono">{removing}</span> and won't use it for failover. Workloads
            already running on the cluster are left untouched.
          </p>
          {workloadCount(removing) > 0 && (
            <p className="text-warn">
              {workloadCount(removing)} deployment{workloadCount(removing) === 1 ? " targets" : "s target"} this cluster;
              their status will show an error until they are redeployed elsewhere.
            </p>
          )}
        </Modal>
      )}
    </>
  );
}
