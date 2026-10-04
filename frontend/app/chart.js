// A small single-series line chart for the operator pages, drawn as SVG.
//
// Follows the house chart rules: a 2px line, a ~10% area wash, hairline grid,
// an end dot with a surface-coloured ring, a crosshair that snaps to the nearest
// point with one tooltip, keyboard arrows for the same readout, and a table
// view so no value is reachable only by hovering. Labels use textContent.
const NS = "http://www.w3.org/2000/svg";
const el = (tag, attrs = {}, parent) => { const n = document.createElementNS(NS, tag); for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v); parent?.append(n); return n; };

/** A tidy upper bound for an axis: 1, 2, 5 times a power of ten. */
export function niceMax(v) {
  if (!(v > 0)) return 1;
  const p = 10 ** Math.floor(Math.log10(v)), n = v / p;
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}

/**
 * @param {HTMLElement} root
 * @param {{ title: string, points: {t:number, v:number}[], format: (n:number)=>string, axisFormat?: (n:number)=>string, height?: number, min?: number }} o
 */
export function lineChart(root, o) {
  const { points, format } = o, axis = o.axisFormat || format;
  const W = Math.max(280, root.clientWidth || 600), H = o.height || 170, L = 46, R = 18, T = 12, B = 22;
  root.textContent = "";
  root.classList.add("chart");

  const svg = el("svg", { viewBox: `0 0 ${W} ${H}`, width: "100%", height: H, role: "img", tabindex: "0", "aria-label": `${o.title}. ${points.length ? `Latest ${format(points.at(-1).v)}.` : "No data yet."}` });
  root.append(svg);
  if (points.length < 2) { const t = el("text", { x: W / 2, y: H / 2, "text-anchor": "middle", class: "ch-muted" }, svg); t.textContent = "Collecting data..."; return; }

  const t0 = points[0].t, t1 = points.at(-1).t, vmax = niceMax(Math.max(...points.map(p => p.v), o.min ?? 0));
  const x = t => L + (t - t0) / Math.max(1, t1 - t0) * (W - L - R), y = v => T + (1 - v / vmax) * (H - T - B);

  for (let i = 0; i <= 4; i++) {                                    // hairline grid and round-number ticks
    const v = vmax * i / 4, yy = y(v);
    el("line", { x1: L, x2: W - R, y1: yy, y2: yy, class: "ch-grid" }, svg);
    const tx = el("text", { x: L - 8, y: yy + 4, "text-anchor": "end", class: "ch-muted" }, svg); tx.textContent = axis(v);
  }
  const d = points.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)} ${y(p.v).toFixed(1)}`).join(" ");
  el("path", { d: `${d} L${x(t1)} ${y(0)} L${x(t0)} ${y(0)} Z`, class: "ch-area" }, svg);
  el("path", { d, class: "ch-line" }, svg);
  const last = points.at(-1);
  el("circle", { cx: x(last.t), cy: y(last.v), r: 4, class: "ch-dot" }, svg);
  const lab = el("text", { x: Math.min(W - R, x(last.t)), y: Math.max(T + 10, y(last.v) - 12), "text-anchor": "end", class: "ch-end" }, svg); lab.textContent = format(last.v);

  // hover and keyboard readout
  const cross = el("line", { y1: T, y2: H - B, class: "ch-cross", visibility: "hidden" }, svg);
  const mark = el("circle", { r: 4, class: "ch-dot", visibility: "hidden" }, svg);
  const tip = document.createElement("div"); tip.className = "ch-tip"; tip.hidden = true; root.append(tip);
  const show = i => {
    const p = points[i], px = x(p.t);
    cross.setAttribute("x1", px); cross.setAttribute("x2", px); cross.setAttribute("visibility", "visible");
    mark.setAttribute("cx", px); mark.setAttribute("cy", y(p.v)); mark.setAttribute("visibility", "visible");
    tip.textContent = "";
    const v = document.createElement("b"); v.textContent = format(p.v);
    const s = document.createElement("span"); s.textContent = new Date(p.t).toLocaleTimeString("en-US", { hour12: false });
    tip.append(v, s); tip.hidden = false;
    const r = svg.getBoundingClientRect(), left = px / W * r.width;
    tip.style.left = Math.min(Math.max(8, left - tip.offsetWidth / 2), r.width - tip.offsetWidth - 8) + "px";
    tip.style.top = Math.max(0, y(p.v) / H * r.height - 52) + "px";
    cur = i;
  };
  const hide = () => { cross.setAttribute("visibility", "hidden"); mark.setAttribute("visibility", "hidden"); tip.hidden = true; };
  let cur = points.length - 1;
  svg.addEventListener("pointermove", e => {
    const r = svg.getBoundingClientRect(), t = t0 + ((e.clientX - r.left) / r.width * W - L) / (W - L - R) * (t1 - t0);
    let best = 0, bd = Infinity; points.forEach((p, i) => { const dd = Math.abs(p.t - t); if (dd < bd) { bd = dd; best = i; } });
    show(best);
  });
  svg.addEventListener("pointerleave", hide);
  svg.addEventListener("focus", () => show(cur));
  svg.addEventListener("blur", hide);
  svg.addEventListener("keydown", e => {
    if (e.key === "ArrowLeft") { e.preventDefault(); show(Math.max(0, cur - 1)); }
    else if (e.key === "ArrowRight") { e.preventDefault(); show(Math.min(points.length - 1, cur + 1)); }
  });
}

/** The same numbers as a table, for anyone who would rather read than hover. */
export function chartTable(points, format, unitLabel) {
  const t = document.createElement("table"); t.className = "tbl";
  const head = t.createTHead().insertRow();
  for (const h of ["Time", unitLabel]) { const th = document.createElement("th"); th.textContent = h; head.append(th); }
  const body = t.createTBody();
  for (const p of points.slice(-30).reverse()) {
    const r = body.insertRow();
    r.insertCell().textContent = new Date(p.t).toLocaleTimeString("en-US", { hour12: false });
    r.insertCell().textContent = format(p.v);
  }
  return t;
}

/**
 * The q-quantile (0..1) of a histogram from cumulative buckets [{le, count}] and the
 * total observation count, in the buckets' own unit. Observations above the last
 * bucket report that bucket's bound.
 */
export function quantile(buckets, total, q) {
  if (!buckets.length || !total) return 0;
  const rank = q * total;
  let prevLe = 0, prevCount = 0;
  for (const b of buckets) {
    if (b.count >= rank) {
      const span = b.count - prevCount;
      return span <= 0 ? b.le : prevLe + (b.le - prevLe) * ((rank - prevCount) / span);
    }
    prevLe = b.le; prevCount = b.count;
  }
  return prevLe;
}
