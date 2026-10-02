import { useSyncExternalStore } from "react";
import type { AuctionSnapshot } from "../api/types";

/**
 * Latest live snapshot per auction. Messages can arrive out of order, so a
 * snapshot is applied only when its version is higher than the one held.
 */
interface Entry {
  snapshot: AuctionSnapshot;
  receivedAt: number;
}

const entries = new Map<string, Entry>();
const listeners = new Map<string, Set<() => void>>();

export function applySnapshot(snapshot: AuctionSnapshot): void {
  const held = entries.get(snapshot.id);
  if (held && held.snapshot.version >= snapshot.version) return;
  entries.set(snapshot.id, { snapshot, receivedAt: Date.now() });
  listeners.get(snapshot.id)?.forEach((notify) => notify());
}

function subscribe(id: string, notify: () => void): () => void {
  let set = listeners.get(id);
  if (!set) {
    set = new Set();
    listeners.set(id, set);
  }
  set.add(notify);
  return () => {
    set.delete(notify);
    if (set.size === 0) listeners.delete(id);
  };
}

/**
 * The live snapshot for an auction, if one arrived after `since` (the time
 * the REST data was fetched). Older snapshots are ignored so a fresh REST
 * response is never overwritten by something staler.
 */
export function useLiveSnapshot(id: string, since: number): AuctionSnapshot | undefined {
  const entry = useSyncExternalStore(
    (notify) => subscribe(id, notify),
    () => entries.get(id),
  );
  return entry && entry.receivedAt >= since ? entry.snapshot : undefined;
}
