// Auction formats: Dutch and sealed-bid, run in the browser with the engine's money rules.
// Demo only. The API sells English auctions; this page says so and never touches it.
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { dutchDuration, dutchPrice, FormatLedger, settleSealed } from "../../app/formats.js";
import { usd } from "../../app/util.js";

const D = 100;
const dutchCfg = { start: 160_000 * D, floor: 90_000 * D, step: 2_500 * D, intervalMs: 700 };
const RIVALS = [["Bidder 184", 118_000], ["Bidder 291", 131_500], ["Bidder 407", 124_000], ["Bidder 52", 109_500]];
const RESERVE = 100_000;

function Dutch() {
  const [running, setRunning] = useState(false);
  const [sold, setSold] = useState(null);
  const [ledger, setLedger] = useState(null);
  const [el, setEl] = useState(0);
  const t0 = useRef(0);
  const raf = useRef(0);

  const tick = () => {
    setEl(performance.now() - t0.current);
    raf.current = requestAnimationFrame(tick);
  };
  useEffect(() => () => cancelAnimationFrame(raf.current), []);
  useEffect(() => {
    if (running && el >= dutchDuration(dutchCfg) + 2000) { cancelAnimationFrame(raf.current); setRunning(false); }
  }, [el, running]);

  const start = () => {
    const l = new FormatLedger(); l.open("you", 200_000 * D); l.open("seller");
    setLedger(l); setSold(null); setEl(0);
    t0.current = performance.now(); setRunning(true);
    cancelAnimationFrame(raf.current); raf.current = requestAnimationFrame(tick);
  };
  const accept = () => {
    if (!running) return;
    cancelAnimationFrame(raf.current);
    const price = dutchPrice(dutchCfg, performance.now() - t0.current);
    ledger.hold("you", price); ledger.settle("you", "seller", price);
    setSold({ price, saved: dutchCfg.start - price }); setRunning(false);
  };
  const price = sold ? sold.price : running ? dutchPrice(dutchCfg, el) : dutchCfg.start;
  const atFloor = running && el >= dutchDuration(dutchCfg);
  return (
    <section className="ops-sec dutch" aria-labelledby="duH">
      <h2 className="h2" id="duH">Dutch auction</h2>
      <p className="lead">The price starts high and falls. The first person to accept pays the price on the clock and the sale is over, so waiting is a bet that nobody else will buy first.</p>
      <b className="dutch-price tnum" aria-live="off">{usd(price)}</b>
      <p className="small muted">{atFloor ? "At the floor. Nobody has bought it." : `${running ? "Falls" : "Starts at " + usd(dutchCfg.start) + ", falls"} ${usd(dutchCfg.step)} every ${dutchCfg.intervalMs / 1000} s to a floor of ${usd(dutchCfg.floor)}`}</p>
      <div className="lab-controls">
        {running ? <button className="btn" onClick={accept}>Buy at the current price</button> : <button className="btn" onClick={start}>{sold ? "Run it again" : "Start the clock"}</button>}
      </div>
      {sold && ledger && <div className="verdict ok" role="status"><b>Sold at {usd(sold.price)}</b><span>That is {usd(sold.saved)} under the opening price. The seller received {usd(ledger.get("seller").available)}. You hold {usd(ledger.get("you").available)} of the {usd(200_000 * D)} you started with, and the journals {ledger.balanced() ? "all sum to zero" : "do not balance"}.</span></div>}
    </section>
  );
}

function Sealed() {
  const [rule, setRule] = useState("second");
  const [bid, setBid] = useState("125,000");
  const [result, setResult] = useState(null);

  const reveal = (nextRule = rule, nextBid = bid) => {
    const you = Math.max(1, Math.round(Number(String(nextBid).replace(/[^0-9]/g, "")) || 125_000)) * D;
    const L = new FormatLedger(); L.open("you", 300_000 * D); L.open("seller");
    const bids = [{ id: "you", amount: you, at: 0 }];
    RIVALS.forEach(([name, v], i) => { L.open(name, 400_000 * D); bids.push({ id: name, amount: v * D, at: i + 1 }); });
    bids.forEach(b => L.hold(b.id, b.amount));            // every bid is held when it is sent
    const r = settleSealed(bids, { rule: nextRule, reserve: RESERVE * D });
    for (const b of bids) { if (r.sold && b.id === r.winner) { L.settle(b.id, "seller", r.price); L.release(b.id, b.amount - r.price); } else L.release(b.id, b.amount); }
    setResult({ ...r, balanced: L.balanced() });
  };
  const pick = r => { setRule(r); if (result) reveal(r); };
  return (
    <section className="ops-sec" aria-labelledby="seH">
      <h2 className="h2" id="seH">Sealed-bid auction</h2>
      <p className="lead">Everyone sends one hidden bid before the close. The highest wins. What the winner pays depends on the rule.</p>
      <div className="presets" role="group" aria-label="Pricing rule">
        <button className="preset" aria-pressed={rule === "first"} onClick={() => pick("first")}>First price</button>
        <button className="preset" aria-pressed={rule === "second"} onClick={() => pick("second")}>Second price</button>
      </div>
      <p className="small muted">{rule === "first" ? "The winner pays their own bid, so the smart move is to shade it below what the car is worth to you." : "The winner pays the runner-up's bid (or the reserve). Bidding exactly what it is worth to you is the best strategy, because your bid never sets your price."}</p>
      <div className="lab-controls">
        <label className="field"><span className="field-label">Your sealed bid (reserve {usd(RESERVE * D)})</span><div className="money"><input inputMode="numeric" value={bid} onChange={e => setBid(e.target.value)} aria-label="Your sealed bid in dollars" /></div></label>
        <button className="btn" onClick={() => reveal()}>Seal and reveal</button>
      </div>
      {result && (
        <>
          <div className={"verdict " + (result.sold && result.winner === "you" ? "ok" : "bad")} role="status">
            <b>{!result.sold ? "No sale" : result.winner === "you" ? "You won" : `${result.winner} won`}</b>
            <span>{!result.sold ? "The highest bid was below the reserve." : `${result.winner === "you" ? "You pay" : "The winner pays"} ${usd(result.price)}${rule === "second" && result.ranked[0].amount !== result.price ? `, not their bid of ${usd(result.ranked[0].amount)}` : ""}. Losing bids were released in full; the ledger ${result.balanced ? "balances" : "does not balance"}.`}</span>
          </div>
          <div className="sealed-grid">{result.ranked.map(b => <div key={b.id} className={"sealed-card" + (result.sold && b.id === result.winner ? " is-winner" : "")}><span className="small muted">{b.id === "you" ? "You" : b.id}</span><b className="tnum">{usd(b.amount)}</b><span className="small">{result.sold && b.id === result.winner ? "winner" : "released"}</span></div>)}</div>
        </>
      )}
    </section>
  );
}

export default function Formats() {
  useDocumentTitle("Marque | Auction formats");
  return (
    <main className="page ops lab">
      <TourBar stage="beyond" />
      <PageHead title="Auction formats" action={<Link className="btn btn-line" to="/system">Architecture map</Link>}>
        The engine sells English auctions. Dutch and sealed-bid are the two other formats worth knowing, run here with the same money rules. Demo only: the API does not offer them.
      </PageHead>
      <p className="banner warn">Demo only. The Marque API sells English (ascending) auctions. These two run in your browser, on a small ledger that follows the same rules: hold the bid, release the losers, settle the winner, every journal sums to zero.</p>
      <div className="split-2"><Dutch /><Sealed /></div>
      <section className="ops-sec" aria-labelledby="cmpH" style={{ marginTop: "clamp(28px,4vw,52px)" }}>
        <h2 className="h2" id="cmpH">How they differ</h2>
        <div className="tscroll"><table className="tbl"><thead><tr><th>Format</th><th>Price is set by</th><th>Money held</th><th>Race condition to design for</th></tr></thead><tbody>
          <tr><td>English (this engine)</td><td>Bids that rise, one increment at a time</td><td>Only the leading bid</td><td>Many bids on one price; solved with a row lock and idempotency keys</td></tr>
          <tr><td>Dutch</td><td>A clock that falls</td><td>Only at the moment of acceptance</td><td>Two buyers accept in the same tick; the first commit wins, the second sees "already sold"</td></tr>
          <tr><td>Sealed first price</td><td>The highest hidden bid</td><td>Every bid, until the close</td><td>None while open; the close must read every bid at one instant</td></tr>
          <tr><td>Sealed second price</td><td>The runner-up's hidden bid</td><td>Every bid, until the close</td><td>As above; the price depends on a bid that lost</td></tr>
        </tbody></table></div>
      </section>
    </main>
  );
}
