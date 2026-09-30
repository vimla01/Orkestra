import type { Snapshot } from "../types";
import { PageHeader } from "../components/PageHeader";
import { FailoverTimeline } from "../components/FailoverTimeline";

export function Failovers({ snapshot, now }: { snapshot: Snapshot; now: Date }) {
  return (
    <>
      <PageHeader
        title="Failovers"
        subtitle="Every time Orkestra moved a workload off a cluster that stayed unhealthy past the grace period."
      />
      <section className="panel panel-flush">
        <FailoverTimeline deployments={snapshot.deployments} now={now} />
      </section>
    </>
  );
}
