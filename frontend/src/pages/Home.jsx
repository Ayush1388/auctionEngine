// The home page: a hero over a full-bleed photo, the lots, the events, and the way in.
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import LotCard from "../components/LotCard.jsx";
import Logo from "../components/Logo.jsx";
import ShellActions from "../components/ShellActions.jsx";
import ThemeToggle from "../components/ThemeToggle.jsx";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useLiveLots } from "../hooks/lots.js";
import { useDocumentTitle } from "../hooks/use.js";
import { toast } from "../lib/toast.js";
import { HOME_LOTS, SLIDES } from "../data/homeLots.js";
import { EVENTS } from "../data/events.js";
import { bands, cards, eventRows, gsap, homeIntro } from "../lib/motion.js";
import { toLot } from "../../app/model.js";
import { usd } from "../../app/util.js";

const pad = n => String(n).padStart(2, "0");
const fmtHero = s => [Math.floor(s / 86400), Math.floor(s % 86400 / 3600), Math.floor(s % 3600 / 60), s % 60].map(pad).join(" : ");

/** A figure that counts up once, when it first scrolls into view. */
function CountUp({ to, prefix = "", suffix = "" }) {
  const el = useRef(null);
  const [v, setV] = useState(0);
  useEffect(() => {
    const node = el.current;
    const io = new IntersectionObserver(entries => {
      if (!entries[0].isIntersecting) return;
      io.disconnect();
      if (matchMedia("(prefers-reduced-motion: reduce)").matches) { setV(to); return; }
      const start = performance.now();
      const tick = now => { const t = Math.min(1, (now - start) / 1200); setV(Math.round(to * (1 - Math.pow(1 - t, 3)))); if (t < 1) requestAnimationFrame(tick); };
      requestAnimationFrame(tick);
    }, { threshold: 0.4 });
    io.observe(node);
    return () => io.disconnect();
  }, [to]);
  return <b ref={el}>{prefix}{v}{suffix}</b>;
}

function Trending() {
  const be = useBackend();
  const [list, setList] = useState([]);
  useEffect(() => { be.trending(8).then(setList).catch(() => {}); }, [be]);   // the page works without it
  if (!list.length) return null;
  return (
    <section className="trend-sec" id="trendSec" aria-labelledby="trH">
      <h2 className="h2" id="trH">Hot right now</h2>
      <ul className="trend">
        {list.map(a => { const l = toLot(a); return (
          <li key={l.id}><Link className="tcard" to={`/lot/${l.id}`}><span className="thumb"><img src={l.img} alt="" style={{ objectPosition: l.pos }} loading="lazy" /></span>
            <span><b>{l.title}</b><span className="small muted tnum">{usd(l.current ?? l.startingPrice)}, {l.bids} {l.bids === 1 ? "bid" : "bids"}</span></span></Link></li>
        ); })}
      </ul>
    </section>
  );
}

const EVENT_ROWS = [
  ["retromobile", "Rétromobile Paris", "Paris Expo, France", "Pre-war racers and postwar coachbuilt rarities in a single hall.", "Feb 3 to 7, 2027", "62", "$18M to $24M", "/assets/events/retromobile.jpg", "1921 Bentley 3 Litre racing car on display at Rétromobile in Paris"],
  ["villa-deste", "Concorso d’Eleganza Villa d’Este", "Lake Como, Italy", "Concours-grade classics sold on the shore of the lake.", "Apr 23 to 25, 2027", "38", "$22M to $30M", "/assets/events/villa-deste.jpg", "Black 1930s Alfa Romeo 8C 2900B coupe on the lawn at Villa d'Este"],
  ["monterey", "Monterey Car Week", "Pebble Beach, California", "The year’s biggest sale, timed to the concours on the 18th fairway.", "Aug 12 to 15, 2027", "96", "$60M to $85M", "/assets/events/pebble-beach.jpg", "Crowds walking the lawn at the Pebble Beach Concours d'Elegance beside the Pacific"],
  ["goodwood", "Goodwood Revival", "West Sussex, England", "Race and road cars from the golden era, sold the weekend they take to the circuit.", "Sep 10 to 12, 2027", "48", "$16M to $22M", "/assets/events/goodwood-revival.jpg", "Red 1950s Ferrari sports racers and spectators in period dress at the Goodwood Revival"],
];

export default function Home() {
  useDocumentTitle("Marque | Vintage Car Auctions");
  const { signedIn } = useAuth();
  const lots = useLiveLots();
  const root = useRef(null);
  const [slide, setSlide] = useState(0);
  const [fade, setFade] = useState(false);
  const [drawer, setDrawer] = useState(false);
  const [hero, setHero] = useState(1 * 86400 + 16 * 3600 + 23 * 60 + 45);
  const burger = useRef(null), drawerInput = useRef(null);
  const s = SLIDES[slide];

  // every motion effect is reverted when the page unmounts
  useLayoutEffect(() => {
    const ctx = gsap.context(() => { homeIntro(root.current); bands(root.current); cards(root.current); eventRows(root.current); }, root);
    return () => ctx.revert();
  }, []);

  const show = i => {
    const n = (i + SLIDES.length) % SLIDES.length;
    setFade(true);
    setTimeout(() => { setSlide(n); setFade(false); }, 250);
  };
  useEffect(() => { const t = setInterval(() => setHero(h => Math.max(0, h - 1)), 1000); return () => clearInterval(t); }, []);
  useEffect(() => {
    if (!drawer) return undefined;
    drawerInput.current?.focus();
    const key = e => { if (e.key === "Escape") { setDrawer(false); burger.current?.focus(); } };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [drawer]);

  const join = !signedIn ? <Link className="pill" to="/account?tab=create">Join now</Link> : null;
  return (
    <main id="top" ref={root}>
      <section className="hero">
        <div className="panel">
          <header className="topbar">
            <a className="brand" href="#top" aria-label="Marque home"><Logo width={46} height={38} /></a>
            <nav className="nav" aria-label="Primary">
              <ul>
                <li><a href="#classic">Classics</a></li><li><a href="#muscle">Muscle</a></li><li><a href="#events">Events</a></li>
                <li><Link to="/search">Search</Link></li><li><Link to="/sell">Sell a car</Link></li><li><a href="#how">How it works</a></li><li><Link to="/tour">Backend</Link></li>
              </ul>
            </nav>
            <div className="actions">
              <span className="shell-actions"><ShellActions /></span>
              <ThemeToggle />
              <button className="burger" ref={burger} aria-label="Open menu" aria-expanded={drawer} aria-controls="drawer" onClick={() => setDrawer(true)}><i /><i /></button>
            </div>
          </header>

          <h1 className="wordmark">MARQUE</h1>

          <ul className="awards" aria-label="Recognition">
            <li><span className="medal">1</span><em>2025</em> “ Most trusted marketplace ”</li>
            <li><span className="medal">2</span><em>2024</em> “ Fastest settlement ”</li>
            <li><span className="medal">3</span><em>2022</em> “ Collector’s choice ”</li>
          </ul>

          <div className="controls">
            <div className="arrows">
              <button className="arrow" aria-label="Previous car" onClick={() => show(slide - 1)}>‹</button>
              <button className="arrow" aria-label="Next car" onClick={() => show(slide + 1)}>›</button>
            </div>
            <span className="mono">2026 Season</span>
            <span className="mono">{s.tag}</span>
            <span className="mono timer" aria-label="Time until next gavel">{fmtHero(hero)}</span>
            {join}
          </div>
        </div>

        <div className="stage">
          <img className={"stage-img" + (fade ? " fade" : "")} src={s.img} alt={s.alt} style={{ objectPosition: s.pos }} fetchPriority="high" />
          <ol className="thumbs" aria-label="Featured lots">
            {[1, 2, 3].map(i => <li key={i}><button className={slide === i ? "on" : ""} aria-label={`Show ${SLIDES[i].title}`} onClick={() => show(i)}><img src={SLIDES[i].img} alt="" /></button></li>)}
          </ol>
          <div className="stage-info">
            <p className="mono">{s.lot}</p>
            <h2>{s.title}</h2>
            <p className="stage-bid"><span className="mono">Current bid</span> <strong>{usd(s.bid * 100)}</strong>
              <a className="btn" href="#classic-lots">Place a bid</a></p>
          </div>
        </div>
      </section>

      <aside className={"drawer" + (drawer ? " open" : "")} id="drawer" aria-hidden={!drawer}>
        <button className="drawer-close" aria-label="Close menu" onClick={() => { setDrawer(false); burger.current?.focus(); }}>×</button>
        <form className="search" role="search" action="/search" method="get">
          <input ref={drawerInput} type="search" name="q" placeholder="Search makes, models, lots" aria-label="Search lots" />
        </form>
        <ul onClick={() => setDrawer(false)}>
          <li><a href="#classic">Classic legends</a></li><li><a href="#muscle">Midnight muscle</a></li><li><a href="#events">Upcoming events</a></li>
          <li><a href="#sell">Sell a car</a></li><li><Link to="/account">Sign in</Link></li>
        </ul>
        {!signedIn && <Link className="pill pill-solid" to="/account?tab=create">Join now</Link>}
      </aside>

      <section className="stats" aria-label="Marketplace figures">
        <div><CountUp to={300} suffix="+" /><span className="mono">Live auctions</span></div>
        <div><CountUp to={50} suffix="K+" /><span className="mono">Registered bidders</span></div>
        <div><CountUp to={40} suffix="+" /><span className="mono">Countries</span></div>
        <div><CountUp to={120} prefix="$" suffix="M+" /><span className="mono">Cars sold</span></div>
      </section>

      <Trending />

      <section className="band band-light" id="classic">
        <div className="band-photo"><img src="/assets/doina.jpg" alt="Red Excalibur roadster with side-mounted exhaust pipes" loading="lazy" /></div>
        <div className="band-copy">
          <h2>Classic Legends</h2>
          <p>Pre-war icons and golden-era masterpieces. Cars that defined a generation.</p>
          <a className="btn btn-ink" href="#classic-lots">Browse classics</a>
        </div>
      </section>
      <section className="lots" id="classic-lots" aria-label="Classic lots"><div className="grid">{HOME_LOTS.classic.map(l => <LotCard key={l.id} item={l} lots={lots} />)}</div></section>

      <section className="band band-light band-flip" id="muscle">
        <div className="band-copy">
          <h2>Midnight Muscle</h2>
          <p>Black paint, chrome and big-block V8s. The cars you hear before you see.</p>
          <a className="btn btn-ink" href="#muscle-lots">Browse muscle</a>
        </div>
        <div className="band-photo"><img src="/assets/camaro.jpg" alt="Black 1967 Chevrolet Camaro SS convertible with white stripes in a tree-lined park" loading="lazy" /></div>
      </section>
      <section className="lots" id="muscle-lots" aria-label="Midnight Muscle lots"><div className="grid">{HOME_LOTS.muscle.map(l => <LotCard key={l.id} item={l} lots={lots} />)}</div></section>

      <section className="events" id="events">
        <div className="head">
          <h2>Upcoming Auctions</h2>
          <p>Live sales held alongside the world’s great concours and shows. Register once to bid at every one.</p>
        </div>
        <ul className="event-list">
          {EVENT_ROWS.map(([slug, name, where, blurb, date, n, est, img, alt]) => (
            <li key={slug}>
              <Link className="event" to={`/event/${slug}`}>
                <div className="event-photo"><img src={img} alt={alt} loading="lazy" /></div>
                <div className="event-main"><h3>{name}</h3><p className="event-where">{where}</p><p className="event-blurb">{blurb}</p></div>
                <dl className="event-facts"><div><dt>Date</dt><dd>{date}</dd></div><div><dt>Lots</dt><dd>{n}</dd></div><div><dt>Estimate</dt><dd>{est}</dd></div></dl>
                <span className="event-cta">Register to bid</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="sell" id="sell">
        <img src="/assets/theodor.jpg" alt="Red 1965 Ford Mustang parked before ornate black and gold gates" loading="lazy" />
        <div className="sell-copy">
          <h2>Sell a Car</h2>
          <p className="sell-lead">Reach collectors in 40 countries. We handle photography, paperwork and payment.</p>
          <ul className="perks">
            <li><b>Worldwide exposure</b><span>Your listing reaches 50K+ verified bidders.</span></li>
            <li><b>Secure settlement</b><span>Funds are held in escrow until the car changes hands.</span></li>
            <li><b>Expert valuation</b><span>A specialist sets the estimate and reserve with you.</span></li>
          </ul>
          <Link className="btn" to="/sell">List your car</Link>
        </div>
      </section>

      <section className="how" id="how">
        <h2>Three steps to the keys</h2>
        <ol className="steps">
          <li><span className="num">01</span><h3>Register and verify</h3><p>Create an account and confirm your identity once. Bidding unlocks within minutes.</p></li>
          <li><span className="num">02</span><h3>Bid in real time</h3><p>Every bid lands instantly for everyone watching. Late bids extend the clock so nobody gets sniped.</p></li>
          <li><span className="num">03</span><h3>Win and settle</h3><p>Funds are held in escrow and released only when the car changes hands.</p></li>
        </ol>
      </section>

      <SiteFooter />
    </main>
  );
}

/** The footer of the home and event pages: the stacked wordmark and the alerts form. */
export function SiteFooter() {
  return (
    <footer className="footer" id="join">
      <div className="footer-top">
        <h2 className="wordmark small">MARQUE</h2>
        <div className="footer-cols">
          <nav aria-label="Auctions"><h3 className="mono">Auctions</h3><ul><li><Link to="/#classic">Classic legends</Link></li><li><Link to="/#muscle">Midnight muscle</Link></li><li><Link to="/#events">Upcoming events</Link></li></ul></nav>
          <nav aria-label="Company"><h3 className="mono">Marque</h3><ul><li><Link to="/#sell">Sell a car</Link></li><li><Link to="/#how">How it works</Link></li><li><Link to="/tour">How the backend works</Link></li></ul></nav>
          <form className="join-form" onSubmit={e => { e.preventDefault(); toast("Thanks. You are on the list."); e.currentTarget.reset(); }}>
            <label className="mono" htmlFor="email">Auction alerts</label>
            <div><input id="email" type="email" placeholder="you@email.com" autoComplete="email" required /><button className="btn" type="submit">Join now</button></div>
          </form>
        </div>
      </div>
      <div className="footer-bottom"><p className="mono">© 2026 Marque Auctions. Bid responsibly.</p><p className="mono"><Link to="/status">System status</Link> &nbsp; <Link to="/trust">Trust page</Link> &nbsp; <Link to="/chaos">Chaos lab</Link></p></div>
    </footer>
  );
}
