// Small hooks the pages share.
import { useCallback, useEffect, useRef, useState } from "react";

/** Run fn every `ms` while mounted (the latest fn is always the one called). */
export function useInterval(fn, ms) {
  const ref = useRef(fn);
  ref.current = fn;
  useEffect(() => {
    if (ms == null) return undefined;
    const t = setInterval(() => ref.current(), ms);
    return () => clearInterval(t);
  }, [ms]);
}

/** Date.now(), refreshed every `ms`, for clocks. */
export function useNow(ms = 1000) {
  const [now, setNow] = useState(Date.now());
  useInterval(() => setNow(Date.now()), ms);
  return now;
}

/** Re-render when an Emitter event fires, returning get() at that time. */
export function useEmitter(emitter, event, get) {
  const [value, setValue] = useState(get);
  useEffect(() => emitter.on(event, (...a) => setValue(get(...a))), [emitter, event]);   // eslint-disable-line react-hooks/exhaustive-deps
  return value;
}

export function useDocumentTitle(title) {
  useEffect(() => { if (title) document.title = title; }, [title]);
}

/** A boolean that closes on Escape and on any click outside `ref`. */
export function useDismiss(ref, open, close) {
  useEffect(() => {
    if (!open) return undefined;
    const click = e => { if (ref.current && !ref.current.contains(e.target)) close(); };
    const key = e => { if (e.key === "Escape") close(); };
    document.addEventListener("click", click);
    document.addEventListener("keydown", key);
    return () => { document.removeEventListener("click", click); document.removeEventListener("keydown", key); };
  }, [open, close, ref]);
}

/** useState for a value that comes from an async call: { data, error, loading, reload }. */
export function useAsync(fn, deps) {
  const [state, setState] = useState({ data: null, error: null, loading: true });
  const run = useCallback(() => {
    let live = true;
    setState(s => ({ ...s, loading: true }));
    Promise.resolve().then(fn).then(
      data => { if (live) setState({ data, error: null, loading: false }); },
      error => { if (live) setState({ data: null, error, loading: false }); });
    return () => { live = false; };
  }, deps);   // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => run(), [run]);
  return { ...state, reload: run };
}

export const prefersReducedMotion = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
