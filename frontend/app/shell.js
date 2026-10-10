// What every page shares: the header (navigation, wallet, alerts, account), the
// footer, the engine badge, toasts, the Noir theme switch, and the alert engine
// that turns live updates into "you were outbid".
import { getBackend } from "./backend.js";
import { toLot } from "./model.js";
import { participatedIds } from "./mybids.js";
import { $, html, raw, esc, usd, ago, local, tab, Emitter, uuid } from "./util.js";

export const shell = new Emitter();           // events: "wallet", "alerts"
let be, started;

/* ------------------------------------------------------------------ toast */
let toastTimer;
export function toast(msg, ms = 2600) {
  let el = $("#toast");
  if (!el) { el = Object.assign(document.createElement("div"), { id: "toast", className: "toast" }); el.setAttribute("role", "status"); el.setAttribute("aria-live", "polite"); document.body.append(el); }
  el.textContent = msg; el.classList.add("show");
  clearTimeout(toastTimer); toastTimer = setTimeout(() => el.classList.remove("show"), ms);
}

/* ------------------------------------------------------------------ theme */
export function wireTheme(btn = $("#modeToggle")) {
  if (!btn || btn.dataset.wired) return;
  btn.dataset.wired = "1";
  const set = dark => {
    dark ? document.documentElement.setAttribute("data-theme", "dark") : document.documentElement.removeAttribute("data-theme");
    btn.setAttribute("aria-checked", dark); local.set("marque-theme", dark ? "dark" : "light");
  };
  btn.setAttribute("aria-checked", document.documentElement.getAttribute("data-theme") === "dark");
  btn.addEventListener("click", () => set(btn.getAttribute("aria-checked") !== "true"));
}

/* ------------------------------------------------------------------ markup */
const LOGO = `<svg viewBox="0 0 40 32" width="40" height="32" aria-hidden="true"><path fill="currentColor" d="M0 14 8 0h24l8 14-6 4-5-9H11l-5 9zM6 24l6-4h16l6 4-4 8H10z"/></svg>`;
const NAV = [["Classics", "index.html#classic"], ["Muscle", "index.html#muscle"], ["Events", "index.html#events"], ["Search", "search.html"], ["Sell a car", "sell.html"], ["Backend", "tour.html"]];

function barHTML() {
  return `
    <a class="brand" href="index.html" aria-label="Marque home">${LOGO}</a>
    <nav class="lt-nav" aria-label="Primary">${NAV.map(([t, h]) => `<a href="${h}" data-nav="${h.split(".")[0]}">${t}</a>`).join("")}</nav>
    <div class="lt-actions">
      <span data-shell-actions></span>
      <button class="mode" id="modeToggle" role="switch" aria-checked="false" aria-label="Noir mode"><span class="mono mode-label">Noir</span><span class="mode-track"><span class="mode-knob"></span></span></button>
    </div>`;
}

function footerHTML() {
  return `
    <div class="footer-top"><h2 class="wordmark small">MARQUE</h2>
      <div class="footer-cols">
        <nav aria-label="Auctions"><h3 class="mono">Auctions</h3><ul><li><a href="index.html#classic">Classic legends</a></li><li><a href="index.html#muscle">Midnight muscle</a></li><li><a href="index.html#events">Upcoming events</a></li><li><a href="search.html">Search and trending</a></li></ul></nav>
        <nav aria-label="Account"><h3 class="mono">Your account</h3><ul><li><a href="bids.html">My bids</a></li><li><a href="wallet.html">Wallet and statement</a></li><li><a href="sell.html">Sell a car</a></li></ul></nav>
        <nav aria-label="Under the hood"><h3 class="mono">Under the hood</h3><ul><li><a href="tour.html">How the backend works</a></li><li><a href="status.html">System status</a></li><li><a href="system.html">Architecture map</a></li><li><a href="chaos.html">Chaos lab</a></li><li><a href="trust.html">Trust page</a></li><li><a href="race.html">Race theater</a></li><li><a href="searchlab.html">Search lab</a></li><li><a href="formats.html">Auction formats</a></li><li><a href="admin.html">Operator console</a></li><li><a href="https://github.com/Ayush1388/auctionEngine#readme" rel="noopener">How it is built</a></li></ul></nav>
      </div>
    </div>
    <div class="footer-bottom"><p class="mono">© 2026 Marque Auctions. Bid responsibly.</p></div>`;
}

/* ------------------------------------------------------------------ account area */
function actionsHTML(user, unread) {
  if (!user) return `<a class="btn" href="account.html?next=${encodeURIComponent(location.pathname.split("/").pop() + location.search)}">Sign in</a>`;
  const initial = (user.email || "?")[0].toUpperCase();
  return `
    <a class="chip" href="wallet.html" id="walletChip" aria-label="Wallet"><span class="mono">Available</span><b id="walletAvail">-</b><span class="chip-held" id="walletHeld" hidden></span></a>
    <div class="pop-wrap">
      <button class="bell" id="bellBtn" aria-expanded="false" aria-controls="bellPanel" aria-label="Alerts"><span class="mono">Alerts</span><b id="bellCount" ${unread ? "" : "hidden"}>${unread}</b></button>
      <div class="pop pop-alerts" id="bellPanel" hidden></div>
    </div>
    <div class="pop-wrap">
      <button class="acct" id="acctBtn" aria-expanded="false" aria-controls="acctMenu"><span class="acct-i" aria-hidden="true">${esc(initial)}</span><span class="acct-n">${esc((user.email || "").split("@")[0])}</span></button>
      <div class="pop pop-menu" id="acctMenu" hidden>
        <p class="mono pop-who">${esc(user.email)}${user.role === "admin" ? " (operator)" : ""}</p>
        <a href="bids.html">My bids</a><a href="wallet.html">Wallet and statement</a><a href="sell.html">Sell a car</a><a href="sell.html#listings">My listings</a>
        ${user.role === "admin" ? `<a href="admin.html">Operator console</a>` : ""}
        <button id="signOut">Sign out</button>
      </div>
    </div>`;
}

function bindPopover(btn, panel) {
  const set = open => { panel.hidden = !open; btn.setAttribute("aria-expanded", open); };
  btn.addEventListener("click", e => { e.stopPropagation(); const open = panel.hidden; closePops(); set(open); });
  panel.addEventListener("click", e => e.stopPropagation());
  return set;
}
function closePops() { document.querySelectorAll(".pop").forEach(p => { p.hidden = true; }); document.querySelectorAll("[aria-controls][aria-expanded=true]").forEach(b => { if (b.id === "bellBtn" || b.id === "acctBtn" || b.id === "enginePill") b.setAttribute("aria-expanded", "false"); }); }
addEventListener("click", closePops);
addEventListener("keydown", e => { if (e.key === "Escape") closePops(); });

/* ------------------------------------------------------------------ wallet chip */
let walletTimer;
export async function refreshWallet() {
  if (!be?.session.id) return null;
  try {
    const w = await be.wallet();
    const a = $("#walletAvail"), h = $("#walletHeld");
    if (a) a.textContent = usd(w.available);
    if (h) { h.hidden = !w.reserved; h.textContent = w.reserved ? `${usd(w.reserved)} held` : ""; }
    shell.emit("wallet", w);
    return w;
  } catch { return null; }
}

/* ------------------------------------------------------------------ alerts */
const notesKey = () => `marque-alerts:${be.kind}:${be.session.id}`;
const loadNotes = () => (be.session.id ? local.json(notesKey(), []) : []);
const saveNotes = n => local.setJson(notesKey(), n.slice(0, 40));
const titles = new Map();
async function titleOf(id) {
  if (titles.has(id)) return titles.get(id);
  try { const t = toLot(await be.getAuction(id)).title; titles.set(id, t); return t; } catch { return "a lot"; }
}

export function notify(n) {
  if (!be.session.id) return;
  const list = loadNotes();
  list.unshift({ id: uuid(), at: new Date().toISOString(), read: false, ...n });
  saveNotes(list);
  shell.emit("alerts");
  renderAlerts();
  toast(n.title);
}

function renderAlerts() {
  const list = loadNotes(), unread = list.filter(n => !n.read).length;
  const count = $("#bellCount"), panel = $("#bellPanel");
  if (count) { count.hidden = !unread; count.textContent = unread; }
  if (!panel) return;
  panel.innerHTML = `
    <div class="pop-head"><h3>Alerts</h3>${unread ? `<button id="readAll" class="linkish">Mark all read</button>` : ""}</div>
    ${list.length ? `<ul class="alerts">${list.slice(0, 12).map(n => `<li class="${n.read ? "" : "unread"}"><a href="${esc(n.href || "#")}" data-note="${esc(n.id)}"><b>${esc(n.title)}</b><span>${esc(n.body || "")}</span><time class="mono">${esc(ago(n.at))}</time></a></li>`).join("")}</ul>`
      : `<p class="pop-empty">Nothing yet. You will hear here when someone outbids you or an auction you bid on closes.</p>`}`;
  $("#readAll")?.addEventListener("click", () => { saveNotes(loadNotes().map(n => ({ ...n, read: true }))); renderAlerts(); });
  panel.querySelectorAll("[data-note]").forEach(a => a.addEventListener("click", () => saveNotes(loadNotes().map(n => (n.id === a.dataset.note ? { ...n, read: true } : n)))));
}

/** Which auctions the alert engine is following (all the ones I have bid on). */
const followed = new Map();
export async function follow(auctionId) {
  if (followed.has(auctionId) || !be.session.id) return;
  followed.set(auctionId, be.feed.subscribe(auctionId, ev => onUpdate(ev)));
}
async function onUpdate(ev) {
  const me = be.session.id, a = ev.auction;
  if (!me) return;
  if (ev.bid && ev.bid.previous_bidder_id === me && ev.bid.bidder_id !== me) {
    notify({ kind: "outbid", title: `You were outbid on ${await titleOf(a.id)}`, body: `New high bid ${usd(a.current_bid)}. Your hold was released.`, href: `lot.html?id=${a.id}` });
    refreshWallet();
  } else if (ev.cause === "auction.completed") {
    const t = await titleOf(a.id);
    notify(a.current_bidder_id === me
      ? { kind: "won", title: `You won ${t}`, body: `Winning bid ${usd(a.current_bid)}. Payment is being settled.`, href: `lot.html?id=${a.id}` }
      : { kind: "lost", title: `${t} has closed`, body: `It sold for ${usd(a.current_bid)}. Your hold was released.`, href: `lot.html?id=${a.id}` });
    refreshWallet();
  } else if (ev.cause === "auction.settled" && a.current_bidder_id === me) {
    refreshWallet();
  }
}

async function startAlerts() {
  if (!be.session.id) return;
  try { (await participatedIds(be, 1)).forEach(follow); } catch { /* alerts still work for lots bid on from now */ }
}

/* ------------------------------------------------------------------ engine badge */
function engineBadge() {
  const live = be.kind === "live", detail = be.detail;
  const el = document.createElement("div");
  el.className = "engine";
  el.innerHTML = `
    <button class="engine-pill ${live ? "is-live" : "is-demo"}" id="enginePill" aria-expanded="false" aria-controls="enginePanel"><i aria-hidden="true"></i><span>${live ? "Live backend" : "Demo engine"}</span><b class="tnum" id="pulseMs"></b></button>
    <div class="pop pop-engine" id="enginePanel" hidden>
      <div class="botsrow" id="botsRow" hidden>
        <div><b>Demo bots</b><span class="small" id="botsNote"></span></div>
        <button class="switch" id="botsSwitch" role="switch" aria-checked="false" aria-label="Demo bots"></button>
      </div>
      <div class="pulse" id="pulse" aria-live="off"><div class="pulse-head"><b>System pulse</b><span class="mono" id="pulseState">checking</span></div>
        <svg class="pulse-spark" id="pulseSpark" viewBox="0 0 240 36" preserveAspectRatio="none" aria-hidden="true"></svg>
        <ul class="pulse-list" id="pulseList"></ul>
        <div class="pop-actions"><a class="btn btn-line btn-sm" href="system.html">Architecture map</a><a class="btn btn-line btn-sm" href="chaos.html">Chaos lab</a></div>
      </div>
      ${live ? `<h3>Connected to the real API</h3>
        <p>Every bid on this page goes to <b class="mono">${esc(detail)}</b>: PostgreSQL, the outbox and a WebSocket. Timings in the Behind the bid panel are measured.</p>
        <p class="engine-ws mono" id="engineWs">WebSocket: waiting for a lot</p>
        <div class="pop-actions"><a class="btn btn-line" href="?engine=sim">Use the demo engine</a></div>`
      : `<h3>Demo engine</h3>
        <p>${be.fellBack ? `The real API at <b class="mono">${esc(be.fellBack.api.replace(/^https?:\/\//, ""))}</b> did not answer, so the site is running on its built-in engine. ` : ""}It follows the same rules as the Go backend and runs in this browser. Windows on this device share it, so you can bid from two at once. Timings are simulated and marked as such.</p>
        <div class="pop-actions"><a class="btn btn-line" href="?engine=live">Try the real API</a><button class="btn btn-line" id="engineReset">Reset demo</button></div>`}
    </div>`;
  document.body.append(el);
  bindPopover($("#enginePill"), $("#enginePanel"));
  $("#engineReset")?.addEventListener("click", () => { if (confirm("Reset the demo? Accounts, bids and balances go back to the start.")) be.reset(); });
  startPulse();
  wireBots();
  if (live) be.feed.on("state", s => { const w = $("#engineWs"); if (w) w.textContent = "WebSocket: " + ({ open: "open", connecting: "connecting", retrying: `reconnecting in ${Math.round((s.retryIn || 0) / 100) / 10} s`, closed: "closed", idle: "waiting for a lot" })[s.state]; });
}

/* ------------------------------------------------------------------ demo bots switch */
// Rival bidders that keep a quiet site alive. In the demo engine they run in this
// browser; on the real API they are built in (DEMO_BOTS_ENABLED=true) and this
// switch is the only thing needed: no second server.
let botsBusy = false, botsState = null;
async function paintBots() {
  const row = $("#botsRow"); if (!row || !be.bots) return;
  try { botsState = await be.bots.get(); } catch { botsState = null; }
  const sw = $("#botsSwitch"), note = $("#botsNote");
  row.hidden = false;
  if (!botsState || !botsState.enabled) {
    sw.disabled = true; sw.setAttribute("aria-checked", "false");
    note.textContent = botsState ? "Not built into this API. Start it with DEMO_BOTS_ENABLED=true." : "Cannot reach the API.";
    return;
  }
  const a = botsState.ambient || {}, signedIn = !!be.session.id || be.kind === "sim";
  sw.setAttribute("aria-checked", String(!!a.on));
  sw.disabled = botsBusy || !signedIn;
  note.textContent = !signedIn ? "Sign in to switch them on."
    : a.on ? `On: ${a.bids} bids placed so far. Stress it on a lot page uses them too.`
    : "Off. Switch on for rival bidders on every open lot. Stress it works either way.";
}
function wireBots() {
  const sw = $("#botsSwitch"); if (!sw) return;
  sw.addEventListener("click", async () => {
    botsBusy = true; sw.disabled = true;
    try { botsState = await be.bots.setAmbient(sw.getAttribute("aria-checked") !== "true"); toast(botsState.ambient.on ? "Demo bots on" : "Demo bots off"); }
    catch (e) { toast(e.message || "Could not change the demo bots"); }
    botsBusy = false; paintBots();
  });
  paintBots();
}

/* ------------------------------------------------------------------ system pulse */
// A small always-there readout of the engine: is every dependency answering, and how long
// does a round trip take. It reads the public probes, so it works for every visitor.
const pulseRtt = [];
export const pulse = new Emitter();           // "tick" { ok, state, checks, ms, history }
async function pulseOnce() {
  if (document.hidden) return;
  let ready = null, ms = null;
  try { const l = await be.livez(); ms = l.ms; } catch { /* shown as down */ }
  try { ready = await be.readyz(); } catch (e) { ready = e.body?.checks ? e.body : null; }
  const checks = ready?.checks || {};
  const failing = Object.entries(checks).filter(([, v]) => v !== "ok").map(([k]) => k);
  const state = ms == null || ready?.status === "unavailable" ? "bad" : failing.length ? "warn" : "ok";
  if (ms != null) { pulseRtt.push(ms); if (pulseRtt.length > 40) pulseRtt.shift(); }
  const btn = $("#enginePill"), label = $("#pulseMs");
  if (btn) btn.dataset.state = state;
  if (label) label.textContent = ms != null ? (ms < 10 ? ms.toFixed(1) : Math.round(ms)) + " ms" : "down";
  const st = $("#pulseState"); if (st) st.textContent = ({ ok: "All systems answering", warn: failing.length + " degraded", bad: "Not answering" })[state];
  const list = $("#pulseList");
  if (list) list.innerHTML = Object.entries({ api: ms != null ? "ok" : "failing", ...checks }).map(([k, v]) => `<li class="${v === "ok" ? "ok" : "bad"}"><span>${esc(k)}</span><b class="mono">${v === "ok" ? "ok" : "failing"}</b></li>`).join("");
  const sp = $("#pulseSpark");
  if (sp && pulseRtt.length > 1) {
    const hi = Math.max(...pulseRtt, 1), pts = pulseRtt.map((v, i) => `${(i / (pulseRtt.length - 1) * 240).toFixed(1)},${(34 - v / hi * 30).toFixed(1)}`).join(" ");
    sp.innerHTML = `<polyline points="${pts}" fill="none" stroke="currentColor" stroke-width="2" vector-effect="non-scaling-stroke"/>`;
  }
  paintBots();
  pulse.emit("tick", { ok: state === "ok", state, checks, ms, history: [...pulseRtt] });
}
function startPulse() { pulseOnce(); setInterval(pulseOnce, 5000); }

/* ------------------------------------------------------------------ boot */
/**
 * @param {{ bar?: boolean, footer?: boolean, nav?: string, requireAuth?: boolean }} opts
 *   bar:    fill <header id="appbar"> with the standard bar
 *   footer: fill <footer id="appfooter">
 *   nav:    which nav item to mark current ("search", "sell", ...)
 *   requireAuth: send signed-out visitors to the sign-in page
 */
export function initShell(opts = {}) {
  return started ||= (async () => {
    if (opts.bar) { const b = $("#appbar"); if (b) { b.className = "lt-bar"; b.innerHTML = barHTML(); } }
    if (opts.footer) { const f = $("#appfooter"); if (f) { f.className = "footer"; f.innerHTML = footerHTML(); } }
    if (opts.nav) $(`[data-nav="${opts.nav}"]`)?.setAttribute("aria-current", "page");
    wireTheme();

    be = await getBackend();
    if (opts.requireAuth && !be.session.id) { location.replace("account.html?next=" + encodeURIComponent(location.pathname.split("/").pop() + location.search)); return be; }

    const draw = () => {
      document.querySelectorAll("[data-join]").forEach(a => { a.hidden = !!be.session.id; });
      const slot = $("[data-shell-actions]"); if (!slot) return;
      slot.className = "shell-actions";
      slot.innerHTML = actionsHTML(be.session.user, loadNotes().filter(n => !n.read).length);
      if (be.session.id) {
        bindPopover($("#bellBtn"), $("#bellPanel")); renderAlerts();
        bindPopover($("#acctBtn"), $("#acctMenu"));
        $("#signOut").addEventListener("click", async () => { await be.logout(); location.href = "index.html"; });
        refreshWallet();
      }
    };
    draw();
    be.onAuth(() => { draw(); startAlerts(); });
    engineBadge();
    startAlerts();
    clearInterval(walletTimer); walletTimer = setInterval(refreshWallet, 30000);
    if (be.unreachable) toast("Cannot reach the API at " + be.base);
    return be;
  })();
}

/** The backend this page resolved, once initShell has finished. */
export const backend = () => be;
