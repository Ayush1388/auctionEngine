// A small single-series line chart for the operator pages, drawn as SVG.
//
// Follows the house chart rules: a 2px line, a ~10% area wash, hairline grid, an end dot with a
// surface-coloured ring, a crosshair that snaps to the nearest point with one tooltip, keyboard
// arrows for the same readout, and a table view (ChartTable) so no value is reachable only by
// hovering.
import { useLayoutEffect, useRef, useState } from "react";
import { niceMax } from "../../app/stats.js";

const L = 46, R = 18, T = 12, B = 22;

export function LineChart({ title, points, format, axisFormat, height = 170, min = 0 }) {
  const root = useRef(null);
  const [W, setW] = useState(600);
  const [cur, setCur] = useState(null);       // index under the pointer or the keyboard
  useLayoutEffect(() => {
    const el = root.current;
    if (!el) return undefined;
    const fit = () => setW(Math.max(280, el.clientWidth || 600));
    fit();
    const ro = new ResizeObserver(fit);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const axis = axisFormat || format, H = height;
  const label = `${title}. ${points.length ? `Latest ${format(points.at(-1).v)}.` : "No data yet."}`;
  if (points.length < 2) {
    return (
      <div className="chart" ref={root}>
        <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} role="img" aria-label={label} tabIndex={0}>
          <text x={W / 2} y={H / 2} textAnchor="middle" className="ch-muted">Collecting data...</text>
        </svg>
      </div>
    );
  }
  const t0 = points[0].t, t1 = points.at(-1).t, vmax = niceMax(Math.max(...points.map(p => p.v), min));
  const x = t => L + (t - t0) / Math.max(1, t1 - t0) * (W - L - R), y = v => T + (1 - v / vmax) * (H - T - B);
  const d = points.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)} ${y(p.v).toFixed(1)}`).join(" ");
  const last = points.at(-1), shown = cur != null ? points[Math.min(cur, points.length - 1)] : null;

  const nearest = e => {
    const r = e.currentTarget.getBoundingClientRect(), t = t0 + ((e.clientX - r.left) / r.width * W - L) / (W - L - R) * (t1 - t0);
    let best = 0, bd = Infinity;
    points.forEach((p, i) => { const dd = Math.abs(p.t - t); if (dd < bd) { bd = dd; best = i; } });
    return best;
  };
  const key = e => {
    const c = cur ?? points.length - 1;
    if (e.key === "ArrowLeft") { e.preventDefault(); setCur(Math.max(0, c - 1)); }
    else if (e.key === "ArrowRight") { e.preventDefault(); setCur(Math.min(points.length - 1, c + 1)); }
  };
  return (
    <div className="chart" ref={root}>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} role="img" tabIndex={0} aria-label={label}
        onPointerMove={e => setCur(nearest(e))} onPointerLeave={() => setCur(null)} onFocus={() => setCur(points.length - 1)} onBlur={() => setCur(null)} onKeyDown={key}>
        {[0, 1, 2, 3, 4].map(i => {
          const v = vmax * i / 4, yy = y(v);
          return <g key={i}><line x1={L} x2={W - R} y1={yy} y2={yy} className="ch-grid" /><text x={L - 8} y={yy + 4} textAnchor="end" className="ch-muted">{axis(v)}</text></g>;
        })}
        <path d={`${d} L${x(t1)} ${y(0)} L${x(t0)} ${y(0)} Z`} className="ch-area" />
        <path d={d} className="ch-line" />
        <circle cx={x(last.t)} cy={y(last.v)} r="4" className="ch-dot" />
        <text x={Math.min(W - R, x(last.t))} y={Math.max(T + 10, y(last.v) - 12)} textAnchor="end" className="ch-end">{format(last.v)}</text>
        {shown && <>
          <line x1={x(shown.t)} x2={x(shown.t)} y1={T} y2={H - B} className="ch-cross" />
          <circle cx={x(shown.t)} cy={y(shown.v)} r="4" className="ch-dot" />
        </>}
      </svg>
      {shown && (
        <div className="ch-tip" style={{ left: Math.min(Math.max(8, x(shown.t) / W * (root.current?.clientWidth || W) - 40), (root.current?.clientWidth || W) - 96), top: Math.max(0, y(shown.v) / H * H - 52) }}>
          <b>{format(shown.v)}</b><span>{new Date(shown.t).toLocaleTimeString("en-US", { hour12: false })}</span>
        </div>
      )}
    </div>
  );
}

/** The same numbers as a table, for anyone who would rather read than hover. */
export function ChartTable({ points, format, unitLabel }) {
  return (
    <table className="tbl">
      <thead><tr><th>Time</th><th>{unitLabel}</th></tr></thead>
      <tbody>{points.slice(-30).reverse().map(p => <tr key={p.t}><td>{new Date(p.t).toLocaleTimeString("en-US", { hour12: false })}</td><td>{format(p.v)}</td></tr>)}</tbody>
    </table>
  );
}
