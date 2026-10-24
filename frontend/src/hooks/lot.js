// One lot, live: wraps the framework-free LotSession controller (app/bidding.js) for React.
import { useEffect, useReducer, useRef, useState } from "react";
import { LotSession } from "../../app/bidding.js";

const EVENTS = ["lot", "history", "wallet", "attempt", "outcome", "event", "extended", "outbid"];

/**
 * @returns { s: LotSession | null, error }  s is re-read on every change, at most once per frame:
 * a Stress it run sends hundreds of updates a second and the page should paint once per frame.
 */
export function useLotSession(be, id) {
  const [s, setS] = useState(null);
  const [error, setError] = useState(null);
  const [, bump] = useReducer(x => x + 1, 0);
  const raf = useRef(0);

  useEffect(() => {
    if (!id) return undefined;
    const session = new LotSession(be, id);
    let live = true;
    const schedule = () => { if (!raf.current) raf.current = requestAnimationFrame(() => { raf.current = 0; bump(); }); };
    const offs = EVENTS.map(ev => session.on(ev, schedule));
    setS(null); setError(null);
    session.load().then(() => { if (live) setS(session); }).catch(e => { if (live) setError(e); });
    return () => { live = false; offs.forEach(off => off()); cancelAnimationFrame(raf.current); raf.current = 0; session.destroy(); };
  }, [be, id]);

  return { s, error };
}
