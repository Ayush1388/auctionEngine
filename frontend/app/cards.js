// Brings the lot cards on the home and event pages to life.
//
// The pages draw their cards from the catalogue (so they are never empty). This
// replaces the numbers with the backend's, follows each lot live, and makes the
// Bid button place a real bid at the minimum, or send you to sign in.
import { liveLots } from "./lots.js";
import { applySnapshot, secondsLeft } from "./model.js";
import { toast } from "./shell.js";
import { usd, clock, $, $$ } from "./util.js";

const ALIAS = { boss: "boss429" };

export async function hydrateCards(be, root = document) {
  const cards = $$(".card[id^='lot-']", root);
  if (!cards.length) return;
  let lots;
  try { lots = await liveLots(be); } catch { return; }
  const live = cards.map(el => ({ el, key: ALIAS[el.id.slice(4)] || el.id.slice(4) })).filter(c => lots.has(c.key));
  if (!live.length) return;
  window.marqueLive = true;              // tells the page scripts to stop their simulated bidding

  for (const c of live) {
    c.lot = lots.get(c.key);
    c.bidEl = $(".bid", c.el); c.timeEl = $(".time", c.el); c.nextEl = $(".next", c.el); c.bidsEl = $(".bids", c.el); c.btn = $(".bid-btn", c.el);
    const link = c.el.querySelector("h3 a"); if (link) link.href = "lot.html?id=" + c.lot.id;
    c.el.querySelectorAll("a[href^='lot.html?id=']").forEach(a => { a.href = "lot.html?id=" + c.lot.id; });
    const paint = flash => {
      const l = c.lot;
      window.tickBid ? window.tickBid(c.bidEl, l.current ?? l.startingPrice, usd) : (c.bidEl.textContent = usd(l.current ?? l.startingPrice));
      c.nextEl.textContent = usd(l.minNext); c.bidsEl.textContent = l.bids + (l.bids === 1 ? " bid" : " bids");
      const open = l.status === "ACTIVE";
      c.btn.disabled = !open; c.btn.firstChild.textContent = open ? "Bid " : l.status === "COMPLETED" ? "Auction closed" : "Not open";
      c.nextEl.hidden = !open; c.el.classList.toggle("closed", !open);
      if (flash) { c.el.classList.remove("flash"); void c.el.offsetWidth; c.el.classList.add("flash"); }
    };
    const tick = () => {
      const l = c.lot, left = secondsLeft(l, be.feed.serverNow());
      c.timeEl.textContent = l.status === "ACTIVE" ? clock(left) : l.status === "COMPLETED" ? "Closed" : l.status === "NOT_ACTIVE" ? "Soon" : "Cancelled";
      c.el.classList.toggle("urgent", l.status === "ACTIVE" && left < 1800);
    };
    c.paint = paint; c.tick = tick; paint(false); tick();
    be.feed.subscribe(c.lot.id, ev => { c.lot = applySnapshot(c.lot, ev.auction); paint(!!ev.bid); tick(); });
    c.btn.addEventListener("click", async () => {
      if (!be.session.id) { location.href = "account.html?next=" + encodeURIComponent(location.pathname.split("/").pop() + location.search); return; }
      c.btn.disabled = true;
      const r = await be.placeBid(c.lot.id, c.lot.minNext);
      c.btn.disabled = c.lot.status !== "ACTIVE";
      if (r.ok) { toast("Bid placed: " + usd(r.amount)); }
      else if (r.error?.minimum != null) toast("Another bid landed first. The minimum is now " + usd(r.error.minimum));
      else toast(r.error?.message || "Could not place the bid");
    });
  }
  setInterval(() => live.forEach(c => c.tick()), 1000);
}
