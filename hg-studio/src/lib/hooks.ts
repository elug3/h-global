import { useCallback, useEffect, useRef, useState } from "react";
import { errorText } from "../api/client";

/** Loads data with fn and reloads on demand. Keeps old data while reloading. */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const fnRef = useRef(fn);
  fnRef.current = fn;

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setData(await fnRef.current());
      setError(undefined);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, error, loading, reload, setData };
}

/** Event types of a show's stream (/studio-api/shows/{id}/events). */
export const LIVE_EVENTS = [
  "live.show_started",
  "live.show_ended",
  "live.show_updated",
  "live.item_published",
  "live.item_updated",
  "live.item_pinned",
  "live.item_unpinned",
];

/** Event types of /studio-api/orders/events. */
export const ORDER_EVENTS = ["order.created", "order.updated"];

/**
 * Opens a Server-Sent Events stream and calls onEvent for each event of the
 * given types. With `from`, the stream starts after that event id (from a
 * snapshot); with "now", it starts with the next event. The browser
 * reconnects by itself with Last-Event-ID; `reset` (history gone) calls
 * onReset so the caller reloads its snapshot.
 */
export function useEventStream(
  url: string | undefined,
  from: number | "now" | undefined,
  types: string[],
  onEvent: (type: string, data: unknown) => void,
  onReset: () => void,
) {
  const handlers = useRef({ onEvent, onReset });
  handlers.current = { onEvent, onReset };
  const typeKey = types.join(",");

  useEffect(() => {
    if (!url || from === undefined) return;
    const es = new EventSource(from === "now" ? url : `${url}?last_event_id=${from}`, { withCredentials: true });
    const listener = (ev: MessageEvent) => {
      let data: unknown = undefined;
      try {
        data = JSON.parse(ev.data);
      } catch {
        /* ignore */
      }
      handlers.current.onEvent(ev.type, data);
    };
    for (const ty of typeKey.split(",")) es.addEventListener(ty, listener);
    es.addEventListener("reset", () => handlers.current.onReset());
    return () => es.close();
    // Reconnecting from a new id only when the stream itself changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, typeKey]);
}

/** Merges calls that arrive within ms into one (for reloads after events). */
export function useDebounced(fn: () => void, ms = 300): () => void {
  const ref = useRef(fn);
  ref.current = fn;
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  return useCallback(() => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => ref.current(), ms);
  }, [ms]);
}

/** Calls fn every ms while the component is mounted. */
export function useInterval(fn: () => void, ms: number) {
  const ref = useRef(fn);
  ref.current = fn;
  useEffect(() => {
    const id = setInterval(() => ref.current(), ms);
    return () => clearInterval(id);
  }, [ms]);
}

/** Current time, updated every second. */
export function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useInterval(() => setNow(Date.now()), 1000);
  return now;
}
