// Wallet: balances, adding funds, and the statement (the double-entry ledger).
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import { useBackend, useRequireAuth } from "../hooks/backend.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { refreshWallet, shell } from "../lib/shell.js";
import { toast } from "../lib/toast.js";
import { toLot } from "../../app/model.js";
import { usd, parseUsd, dateTime, uuid } from "../../app/util.js";

const LIMIT = 1_000_000_000;                     // the API's cap on one deposit, in cents ($10,000,000)
const PRESETS = [50_000, 250_000, 1_000_000].map(d => d * 100);
const KIND = { DEPOSIT: "Funds added", RESERVE: "Held for bid", RELEASE: "Hold released", SETTLE: "Sale settled" };
const FILTERS = [["all", "All"], ["DEPOSIT", "Deposits"], ["RESERVE", "Holds"], ["RELEASE", "Releases"], ["SETTLE", "Sales"]];
const titleCache = new Map();

/** The API returns one line per account per journal; a person reads one row per journal. */
function journalsOf(entries) {
  const by = new Map();
  for (const e of entries) {
    const j = by.get(e.transaction_id) || { id: e.transaction_id, kind: e.kind, auction: e.auction_id, at: e.created_at, lines: [], seq: e.id };
    j.lines.push(e); j.seq = Math.max(j.seq, e.id); by.set(e.transaction_id, j);
  }
  return [...by.values()].sort((a, b) => b.seq - a.seq);
}

function Funds({ wallet, onDeposit }) {
  const [amount, setAmount] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const intent = useRef(null);
  const w = wallet, held = w ? w.reserved / (w.total || 1) * 100 : 0;

  const submit = async e => {
    e.preventDefault();
    const cents = parseUsd(amount);
    if (!cents) return setErr("Enter an amount.");
    if (cents > LIMIT) return setErr(`One deposit is limited to ${usd(LIMIT)}.`);
    setErr("");
    // one intent = one key: clicking twice on the same amount is the same deposit
    if (!intent.current || intent.current.cents !== cents || Date.now() - intent.current.at > 5000) intent.current = { cents, key: uuid(), at: Date.now() };
    intent.current.at = Date.now();
    setBusy(true);
    try { await onDeposit(cents, intent.current.key); toast(`${usd(cents)} added`); setAmount(""); }
    catch (x) { setErr(x.status === 422 ? "That amount was refused. Try a smaller one." : x.message); }
    setBusy(false);
  };
  const preset = c => setAmount((c / 100).toLocaleString("en-US"));
  const pressed = c => parseUsd(amount) === c;
  return (
    <>
      <div className="bal">
        <span className="mono lab">Available to bid</span>
        <b className="bal-big tnum">{w ? usd(w.available) : "-"}</b>
        <div className="bal-two">
          <div><span className="mono lab">Held in bids</span><b className={"tnum" + (w?.reserved ? " pos" : "")}>{w ? usd(w.reserved) : "-"}</b></div>
          <div><span className="mono lab">Total</span><b className="tnum">{w ? usd(w.total) : "-"}</b></div>
        </div>
        <div className="ratio" role="img" aria-label={w ? `${Math.round(held)} percent of your money is held in bids` : ""}><i style={{ width: `${held.toFixed(1)}%` }} /><b style={{ width: `${(100 - held).toFixed(1)}%` }} /></div>
        <p className="small muted">The red part is held while you lead a lot. It returns to available the moment you are outbid.</p>
      </div>
      <form className="fund" onSubmit={submit} noValidate>
        <h2 className="h2">Add funds</h2>
        <div className="presets" role="group" aria-label="Amounts">{PRESETS.map(c => <button key={c} type="button" className="preset" aria-pressed={pressed(c)} onClick={() => preset(c)}>{usd(c)}</button>)}</div>
        <div className={"field" + (err ? " invalid" : "")}><label htmlFor="fundAmt">Amount in dollars</label>
          <div className="money"><input id="fundAmt" inputMode="numeric" autoComplete="off" placeholder="250,000" value={amount} onChange={e => setAmount(e.target.value)} /></div><span className="err">{err}</span></div>
        <button className={"btn btn-block" + (busy ? " busy" : "")} disabled={busy}>Add funds</button>
        <p className="small muted">These are test funds. Each deposit carries an Idempotency-Key, so a double click or a retry adds the money once.</p>
      </form>
    </>
  );
}

function Statement({ entries, nextBefore, onOlder }) {
  const be = useBackend();
  const [filter, setFilter] = useState("all");
  const [open, setOpen] = useState(() => new Set());
  const [names, setNames] = useState({});
  const js = useMemo(() => journalsOf(entries).filter(j => filter === "all" || j.kind === filter), [entries, filter]);

  useEffect(() => {
    let on = true;
    const ids = [...new Set(js.map(j => j.auction).filter(Boolean))].filter(id => !(id in names));
    ids.forEach(async id => {
      const key = be.kind + id;
      if (!titleCache.has(key)) titleCache.set(key, be.getAuction(id).then(a => toLot(a).title).catch(() => "a lot"));
      const t = await titleCache.get(key);
      if (on) setNames(n => ({ ...n, [id]: t }));
    });
    return () => { on = false; };
  }, [js, be]);   // eslint-disable-line react-hooks/exhaustive-deps

  const money = v => v ? <span className={"tnum " + (v > 0 ? "pos" : "neg")}>{v > 0 ? "+" : "-"}{usd(Math.abs(v))}</span> : <span className="muted">-</span>;
  return (
    <>
      <div className="tabs" role="tablist" aria-label="Filter the statement">{FILTERS.map(([k, l]) => <button key={k} role="tab" aria-selected={filter === k} onClick={() => setFilter(k)}>{l}</button>)}</div>
      {js.length ? (
        <>
          <table className="tbl stmt"><thead><tr><th>Date</th><th>Movement</th><th className="r">Available</th><th className="r">Held</th><th className="r"><span className="visually">Details</span></th></tr></thead>
            <tbody>
              {js.map(j => {
                const av = j.lines.filter(l => l.account === "available").reduce((s, l) => s + l.amount, 0);
                const rs = j.lines.filter(l => l.account === "reserved").reduce((s, l) => s + l.amount, 0);
                const sum = j.lines.reduce((s, l) => s + l.amount, 0);
                const what = j.kind === "SETTLE" ? (av > 0 ? "Sold" : "Paid for") : (KIND[j.kind] || j.kind);
                const isOpen = open.has(j.id);
                return [
                  <tr key={j.id}>
                    <td className="nowrap">{dateTime(j.at)}</td>
                    <td><b>{what}</b>{names[j.auction] ? <><br /><Link className="small" to={`/lot/${j.auction}`}>{names[j.auction]}</Link></> : null}</td>
                    <td className="r">{money(av)}</td><td className="r">{money(rs)}</td>
                    <td className="r"><button className="linkish" aria-expanded={isOpen} aria-controls={`j-${j.id}`} onClick={() => setOpen(s => { const n = new Set(s); isOpen ? n.delete(j.id) : n.add(j.id); return n; })}>{isOpen ? "Hide" : "Lines"}</button></td>
                  </tr>,
                  isOpen && (
                    <tr className="jl" id={`j-${j.id}`} key={j.id + "l"}><td colSpan={5}>
                      <ul className="lines">{[...j.lines].sort((a, b) => a.id - b.id).map(l => <li key={l.id}><span className="mono">{l.account}</span><span className={"tnum " + (l.amount > 0 ? "pos" : "neg")}>{l.amount > 0 ? "+" : "-"}{usd(Math.abs(l.amount))}</span></li>)}</ul>
                      <p className="small muted">{j.lines.length > 1 ? (sum === 0 ? "Both lines are yours and they add up to zero: money moved between your two accounts and none was created." : "Lines do not add up here.") : "The other line of this journal is on another account (the funding source or the seller), so it is not shown here."} Journal <span className="mono">{j.id.slice(0, 13)}</span></p>
                    </td></tr>
                  ),
                ];
              })}
            </tbody></table>
          {nextBefore != null && <div className="more"><button className="btn btn-line btn-sm" onClick={onOlder}>Load older entries</button></div>}
        </>
      ) : (
        <div className="empty"><h3>{filter === "all" ? "Nothing here yet" : "No entries of this kind"}</h3>
          <p>{filter === "all" ? "Add funds, then place a bid. Each hold, release and sale shows up here the moment it happens." : "Try another filter."}</p>
          {filter === "all" && <Link className="btn" to="/#classic">Browse cars</Link>}</div>
      )}
    </>
  );
}

export default function Wallet() {
  useDocumentTitle("Marque | Wallet");
  const be = useBackend();
  const { redirect } = useRequireAuth();
  const [wallet, setWallet] = useState(null);
  const [entries, setEntries] = useState([]);
  const [nextBefore, setNextBefore] = useState(null);
  const [error, setError] = useState(null);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async before => {
    const p = await be.ledger({ limit: 50, before });
    setEntries(e => (before ? e.concat(p.entries) : p.entries));
    setNextBefore(p.next_before);
  }, [be]);

  useEffect(() => {
    if (redirect) return;
    (async () => {
      try { setWallet(await be.wallet()); await load(); setLoaded(true); setError(null); } catch (e) { setError(e.message); }
    })();
  }, [be, load, !!redirect]);   // eslint-disable-line react-hooks/exhaustive-deps

  // a bid anywhere changes the wallet: follow it
  useEffect(() => shell.on("wallet", async w => {
    if (!w) return;
    setWallet(prev => (!prev || w.available !== prev.available || w.reserved !== prev.reserved ? w : prev));
    try { await load(); } catch { /* the next change tries again */ }
  }), [load]);

  const deposit = async (cents, key) => {
    setWallet(await be.deposit(cents, key));
    refreshWallet();
    await load();
  };

  if (redirect) return redirect;
  return (
    <main className="page" id="main">
      <PageHead title="Wallet">Money you bid is held here, then released if you are outbid or paid out if you win. Every movement is on the statement.</PageHead>
      {error ? <div className="empty"><h3>Could not load your wallet</h3><p>{error}</p><button className="btn" onClick={() => location.reload()}>Try again</button></div> : (
        <div className="split">
          <section className="wallet-card" aria-label="Balance">{wallet ? <Funds wallet={wallet} onDeposit={deposit} /> : <div className="skel" style={{ height: 260 }} />}</section>
          <section aria-label="Statement"><h2 className="h2">Statement</h2>
            {loaded ? <Statement entries={entries} nextBefore={nextBefore} onOlder={() => load(nextBefore)} /> : <div className="skel" style={{ height: 320, marginTop: 16 }} />}</section>
        </div>
      )}
    </main>
  );
}
