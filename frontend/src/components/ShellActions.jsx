// The account area of the header: sign in, or wallet, alerts and the account menu.
import { useCallback, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useDismiss, useEmitter } from "../hooks/use.js";
import { shell, lastWallet, loadNotes, markAllRead, markRead } from "../lib/shell.js";
import { usd, ago } from "../../app/util.js";

function Popover({ label, panelClass, button, children }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(ref, open, close);
  return (
    <div className="pop-wrap" ref={ref}>
      {button({ open, toggle: () => setOpen(o => !o) })}
      {open && <div className={"pop " + panelClass} aria-label={label}>{children(close)}</div>}
    </div>
  );
}

function Alerts() {
  const notes = useEmitter(shell, "alerts", () => loadNotes());
  const unread = notes.filter(n => !n.read).length;
  return (
    <Popover label="Alerts" panelClass="pop-alerts"
      button={({ open, toggle }) => (
        <button className="bell" aria-expanded={open} aria-label="Alerts" onClick={toggle}>
          <span className="mono">Alerts</span>{unread > 0 && <b>{unread}</b>}
        </button>
      )}>
      {close => (
        <>
          <div className="pop-head"><h3>Alerts</h3>{unread > 0 && <button className="linkish" onClick={markAllRead}>Mark all read</button>}</div>
          {notes.length ? (
            <ul className="alerts">
              {notes.slice(0, 12).map(n => (
                <li key={n.id} className={n.read ? "" : "unread"}>
                  <Link to={String(n.href || "/").startsWith("/") ? n.href : "/"} onClick={() => { markRead(n.id); close(); }}>
                    <b>{n.title}</b><span>{n.body}</span><time className="mono">{ago(n.at)}</time>
                  </Link>
                </li>
              ))}
            </ul>
          ) : <p className="pop-empty">Nothing yet. You will hear here when someone outbids you or an auction you bid on closes.</p>}
        </>
      )}
    </Popover>
  );
}

export default function ShellActions() {
  const be = useBackend();
  const { user } = useAuth();
  const loc = useLocation();
  const navigate = useNavigate();
  const wallet = useEmitter(shell, "wallet", x => x);
  const w = wallet === undefined ? lastWallet : wallet;

  if (!user) return <Link className="btn" to={`/account?next=${encodeURIComponent(loc.pathname + loc.search)}`}>Sign in</Link>;
  const initial = (user.email || "?")[0].toUpperCase();
  return (
    <>
      <Link className="chip" to="/wallet" aria-label="Wallet">
        <span className="mono">Available</span><b>{w ? usd(w.available) : "-"}</b>
        {w?.reserved ? <span className="chip-held">{usd(w.reserved)} held</span> : null}
      </Link>
      <Alerts />
      <Popover label="Account" panelClass="pop-menu"
        button={({ open, toggle }) => (
          <button className="acct" aria-expanded={open} onClick={toggle}>
            <span className="acct-i" aria-hidden="true">{initial}</span><span className="acct-n">{(user.email || "").split("@")[0]}</span>
          </button>
        )}>
        {close => (
          <>
            <p className="mono pop-who">{user.email}{user.role === "admin" ? " (operator)" : ""}</p>
            <Link to="/bids" onClick={close}>My bids</Link>
            <Link to="/wallet" onClick={close}>Wallet and statement</Link>
            <Link to="/sell" onClick={close}>Sell a car</Link>
            <Link to="/sell#listings" onClick={close}>My listings</Link>
            {user.role === "admin" && <Link to="/admin" onClick={close}>Operator console</Link>}
            <button onClick={async () => { close(); await be.logout(); navigate("/"); }}>Sign out</button>
          </>
        )}
      </Popover>
    </>
  );
}
