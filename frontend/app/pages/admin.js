// Operator console: GET /v1/admin/reconcile and the dead-letter outbox.
import { initShell, toast } from "../shell.js";
import { html, raw, $, $$, usd, dateTime, ago } from "../util.js";

const be = await initShell({ bar: true, footer: true, requireAuth: true });
if (!be.session.id) throw new Error("redirecting to sign in");
const body = $("#adminBody");
let books = null, failed = [], error = null, checking = false;

function gate() {
  body.setAttribute("aria-busy", "false");
  body.innerHTML = String(html`<div class="empty"><h3>Operators only</h3><p>This account is not an operator, so the API refuses these checks. ${be.kind === "sim" ? "In the demo, sign in as the Operator account." : "Make an account an operator with: go run ./cmd/admin set-role <email> admin"}</p><a class="btn" href="account.html?next=admin.html">Switch account</a></div>`);
}

function draw() {
  body.setAttribute("aria-busy", "false");
  const b = books;
  body.innerHTML = String(html`
    <section class="ops-sec" aria-labelledby="bkH">
      <div class="sec-head"><h2 class="h2" id="bkH">Do the books balance?</h2><button class="btn btn-line btn-sm ${checking ? "busy" : ""}" id="run">Run the check</button></div>
      ${b ? html`
        <div class="verdict ${b.ok ? "ok" : "bad"}" role="status">
          <b>${b.ok ? "Balanced" : "Does not balance"}</b>
          <span>${b.ok ? "Every wallet equals the sum of its ledger lines, every journal sums to zero, and the held money equals the leading bids." : `${b.mismatches} wallets disagree with the ledger and ${b.unbalanced_journals} journals do not sum to zero. Do not move money until this is understood.`}</span>
        </div>
        <div class="tiles">
          <div class="tile"><span class="lab mono">Money deposited</span><b class="tnum">${usd(b.money_deposited)}</b><span class="small muted">all deposits ever made</span></div>
          <div class="tile"><span class="lab mono">In wallets</span><b class="tnum">${usd(b.money_in_wallets)}</b><span class="small muted">available plus held</span></div>
          <div class="tile"><span class="lab mono">Held in wallets</span><b class="tnum">${usd(b.reserved_in_wallets)}</b><span class="small muted">reserved by bids</span></div>
          <div class="tile"><span class="lab mono">Leading bids</span><b class="tnum">${usd(b.active_reservations_total)}</b><span class="small muted">on open auctions</span></div>
        </div>
        <p class="small muted">The check recomputes every balance from the append-only ledger and compares it with the stored balance. Money is never changed in place: each movement is a journal whose lines sum to zero.</p>`
        : html`<div class="skel" style="height:140px"></div>`}
    </section>

    <section class="ops-sec" aria-labelledby="dlH">
      <div class="sec-head"><h2 class="h2" id="dlH">Dead-letter outbox</h2><span class="mono muted">${failed.length} parked</span></div>
      <p class="small muted">An event that failed 8 times with backoff is parked here instead of blocking the queue. Fix what made it fail, then retry it.</p>
      ${failed.length ? html`
        <div class="tscroll"><table class="tbl"><thead><tr><th>Event</th><th>Attempts</th><th>Last error</th><th>Failed</th><th class="r"><span class="visually">Action</span></th></tr></thead>
          <tbody>${failed.map(e => html`<tr><td><b>${e.event_type}</b><br><span class="mono small muted">${String(e.id).slice(0, 13)}</span></td><td class="tnum">${e.attempts}</td><td class="err-text">${e.last_error || "no error recorded"}</td><td class="nowrap">${ago(e.failed_at)}</td><td class="r"><button class="btn btn-line btn-sm" data-retry="${e.id}">Retry</button></td></tr>`)}</tbody></table></div>`
        : html`<div class="empty"><h3>Nothing parked</h3><p>Every event has been delivered. If one fails 8 times it will appear here.</p></div>`}
    </section>
    ${error ? html`<p class="form-error" role="alert">${error}</p>` : ""}`);
  $("#run")?.addEventListener("click", () => load(true));
  $$("[data-retry]", body).forEach(b => b.addEventListener("click", async () => {
    b.classList.add("busy");
    try { await be.retryOutbox(b.dataset.retry); toast("Put back in the queue"); await load(); } catch (e) { b.classList.remove("busy"); toast(e.message); }
  }));
}

async function load(manual) {
  checking = true; if (manual) draw();
  try {
    [books, failed] = await Promise.all([be.reconcile(), be.failedOutbox()]);
    error = null;
  } catch (e) {
    if (e.status === 403 || e.status === 401) { gate(); checking = false; return; }
    error = e.message;
  }
  checking = false; draw();
}

if (!be.session.isAdmin) gate(); else { draw(); load(); }
