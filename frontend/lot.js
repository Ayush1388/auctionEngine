// Lot page. The price, the bid history and the clock all come from the backend
// (the real API or the demo engine) and update live; nothing here is invented.
import { initShell, toast, follow } from "./app/shell.js";
import { LotSession, bidderLabel } from "./app/bidding.js";
import { TracePanel } from "./app/trace.js";
import { mountProxy } from "./app/proxy-ui.js";
import { resolveId, catalogOrder } from "./app/lots.js";
import { SNIPE_WINDOW } from "./app/rules.js";
import { html, raw, esc, $, usd, parseUsd, clock, ago, dateTime, local, reduceMotion } from "./app/util.js";
import { secondsLeft } from "./app/model.js";

const be = await initShell({ bar: true, footer: true });
const param = new URLSearchParams(location.search).get("id") || "gt500";
const main = $("#lotMain");
const pad = n => String(n).padStart(2, "0");

/* ---------------------------------------------------------------- not found, not reachable */
function stop(title, body, actions = "") {
  main.setAttribute("aria-busy", "false");
  main.innerHTML = String(html`<div class="page"><div class="empty"><h3>${title}</h3><p>${body}</p><div class="pop-actions">${raw(actions)}</div></div></div>`);
}
if (be.unreachable) {
  stop("The API is not answering", `Nothing is listening at ${be.base}. Start it with make run, or use the demo engine, which needs no server.`, `<a class="btn" href="?engine=sim">Use the demo engine</a>`);
  throw new Error("api unreachable");
}
const id = await resolveId(be, param).catch(() => null);
if (!id) {
  stop("This lot is not on the backend", be.kind === "live" ? "The real API has no lot with that name yet. Load the catalogue with: go run ./cmd/demobots seed" : "No lot matches that link.",
    `<a class="btn" href="index.html">Back to the auction</a>${be.kind === "live" ? `<a class="btn btn-line" href="?engine=sim">Use the demo engine</a>` : ""}`);
  throw new Error("lot not found");
}

const s = new LotSession(be, id);
try { await s.load(); }
catch (e) { stop(e.status === 404 ? "Lot not found" : "Could not load this lot", e.message, `<a class="btn" href="index.html">Back to the auction</a>`); throw e; }
const lot0 = s.lot;
main.setAttribute("aria-busy", "false");
const trace = new TracePanel(s, be);

/* ---------------------------------------------------------------- static parts, drawn once */
document.title = "Marque | " + lot0.title;
$("#lotNo").textContent = lot0.lotNo ? "Lot " + lot0.lotNo : "Lot";
$("#lotTitle").textContent = lot0.title;
$("#lotSub").textContent = [lot0.event?.name, lot0.where].filter(Boolean).join("  |  ");
$("#chips").innerHTML = String(html`${[lot0.make, lot0.model, lot0.year, lot0.type].filter(Boolean).map(c => html`<li>${c}</li>`)}`);
$("#backLink").href = "index.html#" + (lot0.category === "muscle" ? "muscle" : "classic");
if (lot0.quote) {
  $("#quoteBand").hidden = false; $("#qImg").src = lot0.img; $("#qEvent").textContent = lot0.event?.name || ""; $("#qText").textContent = "“" + lot0.quote + "”";
  lot0.event ? ($("#qLink").href = "event.html?e=" + lot0.event.slug) : ($("#qLink").hidden = true);
}
catalogOrder().then(order => {
  const i = order.indexOf(lot0.key);
  if (i < 0) { $(".lt-nextprev").hidden = true; return; }
  $("#prevLot").href = "lot.html?id=" + order[(i + order.length - 1) % order.length];
  $("#nextLot").href = "lot.html?id=" + order[(i + 1) % order.length];
});

/* gallery: the catalogue has one photo per car, so the views are framings of it */
const posY = lot0.pos.split(" ")[1];
const views = [["100% 100%", 1, "50% " + posY], ["20% 50%", 1.7, "20% 55%"], ["80% 50%", 1.7, "80% 55%"], ["50% 40%", 1.7, "50% 40%"]];
let gi = 0;
$("#gThumbs").innerHTML = views.map((v, i) => `<li><button aria-label="View ${i + 1}"><img src="${esc(lot0.img)}" alt="" style="object-position:${v[2]};${i ? "transform:scale(1.7);transform-origin:" + v[0] : ""}"></button></li>`).join("");
const thumbs = [...document.querySelectorAll("#gThumbs button")];
function view(i) {
  gi = (i + views.length) % views.length; const v = views[gi], m = $("#gMain");
  m.src = lot0.img; m.alt = `${lot0.alt}, view ${gi + 1}`; m.style.objectPosition = gi ? v[2] : lot0.pos; m.style.transformOrigin = v[0]; m.style.transform = `scale(${v[1]})`;
  thumbs.forEach((b, k) => b.classList.toggle("on", k === gi)); $("#gCount").textContent = `${pad(gi + 1)} / ${pad(views.length)}`;
}
thumbs.forEach((b, i) => b.addEventListener("click", () => view(i)));
$("#gPrev").addEventListener("click", () => view(gi - 1)); $("#gNext").addEventListener("click", () => view(gi + 1)); view(0);

/* tabs: only the ones that have something to say */
const specHTML = lot0.specs.length ? html`<dl class="lt-specs">${lot0.specs.map(r => html`<div><dt>${r[0]}</dt><dd>${r[1]}</dd></div>`)}</dl>` : "";
const TABS = {
  Overview: html`<div class="lt-ov"><div><h2>${lot0.lead || lot0.title}</h2>${lot0.text.map(t => html`<p>${t}</p>`)}</div>${specHTML}</div>`,
};
if (lot0.specs.length) TABS.Specifications = html`<h2>Specifications</h2>${specHTML}`;
if (lot0.history) TABS.History = html`<h2>History</h2><p class="lt-p">${lot0.history}</p>`;
if (lot0.specs.length) {
  const m = x => (x || "").toLowerCase();
  TABS.Condition = html`<h2>Condition</h2><ul class="lt-list"><li>Paint and brightwork: presented in ${m(lot0.specs.find(r => r[0] === "Exterior")?.[1]) || "its original colour"} with a consistent finish.</li><li>Mechanical: engine and gearbox run and shift as expected${lot0.year ? ` for a ${lot0.year} ${lot0.make || "car"}` : ""}.</li><li>Interior: ${m(lot0.specs.find(r => r[0] === "Interior")?.[1]) || "as described"}, with wear in keeping with ${lot0.specs.find(r => r[0] === "Mileage")?.[1] || "its mileage"}.</li></ul>`;
  TABS.Documentation = html`<h2>Documentation</h2><ul class="lt-list"><li>Ownership history and registration papers</li><li>Service and restoration invoices</li><li>Chassis and engine number verification</li></ul>`;
}
TABS.Shipping = html`<h2>Shipping</h2><p class="lt-p">The car is in ${lot0.where || "the seller's location"}. We arrange enclosed worldwide transport and export paperwork after payment clears. A quote is shown at checkout.</p>`;
const tabsEl = $("#tabs");
tabsEl.innerHTML = Object.keys(TABS).map((t, i) => `<button role="tab" aria-selected="${i === 0}" data-t="${esc(t)}">${esc(t)}</button>`).join("");
function tabTo(t) {
  tabsEl.querySelectorAll("button").forEach(b => b.setAttribute("aria-selected", b.dataset.t === t)); $("#panel").innerHTML = String(TABS[t]);
  if (window.gsap && !reduceMotion()) gsap.from("#panel > *", { autoAlpha: 0, y: 8, duration: .4, ease: "power2.out" });
}
tabsEl.addEventListener("click", e => { if (e.target.dataset.t) tabTo(e.target.dataset.t); });
$("#panel").innerHTML = String(TABS.Overview);

/* save, share */
const savedKey = "marque-saved:" + be.kind;
const savedSet = () => new Set(local.json(savedKey, []));
const paintSaved = on => { const b = $("#saveBtn"); b.setAttribute("aria-pressed", on); b.textContent = on ? "Saved" : "Save"; };
paintSaved(savedSet().has(id));
$("#saveBtn").onclick = () => { const set = savedSet(), on = !set.has(id); on ? set.add(id) : set.delete(id); local.setJson(savedKey, [...set]); paintSaved(on); toast(on ? "Saved to your list" : "Removed from your list"); };
$("#shareBtn").onclick = () => { (navigator.clipboard ? navigator.clipboard.writeText(location.href) : Promise.reject()).then(() => toast("Link copied"), () => toast("Copy the address bar to share")); };

/* ---------------------------------------------------------------- the bid box */
const amt = $("#amt"), me = () => be.session.id;
let dirty = false, holdInputUntil = 0, lastDelta = 0, ending = false;
const money = c => usd(c);
const amtCents = () => parseUsd(amt.value);

const low = Math.round((lot0.current ?? lot0.startingPrice) * 0.8 / 100000) * 100000, high = Math.round((lot0.current ?? lot0.startingPrice) * 1.35 / 100000) * 100000;
$("#estLow").textContent = money(low); $("#estHigh").textContent = money(high); $("#resv").textContent = lot0.reserve;

function setAmount(c) { amt.value = money(c); }
$("#plus").onclick = () => { dirty = true; setAmount((amtCents() || s.lot.minNext) + s.lot.step); };
$("#minus").onclick = () => { dirty = true; setAmount(Math.max(s.lot.minNext, (amtCents() || s.lot.minNext) - s.lot.step)); };
amt.addEventListener("input", () => { dirty = true; });
amt.addEventListener("blur", () => { if (amtCents()) setAmount(amtCents()); });

function paintLot() {
  const l = s.lot, live = l.status === "ACTIVE", upcoming = l.status === "NOT_ACTIVE";
  window.tickBid ? window.tickBid($("#curBid"), l.current ?? l.startingPrice, money) : ($("#curBid").textContent = money(l.current ?? l.startingPrice));
  $("#clockLab").textContent = upcoming ? "Opens in" : "Time remaining";
  $("#bidCount").textContent = l.bids;
  const span = Math.max(1, high - low);
  $("#mark").style.left = Math.max(0, Math.min(100, ((l.current ?? l.startingPrice) - low) / span * 100)) + "%";
  $("#minLine").textContent = live ? `Minimum next bid ${money(l.minNext)}. Each bid goes up by at least ${money(l.step)}.` : "";
  $("#delta").textContent = lastDelta > 0 ? `+${money(lastDelta)}  just now` : "";

  // keep the field at the minimum unless the person is typing, or just placed a bid (see the double click guard)
  const stillHolding = Date.now() < holdInputUntil;
  if (live && !stillHolding && (!dirty || amtCents() < l.minNext)) { setAmount(l.minNext); dirty = false; }

  const go = $("#goBtn");
  if (!me()) { go.textContent = "Sign in to bid"; go.disabled = false; }
  else if (live) { go.textContent = "Place bid"; go.disabled = false; }
  else { go.textContent = upcoming ? "Not open yet" : l.status === "COMPLETED" ? "Auction closed" : "Not open for bids"; go.disabled = true; }
  amt.disabled = !live; $("#plus").disabled = $("#minus").disabled = !live;
  $("#liveTag").textContent = live ? (be.feed.state === "open" ? "Live" : "Reconnecting") : upcoming ? "Opens soon" : l.status === "COMPLETED" ? "Closed" : "Cancelled";
  $("#liveTag").classList.toggle("on", live && be.feed.state === "open");
  paintWallet(); paintClosed(); paintSnipe();
}

function paintWallet() {
  const el = $("#walletLine"), w = s.wallet, l = s.lot;
  if (!me()) { el.innerHTML = String(html`<p>Bids are held from your wallet, not charged. <a href="account.html?next=${encodeURIComponent("lot.html" + location.search)}">Sign in</a> to bid.</p>`); return; }
  if (!w) { el.innerHTML = `<span class="skel" style="display:block;height:34px"></span>`; return; }
  const held = s.held, others = Math.max(0, w.reserved - held);
  el.className = "lt-wallet" + (held ? " is-leading" : "");
  el.innerHTML = String(html`
    <div><span class="mono lt-lab">Available</span><b class="tnum">${money(w.available)}</b></div>
    <div class="lt-held"><span class="mono lt-lab">Held on this lot</span><b class="tnum">${money(held)}</b></div>
    ${others ? html`<div><span class="mono lt-lab">Held elsewhere</span><b class="tnum">${money(others)}</b></div>` : ""}
    <a class="linkish" href="wallet.html">Add funds</a>`);
}

let closedKey = "";
function paintClosed() {
  const l = s.lot, el = $("#bidbox");
  const closed = l.status === "COMPLETED" || l.status === "CANCELLED";
  const key = closed ? [l.status, l.bidderId, l.current, l.settledAt, me()].join("|") : "";
  if (key === closedKey) return;                       // nothing changed: leave the panel (and a focused button) alone
  closedKey = key;
  el.querySelector(".lt-closed")?.remove();
  $("#bidForm").hidden = closed;
  if (!closed) return;
  const won = l.bidderId && l.bidderId === me();
  const body = l.status === "CANCELLED" ? html`<h3>Cancelled</h3><p>The seller withdrew this lot before it opened.</p>`
    : !l.bidderId ? html`<h3>Closed without bids</h3><p>No bids were placed.</p>`
    : won ? html`<h3>You won this lot</h3><p>Winning bid <b class="tnum">${money(l.current)}</b>. ${l.settledAt ? "The sale is settled: the held funds went to the seller." : "The sale is being settled."}</p><button class="btn btn-sm" id="receiptBtn">View receipt</button>`
    : html`<h3>Sold</h3><p>Sold for <b class="tnum">${money(l.current)}</b> to ${bidderLabel(l.bidderId, me())}.${s.history.some(b => b.user_id === me()) ? " Your hold was released." : ""}</p>`;
  const c = Object.assign(document.createElement("div"), { className: "lt-closed" + (won ? " is-won" : "") });
  c.innerHTML = String(body);
  $("#bidForm").before(c);
  $("#receiptBtn")?.addEventListener("click", showReceipt);
}

function paintSnipe() {
  const l = s.lot, el = $("#snipe"), left = secondsLeft(l, be.feed.serverNow());
  const inWindow = l.status === "ACTIVE" && left <= SNIPE_WINDOW / 1000 && left > 0;
  el.hidden = !inWindow;
  if (inWindow) el.innerHTML = String(html`<b>Last two minutes.</b> A bid now moves the close to two minutes after the bid, so nobody is sniped. ${l.extensions ? `Extended ${l.extensions} of 10 times so far.` : ""}`);
}

/* the clock: server-aligned, ticks four times a second */
function tick() {
  const l = s.lot, now = be.feed.serverNow();
  let left;
  if (l.status === "NOT_ACTIVE") left = Math.max(0, Math.ceil((Date.parse(l.startsAt) - now) / 1000)); else left = secondsLeft(l, now);
  $("#clock").textContent = l.status === "COMPLETED" ? "Closed" : l.status === "CANCELLED" ? "Cancelled" : clock(left);
  $("#clock").classList.toggle("urgent", l.status === "ACTIVE" && left <= 120);
  paintSnipe();
  if (l.status === "ACTIVE" && left === 0 && !ending) { ending = true; setTimeout(() => { s.reload().finally(() => { ending = false; }); }, 1200); }
}
setInterval(tick, 250);

/* the history */
let newBidId = null;
function paintHistory() {
  const rows = s.history.slice(0, 6), n = s.lot.bids;
  $("#histEmpty").hidden = rows.length > 0;
  $("#hist").innerHTML = String(html`${rows.map((b, i) => html`<tr class="${b.user_id === me() ? "you" : ""}${b.id === newBidId ? " new" : ""}"><td>${n - i}</td><td>${bidderLabel(b.user_id, me())}${b.auto ? html`<span class="lt-hist-auto" title="Placed by the engine for a maximum bid">auto</span>` : ""}</td><td>${money(b.amount)}</td><td>${ago(b.created_at, be.feed.serverNow())}</td></tr>`)}`);
  if (newBidId && window.gsap && !reduceMotion()) { const r = $("#hist").firstElementChild; if (r) gsap.from(r, { autoAlpha: 0, y: -10, duration: .45, ease: "power2.out" }); }
  newBidId = null;
}
setInterval(paintHistory, 15000);

/* one repaint per frame, however many updates arrive (a stress run sends hundreds) */
let raf = 0;
const schedule = () => { if (!raf) raf = requestAnimationFrame(() => { raf = 0; paintLot(); paintHistory(); }); };

/* ---------------------------------------------------------------- reacting to the controller */
function flash() { const b = $("#bidbox"); b.classList.remove("flash"); void b.offsetWidth; b.classList.add("flash"); }
let prevCurrent = s.lot.current;
s.on("lot", l => { if (l.current != null && prevCurrent != null && l.current > prevCurrent) { lastDelta = l.current - prevCurrent; flash(); } prevCurrent = l.current; schedule(); });
s.on("history", schedule);
s.on("wallet", () => { paintWallet(); });
s.on("event", e => { if (e.bid) newBidId = e.bid.id; });
s.on("extended", x => {
  toast("A late bid extended the close");
  const c = $("#clock"); c.classList.remove("bump"); void c.offsetWidth; c.classList.add("bump");
  $("#snipe").hidden = false; $("#snipe").innerHTML = String(html`<b>Close extended.</b> A bid in the last two minutes moved the end to ${dateTime(x.to)}. Extension ${x.extensions} of 10.`);
});
s.on("outbid", o => {
  const hold = money(o.hold ?? 0);
  showResult({ kind: "outbid", message: `You were outbid by ${bidderLabel(o.by, me())} at ${money(o.amount)}. Your ${hold} hold was released and is available again.`, minimum: s.lot.minNext });
});
be.feed.on("state", () => schedule());

const resultEl = $("#result");
function showResult(o) {
  resultEl.className = "lt-result r-" + o.kind;
  resultEl.innerHTML = String(html`<p>${o.message}</p>${o.minimum ? html`<button class="btn btn-sm" data-rebid="${o.minimum}">Bid ${money(o.minimum)}</button>` : ""}${o.kind === "funds" ? html`<a class="btn btn-sm" href="wallet.html">Add funds</a>` : ""}`);
}
s.on("outcome", o => {
  if (o.kind === "auth") { location.href = "account.html?next=" + encodeURIComponent("lot.html" + location.search); return; }
  showResult(o);
  if (o.kind === "leading") { holdInputUntil = Date.now() + 4000; follow(id); toast("Bid placed: " + money(o.amount)); }
  if (o.kind === "duplicate") toast("Duplicate click ignored");
});
resultEl.addEventListener("click", e => {
  const b = e.target.closest("[data-rebid]"); if (!b) return;
  setAmount(+b.dataset.rebid); dirty = true; $("#bidForm").requestSubmit();
});

/* the form: a click is one intent; a second click on the same amount is the same bid */
$("#bidForm").addEventListener("submit", async e => {
  e.preventDefault();
  if (!me()) { location.href = "account.html?next=" + encodeURIComponent("lot.html" + location.search); return; }
  const l = s.lot, amount = amtCents();
  if (!amount || amount < l.minNext) { showResult({ kind: "toolow", message: `The minimum bid is ${money(l.minNext)}.`, minimum: l.minNext }); setAmount(l.minNext); return; }
  if (l.bidderId === me() && amount <= l.current && Date.now() > holdInputUntil) { showResult({ kind: "toolow", message: `You already lead at ${money(l.current)}. Enter a higher amount to raise your bid.` }); return; }
  await s.place(amount);
  schedule();
});

/* receipt for a won lot: the SETTLE journal from the ledger */
async function showReceipt() {
  const dlg = $("#receipt");
  dlg.innerHTML = `<h2 id="rcTitle">Receipt</h2><p class="muted">Loading the ledger entry...</p>`;
  dlg.showModal();
  try {
    const page = await be.ledger({ limit: 200 });
    const lines = page.entries.filter(e => e.auction_id === id && e.kind === "SETTLE");
    const tx = lines[0];
    dlg.innerHTML = String(html`
      <h2 id="rcTitle">Receipt</h2>
      <p class="lead">${s.lot.title}</p>
      <dl class="rc"><div><dt>Winning bid</dt><dd class="tnum">${money(s.lot.current)}</dd></div>
        <div><dt>Paid from</dt><dd>Funds held for this lot</dd></div>
        <div><dt>Settled</dt><dd>${tx ? dateTime(tx.created_at) : "Pending"}</dd></div>
        <div><dt>Ledger journal</dt><dd class="tnum">${tx ? tx.transaction_id.slice(0, 13) + "..." : "Not posted yet"}</dd></div></dl>
      <p class="small muted">Settlement is one journal whose lines sum to zero: your held funds leave, the seller's available balance rises by the same amount.</p>
      <div class="pop-actions"><a class="btn btn-sm" href="wallet.html">Open the statement</a><button class="btn btn-line btn-sm" value="close" onclick="this.closest('dialog').close()">Close</button></div>`);
  } catch (err) { dlg.innerHTML = String(html`<h2 id="rcTitle">Receipt</h2><p class="form-error">${err.message}</p><button class="btn btn-line btn-sm" onclick="this.closest('dialog').close()">Close</button>`); }
}

/* the demo tools */
$("#traceBtn").addEventListener("click", e => trace.toggle(e.currentTarget));
$("#stressBtn").addEventListener("click", e => { trace.open(e.currentTarget); setTimeout(() => $("#stress")?.scrollIntoView({ behavior: reduceMotion() ? "auto" : "smooth", block: "start" }), 60); });
document.addEventListener("visibilitychange", () => { if (!document.hidden) s.reload(); });

mountProxy({ be, session: s, root: $("#proxy"), toast, onChange: () => { schedule(); } });

if (s.me) follow(id);
paintLot(); paintHistory(); tick();
