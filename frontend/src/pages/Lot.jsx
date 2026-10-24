// Lot page. The price, the bid history and the clock all come from the backend (the real API or the
// demo engine) and update live; nothing here is invented.
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import gsap from "gsap";
import ProxyBox from "../components/ProxyBox.jsx";
import RollNumber from "../components/RollNumber.jsx";
import TracePanel from "../components/TracePanel.jsx";
import { useBackend } from "../hooks/backend.jsx";
import { useLotSession } from "../hooks/lot.js";
import { useAsync, useNow, prefersReducedMotion } from "../hooks/use.js";
import { follow, refreshWallet } from "../lib/shell.js";
import { toast } from "../lib/toast.js";
import { bidderLabel } from "../../app/bidding.js";
import { catalogOrder, resolveId } from "../../app/lots.js";
import { secondsLeft } from "../../app/model.js";
import { SNIPE_WINDOW } from "../../app/rules.js";
import { ago, clock, dateTime, local, parseUsd, usd } from "../../app/util.js";

const pad = n => String(n).padStart(2, "0");
const VIEWS = [["100% 100%", 1, null], ["20% 50%", 1.7, "20% 55%"], ["80% 50%", 1.7, "80% 55%"], ["50% 40%", 1.7, "50% 40%"]];

function Stop({ title, children, actions }) {
  return <main className="lt"><div className="page"><div className="empty"><h3>{title}</h3><p>{children}</p><div className="pop-actions">{actions}</div></div></div></main>;
}

function Gallery({ lot }) {
  const [gi, setGi] = useState(0);
  const posY = lot.pos.split(" ")[1];
  const go = i => setGi((i + VIEWS.length) % VIEWS.length);
  const [origin, scale, pos] = VIEWS[gi];
  const objectPosition = gi ? pos : lot.pos;
  void posY;
  return (
    <>
      <div className="lt-stage">
        <img src={lot.img} alt={`${lot.alt}, view ${gi + 1}`} style={{ objectPosition, transformOrigin: origin, transform: `scale(${scale})` }} />
        <Link className="lt-back mono" to={`/#${lot.category === "muscle" ? "muscle" : "classic"}`}>Back to auction</Link>
        <div className="lt-arrows"><button className="arrow" aria-label="Previous view" onClick={() => go(gi - 1)}>‹</button><button className="arrow" aria-label="Next view" onClick={() => go(gi + 1)}>›</button></div>
        <span className="lt-count mono">{pad(gi + 1)} / {pad(VIEWS.length)}</span>
      </div>
      <ol className="lt-thumbs">
        {VIEWS.map((v, i) => (
          <li key={i}><button className={i === gi ? "on" : ""} aria-label={`View ${i + 1}`} onClick={() => go(i)}>
            <img src={lot.img} alt="" style={{ objectPosition: v[2] || lot.pos, ...(i ? { transform: "scale(1.7)", transformOrigin: v[0] } : {}) }} />
          </button></li>
        ))}
      </ol>
    </>
  );
}

function Tabs({ lot }) {
  const specs = lot.specs.length ? <dl className="lt-specs">{lot.specs.map(r => <div key={r[0]}><dt>{r[0]}</dt><dd>{r[1]}</dd></div>)}</dl> : null;
  const m = x => (x || "").toLowerCase();
  const tabs = { Overview: <div className="lt-ov"><div><h2>{lot.lead || lot.title}</h2>{lot.text.map((t, i) => <p key={i}>{t}</p>)}</div>{specs}</div> };
  if (lot.specs.length) tabs.Specifications = <><h2>Specifications</h2>{specs}</>;
  if (lot.history) tabs.History = <><h2>History</h2><p className="lt-p">{lot.history}</p></>;
  if (lot.specs.length) {
    tabs.Condition = <><h2>Condition</h2><ul className="lt-list"><li>Paint and brightwork: presented in {m(lot.specs.find(r => r[0] === "Exterior")?.[1]) || "its original colour"} with a consistent finish.</li><li>Mechanical: engine and gearbox run and shift as expected{lot.year ? ` for a ${lot.year} ${lot.make || "car"}` : ""}.</li><li>Interior: {m(lot.specs.find(r => r[0] === "Interior")?.[1]) || "as described"}, with wear in keeping with {lot.specs.find(r => r[0] === "Mileage")?.[1] || "its mileage"}.</li></ul></>;
    tabs.Documentation = <><h2>Documentation</h2><ul className="lt-list"><li>Ownership history and registration papers</li><li>Service and restoration invoices</li><li>Chassis and engine number verification</li></ul></>;
  }
  tabs.Shipping = <><h2>Shipping</h2><p className="lt-p">The car is in {lot.where || "the seller's location"}. We arrange enclosed worldwide transport and export paperwork after payment clears. A quote is shown at checkout.</p></>;
  const names = Object.keys(tabs);
  const [tab, setTab] = useState("Overview");
  const panel = useRef(null);
  useEffect(() => { if (panel.current && !prefersReducedMotion()) gsap.from(panel.current.children, { autoAlpha: 0, y: 8, duration: 0.4, ease: "power2.out" }); }, [tab]);
  return (
    <>
      <div className="lt-tabs" role="tablist" aria-label="Lot details">{names.map(t => <button key={t} role="tab" aria-selected={tab === t} onClick={() => setTab(t)}>{t}</button>)}</div>
      <div className="lt-panel" role="tabpanel" ref={panel} key={tab}>{tabs[tab]}</div>
    </>
  );
}

function Receipt({ s, be, dialog }) {
  const [state, setState] = useState({ loading: true });
  useEffect(() => {
    let on = true;
    be.ledger({ limit: 200 }).then(page => { if (on) setState({ tx: page.entries.find(e => e.auction_id === s.id && e.kind === "SETTLE") }); }).catch(e => { if (on) setState({ error: e.message }); });
    return () => { on = false; };
  }, [be, s.id]);
  const l = s.lot;
  return (
    <>
      <h2 id="rcTitle">Receipt</h2>
      {state.error ? <p className="form-error">{state.error}</p> : state.loading ? <p className="muted">Loading the ledger entry...</p> : (
        <>
          <p className="lead">{l.title}</p>
          <dl className="rc"><div><dt>Winning bid</dt><dd className="tnum">{usd(l.current)}</dd></div>
            <div><dt>Paid from</dt><dd>Funds held for this lot</dd></div>
            <div><dt>Settled</dt><dd>{state.tx ? dateTime(state.tx.created_at) : "Pending"}</dd></div>
            <div><dt>Ledger journal</dt><dd className="tnum">{state.tx ? state.tx.transaction_id.slice(0, 13) + "..." : "Not posted yet"}</dd></div></dl>
          <p className="small muted">Settlement is one journal whose lines sum to zero: your held funds leave, the seller's available balance rises by the same amount.</p>
        </>
      )}
      <div className="pop-actions"><Link className="btn btn-sm" to="/wallet">Open the statement</Link><button className="btn btn-line btn-sm" onClick={() => dialog.current?.close()}>Close</button></div>
    </>
  );
}

function LotView({ s, be }) {
  const navigate = useNavigate();
  const lot0 = useRef(s.lot).current;
  const l = s.lot, me = s.me;
  const live = l.status === "ACTIVE", upcoming = l.status === "NOT_ACTIVE", closed = l.status === "COMPLETED" || l.status === "CANCELLED";
  const now = useNow(250);
  const [amount, setAmount] = useState("");
  const dirty = useRef(false), holdUntil = useRef(0);
  const [result, setResult] = useState(null);
  const [delta, setDelta] = useState(0);
  const [flash, setFlash] = useState(0);
  const [bump, setBump] = useState(0);
  const [extendedNote, setExtendedNote] = useState(null);
  const [traceOpen, setTraceOpen] = useState(false);
  const [saved, setSaved] = useState(() => new Set(local.json("marque-saved:" + be.kind, [])).has(s.id));
  const [receiptOpen, setReceiptOpen] = useState(false);
  const dialog = useRef(null);
  const traceBtn = useRef(null);
  const prevCurrent = useRef(l.current);
  const newBid = useRef(null);
  const histBody = useRef(null);

  const low = Math.round((lot0.current ?? lot0.startingPrice) * 0.8 / 100000) * 100000, high = Math.round((lot0.current ?? lot0.startingPrice) * 1.35 / 100000) * 100000;
  const amtCents = parseUsd(amount);
  const fmtAmt = c => usd(c);

  // the controller's events become page state
  useEffect(() => {
    const offs = [
      s.on("outcome", o => {
        if (o.kind === "auth") { navigate(`/account?next=${encodeURIComponent(location.pathname)}`); return; }
        setResult(o);
        if (o.kind === "leading") { holdUntil.current = Date.now() + 4000; follow(s.id); toast("Bid placed: " + usd(o.amount)); }
        if (o.kind === "duplicate") toast("Duplicate click ignored");
      }),
      s.on("outbid", o => setResult({ kind: "outbid", message: `You were outbid by ${bidderLabel(o.by, s.me)} at ${usd(o.amount)}. Your ${usd(o.hold ?? 0)} hold was released and is available again.`, minimum: s.lot.minNext })),
      s.on("extended", x => { toast("A late bid extended the close"); setBump(b => b + 1); setExtendedNote(x); }),
      s.on("event", e => { if (e.bid) newBid.current = e.bid.id; }),
    ];
    return () => offs.forEach(off => off());
  }, [s, navigate]);

  // price went up: show by how much, and flash the box
  useEffect(() => {
    if (l.current != null && prevCurrent.current != null && l.current > prevCurrent.current) { setDelta(l.current - prevCurrent.current); setFlash(f => f + 1); }
    prevCurrent.current = l.current;
  }, [l.current]);   // eslint-disable-line react-hooks/exhaustive-deps

  // keep the field at the minimum unless the person is typing, or just placed a bid (see the double click guard)
  useEffect(() => {
    if (live && Date.now() >= holdUntil.current && (!dirty.current || parseUsd(amount) < l.minNext)) { setAmount(usd(l.minNext)); dirty.current = false; }
  }, [l.minNext, live]);   // eslint-disable-line react-hooks/exhaustive-deps

  // a new row slides in
  useEffect(() => {
    if (newBid.current && histBody.current && !prefersReducedMotion()) { const r = histBody.current.firstElementChild; if (r) gsap.from(r, { autoAlpha: 0, y: -10, duration: 0.45, ease: "power2.out" }); }
    newBid.current = null;
  }, [s.history]);

  useEffect(() => { if (!flash) return undefined; const t = setTimeout(() => setFlash(0), 900); return () => clearTimeout(t); }, [flash]);

  // the last two minutes
  const left = secondsLeft(l, be.feed.serverNow());
  const startLeft = Math.max(0, Math.ceil((Date.parse(l.startsAt) - be.feed.serverNow()) / 1000));
  const inSnipe = live && left <= SNIPE_WINDOW / 1000 && left > 0;
  useEffect(() => {
    if (live && left === 0) { const t = setTimeout(() => s.reload(), 1200); return () => clearTimeout(t); }
    return undefined;
  }, [live, left === 0]);   // eslint-disable-line react-hooks/exhaustive-deps
  void now;

  const place = async cents => { await s.place(cents); };
  const submit = async e => {
    e.preventDefault();
    if (!me) { navigate(`/account?next=${encodeURIComponent(location.pathname)}`); return; }
    const cents = parseUsd(amount);
    if (!cents || cents < l.minNext) { setResult({ kind: "toolow", message: `The minimum bid is ${usd(l.minNext)}.`, minimum: l.minNext }); setAmount(usd(l.minNext)); return; }
    if (l.bidderId === me && cents <= l.current && Date.now() > holdUntil.current) { setResult({ kind: "toolow", message: `You already lead at ${usd(l.current)}. Enter a higher amount to raise your bid.` }); return; }
    await place(cents);
  };
  const rebid = min => { setAmount(usd(min)); dirty.current = true; place(min); };
  const step = dir => { dirty.current = true; setAmount(usd(dir > 0 ? (amtCents || l.minNext) + l.step : Math.max(l.minNext, (amtCents || l.minNext) - l.step))); };

  const toggleSave = () => {
    const set = new Set(local.json("marque-saved:" + be.kind, [])), on = !set.has(s.id);
    on ? set.add(s.id) : set.delete(s.id);
    local.setJson("marque-saved:" + be.kind, [...set]); setSaved(on); toast(on ? "Saved to your list" : "Removed from your list");
  };
  const share = () => { (navigator.clipboard ? navigator.clipboard.writeText(location.href) : Promise.reject()).then(() => toast("Link copied"), () => toast("Copy the address bar to share")); };

  useEffect(() => { if (receiptOpen && dialog.current && !dialog.current.open) dialog.current.showModal(); }, [receiptOpen]);

  const w = s.wallet, held = s.held, others = w ? Math.max(0, w.reserved - held) : 0;
  const won = l.bidderId && l.bidderId === me;
  const order = useAsync(() => catalogOrder(), []);
  const idx = order.data ? order.data.indexOf(lot0.key) : -1;
  const goLot = d => `/lot/${order.data[(idx + d + order.data.length) % order.data.length]}`;
  const rows = s.history.slice(0, 6), n = l.bids;

  const clockText = l.status === "COMPLETED" ? "Closed" : l.status === "CANCELLED" ? "Cancelled" : clock(upcoming ? startLeft : left);
  return (
    <main className="lt" id="lotMain">
      <div className="lt-grid">
        <div className="lt-left"><Gallery lot={lot0} /><Tabs lot={lot0} /></div>

        <aside className="lt-right">
          <div className="lt-nextprev mono">
            {idx >= 0 ? <><Link to={goLot(-1)}>Prev lot</Link><Link to={goLot(1)}>Next lot</Link></> : null}
          </div>
          <div className="lt-top">
            <p className="mono lt-lotline"><span>{lot0.lotNo ? "Lot " + lot0.lotNo : "Lot"}</span>
              <span className={"lt-live" + (live && be.feed.state === "open" ? " on" : "")}>{live ? (be.feed.state === "open" ? "Live" : "Reconnecting") : upcoming ? "Opens soon" : l.status === "COMPLETED" ? "Closed" : "Cancelled"}</span></p>
            <div className="lt-tools"><button className="lt-tool" aria-pressed={saved} onClick={toggleSave}>{saved ? "Saved" : "Save"}</button><button className="lt-tool" onClick={share}>Share</button></div>
          </div>
          <h1>{lot0.title}</h1>
          <p className="lt-sub">{[lot0.event?.name, lot0.where].filter(Boolean).join("  |  ")}</p>
          <ul className="lt-chips">{[lot0.make, lot0.model, lot0.year, lot0.type].filter(Boolean).map(c => <li key={c}>{c}</li>)}</ul>

          <section className={"lt-bidbox" + (flash ? " flash" : "")} id="bidbox" aria-label="Bidding">
            <div className="lt-bid-row">
              <div><span className="mono lt-lab">Current bid</span><RollNumber className="lt-big" value={l.current ?? l.startingPrice} format={usd} /><span className="lt-delta">{delta > 0 ? `+${usd(delta)}  just now` : ""}</span></div>
              <div className="lt-cell"><span className="mono lt-lab">Bids</span><strong className="lt-mid">{l.bids}</strong></div>
              <div className="lt-cell"><span className="mono lt-lab">{upcoming ? "Opens in" : "Time remaining"}</span><strong className={"lt-clock" + (live && left <= 120 ? " urgent" : "") + (bump ? " bump" : "")} key={bump}>{clockText}</strong></div>
            </div>

            {(inSnipe || extendedNote) && live && <div className="lt-snipe" role="status">{extendedNote && !inSnipe
              ? <><b>Close extended.</b> A bid in the last two minutes moved the end to {dateTime(extendedNote.to)}. Extension {extendedNote.extensions} of 10.</>
              : <><b>Last two minutes.</b> A bid now moves the close to two minutes after the bid, so nobody is sniped. {l.extensions ? `Extended ${l.extensions} of 10 times so far.` : ""}</>}</div>}

            <div className="lt-range"><div className="lt-range-bar"><i style={{ left: `${Math.max(0, Math.min(100, ((l.current ?? l.startingPrice) - low) / Math.max(1, high - low) * 100))}%` }} /></div>
              <div className="lt-range-lab mono"><span>{usd(low)}</span><span>{lot0.reserve}</span><span>{usd(high)}</span></div></div>

            <div className={"lt-wallet" + (held ? " is-leading" : "")}>
              {!me ? <p>Bids are held from your wallet, not charged. <Link to={`/account?next=${encodeURIComponent(location.pathname)}`}>Sign in</Link> to bid.</p>
                : !w ? <span className="skel" style={{ display: "block", height: 34 }} />
                : <>
                  <div><span className="mono lt-lab">Available</span><b className="tnum">{usd(w.available)}</b></div>
                  <div className="lt-held"><span className="mono lt-lab">Held on this lot</span><b className="tnum">{usd(held)}</b></div>
                  {others > 0 && <div><span className="mono lt-lab">Held elsewhere</span><b className="tnum">{usd(others)}</b></div>}
                  <Link className="linkish" to="/wallet">Add funds</Link>
                </>}
            </div>

            {closed ? <ClosedPanel s={s} me={me} won={won} onReceipt={() => { setReceiptOpen(true); dialog.current?.showModal(); }} /> : (
              <form className="lt-form" onSubmit={submit} noValidate>
                <div className="lt-field"><button type="button" aria-label="Decrease" disabled={!live} onClick={() => step(-1)}>−</button>
                  <label className="visually" htmlFor="amt">Bid amount in dollars</label>
                  <input id="amt" inputMode="numeric" autoComplete="off" disabled={!live} value={amount} onChange={e => { dirty.current = true; setAmount(e.target.value); }} onBlur={() => { if (amtCents) setAmount(fmtAmt(amtCents)); }} />
                  <button type="button" aria-label="Increase" disabled={!live} onClick={() => step(1)}>+</button></div>
                <button className="btn lt-go" type="submit" disabled={!me ? false : !live}>{!me ? "Sign in to bid" : live ? "Place bid" : upcoming ? "Not open yet" : "Not open for bids"}</button>
              </form>
            )}
            <p className="lt-min">{live ? `Minimum next bid ${usd(l.minNext)}. Each bid goes up by at least ${usd(l.step)}.` : ""}</p>
            <div className={"lt-result" + (result ? " r-" + result.kind : "")} role="status" aria-live="polite">
              {result && <><p>{result.message}</p>
                {result.minimum ? <button className="btn btn-sm" onClick={() => rebid(result.minimum)}>Bid {usd(result.minimum)}</button> : null}
                {result.kind === "funds" ? <Link className="btn btn-sm" to="/wallet">Add funds</Link> : null}</>}
            </div>

            <ProxyBox be={be} s={s} />

            <div className="lt-demo">
              <button className="btn btn-line btn-sm" ref={traceBtn} aria-controls="trace" onClick={() => setTraceOpen(o => !o)}>Behind the bid</button>
              <button className="btn btn-line btn-sm" aria-controls="trace" onClick={() => { setTraceOpen(true); setTimeout(() => document.getElementById("stress")?.scrollIntoView({ behavior: prefersReducedMotion() ? "auto" : "smooth", block: "start" }), 60); }}>Stress it</button>
            </div>
          </section>

          <section className="lt-history" aria-label="Bid history">
            <div className="lt-hist-head"><h2>Live Bid History</h2><span className="mono lt-live">Live updates</span></div>
            <table><thead><tr><th>#</th><th>Bidder</th><th>Amount</th><th>Time</th></tr></thead>
              <tbody ref={histBody}>{rows.map((b, i) => (
                <tr key={b.id} className={b.user_id === me ? "you" : ""}><td>{n - i}</td><td>{bidderLabel(b.user_id, me)}{b.auto ? <span className="lt-hist-auto" title="Placed by the engine for a maximum bid">auto</span> : null}</td><td>{usd(b.amount)}</td><td>{ago(b.created_at, be.feed.serverNow())}</td></tr>
              ))}</tbody></table>
            {!rows.length && <p className="lt-hist-empty">No bids yet. The first bid sets the price.</p>}
          </section>
        </aside>
      </div>

      {lot0.quote && (
        <section className="lt-quote">
          <img src={lot0.img} alt="" />
          <div className="lt-quote-in"><p className="mono">{lot0.event?.name || ""}</p><blockquote>“{lot0.quote}”</blockquote></div>
          {lot0.event && <Link className="mono" to={`/event/${lot0.event.slug}`}>Explore the event</Link>}
        </section>
      )}

      <dialog className="sheet" id="receipt" ref={dialog} aria-labelledby="rcTitle" onClose={() => setReceiptOpen(false)}>{receiptOpen && <Receipt s={s} be={be} dialog={dialog} />}</dialog>
      <TracePanel s={s} be={be} open={traceOpen} onClose={() => { setTraceOpen(false); traceBtn.current?.focus(); }} />
    </main>
  );
}

function ClosedPanel({ s, me, won, onReceipt }) {
  const l = s.lot;
  return (
    <div className={"lt-closed" + (won ? " is-won" : "")}>
      {l.status === "CANCELLED" ? <><h3>Cancelled</h3><p>The seller withdrew this lot before it opened.</p></>
        : !l.bidderId ? <><h3>Closed without bids</h3><p>No bids were placed.</p></>
        : won ? <><h3>You won this lot</h3><p>Winning bid <b className="tnum">{usd(l.current)}</b>. {l.settledAt ? "The sale is settled: the held funds went to the seller." : "The sale is being settled."}</p><button className="btn btn-sm" onClick={onReceipt}>View receipt</button></>
        : <><h3>Sold</h3><p>Sold for <b className="tnum">{usd(l.current)}</b> to {bidderLabel(l.bidderId, me)}.{s.history.some(b => b.user_id === me) ? " Your hold was released." : ""}</p></>}
    </div>
  );
}

export default function Lot() {
  const { id: param } = useParams();
  const be = useBackend();
  const [resolved, setResolved] = useState({ state: "loading" });
  useEffect(() => {
    let on = true;
    setResolved({ state: "loading" });
    if (be.unreachable) { setResolved({ state: "unreachable" }); return undefined; }
    resolveId(be, param).then(id => { if (on) setResolved(id ? { state: "ok", id } : { state: "missing" }); }).catch(() => { if (on) setResolved({ state: "missing" }); });
    return () => { on = false; };
  }, [be, param]);
  const { s, error } = useLotSession(be, resolved.state === "ok" ? resolved.id : null);
  useEffect(() => { if (s) document.title = "Marque | " + s.lot.title; }, [s]);
  useEffect(() => { refreshWallet(); }, [s]);

  if (resolved.state === "unreachable") return <Stop title="The API is not answering" actions={<a className="btn" href="?engine=sim">Use the demo engine</a>}>Nothing is listening at {be.base}. Start it with make run, or use the demo engine, which needs no server.</Stop>;
  if (resolved.state === "missing") return <Stop title="This lot is not on the backend" actions={<><Link className="btn" to="/">Back to the auction</Link>{be.kind === "live" && <a className="btn btn-line" href="?engine=sim">Use the demo engine</a>}</>}>{be.kind === "live" ? "The real API has no lot with that name yet. Start it with DEMO_SEED=true, or load the cars from the operator's badge." : "No lot matches that link."}</Stop>;
  if (error) return <Stop title={error.status === 404 ? "Lot not found" : "Could not load this lot"} actions={<Link className="btn" to="/">Back to the auction</Link>}>{error.message}</Stop>;
  if (!s) return <main className="lt" aria-busy="true"><div className="lt-grid"><div className="lt-left"><div className="skel" style={{ height: 520 }} /></div><aside className="lt-right"><div className="skel" style={{ height: 48, width: "80%" }} /></aside></div></main>;
  return <LotView s={s} be={be} />;
}
