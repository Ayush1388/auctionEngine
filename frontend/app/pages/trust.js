// Trust page: attack the money, retry a bid, audit the books, watch the rate limiter.
// Everything is done through the same public calls a client uses; the page checks
// the answers itself instead of trusting a summary.
import { initShell } from "../shell.js";
import { operatorGate, openLots, plural } from "../lab.js";
import { html, raw, esc, $, $$, usd, uuid, sleep, dur, short, ago } from "../util.js";
import { toLot } from "../model.js";
import { mountTourBar } from "../tour.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
const body = $("#trustBody");
const sim = be.kind === "sim";

/** A read that the shared per-IP limiter shed: wait as Retry-After says and ask again. */
async function patient(fn, tries = 4) {
  for (let i = 0; ; i++) {
    try { return await fn(); }
    catch (e) { if (e.status !== 429 || i >= tries) throw e; await sleep(Math.max(600, (e.retryAfter || 1) * 1000)); }
  }
}
/** A bid that was shed with 429 is sent again, with the same key, after Retry-After. */
async function bidWithRetry(id, amount, key, tries = 4) {
  let r, n = 0;
  do {
    r = await be.placeBid(id, amount, { key });
    n++;
    if (r.status !== 429) break;
    await sleep(Math.max(600, (r.error?.retryAfter || 1) * 1000));
  } while (n <= tries);
  return { ...r, tries: n };
}

const state = {
  books: { checks: 0, balanced: 0, last: null, report: null, err: null },
  attack: { running: false, bids: 50, budget: 400_000, keepLimits: false, result: null, err: null },
  retry: { running: false, lots: [], lotId: "", copies: 5, result: null, err: null },
  bucket: { running: false, samples: [], limit: null, off: false, err: null },
};

/* ------------------------------------------------------------------ books balance */
let bookTimer;
async function checkBooks() {
  if (!be.session.isAdmin) return;
  try {
    const r = await patient(() => be.reconcile(), 1);
    const b = state.books;
    b.report = r; b.checks++; if (r.ok) b.balanced++; b.last = Date.now(); b.err = null;
  } catch (e) { state.books.err = e.message; }
  paintBooks();
}

function booksHTML() {
  const b = state.books, r = b.report;
  if (!be.session.isAdmin) {
    return html`
      <section class="ops-sec" aria-labelledby="bkH">
        <h2 class="h2" id="bkH">Do the books balance?</h2>
        <div class="empty"><h3>The audit is an operator check</h3>
          <p>It recomputes every wallet from the append-only ledger and checks that each journal sums to zero. ${be.session.id ? "This account is not an operator." : "Sign in as the operator to run it live while you attack the system below."}
            ${sim ? " In the demo the operator is admin@marque.test." : ""}</p>
          <a class="btn" href="account.html?next=trust.html">${be.session.id ? "Switch account" : "Sign in"}</a></div>
      </section>`;
  }
  return html`
    <section class="ops-sec" aria-labelledby="bkH">
      <div class="sec-head"><h2 class="h2" id="bkH">Do the books balance?</h2><span class="mono muted">${b.checks ? `checked ${plural(b.checks, "time")}, balanced ${b.balanced}` : "checking"}</span></div>
      ${r ? html`
        <div class="verdict ${r.ok ? "ok" : "bad"}" role="status">
          <b>${r.ok ? "Balanced" : "Does not balance"}</b>
          <span>${r.ok ? "Every wallet equals the sum of its ledger lines, every journal sums to zero, and the held money equals the leading bids. Checked again every 3 seconds." : `${r.mismatches} wallets disagree with the ledger and ${r.unbalanced_journals} journals do not sum to zero.`}</span>
        </div>
        <div class="tiles">
          <div class="tile"><span class="lab mono">Money deposited</span><b class="tnum">${usd(r.money_deposited)}</b><span class="small muted">all deposits ever made</span></div>
          <div class="tile"><span class="lab mono">In wallets</span><b class="tnum">${usd(r.money_in_wallets)}</b><span class="small muted">available plus held</span></div>
          <div class="tile"><span class="lab mono">Held in wallets</span><b class="tnum">${usd(r.reserved_in_wallets)}</b><span class="small muted">reserved by bids</span></div>
          <div class="tile"><span class="lab mono">Leading bids</span><b class="tnum">${usd(r.active_reservations_total)}</b><span class="small muted">on open auctions</span></div>
        </div>` : html`<div class="skel" style="height:120px"></div>`}
      ${b.err ? html`<p class="form-error" role="alert">${b.err}</p>` : ""}
    </section>`;
}
function paintBooks() { const el = $("#books"); if (el) el.innerHTML = String(booksHTML()); }

/* ------------------------------------------------------------------ double-spend attack */
const BUDGETS = [100_000, 400_000, 1_000_000];
const COUNTS = [10, 25, 50];

function attackHTML() {
  const a = state.attack, r = a.result;
  return html`
    <section class="ops-sec" aria-labelledby="atH">
      <h2 class="h2" id="atH">Double-spend attack</h2>
      <p class="lead">One wallet, many bids, all in the same instant, each on a different car. If the wallet could be spent twice, more would win than the money allows.</p>
      <div class="lab-controls">
        <div class="field"><span class="field-label">Bids fired at once</span>
          <div class="presets" role="group" aria-label="Bids fired at once">${COUNTS.map(n => html`<button class="preset" data-bids="${n}" aria-pressed="${a.bids === n}" ${a.running ? "disabled" : ""}>${n}</button>`)}</div></div>
        ${sim ? html`<div class="field"><span class="field-label">Money in the wallet</span>
          <div class="presets" role="group" aria-label="Money in the wallet">${BUDGETS.map(n => html`<button class="preset" data-budget="${n}" aria-pressed="${a.budget === n}" ${a.running ? "disabled" : ""}>${usd(n * 100)}</button>`)}</div></div>` : ""}
        <button class="btn ${a.running ? "busy" : ""}" id="fire" ${a.running ? "disabled" : ""}>Fire ${a.bids} bids</button>
      </div>
      ${sim ? html`<label class="check"><input type="checkbox" id="keepLimits" ${a.keepLimits ? "checked" : ""} ${a.running ? "disabled" : ""}> Keep the rate limiter on <span class="muted">(it sheds most of a burst before the wallet is reached)</span></label>`
        : html`<p class="small muted">This uses your own wallet on the real API. If rate limits are on, the gateway sheds most of the burst first (429); start the API with RATE_LIMITS=off to put the wallet alone to the test.</p>`}
      ${!be.session.id ? html`<p class="form-error">Sign in first: the attack bids with your account. <a href="account.html?next=trust.html" class="linkish">Sign in</a></p>` : ""}
      ${a.err ? html`<p class="form-error" role="alert">${a.err}</p>` : ""}
      ${r ? attackResult(r) : html`<p class="small muted">The page reads the wallet before and after, then checks the result against the ledger rules itself.</p>`}
    </section>`;
}

function attackResult(r) {
  const rows = [
    ["Accepted", r.counts.accepted, "bids that became the leading bid"],
    ["Refused: not enough money", r.counts.funds, "the wallet said no"],
    ["Refused: price moved", r.counts.toolow, "another bid on that car landed first"],
    ["Shed by the rate limiter", r.counts.limited, "429 before the wallet was reached"],
    ["Other", r.counts.other, "unavailable, ended, or an error"],
  ];
  return html`
    <div class="verdict ${r.ok ? "ok" : "bad"}" role="status">
      <b>${r.ok ? "The wallet held" : "Something is wrong"}</b>
      <span>${plural(r.fired, "bid")} fired in the same instant from ${esc(r.actor)} holding ${usd(r.before.total)}. ${plural(r.counts.accepted, "bid")} won, holding ${usd(r.after.reserved - r.before.reserved)} more. The wallet ends at ${usd(r.after.available)} available and ${usd(r.after.reserved)} held, never below zero.</span>
    </div>
    <div class="split-2">
      <div>
        <h3 class="h3">What the server said</h3>
        <table class="tbl"><thead><tr><th>Outcome</th><th class="r">Bids</th></tr></thead>
          <tbody>${rows.map(([k, v, n]) => html`<tr><td>${k}<span class="small muted block">${n}</span></td><td class="r tnum">${v}</td></tr>`)}</tbody></table>
        <p class="small muted">Answered in ${dur(r.ms)} for the whole burst.</p>
      </div>
      <div>
        <h3 class="h3">What the page checked</h3>
        <ul class="checks">${r.checks.map(c => html`<li class="${c.ok ? "ok" : "bad"}"><b>${c.ok ? "Pass" : "Fail"}</b><span>${c.name}</span><em class="muted">${c.detail}</em></li>`)}</ul>
      </div>
    </div>`;
}

async function runAttack() {
  const a = state.attack;
  if (a.running || !be.session.id) return;
  a.running = true; a.err = null; paintAttack();
  try {
    // an actor with a wallet: a fresh account with a set budget in the demo, your own wallet otherwise
    const actor = sim && be.trust ? await be.trust.attacker(a.budget * 100) : { label: "you", userId: be.session.id, wallet: () => be.wallet(), placeBid: (id, amt, key) => be.placeBid(id, amt, { key }) };
    const actorName = sim ? actor.label : "your account";
    if (sim && !a.keepLimits) await be.chaos.setRateLimits(false);

    const lots = (await patient(() => be.allAuctions({ status: "ACTIVE" }, 3))).filter(x => x.owner_id !== be.session.id && Date.parse(x.ends_at) - Date.now() > 25_000);
    if (lots.length < 3) throw new Error("There are not enough open lots to attack. Open lots: " + lots.length);
    const before = await patient(() => actor.wallet()); before.total = before.available + before.reserved;
    // every bid goes to the lot's current minimum; spread over the lots round-robin
    const shots = Array.from({ length: a.bids }, (_, i) => { const l = lots[i % lots.length]; return { id: l.id, amount: l.current_bid == null ? Math.max(l.starting_price, 1) : l.current_bid + l.min_increment }; });
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
    a.result = { fired: shots.length, counts, before, after, checks, ms, actor: actorName, ok: checks.every(c => c.ok) };
    checkBooks();
  } catch (e) { a.err = e.message || String(e); }
  finally {
    if (sim && !a.keepLimits) { try { await be.chaos.setRateLimits(true); } catch { /* demo only */ } }
    a.running = false; paintAttack();
  }
}
function paintAttack() { const el = $("#attack"); if (el) { el.innerHTML = String(attackHTML()); bindAttack(); } }
function bindAttack() {
  $$("[data-bids]", body).forEach(b => b.addEventListener("click", () => { state.attack.bids = +b.dataset.bids; paintAttack(); }));
  $$("[data-budget]", body).forEach(b => b.addEventListener("click", () => { state.attack.budget = +b.dataset.budget; paintAttack(); }));
  $("#fire")?.addEventListener("click", runAttack);
  $("#keepLimits")?.addEventListener("change", e => { state.attack.keepLimits = e.target.checked; });
}

/* ------------------------------------------------------------------ retry with one key */
function retryHTML() {
  const r = state.retry, res = r.result;
  return html`
    <section class="ops-sec" aria-labelledby="rtH">
      <h2 class="h2" id="rtH">Retry with the same key</h2>
      <p class="lead">A phone loses signal after a bid is sent. The app cannot know whether it landed, so it sends it again. With one Idempotency-Key the server places one bid and recognises the rest.</p>
      <div class="lab-controls">
        <label class="field"><span class="field-label">Lot</span>
          <select id="rtLot" ${r.running ? "disabled" : ""}>${r.lots.map(l => html`<option value="${l.id}" ${l.id === r.lotId ? "selected" : ""}>${l.lot.title} (minimum ${usd(l.lot.minNext)})</option>`)}</select></label>
        <label class="field"><span class="field-label">Copies sent</span>
          <select id="rtN" ${r.running ? "disabled" : ""}>${[3, 5, 10].map(n => html`<option ${n === r.copies ? "selected" : ""}>${n}</option>`)}</select></label>
        <button class="btn ${r.running ? "busy" : ""}" id="retry" ${r.running || !r.lots.length || !be.session.id ? "disabled" : ""}>Send the same bid ${r.copies} times</button>
      </div>
      ${!be.session.id ? html`<p class="form-error">Sign in first. <a class="linkish" href="account.html?next=trust.html">Sign in</a></p>` : ""}
      ${r.err ? html`<p class="form-error" role="alert">${r.err}</p>` : ""}
      ${res ? html`
        <div class="verdict ${res.ok ? "ok" : "bad"}" role="status"><b>${res.ok ? "One bid" : res.distinct === 0 ? "No bid was placed" : "More than one bid"}</b>
          <span>${plural(res.copies, "request")} carried the key <code>${short(res.key)}</code>. The server created ${plural(res.distinct, "bid")} (${usd(res.amount)}) and answered the other ${res.replayed} as repeats. Your hold changed by ${usd(res.heldDelta)}, once.</span></div>
        <table class="tbl"><thead><tr><th>Copy</th><th>Status</th><th>Marked as replay</th><th>Bid</th><th>Tries</th><th class="r">Time</th></tr></thead>
          <tbody>${res.rows.map((x, i) => html`<tr><td class="tnum">${i + 1}</td><td><span class="tag ${x.ok ? (x.replayed ? "tag-warn" : "tag-ok") : "tag-bad"}">${x.status}</span></td><td>${x.replayed ? "yes" : "no"}</td><td><code>${x.bid ? short(x.bid) : "-"}</code></td><td class="tnum">${x.tries}</td><td class="r tnum">${dur(x.ms)}</td></tr>`)}</tbody></table>`
        : html`<p class="small muted">Requests are sent in parallel, which is the hard case: two copies can reach the server at the same instant. A copy the rate limiter sheds is sent again with the same key after Retry-After.</p>`}
    </section>`;
}
async function runRetry() {
  const r = state.retry;
  const l = r.lots.find(x => x.id === r.lotId);
  if (!l || r.running) return;
  r.running = true; r.err = null; paintRetry();
  try {
    const key = uuid();
    // the price may have moved since the list was drawn: ask for the minimum now
    const amount = toLot(await patient(() => be.getAuction(l.id))).minNext;
    const w0 = await patient(() => be.wallet());
    const answers = await Promise.all(Array.from({ length: r.copies }, () => bidWithRetry(l.id, amount, key)));
    const w1 = await patient(() => be.wallet());
    const bids = answers.filter(x => x.ok && x.data?.bid).map(x => x.data.bid.id);
    const distinct = new Set(bids).size;
    const history = await patient(() => be.bids(l.id, { limit: 50 }));
    const stored = history.bids.filter(b => bids.includes(b.id)).length;
    r.result = {
      key, amount, copies: r.copies, distinct, replayed: answers.filter(x => x.ok && x.replayed).length,
      heldDelta: w1.reserved - w0.reserved, rows: answers.map(x => ({ ok: x.ok, status: x.status, replayed: x.replayed, bid: x.data?.bid?.id, ms: x.ms, tries: x.tries })),
      ok: distinct === 1 && stored <= 1 && answers.every(x => x.ok),
    };
    if (!answers.every(x => x.ok)) r.err = "Some copies were refused: " + [...new Set(answers.filter(x => !x.ok).map(x => x.error?.message))].join(", ");
    checkBooks();
  } catch (e) { r.err = e.message; }
  r.running = false; paintRetry();
}
function paintRetry() { const el = $("#retrySec"); if (el) { el.innerHTML = String(retryHTML()); bindRetry(); } }
function bindRetry() {
  $("#rtLot")?.addEventListener("change", e => { state.retry.lotId = e.target.value; });
  $("#rtN")?.addEventListener("change", e => { state.retry.copies = +e.target.value; paintRetry(); });
  $("#retry")?.addEventListener("click", runRetry);
}

/* ------------------------------------------------------------------ token bucket */
function bucketHTML() {
  const b = state.bucket, last = b.samples.at(-1);
  const cap = b.limit || 10, left = last?.remaining ?? cap;
  return html`
    <section class="ops-sec" aria-labelledby="tbH">
      <h2 class="h2" id="tbH">Rate limit bucket</h2>
      <p class="lead">Every bidder owns a bucket of ${cap} tokens that refills at 5 a second. Each bid takes one. When it is empty the gateway answers 429 with Retry-After, before any database work.</p>
      <div class="bucket" role="img" aria-label="${left} of ${cap} tokens left">
        ${Array.from({ length: cap }, (_, i) => html`<i class="${i < left ? "on" : ""}"></i>`)}
        <b class="tnum">${left}<span class="muted"> / ${cap}</span></b>
      </div>
      <div class="lab-controls">
        <button class="btn btn-line ${b.running ? "busy" : ""}" id="burst" ${b.running || !be.session.id ? "disabled" : ""}>Send 15 requests in a burst</button>
        <span class="small muted">They are bids at a price too low to place, so the rules refuse them and nothing changes hands. The limiter still counts each one.</span>
      </div>
      ${b.err ? html`<p class="form-error" role="alert">${b.err}</p>` : ""}
      ${b.off ? html`<p class="small muted">This API sends no RateLimit headers, so rate limits are off (RATE_LIMITS=off).</p>` : ""}
      ${b.samples.length ? html`
        <ol class="burst">${b.samples.map((s, i) => html`<li class="${s.status === 429 ? "bad" : "ok"}" title="Request ${i + 1}: ${s.status}"><span class="tnum">${s.status}</span></li>`)}</ol>
        <p class="small muted">${b.samples.filter(s => s.status === 429).length} of ${b.samples.length} were shed with 429. A 422 means the request got past the limiter and was refused by the rules.</p>` : ""}
    </section>`;
}
async function runBurst() {
  const b = state.bucket;
  if (b.running) return;
  b.running = true; b.err = null; b.samples = []; paintBucket();
  try {
    if (sim) await be.chaos.setRateLimits(true);
    const lots = (await be.allAuctions({ status: "ACTIVE" }, 1)).filter(x => x.owner_id !== be.session.id);
    if (!lots.length) throw new Error("There is no open lot to aim at.");
    const target = lots[0].id;
    const out = await Promise.all(Array.from({ length: 15 }, () => be.placeBid(target, 1, { key: uuid() })));
    b.samples = out.map(r => ({ status: r.status, remaining: r.rate?.remaining ?? null, limit: r.rate?.limit ?? null }));
    const withRate = b.samples.filter(s => s.limit != null);
    b.off = !withRate.length;
    b.limit = withRate[0]?.limit ?? b.limit;
    b.samples.sort((x, y) => (x.status === 429) - (y.status === 429));
    const lastKnown = withRate.map(s => s.remaining).filter(v => v != null);
    if (lastKnown.length) b.samples[b.samples.length - 1].remaining = Math.min(...lastKnown);
  } catch (e) { b.err = e.message; }
  b.running = false; paintBucket();
}
function paintBucket() { const el = $("#bucketSec"); if (el) { el.innerHTML = String(bucketHTML()); $("#burst")?.addEventListener("click", runBurst); } }

/* ------------------------------------------------------------------ boot */
async function boot() {
  if (!operatorGateless()) return;
  body.setAttribute("aria-busy", "false");
  // the two halves of this page sit at different points of the tour
  const part = new URLSearchParams(location.search).get("part");
  const parts = {
    request: html`<div id="retrySec">${retryHTML()}</div><div id="bucketSec">${bucketHTML()}</div>`,
    money: html`<div id="books">${booksHTML()}</div><div id="attack">${attackHTML()}</div>`,
  };
  const all = html`${parts.request}${parts.money}`;
  body.innerHTML = String(part === "request" ? parts.request : part === "money" ? parts.money : all);
  const head = {
    request: ["The request", "Before a bid reaches the rules it has to get past the gateway. A retry must never bid twice, and a burst must be shed before it costs any database work."],
    money: ["The money", "Money is the part of an auction nobody can afford to get wrong. Attack it and audit it from here, and read the result yourself."],
  }[part];
  if (head) { $(".page-head h1").textContent = head[0]; $(".page-head p").textContent = head[1]; document.title = "Marque | " + head[0]; }
  mountTourBar(part === "request" ? "request" : "money");
  bindAttack(); bindRetry(); paintBucket();
  try {
    const lots = (await openLots(be)).sort((a, b) => a.lot.minNext - b.lot.minNext);
    state.retry.lots = lots; state.retry.lotId = lots[0]?.id || "";
    paintRetry();
  } catch (e) { state.retry.err = e.message; paintRetry(); }
  if ($("#books")) checkBooks();
  if ($("#books")) bookTimer = setInterval(() => { if (!document.hidden) checkBooks(); }, 3000);
}
// the page works for everyone; only the audit needs an operator
function operatorGateless() {
  if (be.unreachable) { operatorGate(be, body, { next: "trust.html", what: "The trust checks need an API to talk to." }); return false; }
  return true;
}
boot();
