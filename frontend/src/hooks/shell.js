// Hooks over the shared stores in lib/shell.js and the demo bots.
import { useCallback, useEffect, useState } from "react";
import { useBackend } from "./backend.jsx";
import { pulse, lastPulse } from "../lib/shell.js";

/** The latest system-pulse reading (or null before the first). */
export function usePulse() {
  const [p, setP] = useState(lastPulse);
  useEffect(() => { setP(lastPulse); return pulse.on("tick", setP); }, []);
  return p;
}

/** The demo bots' state, kept fresh on every pulse, with the switch. */
export function useBots(enabled = true) {
  const be = useBackend();
  const [state, setState] = useState(null);
  const refresh = useCallback(async () => {
    if (!be.bots) return;
    try { setState(await be.bots.get()); } catch { setState(null); }
  }, [be]);
  useEffect(() => { if (!enabled) return undefined; refresh(); return pulse.on("tick", refresh); }, [enabled, refresh]);
  const setAmbient = async on => { const s = await be.bots.setAmbient(on); setState(s); return s; };
  return { state, refresh, setAmbient };
}
