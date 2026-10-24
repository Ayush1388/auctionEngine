// Sign in or create an account: an email and a password.
import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { DEMO_ACCOUNTS, DEMO_PASSWORD } from "../../app/demo.js";
import { checkPassword } from "../../app/passwordPolicy.js";

/** Only same-site paths: a login link must not become an open redirect. */
const safeNext = n => (/^\/(?!\/)[^\s]*$/.test(n || "") ? n : "/");

function errorsFor(e, map = {}) {
  // field errors from the server go under their fields; anything else becomes the form message
  const fields = {};
  for (const [k, v] of Object.entries(e.fields || {})) fields[map[k] || k] = v;
  const msg = Object.keys(fields).length ? "" : e.status === 429 ? `Too many attempts. Try again${e.retryAfter ? ` in ${e.retryAfter} seconds` : " in a moment"}.` : e.network ? "Cannot reach the server. Check your connection and try again." : e.message;
  return { fields, msg };
}

function Field({ id, label, err, hint, ...input }) {
  return (
    <div className={"field" + (err ? " invalid" : "")}>
      <label htmlFor={id}>{label}</label>
      <input id={id} aria-invalid={!!err} aria-describedby={err ? `${id}-e` : undefined} {...input} />
      {hint ? <span className="hint">{hint}</span> : null}
      <span className="err" id={`${id}-e`}>{err || ""}</span>
    </div>
  );
}

export default function Account() {
  useDocumentTitle("Marque | Sign in");
  const be = useBackend();
  const { user } = useAuth();
  const navigate = useNavigate();
  const [q, setQ] = useSearchParams();
  const next = safeNext(q.get("next"));
  const [tab, setTab] = useState(q.get("tab") === "create" ? "create" : "signin");
  const [state, setState] = useState({});                  // msg, fields
  const [busy, setBusy] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const policy = checkPassword(password, email);

  const goTab = t => { setTab(t); setState({}); };
  const done = () => navigate(next === "/" ? "/bids" : next);

  const signIn = async (e, p) => {
    setBusy(true);
    try { await be.login(e, p); done(); }
    catch (x) { const { fields, msg } = errorsFor(x); setState({ fields, msg: x.status === 401 ? "Wrong email or password." : msg }); setBusy(false); }
  };
  // creating an account signs you straight in: there is no email to wait for
  const register = async () => {
    if (!policy.ok) { setState({ fields: { password: "Your password does not meet the policy yet." } }); return; }
    setBusy(true);
    try { await be.register(email.trim(), password); await be.login(email.trim(), password); done(); }
    catch (x) { const { fields, msg } = errorsFor(x); setState({ fields, msg: x.status === 409 ? "An account with that email already exists. Sign in instead." : msg }); setBusy(false); }
  };
  const submit = async e => {
    e.preventDefault();
    if (tab === "signin") await signIn(email.trim(), password);
    else await register();
  };

  let body;
  if (user) {
    body = (
      <>
        <h1 id="authTitle" className="h1">You are signed in</h1>
        <p className="lead">{user.email}{user.role === "admin" ? " (operator)" : ""}</p>
        <div className="pop-actions">
          <Link className="btn" to={next === "/" ? "/bids" : next}>{next === "/" ? "See my bids" : "Continue"}</Link>
          <Link className="btn btn-line" to="/wallet">Wallet</Link>
          <button className="btn btn-line" onClick={async () => { await be.logout(); setState({}); }}>Sign out</button>
        </div>
      </>
    );
  } else {
    const f = state.fields || {};
    body = (
      <>
        <h1 id="authTitle" className="h1">{tab === "signin" ? "Sign in" : "Create your account"}</h1>
        <div className="tabs" role="tablist" aria-label="Account">
          {[["signin", "Sign in"], ["create", "Create account"]].map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} onClick={() => goTab(k)}>{l}</button>)}
        </div>
        <div className="auth-panel">
          {state.msg && <p className="form-error" role="alert">{state.msg}</p>}
          {tab === "signin" && (
            <>
              <form onSubmit={submit} noValidate className="stack">
                <Field id="email" label="Email" type="email" autoComplete="username" required value={email} onChange={e => setEmail(e.target.value)} err={f.email} />
                <Field id="password" label="Password" type="password" autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)} err={f.password} />
                <button className={"btn btn-block" + (busy ? " busy" : "")} disabled={busy}>Sign in</button>
              </form>
              <div className="auth-demo">
                <h2 className="h2">Try it as someone</h2>
                <p className="small muted">{be.kind === "sim" ? "These accounts live in this browser's demo engine, funded with $2,000,000." : "These accounts exist once the demo cars are loaded, funded with $2,000,000."} Open a second window and sign in as someone else to see two people bid on one lot.{be.kind === "sim" ? "" : " The operator account is private: sign in with its own email and password."}</p>
                <div className="presets">{DEMO_ACCOUNTS.filter(a => be.kind === "sim" || a.role !== "admin").map(a => <button key={a.email} type="button" className="preset" onClick={() => signIn(a.email, DEMO_PASSWORD)}>{a.name}</button>)}</div>
                <p className="small muted mono">Password for all: {DEMO_PASSWORD}</p>
              </div>
            </>
          )}
          {tab === "create" && (
            <form onSubmit={submit} noValidate className="stack">
              <Field id="email" label="Email" type="email" autoComplete="username" required value={email} onChange={e => setEmail(e.target.value)} err={f.email} />
              <Field id="password" label="Password" type="password" autoComplete="new-password" required value={password} onChange={e => setPassword(e.target.value)} err={f.password} />
              <div className="policy" aria-live="polite">
                <p className="mono policy-head">Password policy</p>
                <ul>
                  {policy.results.map(r => <li key={r.id} className={password ? (r.ok ? "met" : "unmet") : ""}><span aria-hidden="true">{r.ok && password ? "✓" : "○"}</span> {r.label}</li>)}
                </ul>
                <p className="small muted">Length is what makes a password hard to guess, so there is no capital-and-symbol rule. A few random words work well. {[...password].length} / 15 characters.</p>
              </div>
              <button className={"btn btn-block" + (busy ? " busy" : "")} disabled={busy || !policy.ok}>Create account</button>
              <p className="small muted">Passwords are stored as Argon2id hashes. We cannot read yours.</p>
            </form>
          )}
        </div>
      </>
    );
  }

  return (
    <main className="auth">
      <aside className="auth-side" aria-label="How bidding works">
        <img src="/assets/hero.jpg" alt="" className="auth-photo" />
        <div className="auth-copy">
          <h2>Bids are held, not charged</h2>
          <ul>
            <li>A bid sets that money aside in your wallet. If someone outbids you, it comes back at once.</li>
            <li>Win, and the held amount is paid to the seller. Every movement is a ledger journal that sums to zero.</li>
            <li>Sign in as two different people in two windows to watch a bid land on both screens.</li>
          </ul>
        </div>
      </aside>
      <section className="auth-main" aria-labelledby="authTitle"><div>{body}</div></section>
    </main>
  );
}
