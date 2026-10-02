import { useSyncExternalStore } from "react";

/**
 * Device-local watchlist (plan.md 4.8): the backend has no watchlist API,
 * so watched auction ids live in localStorage.
 */
const KEY = "watchlist.v1";
const listeners = new Set<() => void>();
let cache: ReadonlySet<string> | null = null;

function read(): ReadonlySet<string> {
  if (cache) return cache;
  try {
    const raw = localStorage.getItem(KEY);
    const ids: unknown = raw ? JSON.parse(raw) : [];
    cache = new Set(Array.isArray(ids) ? ids.filter((id): id is string => typeof id === "string") : []);
  } catch {
    cache = new Set();
  }
  return cache;
}

export function toggleWatched(id: string): void {
  const next = new Set(read());
  if (!next.delete(id)) next.add(id);
  cache = next;
  try {
    localStorage.setItem(KEY, JSON.stringify([...next]));
  } catch {
    // Storage blocked or full: the choice still holds for this visit.
  }
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void): () => void {
  const onStorage = (event: StorageEvent) => {
    if (event.key === KEY) {
      cache = null;
      listener();
    }
  };
  listeners.add(listener);
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
}

export function useIsWatched(id: string): boolean {
  return useSyncExternalStore(subscribe, () => read().has(id));
}
