// The live system view shared by the tour, the architecture map and the chaos lab: health from the
// probes, metrics and fault state for operators, and a pulse for every real event.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useBackend } from "./backend.jsx";
import { useInterval } from "./use.js";
import { PATHS } from "../lib/archmap.js";

const POLL = 2000;
export const fam = (m, name) => m?.families.find(f => f.name === name);
export const sum = f => (f?.samples || []).reduce((a, s) => a + (s.value ?? 0), 0);

/**
 * @param {{ watch?: number }} o  watch: how many open lots to follow for pulses (0 = none)
 * @returns { map (ref to the ArchMap), state, health, bid(result), search(backend, ms), poll() }
 */
export function useSystem({ watch = 10 } = {}) {
  const be = useBackend();
  const map = useRef(null);
  const events = useRef(0);
  const [state, setState] = useState({ ready: null, ms: null, metrics: null, chaos: null, lastBid: null, lastSearch: null, events: 0, err: null });

  const poll = useCallback(async () => {
    if (document.hidden) return;
    const next = {};
    try { next.ms = (await be.livez()).ms; } catch { next.ms = null; }
    try { next.ready = await be.readyz(); next.err = null; } catch (e) { next.ready = e.body?.checks ? e.body : null; next.err = e.message; }
    if (be.session.isAdmin) {
      try { next.metrics = await be.metrics(); } catch { /* shown as unknown */ }
      try { next.chaos = await be.chaos.get(); } catch { next.chaos = null; }
    } else { next.metrics = null; next.chaos = null; }
    setState(s => ({ ...s, ...next, events: events.current }));
  }, [be]);

  useEffect(() => { poll(); }, [poll]);
  useInterval(poll, POLL);

  // follow the open lots, so every bid anyone places makes a pulse
  useEffect(() => {
    if (!watch) return undefined;
    let off = [], gone = false;
    (async () => {
      let list = [];
      try { list = (await be.allAuctions({ status: "ACTIVE" }, 1)).slice(0, watch); } catch { return; }
      if (gone) return;
      off = list.map(a => be.feed.subscribe(a.id, ev => {
        events.current++;
        if (ev.bid) { map.current?.pulse(PATHS.bid.slice(1), { hop: 90 }); setTimeout(() => map.current?.pulse(PATHS.fanout, { hop: 110 }), 320); }
        else map.current?.pulse(PATHS.fanout, { hop: 110 });
      }));
    })();
    return () => { gone = true; off.forEach(f => f()); };
  }, [be, watch]);

  const health = useMemo(() => {
    const c = state.ready?.checks || {}, f = Object.fromEntries((state.chaos?.faults || []).map(x => [x.name, x.active]));
    const br = name => state.metrics ? fam(state.metrics, "auction_circuit_breaker_state")?.samples.find(x => x.labels?.name === name)?.value ?? null : null;
    const fromCheck = k => (!(k in c) ? "off" : c[k] === "ok" ? "ok" : "bad");
    const pend = sum(fam(state.metrics, "auction_outbox_pending")), dead = sum(fam(state.metrics, "auction_outbox_dead_lettered"));
    const bidB = br("bidding");
    return {
      browser: "ok",
      gateway: state.ms == null ? "bad" : "ok",
      bidding: f.bidding || bidB === 2 ? "bad" : bidB === 1 ? "warn" : "ok",
      postgres: c.postgres && c.postgres !== "ok" ? "bad" : f.postgres_slow ? "warn" : "ok",
      outbox: dead > 0 ? "warn" : pend > 50 ? "warn" : "ok",
      kafka: fromCheck("kafka"),
      redis: fromCheck("redis"),
      elasticsearch: br("elasticsearch") === 2 ? "bad" : fromCheck("elasticsearch"),
      websocket: be.feed.state === "open" || be.feed.state === "idle" ? "ok" : "warn",
    };
  }, [state, be]);

  /** A bid this page placed: pulse its real path, hop time scaled to the measured call. */
  const bid = useCallback(r => {
    setState(s => ({ ...s, lastBid: { status: r.status, timing: r.serverTiming, ms: r.ms } }));
    map.current?.pulse(PATHS.bid, { hop: Math.max(70, Math.min(260, (r.ms || 300) / 4)), tone: r.ok ? "ok" : "bad" });
  }, []);
  const search = useCallback((backend, ms) => {
    setState(s => ({ ...s, lastSearch: { backend, ms } }));
    map.current?.pulse(backend === "postgres" ? PATHS.fallback : PATHS.search, { hop: 110 });
  }, []);

  return { map, state, health, bid, search, poll, be };
}
