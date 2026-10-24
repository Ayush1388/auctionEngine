// "Set a maximum bid": proxy bidding. The engine raises the person's bid for them, one step at a
// time, up to the maximum, so they do not have to watch the clock. Only the visible bid is ever
// held; the maximum is an instruction.
import { useCallback, useEffect, useState } from "react";
import { toast } from "../lib/toast.js";
import { parseUsd, usd } from "../../app/util.js";

export default function ProxyBox({ be, s }) {
  const [open, setOpen] = useState(false);
  const [proxy, setProxy] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [amount, setAmount] = useState("");
  const l = s.lot, me = s.me, live = l.status === "ACTIVE", step = l.step;

  const refresh = useCallback(async () => {
    if (!me) { setProxy(null); return; }
    try { setProxy(await be.getProxy(s.id)); setErr(""); } catch (e) { setErr(e.message); }
  }, [be, s, me]);
  // look again whenever the box is open and the lot moves
  useEffect(() => { if (open) refresh(); }, [open, refresh, l.current, l.bidderId]);

  if (!live && !proxy) return null;
  const leading = l.bidderId === me;

  const submit = async e => {
    e.preventDefault();
    const cents = parseUsd(amount);
    if (!cents) { setErr("Enter an amount in dollars."); return; }
    setBusy(true); setErr("");
    try {
      const p = await be.setProxy(s.id, cents);
      setProxy(p);
      toast(p.leading ? `Maximum set. You lead at ${usd(p.current_bid)}.` : `Maximum set to ${usd(p.max_amount)}.`);
      setAmount("");
      await s.reload();
    } catch (x) {
      setErr(x.status === 422 && x.body?.minimum_amount != null ? `The maximum must be at least ${usd(x.body.minimum_amount)}.`
        : x.status === 422 && /insufficient/i.test(x.message) ? "Not enough available funds to place even the minimum bid." : x.message);
    }
    setBusy(false);
  };
  const remove = async () => {
    setBusy(true);
    try { await be.cancelProxy(s.id); setProxy(null); toast("Maximum removed"); } catch (x) { setErr(x.message); }
    setBusy(false);
  };

  return (
    <details className="lt-proxy" id="proxy" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
      <summary>Set a maximum bid</summary>
      <div id="proxyBody">
        {!me ? <p>Sign in to set a maximum.</p> : (
          <>
            {proxy
              ? <p className="lt-proxy-state"><b>Your maximum is {usd(proxy.max_amount)}.</b> {leading
                  ? `You lead at ${usd(l.current)}. If someone outbids you, the engine bids for you in steps of ${usd(step)}.`
                  : proxy.max_amount < l.minNext ? "You were outbid past your maximum. Raise it to rejoin." : "Your maximum is still above the price; the engine will act on the next bid."}</p>
              : <p className="lt-proxy-state">Enter the most you will pay. When someone outbids you, the engine bids for you, {usd(step)} at a time, up to that. Only your visible bid is held, never the maximum.</p>}
            <form className="lt-proxy-form" onSubmit={submit} noValidate>
              <div className="lt-field"><label className="visually" htmlFor="proxyAmt">Your maximum in dollars</label>
                <input id="proxyAmt" inputMode="numeric" autoComplete="off" placeholder={usd(Math.max(l.minNext, proxy?.max_amount || 0) + step * 4)} value={amount} onChange={e => setAmount(e.target.value)} /></div>
              <button className="btn lt-go" type="submit" disabled={busy || !live}>{proxy ? "Change maximum" : "Set maximum"}</button>
            </form>
            {proxy && <button className="linkish" disabled={busy} onClick={remove}>Remove maximum</button>}
            {err && <p className="lt-proxy-err" role="alert">{err}</p>}
          </>
        )}
      </div>
    </details>
  );
}
