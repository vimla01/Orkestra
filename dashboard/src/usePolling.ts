import { useCallback, useEffect, useRef, useState } from "react";

export interface PollState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
}

export interface Poll<T> extends PollState<T> {
  /** Fetch again right away, e.g. after the user changed something. */
  refresh: () => void;
}

/**
 * Calls fetcher immediately and then intervalMs after each call finishes.
 * Waiting for completion (rather than a fixed setInterval) means slow
 * responses, e.g. from a cluster that is timing out, never pile up. The last
 * good data is kept when a later poll fails.
 */
export function usePolling<T>(fetcher: () => Promise<T>, intervalMs: number): Poll<T> {
  const [state, setState] = useState<PollState<T>>({ data: null, error: null, loading: true });
  const refreshRef = useRef<() => void>(() => {});

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let inFlight = false;
    let rerun = false;

    const tick = async () => {
      if (inFlight) {
        // A refresh arrived mid-fetch; fetch once more when this one ends.
        rerun = true;
        return;
      }
      inFlight = true;
      clearTimeout(timer);

      try {
        const data = await fetcher();
        if (!cancelled) setState({ data, error: null, loading: false });
      } catch (err) {
        const error = err instanceof Error ? err.message : String(err);
        if (!cancelled) setState((prev) => ({ ...prev, error, loading: false }));
      }

      inFlight = false;
      if (cancelled) return;
      if (rerun) {
        rerun = false;
        void tick();
        return;
      }
      timer = setTimeout(tick, intervalMs);
    };

    refreshRef.current = () => void tick();
    void tick();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [fetcher, intervalMs]);

  const refresh = useCallback(() => refreshRef.current(), []);
  return { ...state, refresh };
}
