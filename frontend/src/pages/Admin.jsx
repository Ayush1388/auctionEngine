// Operator console: GET /v1/admin/reconcile and the dead-letter outbox.
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import { useRequireAuth, useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { toast } from "../lib/toast.js";
import { usd, ago } from "../../app/util.js";

export default function Admin() {
  useDocumentTitle("Marque | Operator console");
  const be = useBackend();
  const { redirect, isAdmin, signedIn } = useRequireAuth();
  const [books, setBooks] = useState(null);
  const [failed, setFailed] = useState([]);
  const [error, setError] = useState(null);
  const [checking, setChecking] = useState(false);
  const [gated, setGated] = useState(false);
  const [retrying, setRetrying] = useState(null);

  const load = useCallback(async manual => {
    if (manual) setChecking(true);
    try {
      const [b, f] = await Promise.all([be.reconcile(), be.failedOutbox()]);
      setBooks(b); setFailed(f); setError(null);
    } catch (e) {
      if (e.status === 403 || e.status === 401) setGated(true); else setError(e.message);
    }
    setChecking(false);
  }, [be]);
  useEffect(() => { if (isAdmin) load(); }, [isAdmin, load]);

  if (redirect) return redirect;
  const head = <PageHead title="Operator console" action={<Link className="btn btn-line" to="/status">System status</Link>}>Check that the money adds up, and put stuck events back in the queue.</PageHead>;
  if (!isAdmin || gated) {
    return (
      <main className="page ops">{head}
        <div className="empty"><h3>Operators only</h3>
          <p>This account is not an operator, so the API refuses these checks. {be.kind === "sim" ? "In the demo, sign in as the Operator account." : "Make an account an operator with: go run ./cmd/admin promote <email>"}</p>
          <Link className="btn" to="/account?next=/admin">{signedIn ? "Switch account" : "Sign in"}</Link></div>
      </main>
    );
  }
  const b = books;
  const retry = async id => {
    setRetrying(id);
    try { await be.retryOutbox(id); toast("Put back in the queue"); await load(); } catch (e) { toast(e.message); }
    setRetrying(null);
  };
  return (
    <main className="page ops">{head}
      <section className="ops-sec" aria-labelledby="bkH">
        <div className="sec-head"><h2 className="h2" id="bkH">Do the books balance?</h2><button className={"btn btn-line btn-sm" + (checking ? " busy" : "")} onClick={() => load(true)}>Run the check</button></div>
        {b ? (
          <>
            <div className={"verdict " + (b.ok ? "ok" : "bad")} role="status">
              <b>{b.ok ? "Balanced" : "Does not balance"}</b>
              <span>{b.ok ? "Every wallet equals the sum of its ledger lines, every journal sums to zero, and the held money equals the leading bids." : `${b.mismatches} wallets disagree with the ledger and ${b.unbalanced_journals} journals do not sum to zero. Do not move money until this is understood.`}</span>
            </div>
            <div className="tiles">
              <div className="tile"><span className="lab mono">Money deposited</span><b className="tnum">{usd(b.money_deposited)}</b><span className="small muted">all deposits ever made</span></div>
              <div className="tile"><span className="lab mono">In wallets</span><b className="tnum">{usd(b.money_in_wallets)}</b><span className="small muted">available plus held</span></div>
              <div className="tile"><span className="lab mono">Held in wallets</span><b className="tnum">{usd(b.reserved_in_wallets)}</b><span className="small muted">reserved by bids</span></div>
              <div className="tile"><span className="lab mono">Leading bids</span><b className="tnum">{usd(b.active_reservations_total)}</b><span className="small muted">on open auctions</span></div>
            </div>
            <p className="small muted">The check recomputes every balance from the append-only ledger and compares it with the stored balance. Money is never changed in place: each movement is a journal whose lines sum to zero.</p>
          </>
        ) : <div className="skel" style={{ height: 140 }} />}
      </section>

      <section className="ops-sec" aria-labelledby="dlH">
        <div className="sec-head"><h2 className="h2" id="dlH">Dead-letter outbox</h2><span className="mono muted">{failed.length} parked</span></div>
        <p className="small muted">An event that failed 8 times with backoff is parked here instead of blocking the queue. Fix what made it fail, then retry it.</p>
        {failed.length ? (
          <div className="tscroll"><table className="tbl"><thead><tr><th>Event</th><th>Attempts</th><th>Last error</th><th>Failed</th><th className="r"><span className="visually">Action</span></th></tr></thead>
            <tbody>{failed.map(e => (
              <tr key={e.id}><td><b>{e.event_type}</b><br /><span className="mono small muted">{String(e.id).slice(0, 13)}</span></td><td className="tnum">{e.attempts}</td><td className="err-text">{e.last_error || "no error recorded"}</td><td className="nowrap">{ago(e.failed_at)}</td>
                <td className="r"><button className={"btn btn-line btn-sm" + (retrying === e.id ? " busy" : "")} onClick={() => retry(e.id)}>Retry</button></td></tr>
            ))}</tbody></table></div>
        ) : <div className="empty"><h3>Nothing parked</h3><p>Every event has been delivered. If one fails 8 times it will appear here.</p></div>}
      </section>
      {error && <p className="form-error" role="alert">{error}</p>}
    </main>
  );
}
