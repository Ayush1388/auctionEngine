// One car on the home and event pages. It starts from the catalogue (so a page is never empty),
// then takes the backend's numbers, follows the lot live, and makes Bid place a real bid at the
// minimum, or send you to sign in.
import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import RollNumber from "./RollNumber.jsx";
import { useBackend } from "../hooks/backend.jsx";
import { useNow } from "../hooks/use.js";
import { toast } from "../lib/toast.js";
import { applySnapshot, secondsLeft } from "../../app/model.js";
import { clock, usd } from "../../app/util.js";

const ALIAS = { boss: "boss429" };

/** item: { id (catalogue key), img, alt, title, spec?, where, bid, step, bids, end (seconds), reserve, pos? } */
export default function LotCard({ item, lots }) {
  const be = useBackend();
  const navigate = useNavigate();
  const key = ALIAS[item.id] || item.id;
  const initial = lots?.get(key) || null;
  const [lot, setLot] = useState(initial);
  const [flash, setFlash] = useState(0);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const mounted = useRef(Date.now());
  useNow(1000);

  useEffect(() => { setLot(initial); }, [initial]);
  useEffect(() => {
    if (!lot) return undefined;
    return be.feed.subscribe(lot.id, ev => { setLot(l => applySnapshot(l, ev.auction)); if (ev.bid) setFlash(f => f + 1); });
  }, [be, lot?.id]);   // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (!flash) return undefined; const t = setTimeout(() => setFlash(0), 900); return () => clearTimeout(t); }, [flash]);

  const left = lot ? secondsLeft(lot, be.feed.serverNow()) : Math.max(0, item.end - Math.floor((Date.now() - mounted.current) / 1000));
  const open = lot ? lot.status === "ACTIVE" : left > 0;
  const current = lot ? (lot.current ?? lot.startingPrice) : item.bid;
  const next = lot ? lot.minNext : item.bid + item.step;
  const bids = lot ? lot.bids : item.bids;
  const href = `/lot/${lot ? lot.id : key}`;
  const time = lot ? (lot.status === "ACTIVE" ? clock(left) : lot.status === "COMPLETED" ? "Closed" : lot.status === "NOT_ACTIVE" ? "Soon" : "Cancelled") : (left > 0 ? clock(left) : "Closed");

  const bid = async () => {
    if (!lot) { toast("The backend is not answering, so there is nothing to bid on yet."); return; }
    if (!be.session.id) { navigate(`/account?next=${encodeURIComponent(location.pathname + location.search)}`); return; }
    setBusy(true);
    const r = await be.placeBid(lot.id, lot.minNext);
    setBusy(false);
    if (r.ok) toast("Bid placed: " + usd(r.amount));
    else if (r.error?.minimum != null) toast("Another bid landed first. The minimum is now " + usd(r.error.minimum));
    else toast(r.error?.message || "Could not place the bid");
  };

  return (
    <article className={"card" + (open && left < 1800 ? " urgent" : "") + (!open ? " closed" : "") + (flash ? " flash" : "")} id={"lot-" + item.id}>
      <div className="card-media">
        <Link className="card-link" to={href} aria-label={`Open ${item.title}`}><img src={item.img} alt={item.alt} loading="lazy" style={item.pos ? { objectPosition: item.pos } : undefined} /></Link>
        <button className={"heart" + (saved ? " on" : "")} aria-label={`Save ${item.title}`} aria-pressed={saved} onClick={() => setSaved(s => !s)}>{saved ? "♥" : "♡"}</button>
      </div>
      <div className="card-body">
        <h3><Link to={href}>{item.title}</Link></h3>
        {item.spec ? <p className="spec">{item.spec}</p> : null}
        <p className="where">{item.where}</p>
        <dl className="lot-stats">
          <div><dt>Current bid</dt><RollNumber as="dd" className="bid" value={current} format={usd} /></div>
          <div><dt>Ends in</dt><dd className="time">{time}</dd></div>
        </dl>
        <p className="lot-meta"><span className="bids">{bids} {bids === 1 ? "bid" : "bids"}</span><span className="reserve">{item.reserve}</span></p>
        <button className="btn btn-block bid-btn" disabled={!open || busy} onClick={bid}>{open ? <>Bid <span className="next">{usd(next)}</span></> : lot?.status === "COMPLETED" || (!lot && left <= 0) ? "Auction closed" : "Not open"}</button>
      </div>
    </article>
  );
}
