import { useEffect, useState } from "react";

export const routes = ["overview", "clusters", "deployments", "failovers"] as const;
export type Route = (typeof routes)[number];

function parse(hash: string): Route {
  const name = hash.replace(/^#\/?/, "");
  return (routes as readonly string[]).includes(name) ? (name as Route) : "overview";
}

/** Tiny hash router: #/clusters, #/deployments, ... Survives reloads and works with back/forward. */
export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parse(window.location.hash));
  useEffect(() => {
    const onChange = () => setRoute(parse(window.location.hash));
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return route;
}

export function href(route: Route): string {
  return `#/${route}`;
}
