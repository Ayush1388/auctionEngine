import { useSyncExternalStore } from "react";

/**
 * One shared clock for every countdown on the page (plan.md 11). Components
 * read "now" rounded to their own granularity, so a card with days left
 * re-renders once a minute while a lot in its final minutes ticks every
 * second. The interval stops while the tab is hidden.
 */
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | null = null;

function tick() {
  listeners.forEach((listener) => listener());
}

function start() {
  if (timer !== null || document.hidden) return;
  // Align to the wall-clock second so all countdowns change together.
  const offset = 1000 - (Date.now() % 1000);
  timer = setTimeout(() => {
    tick();
    timer = setInterval(tick, 1000);
  }, offset) as unknown as ReturnType<typeof setInterval>;
}

function stop() {
  if (timer !== null) {
    clearTimeout(timer as unknown as ReturnType<typeof setTimeout>);
    clearInterval(timer);
    timer = null;
  }
}

function onVisibility() {
  if (document.hidden) stop();
  else if (listeners.size > 0) {
    tick(); // recalculate immediately when the tab becomes visible again
    start();
  }
}

function subscribe(listener: () => void): () => void {
  if (listeners.size === 0) document.addEventListener("visibilitychange", onVisibility);
  listeners.add(listener);
  start();
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    }
  };
}

/** Current time in ms, rounded down to `granularity` so renders are sparse. */
export function useNow(granularity: 1000 | 60_000 = 1000): number {
  return useSyncExternalStore(subscribe, () => Math.floor(Date.now() / granularity) * granularity);
}
