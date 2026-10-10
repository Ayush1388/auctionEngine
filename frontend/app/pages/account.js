// Sign in, create an account, activate it.
import { initShell, toast } from "../shell.js";
import { DEMO_ACCOUNTS, DEMO_PASSWORD } from "../demo.js";
import { html, raw, esc, $, $$ } from "../util.js";

const be = await initShell({ bar: true, footer: true });
const q = new URLSearchParams(location.search);

/** Only same-site pages: a login link must not become an open redirect. */
const safeNext = n => (/^[a-z0-9_-]+\.html([?#][^\s]*)?$/i.test(n || "") ? n : "index.html");
const next = safeNext(q.get("next"));
const body = $("#authBody");
let tab = q.get("token") ? "activate" : q.get("tab") === "create" ? "create" : "signin";
let created = null;                      // { email, token } after registering

const demo = be.kind === "sim" || be.kind === "live";

function errorsFor(e, map = {}) {
  // field errors from the server go under their fields; anything else becomes the form message
  const fields = {};
  for (const [k, v] of Object.entries(e.fields || {})) fields[map[k] || k] = v;
  const msg = Object.keys(fields).length ? "" : e.status === 429 ? `Too many attempts. Try again${e.retryAfter ? ` in ${e.retryAfter} seconds` : " in a moment"}.` : e.network ? "Cannot reach the server. Check your connection and try again." : e.message;
  return { fields, msg };
}
const fieldHTML = (id, label, attrs, err, hint = "") => html`
  <div class="field ${err ? "invalid" : ""}"><label for="${id}">${label}</label>
    <input id="${id}" ${raw(attrs)} aria-invalid="${!!err}" ${err ? raw(`aria-describedby="${id}-e"`) : ""}>
    ${hint ? html`<span class="hint">${hint}</span>` : ""}<span class="err" id="${id}-e">${err || ""}</span></div>`;

function tabs() {
  return `<div class="tabs" role="tablist" aria-label="Account">
    ${[["signin", "Sign in"], ["create", "Create account"], ["activate", "Activate"]].map(([k, l]) => `<button role="tab" aria-selected="${tab === k}" data-tab="${k}">${l}</button>`).join("")}</div>`;
}

function draw(state = {}) {
  body.setAttribute("aria-busy", "false");
  const user = be.session.user;
  if (user) {
    body.innerHTML = String(html`
      <h1 id="authTitle" class="h1">You are signed in</h1>
      <p class="lead">${user.email}${user.role === "admin" ? " (operator)" : ""}</p>
      <div class="pop-actions"><a class="btn" href="${next === "index.html" ? "bids.html" : next}">${next === "index.html" ? "See my bids" : "Continue"}</a><a class="btn btn-line" href="wallet.html">Wallet</a><button class="btn btn-line" id="out">Sign out</button></div>`);
    $("#out").onclick = async () => { await be.logout(); draw(); };
    return;
  }
  const t = tab;
  body.innerHTML = String(html`
    <h1 id="authTitle" class="h1">${t === "signin" ? "Sign in" : t === "create" ? "Create your account" : "Activate your account"}</h1>
    ${raw(tabs())}
    <div class="auth-panel">
      ${state.msg ? html`<p class="form-error" role="alert">${state.msg}</p>` : ""}
      ${state.ok ? html`<p class="form-ok" role="status">${state.ok}</p>` : ""}
      ${raw(t === "signin" ? signinHTML(state) : t === "create" ? createHTML(state) : activateHTML(state))}
    </div>`);
  bind();
}

/* ---------------------------------------------------------------- sign in */
function signinHTML(s) {
  return String(html`
    <form id="f" novalidate class="stack">
      ${fieldHTML("email", "Email", `type="email" autocomplete="username" required value="${esc(s.email || "")}"`, s.fields?.email)}
      ${fieldHTML("password", "Password", `type="password" autocomplete="current-password" required`, s.fields?.password)}
      <button class="btn btn-block" id="go">Sign in</button>
      ${s.notActivated ? html`<p class="small">This account is not activated yet. <button type="button" class="linkish" data-tab="activate">Enter your activation token</button> or <button type="button" class="linkish" id="resend">send a new one</button>.</p>` : ""}
    </form>
    <div class="auth-demo">
      <h2 class="h2">Try it as someone</h2>
      <p class="small muted">${be.kind === "sim" ? "These accounts live in this browser's demo engine, funded with $2,000,000." : "These accounts exist after you run go run ./cmd/demobots seed, funded with $2,000,000."} Open a second window and sign in as someone else to see two people bid on one lot.${be.kind === "sim" ? "" : " The operator account is private: sign in with its own email and password."}</p>
      <div class="presets">${DEMO_ACCOUNTS.filter(a => be.kind === "sim" || a.role !== "admin").map(a => html`<button type="button" class="preset" data-demo="${a.email}">${a.name}</button>`)}</div>
      <p class="small muted mono">Password for all: ${DEMO_PASSWORD}</p>
    </div>`);
}

/* ---------------------------------------------------------------- create */
function createHTML(s) {
  if (created) {
    return String(html`
      <div class="stack">
        <p class="lead">Account created for <b>${created.email}</b>.</p>
        ${created.token
          ? html`<p class="form-ok">Demo engine: no email is sent. Your activation token is below, ready to use.</p><code class="token">${created.token}</code><button class="btn" id="actNow">Activate now</button>`
          : html`<p>We sent an activation link to your email. It expires in 24 hours. The token is the part after <code>token=</code>: paste it on the Activate tab.</p><p class="small muted">Running locally? The API sends mail to the SMTP server in your .env (Mailpit at localhost:8025 in the dev setup).</p><button class="btn" data-tab="activate">I have my token</button>`}
      </div>`);
  }
  return String(html`
    <form id="f" novalidate class="stack">
      ${fieldHTML("email", "Email", `type="email" autocomplete="username" required value="${esc(s.email || "")}"`, s.fields?.email)}
      ${fieldHTML("password", "Password", `type="password" autocomplete="new-password" required minlength="15"`, s.fields?.password, "At least 15 characters. A few random words work well.")}
      <p class="small mono" id="count" aria-live="polite">0 / 15</p>
      <button class="btn btn-block" id="go">Create account</button>
      <p class="small muted">Passwords are stored as Argon2id hashes. We cannot read yours.</p>
    </form>`);
}

/* ---------------------------------------------------------------- activate */
function activateHTML(s) {
  return String(html`
    <form id="f" novalidate class="stack">
      ${fieldHTML("token", "Activation token", `autocomplete="off" spellcheck="false" required value="${esc(s.token ?? q.get("token") ?? created?.token ?? "")}"`, s.fields?.token || s.tokenError, "From the link in your activation email.")}
      <button class="btn btn-block" id="go">Activate account</button>
    </form>`);
}

/* ---------------------------------------------------------------- behaviour */
function busy(on) { const g = $("#go"); if (g) { g.classList.toggle("busy", on); g.disabled = on; } }
function bind() {
  $$("[data-tab]", body).forEach(b => b.addEventListener("click", () => { tab = b.dataset.tab; draw(); }));
  $$("[data-demo]", body).forEach(b => b.addEventListener("click", () => signIn(b.dataset.demo, DEMO_PASSWORD)));
  $("#actNow")?.addEventListener("click", () => activate(created.token));
  $("#resend")?.addEventListener("click", async () => {
    try { const t = await be.resendActivation($("#email").value.trim()); if (be.kind === "sim" && t) { created = { email: $("#email").value.trim(), token: t }; tab = "create"; draw(); } else draw({ ok: "If that account exists and is not active, a new activation email is on its way." }); }
    catch (e) { draw({ msg: errorsFor(e).msg }); }
  });
  const pw = $("#password"), count = $("#count");
  if (pw && count) pw.addEventListener("input", () => { count.textContent = `${pw.value.length} / 15`; count.classList.toggle("pos", pw.value.length >= 15); });
  $("#f")?.addEventListener("submit", async e => {
    e.preventDefault();
    const v = id => ($("#" + id)?.value || "").trim();
    if (tab === "signin") await signIn(v("email"), $("#password").value);
    else if (tab === "create") await register(v("email"), $("#password").value);
    else await activate(v("token"));
  });
}

async function signIn(email, password) {
  busy(true);
  try { await be.login(email, password); location.href = next === "index.html" ? "bids.html" : next; }
  catch (e) { const { fields, msg } = errorsFor(e); draw({ email, fields, msg: e.status === 401 ? "Wrong email or password." : e.status === 403 ? "Activate your account first." : msg, notActivated: e.status === 403 }); }
}
async function register(email, password) {
  busy(true);
  try { const r = await be.register(email, password); created = { email, token: r.activation?.token || null }; draw(); }
  catch (e) { const { fields, msg } = errorsFor(e); draw({ email, fields, msg: e.status === 409 ? "An account with that email already exists. Sign in instead." : msg }); }
}
async function activate(token) {
  if (!token) { draw({ token, tokenError: "Paste the token from your activation email." }); return; }
  busy(true);
  try { await be.activate(token); created = null; tab = "signin"; history.replaceState(null, "", "account.html"); draw({ ok: "Account activated. Sign in to start bidding." }); toast("Account activated"); }
  catch (e) { draw({ token, tokenError: e.status === 400 ? "That token is invalid or has expired. Ask for a new one." : errorsFor(e).msg }); }
}

draw();
