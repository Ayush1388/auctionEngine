// Trust page: attack the money, retry a bid, audit the books, watch the rate limiter. Everything is
// done through the same public calls a client uses; the page checks the answers itself instead of
// trusting a summary. The request half (retry, rate limit) and the money half (attack, audit) sit
// at different points of the backend tour; ?part= shows one of them.
import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle, useInterval } from "../hooks/use.js";
import { openLots, plural } from "../lib/lab.js";
import { toLot } from "../../app/model.js";
import { usd, uuid, sleep, dur, short } from "../../app/util.js";

/** A read that the shared per-IP limiter shed: wait as Retry-After says and ask again. */
async function patient(fn, tries = 4) {
  for (let i = 0; ; i++) {
    try { return await fn(); }
    catch (e) { if (e.status !== 429 || i >= tries) throw e; await sleep(Math.max(600, (e.retryAfter || 1) * 1000)); }
  }
}

/* ------------------------------------------------------------------ books balance */
function Books({ books }) {
  const { signedIn, isAdmin } = useAuth();
  const be = useBackend();
  if (!isAdmin) {
    return (
      <section className="ops-sec" aria-labelledby="bkH">
        <h2 className="h2" id="bkH">Do the books balance?</h2>
        <div className="empty"><h3>The audit is an operator check</h3>
          <p>It recomputes every wallet from the append-only ledger and checks that each journal sums to zero. {signedIn ? "This account is not an operator." : "Sign in as the operator to run it live while you attack the system below."}{be.kind === "sim" ? " In the demo the operator is admin@marque.test." : ""}</p>
          <Link className="btn" to="/account?next=/trust%3Fpart%3Dmoney">{signedIn ? "Switch account" : "Sign in"}</Link></div>
      </section>
    );
  }
  const { checks, balanced, report: r, err } = books;
  return (
    <section className="ops-sec" aria-labelledby="bkH">
      <div className="sec-head"><h2 className="h2" id="bkH">Do the books balance?</h2><span className="mono muted">{checks ? `checked ${plural(checks, "time")}, balanced ${balanced}` : "checking"}</span></div>
      {r ? (
        <>
          <div className={"verdict " + (r.ok ? "ok" : "bad")} role="status">
            <b>{r.ok ? "Balanced" : "Does not balance"}</b>
            <span>{r.ok ? "Every wallet equals the sum of its ledger lines, every journal sums to zero, and the held money equals the leading bids. Checked again every 3 seconds." : `${r.mismatches} wallets disagree with the ledger and ${r.unbalanced_journals} journals do not sum to zero.`}</span>
          </div>
          <div className="tiles">
            <div className="tile"><span className="lab mono">Money deposited</span><b className="tnum">{usd(r.money_deposited)}</b><span className="small muted">all deposits ever made</span></div>
            <div className="tile"><span className="lab mono">In wallets</span><b className="tnum">{usd(r.money_in_wallets)}</b><span className="small muted">available plus held</span></div>
            <div className="tile"><span className="lab mono">Held in wallets</span><b className="tnum">{usd(r.reserved_in_wallets)}</b><span className="small muted">reserved by bids</span></div>
            <div className="tile"><span className="lab mono">Leading bids</span><b className="tnum">{usd(r.active_reservations_total)}</b><span className="small muted">on open auctions</span></div>
          </div>
        </>
      ) : <div className="skel" style={{ height: 120 }} />}
      {err && <p className="form-error" role="alert">{err}</p>}
    </section>
  );
}

/* ------------------------------------------------------------------ double-spend attack */
const BUDGETS = [100_000, 400_000, 1_000_000];
const COUNTS = [10, 25, 50];

function AttackResult({ r }) {
  const rows = [
    ["Accepted", r.counts.accepted, "bids that became the leading bid"],
    ["Refused: not enough money", r.counts.funds, "the wallet said no"],
    ["Refused: price moved", r.counts.toolow, "another bid on that car landed first"],
    ["Shed by the rate limiter", r.counts.limited, "429 before the wallet was reached"],
    ["Other", r.counts.other, "unavailable, ended, or an error"],
  ];
  return (
    <>
      <div className={"verdict " + (r.ok ? "ok" : "bad")} role="status">
        <b>{r.ok ? "The wallet held" : "Something is wrong"}</b>
        <span>{plural(r.fired, "bid")} fired in the same instant from {r.actor} holding {usd(r.before.total)}. {plural(r.counts.accepted, "bid")} won, holding {usd(r.after.reserved - r.before.reserved)} more. The wallet ends at {usd(r.after.available)} available and {usd(r.after.reserved)} held, never below zero.</span>
      </div>
      <div className="split-2">
        <div>
          <h3 className="h3">What the server said</h3>
          <table className="tbl"><thead><tr><th>Outcome</th><th className="r">Bids</th></tr></thead>
            <tbody>{rows.map(([k, v, n]) => <tr key={k}><td>{k}<span className="small muted block">{n}</span></td><td className="r tnum">{v}</td></tr>)}</tbody></table>
          <p className="small muted">Answered in {dur(r.ms)} for the whole burst.</p>
        </div>
        <div>
          <h3 className="h3">What the page checked</h3>
          <ul className="checks">{r.checks.map(c => <li key={c.name} className={c.ok ? "ok" : "bad"}><b>{c.ok ? "Pass" : "Fail"}</b><span>{c.name}</span><em className="muted">{c.detail}</em></li>)}</ul>
        </div>
      </div>
    </>
  );
}

function Attack({ checkNow }) {
  const be = useBackend();
  const { signedIn } = useAuth();
  const sim = be.kind === "sim";
  const [bids, setBids] = useState(50);
  const [budget, setBudget] = useState(400_000);
  const [keepLimits, setKeepLimits] = useState(false);
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState(null);
  const [err, setErr] = useState(null);

  const run = async () => {
    if (running || !signedIn) return;
    setRunning(true); setErr(null);
    try {
      // an actor with a wallet: a fresh account with a set budget in the demo, your own wallet otherwise
      const actor = sim && be.trust ? await be.trust.attacker(budget * 100) : { label: "you", userId: be.session.id, wallet: () => be.wallet(), placeBid: (id, amt, key) => be.placeBid(id, amt, { key }) };
      const actorName = sim ? actor.label : "your account";
      if (sim && !keepLimits) await be.chaos.setRateLimits(false);
      const lots = (await patient(() => be.allAuctions({ status: "ACTIVE" }, 3))).filter(x => x.owner_id !== be.session.id && Date.parse(x.ends_at) - Date.now() > 25_000);
      if (lots.length < 3) throw new Error("There are not enough open lots to attack. Open lots: " + lots.length);
      const before = await patient(() => actor.wallet()); before.total = before.available + before.reserved;
      // every bid goes to the lot's current minimum; spread over the lots round-robin
      const shots = Array.from({ length: bids }, (_, i) => { const l = lots[i % lots.length]; return { id: l.id, amount: l.current_bid == null ? Math.max(l.starting_price, 1) : l.current_bid + l.min_increment }; });
      const t0 = performance.now();
      const answers = await Promise.all(shots.map(s => actor.placeBid(s.id, s.amount, uuid())));
      const ms = performance.now() - t0;
      const after = await patient(() => actor.wallet()); after.total = after.available + after.reserved;
      const counts = { accepted: 0, funds: 0, toolow: 0, limited: 0, other: 0 };
      const fundsRefused = [];
      answers.forEach((r, i) => {
        if (r.ok) counts.accepted++;
        else if (r.status === 429) counts.limited++;
        else if (r.status === 422 && /insufficient/i.test(r.error?.message || "")) { counts.funds++; fundsRefused.push(shots[i].amount); }
        else if (r.status === 422 && r.data?.minimum_amount != null) counts.toolow++;
        else counts.other++;
      });
      // what the server now says about who leads where: the held money must equal those bids
      const fresh = await patient(() => be.allAuctions({ status: "ACTIVE" }, 3));
      const leading = fresh.filter(x => x.current_bidder_id === actor.userId).reduce((s, x) => s + (x.current_bid || 0), 0);
      const cheapestRefused = fundsRefused.length ? Math.min(...fundsRefused) : null;
      const checks = [
        { name: "The wallet never went below zero", ok: after.available >= 0 && after.reserved >= 0, detail: `${usd(after.available)} available, ${usd(after.reserved)} held` },
        { name: "No money was created or destroyed", ok: after.total === before.total, detail: `${usd(before.total)} before, ${usd(after.total)} after` },
        { name: "Held money equals the leading bids", ok: after.reserved === leading, detail: `${usd(after.reserved)} held, ${usd(leading)} in bids this account leads` },
      ];
      if (cheapestRefused != null) checks.push({ name: "Every refusal for money was honest", ok: after.available < cheapestRefused, detail: `${usd(after.available)} left, the cheapest refused bid was ${usd(cheapestRefused)}` });
      else checks.push({ name: "More money than the bids needed", ok: true, detail: "nothing was refused for money; try a smaller budget" });
      setResult({ fired: shots.length, counts, before, after, checks, ms, actor: actorName, ok: checks.every(c => c.ok) });
      checkNow();
    } catch (e) { setErr(e.message || String(e)); }
    finally {
      if (sim && !keepLimits) { try { await be.chaos.setRateLimits(true); } catch { /* demo only */ } }
      setRunning(false);
    }
  };

  return (
    <section className="ops-sec" aria-labelledby="atH">
      <h2 className="h2" id="atH">Double-spend attack</h2>
      <p className="lead">One wallet, many bids, all in the same instant, each on a different car. If the wallet could be spent twice, more would win than the money allows.</p>
      <div className="lab-controls">
        <div className="field"><span className="field-label">Bids fired at once</span>
          <div className="presets" role="group" aria-label="Bids fired at once">{COUNTS.map(n => <button key={n} className="preset" aria-pressed={bids === n} disabled={running} onClick={() => setBids(n)}>{n}</button>)}</div></div>
        {sim && <div className="field"><span className="field-label">Money in the wallet</span>
          <div className="presets" role="group" aria-label="Money in the wallet">{BUDGETS.map(n => <button key={n} className="preset" aria-pressed={budget === n} disabled={running} onClick={() => setBudget(n)}>{usd(n * 100)}</button>)}</div></div>}
        <button className={"btn" + (running ? " busy" : "")} disabled={running || !signedIn} onClick={run}>Fire {bids} bids</button>
      </div>
      {sim
        ? <label className="check"><input type="checkbox" checked={keepLimits} disabled={running} onChange={e => setKeepLimits(e.target.checked)} /> Keep the rate limiter on <span className="muted">(it sheds most of a burst before the wallet is reached)</span></label>
        : <p className="small muted">This uses your own wallet on the real API. If rate limits are on, the gateway sheds most of the burst first (429); start the API with RATE_LIMITS=off to put the wallet alone to the test.</p>}
      {!signedIn && <p className="form-error">Sign in first: the attack bids with your account. <Link to="/account?next=/trust%3Fpart%3Dmoney" className="linkish">Sign in</Link></p>}
      {err && <p className="form-error" role="alert">{err}</p>}
      {result ? <AttackResult r={result} /> : <p className="small muted">The page reads the wallet before and after, then checks the result against the ledger rules itself.</p>}
    </section>
  );
}

/* ------------------------------------------------------------------ retry with one key */
function Retry({ checkNow }) {
  const be = useBackend();
  const { signedIn } = useAuth();
  const [lots, setLots] = useState([]);
  const [lotId, setLotId] = useState("");
  const [copies, setCopies] = useState(5);
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState(null);
  const [err, setErr] = useState(null);

  useEffect(() => {
    openLots(be).then(l => { l.sort((a, b) => a.lot.minNext - b.lot.minNext); setLots(l); setLotId(l[0]?.id || ""); }).catch(e => setErr(e.message));
  }, [be]);

  /** A bid that was shed with 429 is sent again, with the same key, after Retry-After. */
  const bidWithRetry = async (id, amount, key, tries = 4) => {
    let r, n = 0;
    do {
      r = await be.placeBid(id, amount, { key });
      n++;
      if (r.status !== 429) break;
      await sleep(Math.max(600, (r.error?.retryAfter || 1) * 1000));
    } while (n <= tries);
    return { ...r, tries: n };
  };

  const run = async () => {
    if (!lotId || running) return;
    setRunning(true); setErr(null);
    try {
      const key = uuid();
      // the price may have moved since the list was drawn: ask for the minimum now
      const amount = toLot(await patient(() => be.getAuction(lotId))).minNext;
      const w0 = await patient(() => be.wallet());
      const answers = await Promise.all(Array.from({ length: copies }, () => bidWithRetry(lotId, amount, key)));
      const w1 = await patient(() => be.wallet());
      const ids = answers.filter(x => x.ok && x.data?.bid).map(x => x.data.bid.id);
      const distinct = new Set(ids).size;
      const history = await patient(() => be.bids(lotId, { limit: 50 }));
      const stored = history.bids.filter(b => ids.includes(b.id)).length;
      setResult({
        key, amount, copies, distinct, replayed: answers.filter(x => x.ok && x.replayed).length, heldDelta: w1.reserved - w0.reserved,
        rows: answers.map(x => ({ ok: x.ok, status: x.status, replayed: x.replayed, bid: x.data?.bid?.id, ms: x.ms, tries: x.tries })),
        ok: distinct === 1 && stored <= 1 && answers.every(x => x.ok),
      });
      if (!answers.every(x => x.ok)) setErr("Some copies were refused: " + [...new Set(answers.filter(x => !x.ok).map(x => x.error?.message))].join(", "));
      checkNow();
    } catch (e) { setErr(e.message); }
    setRunning(false);
  };

  return (
    <section className="ops-sec" aria-labelledby="rtH">
      <h2 className="h2" id="rtH">Retry with the same key</h2>
      <p className="lead">A phone loses signal after a bid is sent. The app cannot know whether it landed, so it sends it again. With one Idempotency-Key the server places one bid and recognises the rest.</p>
      <div className="lab-controls">
        <label className="field"><span className="field-label">Lot</span>
          <select value={lotId} disabled={running} onChange={e => setLotId(e.target.value)}>{lots.map(l => <option key={l.id} value={l.id}>{l.lot.title} (minimum {usd(l.lot.minNext)})</option>)}</select></label>
        <label className="field"><span className="field-label">Copies sent</span>
          <select value={copies} disabled={running} onChange={e => setCopies(+e.target.value)}>{[3, 5, 10].map(n => <option key={n}>{n}</option>)}</select></label>
        <button className={"btn" + (running ? " busy" : "")} disabled={running || !lots.length || !signedIn} onClick={run}>Send the same bid {copies} times</button>
      </div>
      {!signedIn && <p className="form-error">Sign in first. <Link className="linkish" to="/account?next=/trust%3Fpart%3Drequest">Sign in</Link></p>}
      {err && <p className="form-error" role="alert">{err}</p>}
      {result ? (
        <>
          <div className={"verdict " + (result.ok ? "ok" : "bad")} role="status"><b>{result.ok ? "One bid" : result.distinct === 0 ? "No bid was placed" : "More than one bid"}</b>
            <span>{plural(result.copies, "request")} carried the key <code>{short(result.key)}</code>. The server created {plural(result.distinct, "bid")} ({usd(result.amount)}) and answered the other {result.replayed} as repeats. Your hold changed by {usd(result.heldDelta)}, once.</span></div>
          <div className="tscroll"><table className="tbl"><thead><tr><th>Copy</th><th>Status</th><th>Marked as replay</th><th>Bid</th><th>Tries</th><th className="r">Time</th></tr></thead>
            <tbody>{result.rows.map((x, i) => <tr key={i}><td className="tnum">{i + 1}</td><td><span className={"tag " + (x.ok ? (x.replayed ? "tag-warn" : "tag-ok") : "tag-bad")}>{x.status}</span></td><td>{x.replayed ? "yes" : "no"}</td><td><code>{x.bid ? short(x.bid) : "-"}</code></td><td className="tnum">{x.tries}</td><td className="r tnum">{dur(x.ms)}</td></tr>)}</tbody></table></div>
        </>
      ) : <p className="small muted">Requests are sent in parallel, which is the hard case: two copies can reach the server at the same instant. A copy the rate limiter sheds is sent again with the same key after Retry-After.</p>}
    </section>
  );
}

/* ------------------------------------------------------------------ token bucket */
function Bucket() {
  const be = useBackend();
  const { signedIn } = useAuth();
  const sim = be.kind === "sim";
  const [running, setRunning] = useState(false);
  const [samples, setSamples] = useState([]);
  const [limit, setLimit] = useState(null);
  const [off, setOff] = useState(false);
  const [err, setErr] = useState(null);
  const cap = limit || 10, last = samples.at(-1), left = last?.remaining ?? cap;

  const burst = async () => {
    if (running) return;
    setRunning(true); setErr(null); setSamples([]);
    try {
      if (sim) await be.chaos.setRateLimits(true);
      const lots = (await be.allAuctions({ status: "ACTIVE" }, 1)).filter(x => x.owner_id !== be.session.id);
      if (!lots.length) throw new Error("There is no open lot to aim at.");
      const out = await Promise.all(Array.from({ length: 15 }, () => be.placeBid(lots[0].id, 1, { key: uuid() })));
      const s = out.map(r => ({ status: r.status, remaining: r.rate?.remaining ?? null, limit: r.rate?.limit ?? null }));
      const withRate = s.filter(x => x.limit != null);
      setOff(!withRate.length);
      if (withRate[0]) setLimit(withRate[0].limit);
      s.sort((x, y) => (x.status === 429) - (y.status === 429));
      const known = withRate.map(x => x.remaining).filter(v => v != null);
      if (known.length) s[s.length - 1].remaining = Math.min(...known);
      setSamples(s);
    } catch (e) { setErr(e.message); }
    setRunning(false);
  };

  return (
    <section className="ops-sec" aria-labelledby="tbH">
      <h2 className="h2" id="tbH">Rate limit bucket</h2>
      <p className="lead">Every bidder owns a bucket of {cap} tokens that refills at 5 a second. Each bid takes one. When it is empty the gateway answers 429 with Retry-After, before any database work.</p>
      <div className="bucket" role="img" aria-label={`${left} of ${cap} tokens left`}>
        {Array.from({ length: cap }, (_, i) => <i key={i} className={i < left ? "on" : ""} />)}
        <b className="tnum">{left}<span className="muted"> / {cap}</span></b>
      </div>
      <div className="lab-controls">
        <button className={"btn btn-line" + (running ? " busy" : "")} disabled={running || !signedIn} onClick={burst}>Send 15 requests in a burst</button>
        <span className="small muted">They are bids at a price too low to place, so the rules refuse them and nothing changes hands. The limiter still counts each one.</span>
      </div>
      {err && <p className="form-error" role="alert">{err}</p>}
      {off && <p className="small muted">This API sends no RateLimit headers, so rate limits are off (RATE_LIMITS=off).</p>}
      {samples.length > 0 && (
        <>
          <ol className="burst">{samples.map((s, i) => <li key={i} className={s.status === 429 ? "bad" : "ok"} title={`Request ${i + 1}: ${s.status}`}><span className="tnum">{s.status}</span></li>)}</ol>
          <p className="small muted">{samples.filter(s => s.status === 429).length} of {samples.length} were shed with 429. A 422 means the request got past the limiter and was refused by the rules.</p>
        </>
      )}
    </section>
  );
}

/* ------------------------------------------------------------------ page */
export default function Trust() {
  const be = useBackend();
  const { isAdmin } = useAuth();
  const [params] = useSearchParams();
  const part = params.get("part");
  const showMoney = part !== "request", showRequest = part !== "money";
  const head = {
    request: ["The request", "Before a bid reaches the rules it has to get past the gateway. A retry must never bid twice, and a burst must be shed before it costs any database work."],
    money: ["The money", "Money is the part of an auction nobody can afford to get wrong. Attack it and audit it from here, and read the result yourself."],
  }[part] || ["Trust page", "Money is the part of an auction nobody can afford to get wrong. Attack it, retry it and audit it from here, and read the result yourself."];
  useDocumentTitle("Marque | " + head[0]);

  const [books, setBooks] = useState({ checks: 0, balanced: 0, report: null, err: null });
  const checkNow = useCallback(async () => {
    if (!be.session.isAdmin) return;
    try {
      const r = await patient(() => be.reconcile(), 1);
      setBooks(b => ({ checks: b.checks + 1, balanced: b.balanced + (r.ok ? 1 : 0), report: r, err: null }));
    } catch (e) { setBooks(b => ({ ...b, err: e.message })); }
  }, [be]);
  useEffect(() => { if (showMoney) checkNow(); }, [showMoney, checkNow, isAdmin]);
  useInterval(() => { if (showMoney && !document.hidden) checkNow(); }, 3000);

  if (be.unreachable) {
    return <main className="page ops lab"><PageHead title={head[0]}>{head[1]}</PageHead>
      <div className="empty"><h3>The API is not answering</h3><p>The trust checks need an API to talk to. Nothing is listening at {be.base}.</p><a className="btn" href="?engine=sim">Use the demo engine</a></div></main>;
  }
  return (
    <main className="page ops lab">
      <TourBar stage={part === "request" ? "request" : "money"} />
      <PageHead title={head[0]} action={<Link className="btn btn-line" to="/admin">Operator console</Link>}>{head[1]}</PageHead>
      <div className="trust-body">
        {showRequest && <Retry checkNow={checkNow} />}
        {showRequest && <Bucket />}
        {showMoney && <Books books={books} />}
        {showMoney && <Attack checkNow={checkNow} />}
      </div>
    </main>
  );
}
