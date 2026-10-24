// The architecture map: the services a bid touches, drawn as a diagram whose pulses are real
// events. A pulse is drawn when something actually happened (a bid committed, a search was
// answered, an event arrived over the WebSocket), never on a timer. The boxes and lines are
// React; the travelling dots are moved by requestAnimationFrame on the SVG nodes directly,
// because 60 frames a second is not something to ask React state for.
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from "react";
import { EDGES, NODES, NH, NW, H, W, edgeKey, ends } from "../lib/archmap.js";

const BADGE = { ok: "ok", warn: "degraded", bad: "down", off: "off" };
const reduce = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
const NS = "http://www.w3.org/2000/svg";

/**
 * props: health { nodeId: "ok"|"warn"|"bad"|"off" }, selected, onSelect(id), lit [nodeIds]
 * ref:   { pulse(path, { hop, tone }) }
 */
const ArchMap = forwardRef(function ArchMap({ health = {}, selected, onSelect, lit = [] }, ref) {
  const dotLayer = useRef(null);
  const edgeRefs = useRef(new Map());
  const dots = useRef([]);
  const raf = useRef(0);

  const step = useCallback(() => {
    raf.current = 0;
    const now = performance.now();   // not the rAF timestamp: it can be earlier than a t0 taken a moment ago
    const list = dots.current;
    for (let i = list.length - 1; i >= 0; i--) {
      const d = list[i], k = Math.max(0, (now - d.t0) / d.hop), idx = Math.floor(k);
      if (idx >= d.segs.length) { d.dot.remove(); list.splice(i, 1); continue; }
      const s = d.segs[idx], f = k - idx;
      d.dot.setAttribute("cx", s.p.x + (s.q.x - s.p.x) * f);
      d.dot.setAttribute("cy", s.p.y + (s.q.y - s.p.y) * f);
      s.edge?.classList.add("is-hot");
      if (f > 0.9) setTimeout(() => s.edge?.classList.remove("is-hot"), 250);
    }
    if (list.length) raf.current = requestAnimationFrame(step);
  }, []);

  useEffect(() => () => cancelAnimationFrame(raf.current), []);

  useImperativeHandle(ref, () => ({
    pulse(path, { hop = 140, tone = "ok" } = {}) {
      const segs = [];
      for (let i = 0; i < path.length - 1; i++) {
        const edge = edgeRefs.current.get(edgeKey(path[i], path[i + 1]));
        if (!edge) continue;
        const [p, q] = ends(path[i], path[i + 1]);
        segs.push({ p, q, edge });
      }
      if (!segs.length) return;
      if (reduce()) {                       // no travelling dot: light each line in turn
        segs.forEach((s, i) => setTimeout(() => { s.edge.classList.add("is-hot"); setTimeout(() => s.edge.classList.remove("is-hot"), 380); }, i * 60));
        return;
      }
      const dot = document.createElementNS(NS, "circle");
      dot.setAttribute("r", "5"); dot.setAttribute("class", `amap-dot tone-${tone}`);
      dotLayer.current.append(dot);
      dots.current.push({ dot, segs, t0: performance.now(), hop });
      if (!raf.current) raf.current = requestAnimationFrame(step);
    },
  }), [step]);

  const litSet = new Set(lit);
  const broken = (a, b) => health[a] === "bad" || health[b] === "bad";
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className={"amap" + (litSet.size ? " has-lit" : "")} role="group" aria-label="Architecture map. Each box is a service; select one to see its health.">
      {EDGES.map(([a, b]) => {
        const [p, q] = ends(a, b), k = edgeKey(a, b);
        const cls = "amap-edge" + (broken(a, b) ? " is-broken" : "") + (litSet.has(a) && litSet.has(b) ? " is-lit" : "");
        return <line key={k} ref={el => { if (el) edgeRefs.current.set(k, el); }} x1={p.x} y1={p.y} x2={q.x} y2={q.y} className={cls} />;
      })}
      <g className="amap-dots" ref={dotLayer} />
      {NODES.map(n => (
        <g key={n.id} className={"amap-node" + (selected === n.id ? " is-selected" : "") + (litSet.has(n.id) ? " is-lit" : "")} tabIndex={0} role="button"
          data-id={n.id} data-state={health[n.id]} aria-label={`${n.label}: ${n.sub}`} transform={`translate(${n.x} ${n.y})`}
          onClick={() => onSelect?.(n.id)} onKeyDown={e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect?.(n.id); } }}>
          <rect width={NW} height={NH} rx="12" />
          <text x="14" y="26" className="amap-title">{n.label}</text>
          <text x="14" y="45" className="amap-sub">{n.sub}</text>
          <text x="14" y="63" className="amap-badge">{BADGE[health[n.id]] || ""}</text>
        </g>
      ))}
    </svg>
  );
});

export default ArchMap;
