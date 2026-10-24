// One template, four events. The editorial content lives in data/events.js; the lot cards are live.
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import LotCard from "../components/LotCard.jsx";
import Logo from "../components/Logo.jsx";
import ShellActions from "../components/ShellActions.jsx";
import ThemeToggle from "../components/ThemeToggle.jsx";
import { SiteFooter } from "./Home.jsx";
import { useAuth } from "../hooks/backend.jsx";
import { useLiveLots } from "../hooks/lots.js";
import { useDocumentTitle } from "../hooks/use.js";
import { CARS, EVENTS } from "../data/events.js";
import { eventIntro, gsap, ScrollTrigger } from "../lib/motion.js";
import { clock, usd } from "../../app/util.js";

const ENDS = [5400, 28800, 43200, 1500];

/** The ball-and-road walk through the event: a road drawn on scroll with a ball that stops at each item. */
function Experience({ ev }) {
  const box = useRef(null);
  const [live, setLive] = useState(false);

  useEffect(() => {
    const X = box.current;
    let st = null, rz = 0;
    const stop = () => { if (st) st.kill(); st = null; X.querySelectorAll(".x-item").forEach(i => i.classList.remove("on")); };

    const build = () => {
      const items = [...X.querySelectorAll(".x-item")], r0 = X.getBoundingClientRect(), W = r0.width, H = r0.height;
      const svg = X.querySelector(".x-road"); svg.setAttribute("viewBox", `0 0 ${W} ${H}`); svg.style.height = H + "px";
      const pt = d => { const r = d.getBoundingClientRect(); return [r.left - r0.left + r.width / 2, r.top - r0.top + r.height / 2]; };
      const P = items.map(it => pt(it.querySelector(".x-dot")));
      const pts = [[0, P[0][1]], ...P, [W, P[P.length - 1][1]]];
      let d = `M ${pts[0][0]} ${pts[0][1]}`;
      for (let i = 0; i < pts.length - 1; i++) {
        const p0 = pts[i - 1] || pts[i], p1 = pts[i], p2 = pts[i + 1], p3 = pts[i + 2] || p2;
        d += ` C ${p1[0] + (p2[0] - p0[0]) / 6} ${p1[1] + (p2[1] - p0[1]) / 6}, ${p2[0] - (p3[0] - p1[0]) / 6} ${p2[1] - (p3[1] - p1[1]) / 6}, ${p2[0]} ${p2[1]}`;
      }
      svg.querySelectorAll("path").forEach(p => p.setAttribute("d", d));
      return { path: svg.querySelector(".x-road-fg"), P, items };
    };

    const init = () => {
      const { path, P, items } = build(), len = path.getTotalLength(), ball = X.querySelector(".x-ball");
      // length fraction of each stop = nearest sampled point on the road
      const stops = P.map(p => { let best = 0, bd = 1e9; for (let l = 0; l <= len; l += 4) { const q = path.getPointAtLength(l), dd = (q.x - p[0]) ** 2 + (q.y - p[1]) ** 2; if (dd < bd) { bd = dd; best = l; } } return best / len; });
      path.style.strokeDasharray = len; path.style.strokeDashoffset = len;
      const place = f => { const q = path.getPointAtLength(f * len); gsap.set(ball, { x: q.x, y: q.y }); path.style.strokeDashoffset = len * (1 - f); items.forEach((it, i) => it.classList.toggle("on", f >= stops[i] - 0.015)); };
      if (matchMedia("(prefers-reduced-motion: reduce)").matches) { place(1); return; }
      place(0);
      st = ScrollTrigger.create({ trigger: X, start: `top+=${Math.round(P[0][1])} 82%`, end: `+=${Math.round(innerHeight * 0.65)}`, scrub: 1.4, onUpdate: s => place(s.progress) });
    };

    const start = () => { if (innerWidth > 900) { setLive(true); requestAnimationFrame(init); } else setLive(false); };
    const onResize = () => { clearTimeout(rz); rz = setTimeout(() => { stop(); start(); }, 200); };
    // wait for the photos to take their space, otherwise the road is drawn through the wrong height
    const t = setTimeout(start, 300);
    addEventListener("resize", onResize);
    return () => { clearTimeout(t); clearTimeout(rz); removeEventListener("resize", onResize); stop(); };
  }, [ev]);

  return (
    <div className={"x x-" + ev.layout + (live ? " x-live" : "")} ref={box}>
      <svg className="x-road" aria-hidden="true"><path className="x-road-bg" /><path className="x-road-fg" /></svg>
      <div className="x-ball" aria-hidden="true" />
      <ol className="x-items">
        {ev.exp.map((x, i) => (
          <li className="x-item" key={x[0]} data-i={i}>
            <div className="x-photo"><img src={"/" + x[4]} alt="" loading="lazy" /></div>
            <div className="x-body"><span className="x-dot" /><h3>{x[0]}</h3><p>{x[1]}</p><span className="mono x-meta">{x[2]}</span><span className="mono x-meta">{x[3]}</span></div>
          </li>
        ))}
      </ol>
    </div>
  );
}

export default function Event() {
  const { slug } = useParams();
  const key = EVENTS[slug] ? slug : "monterey";
  const ev = EVENTS[key];
  useDocumentTitle("Marque | " + ev.name);
  const { signedIn } = useAuth();
  const lots = useLiveLots();
  const root = useRef(null);
  const [ftLeft, setFtLeft] = useState(ev.ft[6]);

  useEffect(() => { window.scrollTo(0, 0); setFtLeft(ev.ft[6]); }, [key, ev]);
  useEffect(() => { const t = setInterval(() => setFtLeft(s => Math.max(0, s - 1)), 1000); return () => clearInterval(t); }, []);
  useLayoutEffect(() => {
    const ctx = gsap.context(() => eventIntro(root.current), root);
    return () => ctx.revert();
  }, [key]);

  const [ftKey, ftKick, ftMeta, ftText, ftBid, ftBids] = ev.ft;
  const fc = CARS[ftKey];
  const block = ev.block.map((k, i) => ({ ...CARS[k], id: k, end: ENDS[i] }));
  const both = key === "retromobile" || key === "villa-deste";

  return (
    <main ref={root} key={key}>
      <section className="ev-hero" id="evHero">
        <img className="ev-hero-img" src={ev.img} alt={ev.name} style={{ objectPosition: ev.pos }} />
        <header className="ev-bar">
          <Link className="brand" to="/" aria-label="Marque home"><Logo width={46} height={38} /></Link>
          <nav className="ev-nav" aria-label="Primary">
            <Link to="/#classic">Classics</Link><Link to="/#muscle">Muscle</Link><Link to="/#events">All events</Link><Link to="/search">Search</Link><Link to="/sell">Sell a car</Link><Link to="/#how">How it works</Link>
          </nav>
          <div className="ev-actions">
            <span className="shell-actions"><ShellActions /></span>
            <ThemeToggle />
            {!signedIn && <Link className="pill pill-solid" to="/account?tab=create">Join now</Link>}
          </div>
        </header>
        <div className="ev-hero-copy">
          <Link className="ev-back mono" to="/#events">Back to events</Link>
          <p className="mono ev-kicker">{ev.kicker}</p>
          <h1 className="ev-title">{ev.name}</h1>
          <p className="ev-dates">{ev.dates}</p>
          <p className="ev-intro">{ev.intro}</p>
          <div className="ev-cta">
            <a className="btn" href="#block">View auctions</a>
            <a className="btn btn-ghost" href="#experience">Event details</a>
          </div>
        </div>
      </section>

      <section className="ev-facts" aria-label="Event facts">
        {ev.facts.map(f => <div key={f[0]}><dt>{f[0]}</dt><dd>{f[1]}</dd><span>{f[2]}</span></div>)}
      </section>

      <section className="ev-exp" id="experience">
        <div className="ev-exp-head"><h2>{ev.expTitle}</h2><p>{ev.expLead}</p></div>
        <Experience ev={ev} />
      </section>

      <section className={"ev-feature " + (both ? "ft-both" : "ft-bottom")} id="evFeature">
        <div className="ev-feature-copy">
          <p className="mono">{ftKick}</p>
          <h2>{fc.title}</h2>
          <p className="ev-feature-meta mono">{ftMeta}</p>
          <p>{ftText}</p>
          <dl className="ev-feature-stats">
            <div><dt>Current bid</dt><dd>{usd(ftBid * 100)}</dd></div>
            <div><dt>Bids</dt><dd>{ftBids}</dd></div>
            <div><dt>Time remaining</dt><dd className="time">{clock(ftLeft)}</dd></div>
          </dl>
          <a className="btn" href="#block">View lot</a>
        </div>
        <div className="ev-feature-photo"><img src={fc.img} alt={fc.alt} style={{ objectPosition: fc.pos || "50% 60%" }} /></div>
      </section>

      <section className="ev-block" id="block">
        <div className="ev-block-head"><h2>Cars on the Block</h2><p>{ev.lead}</p></div>
        <div className="grid">{block.map(l => <LotCard key={l.id} item={l} lots={lots} />)}</div>
      </section>

      <section className="ev-more">
        <h2>Other events</h2>
        <ul>
          {Object.entries(EVENTS).filter(([k]) => k !== key).map(([k, e]) => (
            <li key={k}><Link to={`/event/${k}`}><b>{e.name}</b><span>{e.dates.split("  |  ")[1]}</span><span>{e.facts[0][1]}</span></Link></li>
          ))}
        </ul>
      </section>

      <SiteFooter />
      <svg width="0" height="0" style={{ position: "absolute" }} aria-hidden="true"><filter id="torn" x="-5%" y="-5%" width="110%" height="110%"><feTurbulence type="fractalNoise" baseFrequency="0.035" numOctaves="2" seed="7" result="n" /><feDisplacementMap in="SourceGraphic" in2="n" scale="9" xChannelSelector="R" yChannelSelector="G" /></filter></svg>
    </main>
  );
}
