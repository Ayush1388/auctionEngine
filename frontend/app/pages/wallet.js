// Wallet: balances, adding funds, and the statement (the double-entry ledger).
import { initShell, toast, shell, refreshWallet } from "../shell.js";
import { toLot } from "../model.js";
import { html, raw, esc, $, $$, usd, parseUsd, dateTime, uuid } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "wallet", requireAuth: true });
if (!be.session.id) throw new Error("redirecting to sign in");
const body = $("#walletBody");

const LIMIT = 1_000_000_000;                     // the API's cap on one deposit, in cents ($10,000,000)
const PRESETS = [50_000, 250_000, 1_000_000].map(d => d * 100);
let wallet = null, entries = [], nextBefore = null, filter = "all", loading = false, intent = null;
const titles = new Map();

async function titleOf(id) {
  if (!id) return "";
  if (titles.has(id)) return titles.get(id);
  const t = be.getAuction(id).then(a => toLot(a).title).catch(() => "a lot");
  titles.set(id, t);
  return t;
}

/* ---------------------------------------------------------------- balances and funding */
function balanceHTML() {
  const w = wallet, held = w ? w.reserved / (w.total || 1) * 100 : 0;
  return html`
    <div class="bal">
      <span class="mono lab">Available to bid</span>
      <b class="bal-big tnum" id="avail">${w ? usd(w.available) : "-"}</b>
      <div class="bal-two">
        <div><span class="mono lab">Held in bids</span><b class="tnum ${w?.reserved ? "pos" : ""}">${w ? usd(w.reserved) : "-"}</b></div>
        <div><span class="mono lab">Total</span><b class="tnum">${w ? usd(w.total) : "-"}</b></div>
      </div>
      <div class="ratio" role="img" aria-label="${w ? `${Math.round(held)} percent of your money is held in bids` : ""}"><i style="width:${held.toFixed(1)}%"></i><b style="width:${(100 - held).toFixed(1)}%"></b></div>
      <p class="small muted">The red part is held while you lead a lot. It returns to available the moment you are outbid.</p>
    </div>
    <form class="fund" id="fund" novalidate>
      <h2 class="h2">Add funds</h2>
      <div class="presets" role="group" aria-label="Amounts">${PRESETS.map(c => html`<button type="button" class="preset" data-amt="${c}">${usd(c)}</button>`)}</div>
      <div class="field" id="fundField"><label for="fundAmt">Amount in dollars</label>
        <div class="money"><input id="fundAmt" inputmode="numeric" autocomplete="off" placeholder="250,000"></div><span class="err" id="fundErr"></span></div>
      <button class="btn btn-block" id="fundGo">Add funds</button>
      <p class="small muted">These are test funds. Each deposit carries an Idempotency-Key, so a double click or a retry adds the money once.</p>
    </form>`;
}

function drawBalance() {
  const card = $(".wallet-card");
  card.innerHTML = String(balanceHTML());
  $$("[data-amt]", card).forEach(b => b.addEventListener("click", () => { $("#fundAmt").value = (+b.dataset.amt / 100).toLocaleString("en-US"); $$("[data-amt]", card).forEach(x => x.setAttribute("aria-pressed", x === b)); }));
  $("#fundAmt").addEventListener("input", () => { $$("[data-amt]", card).forEach(x => x.setAttribute("aria-pressed", "false")); });
  $("#fund").addEventListener("submit", deposit);
}

async function deposit(e) {
  e.preventDefault();
  const cents = parseUsd($("#fundAmt").value), field = $("#fundField"), err = $("#fundErr"), go = $("#fundGo");
  const fail = m => { field.classList.add("invalid"); err.textContent = m; };
  field.classList.remove("invalid"); err.textContent = "";
  if (!cents) return fail("Enter an amount.");
  if (cents > LIMIT) return fail(`One deposit is limited to ${usd(LIMIT)}.`);
  // one intent = one key: clicking twice on the same amount is the same deposit
  if (!intent || intent.cents !== cents || Date.now() - intent.at > 5000) intent = { cents, key: uuid(), at: Date.now() };
  intent.at = Date.now();
  go.classList.add("busy");
  try {
    wallet = await be.deposit(cents, intent.key);
    toast(`${usd(cents)} added`);
    $("#fundAmt").value = "";
    drawBalance(); refreshWallet(); await reload();
  } catch (x) { fail(x.status === 422 ? "That amount was refused. Try a smaller one." : x.message); }
  finally { $("#fundGo")?.classList.remove("busy"); }
}

/* ---------------------------------------------------------------- the statement */
const KIND = {
  DEPOSIT: { label: "Funds added", cls: "pos" },
  RESERVE: { label: "Held for bid", cls: "" },
  RELEASE: { label: "Hold released", cls: "" },
  SETTLE: { label: "Sale settled", cls: "" },
};

/** The API returns one line per account per journal; a person reads one row per journal. */
function journals() {
  const by = new Map();
  for (const e of entries) {
    const j = by.get(e.transaction_id) || { id: e.transaction_id, kind: e.kind, auction: e.auction_id, at: e.created_at, lines: [], seq: e.id };
    j.lines.push(e); j.seq = Math.max(j.seq, e.id); by.set(e.transaction_id, j);
  }
  return [...by.values()].sort((a, b) => b.seq - a.seq);
}

const FILTERS = [["all", "All"], ["DEPOSIT", "Deposits"], ["RESERVE", "Holds"], ["RELEASE", "Releases"], ["SETTLE", "Sales"]];

async function statementHTML() {
  const js = journals().filter(j => filter === "all" || j.kind === filter);
  const names = await Promise.all(js.map(j => titleOf(j.auction)));
  const rows = js.map((j, i) => {
    const av = j.lines.filter(l => l.account === "available").reduce((s, l) => s + l.amount, 0);
    const rs = j.lines.filter(l => l.account === "reserved").reduce((s, l) => s + l.amount, 0);
    const sum = j.lines.reduce((s, l) => s + l.amount, 0);
    const k = KIND[j.kind] || { label: j.kind };
    const what = j.kind === "SETTLE" ? (av > 0 ? "Sold" : "Paid for") : k.label;
    const money = v => v ? html`<span class="tnum ${v > 0 ? "pos" : "neg"}">${v > 0 ? "+" : "-"}${usd(Math.abs(v))}</span>` : html`<span class="muted">-</span>`;
    return html`
      <tr>
        <td class="nowrap">${dateTime(j.at)}</td>
        <td><b>${what}</b>${names[i] ? html`<br><a class="small" href="lot.html?id=${j.auction}">${names[i]}</a>` : ""}</td>
        <td class="r">${money(av)}</td><td class="r">${money(rs)}</td>
        <td class="r"><button class="linkish" data-open="${j.id}" aria-expanded="false" aria-controls="j-${j.id}">Lines</button></td>
      </tr>
      <tr class="jl" id="j-${j.id}" hidden><td colspan="5">
        <ul class="lines">${j.lines.sort((a, b) => a.id - b.id).map(l => html`<li><span class="mono">${l.account}</span><span class="tnum ${l.amount > 0 ? "pos" : "neg"}">${l.amount > 0 ? "+" : "-"}${usd(Math.abs(l.amount))}</span></li>`)}</ul>
        <p class="small muted">${j.lines.length > 1 ? (sum === 0 ? "Both lines are yours and they add up to zero: money moved between your two accounts and none was created." : "Lines do not add up here.") : "The other line of this journal is on another account (the funding source or the seller), so it is not shown here."} Journal <span class="mono">${j.id.slice(0, 13)}</span></p>
      </td></tr>`;
  });
  return html`
    <div class="tabs" role="tablist" aria-label="Filter the statement">${FILTERS.map(([k, l]) => html`<button role="tab" aria-selected="${filter === k}" data-f="${k}">${l}</button>`)}</div>
    ${js.length ? html`
      <table class="tbl stmt"><thead><tr><th>Date</th><th>Movement</th><th class="r">Available</th><th class="r">Held</th><th class="r"><span class="visually">Details</span></th></tr></thead><tbody>${rows}</tbody></table>
      ${nextBefore != null ? html`<div class="more"><button class="btn btn-line btn-sm" id="older">Load older entries</button></div>` : ""}`
      : html`<div class="empty"><h3>${filter === "all" ? "Nothing here yet" : "No entries of this kind"}</h3><p>${filter === "all" ? "Add funds, then place a bid. Each hold, release and sale shows up here the moment it happens." : "Try another filter."}</p>${filter === "all" ? html`<a class="btn" href="index.html#classic">Browse cars</a>` : ""}</div>`}`;
}

async function drawStatement() {
  const el = $("#statement");
  el.innerHTML = String(await statementHTML());
  $$("[data-f]", el).forEach(b => b.addEventListener("click", () => { filter = b.dataset.f; drawStatement(); }));
  $$("[data-open]", el).forEach(b => b.addEventListener("click", () => {
    const row = $("#j-" + b.dataset.open), open = row.hidden; row.hidden = !open; b.setAttribute("aria-expanded", open); b.textContent = open ? "Hide" : "Lines";
  }));
  $("#older")?.addEventListener("click", async () => { await load(nextBefore); drawStatement(); });
}

async function load(before) {
  if (loading) return; loading = true;
  try { const p = await be.ledger({ limit: 50, before }); entries = before ? entries.concat(p.entries) : p.entries; nextBefore = p.next_before; }
  finally { loading = false; }
}
async function reload() { await load(); await drawStatement(); }

/* ---------------------------------------------------------------- boot */
body.setAttribute("aria-busy", "false");
body.innerHTML = `<section class="wallet-card" aria-label="Balance"></section><section aria-label="Statement"><h2 class="h2">Statement</h2><div id="statement"><div class="skel" style="height:320px;margin-top:16px"></div></div></section>`;
try {
  wallet = await be.wallet();
  drawBalance();
  await load();
  await drawStatement();
} catch (e) {
  body.innerHTML = String(html`<div class="empty" style="grid-column:1/-1"><h3>Could not load your wallet</h3><p>${e.message}</p><button class="btn" onclick="location.reload()">Try again</button></div>`);
}
shell.on("wallet", async w => { if (!wallet || w.available !== wallet.available || w.reserved !== wallet.reserved) { wallet = w; drawBalance(); await reload(); } });
