import type { DeploymentStatus, FailoverEvent } from "../types";
import { formatTime, timeAgo } from "../format";

interface Entry extends FailoverEvent {
  deployment: string;
}

export function FailoverTimeline({ deployments, now }: { deployments: DeploymentStatus[]; now: Date }) {
  const entries: Entry[] = deployments
    .flatMap((d) => (d.failovers ?? []).map((f) => ({ ...f, deployment: `${d.namespace}/${d.name}` })))
    .sort((a, b) => b.at.localeCompare(a.at));

  if (entries.length === 0) {
    return <p className="empty">No failovers. When a cluster stays unhealthy past the grace period, moves show up here.</p>;
  }

  return (
    <ol className="timeline">
      {entries.map((e) => (
        <li key={`${e.deployment}-${e.at}-${e.from}`} className="timeline-item">
          <div className="timeline-when" title={formatTime(e.at)}>
            {timeAgo(e.at, now)}
          </div>
          <div className="timeline-body">
            <div>
              <span className="mono">{e.deployment}</span> moved{" "}
              <span className="mono text-bad">{e.from}</span> → <span className="mono text-ok">{e.to}</span>
            </div>
            <div className="subtle small">{e.reason}</div>
            {e.cleanupError && (
              <div className="small text-warn">Old copy may still be running on {e.from}: cleanup failed</div>
            )}
          </div>
        </li>
      ))}
    </ol>
  );
}
