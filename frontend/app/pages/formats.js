// Auction formats: Dutch and sealed-bid, run in the browser with the engine's money rules.
// Demo only. The API sells English auctions; this page says so and never touches it.
import { initShell } from "../shell.js";
import { mountTourBar } from "../tour.js";
import { dutchPrice, dutchDuration, settleSealed, FormatLedger } from "../formats.js";
import { html, $, $$, usd, reduceMotion } from "../util.js";

await initShell({ bar: true, footer: true, nav: "" });
mountTourBar("beyond");
const body = $("#formatsBody");
const D = 100;

/* ------------------------------------------------------------------ Dutch */
const dutchCfg = { start: 160_000 * D, floor: 90_000 * D, step: 2_500 * D, intervalMs: 700 };
const dutch = { t0: 0, running: false, sold: null, ledger: null, raf: 0 };
function dutchStart() {
  dutch.ledger = new FormatLedger(); dutch.ledger.open("you", 200_000 * D); dutch.ledger.open("seller");
  dutch.t0 = performance.now(); dutch.running = true; dutch.sold = null;
  tickDutch();
}
function tickDutch() {
  cancelAnimationFrame(dutch.raf);
  const el = performance.now() - dutch.t0, price = dutchPrice(dutchCfg, el);
  const root = $("#dutchPrice"); if (root) root.textContent = usd(price);
  const left = $("#dutchLeft"); if (left) left.textContent = el >= dutchDuration(dutchCfg) ? "At the floor. Nobody has bought it." : `Falls ${usd(dutchCfg.step)} every ${dutchCfg.intervalMs / 1000} s to a floor of ${usd(dutchCfg.floor)}`;
  if (dutch.running && el < dutchDuration(dutchCfg) + 2000) dutch.raf = requestAnimationFrame(tickDutch);
}
function dutchAccept() {
  if (!dutch.running) return;
  const price = dutchPrice(dutchCfg, performance.now() - dutch.t0);
  dutch.running = false;
  dutch.ledger.hold("you", price); dutch.ledger.settle("you", "seller", price);
  dutch.sold = { price, saved: dutchCfg.start - price };
  paint();
}

/* ------------------------------------------------------------------ sealed */
const RIVALS = [["Bidder 184", 118_000], ["Bidder 291", 131_500], ["Bidder 407", 124_000], ["Bidder 52", 109_500]];
const sealed = { rule: "second", bid: 125_000, result: null, ledger: null, reserve: 100_000 };
function sealedReveal() {
  const you = Math.max(1, Math.round(sealed.bid)) * D;
  const L = new FormatLedger(); L.open("you", 300_000 * D); L.open("seller");
  const bids = [{ id: "you", amount: you, at: 0 }];
  RIVALS.forEach(([name, v], i) => { L.open(name, 400_000 * D); bids.push({ id: name, amount: v * D, at: i + 1 }); });
  // every bid is held when it is sent
  bids.forEach(b => L.hold(b.id, b.amount));
  const r = settleSealed(bids, { rule: sealed.rule, reserve: sealed.reserve * D });
  for (const b of bids) { if (r.sold && b.id === r.winner) { L.settle(b.id, "seller", r.price); L.release(b.id, b.amount - r.price); } else L.release(b.id, b.amount); }
  sealed.ledger = L; sealed.result = { ...r, you };
  paint();
}

/* ------------------------------------------------------------------ page */
function paint() {
  const r = sealed.result;
  body.setAttribute("aria-busy", "false");
  body.innerHTML = String(html`
    <p class="banner warn">Demo only. The Marque API sells English (ascending) auctions. These two run in your browser, on a small ledger that follows the same rules: hold the bid, release the losers, settle the winner, every journal sums to zero.</p>
    <div class="split-2">
      <section class="ops-sec dutch" aria-labelledby="duH">
        <h2 class="h2" id="duH">Dutch auction</h2>
        <p class="lead">The price starts high and falls. The first person to accept pays the price on the clock and the sale is over, so waiting is a bet that nobody else will buy first.</p>
        <b class="dutch-price tnum" id="dutchPrice" aria-live="off">${usd(dutch.sold ? dutch.sold.price : dutchCfg.start)}</b>
        <p class="small muted" id="dutchLeft">${dutch.running ? "" : `Starts at ${usd(dutchCfg.start)}, falls ${usd(dutchCfg.step)} every ${dutchCfg.intervalMs / 1000} s to a floor of ${usd(dutchCfg.floor)}`}</p>
        <div class="lab-controls">
          ${dutch.running ? html`<button class="btn" id="accept">Buy at the current price</button>` : html`<button class="btn" id="dutchGo">${dutch.sold ? "Run it again" : "Start the clock"}</button>`}
        </div>
        ${dutch.sold ? html`<div class="verdict ok" role="status"><b>Sold at ${usd(dutch.sold.price)}</b><span>That is ${usd(dutch.sold.saved)} under the opening price. The seller received ${usd(dutch.ledger.get("seller").available)}. You hold ${usd(dutch.ledger.get("you").available)} of the ${usd(200_000 * D)} you started with, and the journals ${dutch.ledger.balanced() ? "all sum to zero" : "do not balance"}.</span></div>` : ""}
      </section>

      <section class="ops-sec" aria-labelledby="seH">
        <h2 class="h2" id="seH">Sealed-bid auction</h2>
        <p class="lead">Everyone sends one hidden bid before the close. The highest wins. What the winner pays depends on the rule.</p>
        <div class="presets" role="group" aria-label="Pricing rule">
          <button class="preset" data-rule="first" aria-pressed="${sealed.rule === "first"}">First price</button>
          <button class="preset" data-rule="second" aria-pressed="${sealed.rule === "second"}">Second price</button>
        </div>
        <p class="small muted">${sealed.rule === "first" ? "The winner pays their own bid, so the smart move is to shade it below what the car is worth to you." : "The winner pays the runner-up's bid (or the reserve). Bidding exactly what it is worth to you is the best strategy, because your bid never sets your price."}</p>
        <div class="lab-controls">
          <label class="field"><span class="field-label">Your sealed bid (reserve ${usd(sealed.reserve * D)})</span><div class="money"><input id="sb" inputmode="numeric" value="${sealed.bid.toLocaleString("en-US")}" aria-label="Your sealed bid in dollars"></div></label>
          <button class="btn" id="reveal">Seal and reveal</button>
        </div>
        ${r ? html`
          <div class="verdict ${r.sold && r.winner === "you" ? "ok" : r.sold ? "bad" : "bad"}" role="status">
            <b>${!r.sold ? "No sale" : r.winner === "you" ? "You won" : `${r.winner} won`}</b>
            <span>${!r.sold ? `The highest bid was below the reserve.` : `${r.winner === "you" ? "You pay" : "The winner pays"} ${usd(r.price)}${sealed.rule === "second" && r.ranked[0].amount !== r.price ? `, not their bid of ${usd(r.ranked[0].amount)}` : ""}. Losing bids were released in full; the ledger ${sealed.ledger.balanced() ? "balances" : "does not balance"}.`}</span>
          </div>
          <div class="sealed-grid">${r.ranked.map(b => html`<div class="sealed-card ${r.sold && b.id === r.winner ? "is-winner" : ""}"><span class="small muted">${b.id === "you" ? "You" : b.id}</span><b class="tnum">${usd(b.amount)}</b><span class="small">${r.sold && b.id === r.winner ? "winner" : "released"}</span></div>`)}</div>` : ""}
      </section>
    </div>

    <section class="ops-sec" aria-labelledby="cmpH" style="margin-top:clamp(28px,4vw,52px)">
      <h2 class="h2" id="cmpH">How they differ</h2>
      <div class="tscroll"><table class="tbl"><thead><tr><th>Format</th><th>Price is set by</th><th>Money held</th><th>Race condition to design for</th></tr></thead><tbody>
        <tr><td>English (this engine)</td><td>Bids that rise, one increment at a time</td><td>Only the leading bid</td><td>Many bids on one price; solved with a row lock and idempotency keys</td></tr>
        <tr><td>Dutch</td><td>A clock that falls</td><td>Only at the moment of acceptance</td><td>Two buyers accept in the same tick; the first commit wins, the second sees "already sold"</td></tr>
        <tr><td>Sealed first price</td><td>The highest hidden bid</td><td>Every bid, until the close</td><td>None while open; the close must read every bid at one instant</td></tr>
        <tr><td>Sealed second price</td><td>The runner-up's hidden bid</td><td>Every bid, until the close</td><td>As above; the price depends on a bid that lost</td></tr>
      </tbody></table></div>
    </section>`);
  $("#dutchGo")?.addEventListener("click", () => { dutchStart(); paint(); });
  $("#accept")?.addEventListener("click", dutchAccept);
  $$("[data-rule]").forEach(b => b.addEventListener("click", () => { sealed.rule = b.dataset.rule; if (sealed.result) sealedReveal(); else paint(); }));
  $("#reveal")?.addEventListener("click", () => { sealed.bid = Number($("#sb").value.replace(/[^0-9]/g, "")) || sealed.bid; sealedReveal(); });
  if (dutch.running) tickDutch();
}
void reduceMotion;
paint();
