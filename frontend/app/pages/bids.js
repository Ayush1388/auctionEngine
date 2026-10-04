// My bids: Leading, Outbid, Won, Lost, and the lots I saved. Standing is worked
// out from the wallet ledger and the lot itself (see app/mybids.js) and then kept
// current from the live feed.
import { initShell } from "../shell.js";
import { loadMyBids, standing } from "../mybids.js";
import { toLot, applySnapshot, secondsLeft } from "../model.js";
import { html, raw, $, $$, usd, clock, local, reduceMotion } from "../util.js";

const be = await initShell({ bar: true, footer: true, requireAuth: true });
if (!be.session.id) throw new Error("redirecting to sign in");
const body = $("#bidsBody");
const me = be.session.id;

const TABS = [["leading", "Leading"], ["outbid", "Outbid"], ["won", "Won"], ["lost", "Lost"], ["saved", "Saved"]];
const EMPTY = {
  leading: ["You do not lead any lot", "Place a bid and it shows up here, with the amount held from your wallet."],
  outbid: ["You are not outbid anywhere", "When someone takes the lead on a lot you bid on, it moves here so you can answer."],
  won: ["No wins yet", "Lots you win appear here once the sale closes, with a receipt."],
  lost: ["Nothing lost", "Closed lots you did not win appear here. Your held money was released when the sale ended."],
  saved: ["No saved lots", "Use Save on a lot page to keep it here."],
};
let tab = new URLSearchParams(location.search).get("tab") || "leading";
let rows = [], saved = [];
const unsubs = new Map();

function standingOf(r) { return standing(r.lot, me); }
const counts = () => Object.fromEntries(TABS.map(([k]) => [k, k === "saved" ? saved.length : rows.filter(r => standingOf(r) === k).length]));

function rowHTML(r, isSaved) {
  const l = r.lot, st = isSaved ? (l.status === "ACTIVE" ? "active" : l.status.toLowerCase()) : standingOf(r), live = l.status === "ACTIVE";
  const label = { leading: "Leading", outbid: "Outbid", won: "Won", lost: "Lost", active: "Open", completed: "Closed", cancelled: "Cancelled", not_active: "Opens soon" }[st] || st;
  const cta = st === "outbid" ? html`<a class="btn btn-sm" href="lot.html?id=${l.id}">Bid ${usd(l.minNext)}</a>` : st === "won" ? html`<a class="btn btn-sm" href="lot.html?id=${l.id}">Receipt</a>` : html`<a class="btn btn-line btn-sm" href="lot.html?id=${l.id}">${live ? "Open lot" : "View"}</a>`;
  return html`
    <li class="brow" data-id="${l.id}">
      <a class="thumb" href="lot.html?id=${l.id}" tabindex="-1" aria-hidden="true"><img src="${l.img}" alt="" style="object-position:${l.pos}" loading="lazy"></a>
      <div class="brow-main">
        <h2><a href="lot.html?id=${l.id}">${l.title}</a></h2>
        <p class="small muted">${[l.lotNo ? "Lot " + l.lotNo : "", l.where].filter(Boolean).join("  |  ")}</p>
      </div>
      <div class="brow-fig"><span class="mono lab">${isSaved ? "Current bid" : "Your top bid"}</span><b class="tnum">${isSaved ? usd(l.current ?? l.startingPrice) : usd(r.myTop)}</b>${isSaved ? "" : html`<span class="small muted">${r.myCount} ${r.myCount === 1 ? "bid" : "bids"}</span>`}</div>
      <div class="brow-fig"><span class="mono lab">${isSaved ? "Bids" : "Current bid"}</span><b class="tnum">${isSaved ? l.bids : usd(l.current)}</b></div>
      <div class="brow-fig"><span class="mono lab">${live ? "Ends in" : "Status"}</span><b class="tnum brow-clock" data-end="${l.endsAt}">${live ? clock(secondsLeft(l)) : label}</b></div>
      <div class="brow-act"><span class="tag tag-${st}">${label}</span>${cta}</div>
    </li>`;
}

function draw() {
  body.setAttribute("aria-busy", "false");
  const c = counts(), list = tab === "saved" ? saved : rows.filter(r => standingOf(r) === tab);
  body.innerHTML = String(html`
    <div class="tabs" role="tablist" aria-label="My bids">${TABS.map(([k, l]) => html`<button role="tab" aria-selected="${tab === k}" data-tab="${k}">${l}<span class="count tnum">${c[k]}</span></button>`)}</div>
    ${list.length ? html`<ul class="brows" aria-live="polite">${list.map(r => rowHTML(tab === "saved" ? { lot: r } : r, tab === "saved"))}</ul>`
      : html`<div class="empty"><h3>${EMPTY[tab][0]}</h3><p>${EMPTY[tab][1]}</p><a class="btn" href="index.html#classic">Browse cars</a></div>`}`);
  $$("[data-tab]", body).forEach(b => b.addEventListener("click", () => { tab = b.dataset.tab; history.replaceState(null, "", "?tab=" + tab); draw(); }));
}

/* keep every open lot current without a refresh */
function follow(r) {
  if (unsubs.has(r.lot.id) || r.lot.status !== "ACTIVE") return;
  unsubs.set(r.lot.id, be.feed.subscribe(r.lot.id, ev => {
    r.lot = applySnapshot(r.lot, ev.auction);
    if (ev.bid && ev.bid.bidder_id === me) r.myTop = Math.max(r.myTop, ev.bid.amount);
    schedule();
  }));
}
let raf = 0; const schedule = () => { if (!raf) raf = requestAnimationFrame(() => { raf = 0; draw(); }); };
setInterval(() => $$(".brow-clock", body).forEach(el => { if (el.dataset.end && !/[a-z]/i.test(el.textContent)) el.textContent = clock(Math.max(0, Math.ceil((Date.parse(el.dataset.end) - be.feed.serverNow()) / 1000))); }), 1000);

try {
  rows = await loadMyBids(be);
  const ids = local.json("marque-saved:" + be.kind, []);
  saved = (await Promise.all(ids.map(id => be.getAuction(id).then(toLot).catch(() => null)))).filter(Boolean);
  rows.forEach(follow);
  // open on the tab that has something in it
  const c = counts(); if (!c[tab] && !new URLSearchParams(location.search).get("tab")) tab = TABS.map(t => t[0]).find(k => c[k]) || "leading";
  draw();
} catch (e) {
  body.setAttribute("aria-busy", "false");
  body.innerHTML = String(html`<div class="empty"><h3>Could not load your bids</h3><p>${e.message}</p><button class="btn" onclick="location.reload()">Try again</button></div>`);
}
