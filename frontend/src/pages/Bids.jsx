// My bids: Leading, Outbid, Won, Lost, and the lots I saved. Standing is worked out from the wallet
// ledger and the lot itself (see app/mybids.js) and then kept current from the live feed.
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import { useBackend, useRequireAuth } from "../hooks/backend.jsx";
import { useDocumentTitle, useNow } from "../hooks/use.js";
import { loadMyBids, standing } from "../../app/mybids.js";
import { applySnapshot, secondsLeft, toLot } from "../../app/model.js";
import { clock, local, usd } from "../../app/util.js";

const TABS = [["leading", "Leading"], ["outbid", "Outbid"], ["won", "Won"], ["lost", "Lost"], ["saved", "Saved"]];
const EMPTY = {
  leading: ["You do not lead any lot", "Place a bid and it shows up here, with the amount held from your wallet."],
  outbid: ["You are not outbid anywhere", "When someone takes the lead on a lot you bid on, it moves here so you can answer."],
  won: ["No wins yet", "Lots you win appear here once the sale closes, with a receipt."],
  lost: ["Nothing lost", "Closed lots you did not win appear here. Your held money was released when the sale ended."],
  saved: ["No saved lots", "Use Save on a lot page to keep it here."],
};
const LABEL = { leading: "Leading", outbid: "Outbid", won: "Won", lost: "Lost", active: "Open", completed: "Closed", cancelled: "Cancelled", not_active: "Opens soon" };

function Row({ r, isSaved, me, now, serverNow }) {
  const l = r.lot, st = isSaved ? (l.status === "ACTIVE" ? "active" : l.status.toLowerCase()) : standing(l, me), live = l.status === "ACTIVE";
  const label = LABEL[st] || st;
  const href = `/lot/${l.id}`;
  const cta = st === "outbid" ? <Link className="btn btn-sm" to={href}>Bid {usd(l.minNext)}</Link> : st === "won" ? <Link className="btn btn-sm" to={href}>Receipt</Link> : <Link className="btn btn-line btn-sm" to={href}>{live ? "Open lot" : "View"}</Link>;
  void now;
  return (
    <li className="brow">
      <Link className="thumb" to={href} tabIndex={-1} aria-hidden="true"><img src={l.img} alt="" style={{ objectPosition: l.pos }} loading="lazy" /></Link>
      <div className="brow-main">
        <h2><Link to={href}>{l.title}</Link></h2>
        <p className="small muted">{[l.lotNo ? "Lot " + l.lotNo : "", l.where].filter(Boolean).join("  |  ")}</p>
      </div>
      <div className="brow-fig"><span className="mono lab">{isSaved ? "Current bid" : "Your top bid"}</span><b className="tnum">{isSaved ? usd(l.current ?? l.startingPrice) : usd(r.myTop)}</b>{isSaved ? null : <span className="small muted">{r.myCount} {r.myCount === 1 ? "bid" : "bids"}</span>}</div>
      <div className="brow-fig"><span className="mono lab">{isSaved ? "Bids" : "Current bid"}</span><b className="tnum">{isSaved ? l.bids : usd(l.current)}</b></div>
      <div className="brow-fig"><span className="mono lab">{live ? "Ends in" : "Status"}</span><b className="tnum brow-clock">{live ? clock(secondsLeft(l, serverNow)) : label}</b></div>
      <div className="brow-act"><span className={"tag tag-" + st}>{label}</span>{cta}</div>
    </li>
  );
}

export default function Bids() {
  useDocumentTitle("Marque | My bids");
  const be = useBackend();
  const { redirect, id: me } = useRequireAuth();
  const [params, setParams] = useSearchParams();
  const [rows, setRows] = useState([]);
  const [saved, setSaved] = useState([]);
  const [tab, setTab] = useState(params.get("tab") || "leading");
  const [state, setState] = useState("loading");     // loading | ready | error
  const [error, setError] = useState(null);
  const unsubs = useRef(new Map());
  const now = useNow(1000);

  useEffect(() => {
    if (redirect) return undefined;
    let on = true;
    (async () => {
      try {
        const r = await loadMyBids(be);
        const ids = local.json("marque-saved:" + be.kind, []);
        const s = (await Promise.all(ids.map(id => be.getAuction(id).then(toLot).catch(() => null)))).filter(Boolean);
        if (!on) return;
        setRows(r); setSaved(s); setState("ready");
        // open on the tab that has something in it
        if (!params.get("tab")) {
          const c = { ...Object.fromEntries(TABS.map(([k]) => [k, r.filter(x => standing(x.lot, me) === k).length])), saved: s.length };
          setTab(t => (c[t] ? t : TABS.map(x => x[0]).find(k => c[k]) || "leading"));
        }
      } catch (e) { if (on) { setError(e.message); setState("error"); } }
    })();
    const subs = unsubs.current;
    return () => { on = false; subs.forEach(off => off()); subs.clear(); };
  }, [be, me, !!redirect]);   // eslint-disable-line react-hooks/exhaustive-deps

  // keep every open lot current without a refresh
  useEffect(() => {
    rows.forEach(r => {
      if (unsubs.current.has(r.lot.id) || r.lot.status !== "ACTIVE") return;
      unsubs.current.set(r.lot.id, be.feed.subscribe(r.lot.id, ev => {
        setRows(list => list.map(x => (x.lot.id !== r.lot.id ? x : { ...x, lot: applySnapshot(x.lot, ev.auction), myTop: ev.bid && ev.bid.bidder_id === me ? Math.max(x.myTop, ev.bid.amount) : x.myTop })));
      }));
    });
  }, [rows, be, me]);

  const counts = useMemo(() => Object.fromEntries(TABS.map(([k]) => [k, k === "saved" ? saved.length : rows.filter(r => standing(r.lot, me) === k).length])), [rows, saved, me]);
  if (redirect) return redirect;

  const list = tab === "saved" ? saved.map(lot => ({ lot })) : rows.filter(r => standing(r.lot, me) === tab);
  const pick = k => { setTab(k); setParams({ tab: k }, { replace: true }); };
  return (
    <main className="page">
      <PageHead title="My bids" action={<Link className="btn btn-line" to="/#classic">Find a car</Link>}>Every lot you have bid on, and where you stand. This page updates as bids land.</PageHead>
      {state === "loading" && <div aria-busy="true"><div className="skel" style={{ height: 44 }} /><div className="skel" style={{ height: 96, marginTop: 16 }} /><div className="skel" style={{ height: 96, marginTop: 12 }} /></div>}
      {state === "error" && <div className="empty"><h3>Could not load your bids</h3><p>{error}</p><button className="btn" onClick={() => location.reload()}>Try again</button></div>}
      {state === "ready" && (
        <>
          <div className="tabs" role="tablist" aria-label="My bids">{TABS.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} onClick={() => pick(k)}>{l}<span className="count tnum">{counts[k]}</span></button>)}</div>
          {list.length
            ? <ul className="brows" aria-live="polite">{list.map(r => <Row key={r.lot.id} r={r} isSaved={tab === "saved"} me={me} now={now} serverNow={be.feed.serverNow()} />)}</ul>
            : <div className="empty"><h3>{EMPTY[tab][0]}</h3><p>{EMPTY[tab][1]}</p><Link className="btn" to="/#classic">Browse cars</Link></div>}
        </>
      )}
    </main>
  );
}
