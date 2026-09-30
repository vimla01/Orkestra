import { useEffect, useState, type ComponentType } from "react";
import { fetchSnapshot } from "./api";
import { usePolling } from "./usePolling";
import { timeAgo } from "./format";
import { href, useRoute, type Route } from "./router";
import { StatusBadge } from "./components/StatusBadge";
import { Toasts, useToasts } from "./components/Toasts";
import { ClusterIcon, DeploymentIcon, FailoverIcon, OverviewIcon, RefreshIcon } from "./components/Icons";
import { Overview } from "./pages/Overview";
import { Clusters } from "./pages/Clusters";
import { Deployments } from "./pages/Deployments";
import { Failovers } from "./pages/Failovers";

const POLL_INTERVAL_MS = 3000;

const nav: { route: Route; label: string; icon: ComponentType<{ size?: number }> }[] = [
  { route: "overview", label: "Overview", icon: OverviewIcon },
  { route: "clusters", label: "Clusters", icon: ClusterIcon },
  { route: "deployments", label: "Deployments", icon: DeploymentIcon },
  { route: "failovers", label: "Failovers", icon: FailoverIcon },
];

/** Re-renders every second so relative times ("12s ago") stay current. */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

export function App() {
  const { data, error, loading, refresh } = usePolling(fetchSnapshot, POLL_INTERVAL_MS);
  const { toasts, push } = useToasts();
  const route = useRoute();
  const now = useNow();

  const counts: Partial<Record<Route, number>> = data
    ? {
        clusters: data.clusters.length,
        deployments: data.deployments.length,
        failovers: data.deployments.reduce((n, d) => n + (d.failovers?.length ?? 0), 0),
      }
    : {};
  const unhealthy = data?.clusters.filter((c) => c.status === "Unhealthy").length ?? 0;

  return (
    <div className="app">
      <aside className="sidebar">
        <a className="brand" href={href("overview")}>
          Orkestra
        </a>

        <nav className="nav">
          {nav.map(({ route: r, label, icon: Icon }) => (
            <a key={r} href={href(r)} className={`nav-item ${route === r ? "nav-active" : ""}`}>
              <Icon size={18} />
              <span>{label}</span>
              {r === "clusters" && unhealthy > 0 ? (
                <span className="nav-count nav-count-bad">{unhealthy}</span>
              ) : counts[r] !== undefined ? (
                <span className="nav-count">{counts[r]}</span>
              ) : null}
            </a>
          ))}
        </nav>

        <div className="sidebar-foot">
          <div className="muted small">Control plane</div>
          <div className="sidebar-host mono">{window.location.host}</div>
        </div>
      </aside>

      <div className="main">
        <header className="topbar">
          <div className="topbar-status">
            {error ? (
              <StatusBadge tone="bad" label="Disconnected" />
            ) : data ? (
              <StatusBadge tone="ok" label="Live" />
            ) : (
              <StatusBadge tone="muted" label="Connecting…" />
            )}
            {data && <span className="muted small">Updated {timeAgo(data.fetchedAt.toISOString(), now)}</span>}
          </div>
          <button className="btn btn-sm" onClick={refresh}>
            <RefreshIcon size={14} /> Refresh
          </button>
        </header>

        <main className="content">
          {error && (
            <div className="banner" role="alert">
              Can't reach the Orkestra API: {error}.{" "}
              {data ? "Showing the last known state." : "Is `orkestra serve` running?"}
            </div>
          )}

          {loading && !data && <p className="empty">Loading fleet state…</p>}

          {data && route === "overview" && <Overview snapshot={data} now={now} />}
          {data && route === "clusters" && <Clusters snapshot={data} now={now} refresh={refresh} notify={push} />}
          {data && route === "deployments" && <Deployments snapshot={data} now={now} refresh={refresh} />}
          {data && route === "failovers" && <Failovers snapshot={data} now={now} />}
        </main>
      </div>

      <Toasts toasts={toasts} />
    </div>
  );
}
