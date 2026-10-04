// Search with type-ahead, and the trending strip.
import { initShell } from "../shell.js";
import { toLot, secondsLeft } from "../model.js";
import { html, raw, esc, $, $$, usd, clock, debounce, local } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "search" });
const q = $("#q"), out = $("#results"), meta = $("#resMeta"), sugg = $("#sugg"), moreBtn = $("#more");
const url = new URLSearchParams(location.search);
let query = url.get("q") || "", status = url.get("status") || "ACTIVE", cursor = null, shown = [];
q.value = query;

const STATUS = [["ACTIVE", "Open now"], ["NOT_ACTIVE", "Opens soon"], ["COMPLETED", "Closed"], ["", "Any"]];
const BACKEND = { elasticsearch: "Elasticsearch", postgres: "PostgreSQL full-text search (Elasticsearch is not running)", demo: "the demo index" };

function chips() {
  $("#status").innerHTML = String(html`${STATUS.map(([k, l]) => html`<button class="preset" aria-pressed="${status === k}" data-s="${k}">${l}</button>`)}`);
  $$("[data-s]", $("#status")).forEach(b => b.addEventListener("click", () => { status = b.dataset.s; run(true); }));
}

/** The server marks matches with <em>; allow exactly that tag and nothing else. */
const safeHighlight = h => String(h || "").split(/(<\/?em>)/).map(p => (p === "<em>" || p === "</em>" ? p : esc(p))).join("");

function card(a) {
  const l = toLot(a), live = l.status === "ACTIVE";
  return html`
    <article class="card">
      <div class="card-media"><a class="card-link" href="lot.html?id=${l.id}" aria-label="Open ${l.title}"><img src="${l.img}" alt="${l.alt}" loading="lazy" style="object-position:${l.pos}"></a></div>
      <div class="card-body">
        <h3><a href="lot.html?id=${l.id}">${a.highlight ? raw(safeHighlight(a.highlight)) : l.title}</a></h3>
        <p class="spec">${l.spec}</p>
        <p class="where">${l.where}</p>
        <dl class="lot-stats"><div><dt>${l.current == null ? "Opening bid" : "Current bid"}</dt><dd class="bid">${usd(l.current ?? l.startingPrice)}</dd></div><div><dt>${live ? "Ends in" : "Status"}</dt><dd class="time" data-end="${l.endsAt}">${live ? clock(secondsLeft(l, be.feed.serverNow())) : l.status === "COMPLETED" ? "Closed" : l.status === "NOT_ACTIVE" ? "Soon" : "Cancelled"}</dd></div></dl>
        <p class="lot-meta"><span>${l.bids} ${l.bids === 1 ? "bid" : "bids"}</span><span class="reserve">${l.reserve}</span></p>
        <a class="btn btn-block" href="lot.html?id=${l.id}">${live ? "Open lot" : "View lot"}</a>
      </div>
    </article>`;
}

async function run(reset) {
  if (reset) { cursor = null; shown = []; }
  out.setAttribute("aria-busy", "true");
  if (reset) out.innerHTML = `<div class="skel" style="height:360px"></div>`.repeat(4);
  const params = new URLSearchParams(); if (query) params.set("q", query); if (status !== "ACTIVE") params.set("status", status);
  history.replaceState(null, "", "search.html" + (params.size ? "?" + params : ""));
  $("#resH").textContent = query ? `Results for "${query}"` : STATUS.find(s => s[0] === status)?.[1] || "Lots";
  chips();
  try {
    let page, via = "";
    if (query) { page = await be.search(query, { status: status || undefined, limit: 12, cursor }); via = BACKEND[page.backend] || page.backend; }
    else { page = await be.listAuctions({ status: status || undefined, limit: 12, cursor }); }
    shown = shown.concat(page.auctions); cursor = page.next_cursor;
    out.setAttribute("aria-busy", "false");
    out.innerHTML = shown.length ? String(html`${shown.map(card)}`)
      : String(html`<div class="empty" style="grid-column:1/-1"><h3>${query ? "Nothing matches that" : "No lots here"}</h3><p>${query ? "Check the spelling, or search for a make or a model instead." : "Try another filter."}</p></div>`);
    meta.textContent = shown.length ? `${shown.length} shown${via ? ", answered by " + via : ""}` : (via ? "Answered by " + via : "");
    moreBtn.hidden = !cursor;
  } catch (e) {
    out.setAttribute("aria-busy", "false");
    out.innerHTML = String(html`<div class="empty" style="grid-column:1/-1"><h3>Search is not available</h3><p>${e.message}</p><button class="btn" id="retry">Try again</button></div>`);
    $("#retry")?.addEventListener("click", () => run(true));
  }
}
moreBtn.addEventListener("click", () => run(false));
$("#searchForm").addEventListener("submit", e => { e.preventDefault(); query = q.value.trim(); close(); run(true); });

/* ---------------------------------------------------------------- type-ahead */
let items = [], active = -1;
const close = () => { sugg.hidden = true; q.setAttribute("aria-expanded", "false"); q.removeAttribute("aria-activedescendant"); active = -1; };
function paint() {
  sugg.innerHTML = items.map((s, i) => `<li role="option" id="s${i}" aria-selected="${i === active}" data-i="${i}">${esc(s.title)}</li>`).join("");
  sugg.hidden = !items.length; q.setAttribute("aria-expanded", String(!!items.length));
  if (active >= 0) q.setAttribute("aria-activedescendant", "s" + active); else q.removeAttribute("aria-activedescendant");
}
let pool;
async function fromOpenLots(t) {
  pool ||= be.allAuctions({ status: "ACTIVE" }, 2).then(l => l.map(a => ({ id: a.id, title: a.item.name }))).catch(() => []);
  const needle = t.toLowerCase();
  return (await pool).filter(x => x.title.toLowerCase().split(/\s+/).some(w => w.startsWith(needle)) || x.title.toLowerCase().includes(needle)).slice(0, 6);
}
const suggest = debounce(async () => {
  const t = q.value.trim();
  if (t.length < 2) { items = []; paint(); return; }
  try { items = await be.suggest(t); } catch { items = []; }
  if (!items.length) items = await fromOpenLots(t);     // Elasticsearch down: the API has no type-ahead, so match the open lots here
  active = -1; paint();
}, 150);
q.addEventListener("input", suggest);
q.addEventListener("keydown", e => {
  if (!items.length) return;
  if (e.key === "ArrowDown") { e.preventDefault(); active = (active + 1) % items.length; paint(); }
  else if (e.key === "ArrowUp") { e.preventDefault(); active = (active - 1 + items.length) % items.length; paint(); }
  else if (e.key === "Enter" && active >= 0) { e.preventDefault(); location.href = "lot.html?id=" + items[active].id; }
  else if (e.key === "Escape") close();
});
sugg.addEventListener("mousedown", e => { const li = e.target.closest("li"); if (li) location.href = "lot.html?id=" + items[+li.dataset.i].id; });
q.addEventListener("blur", () => setTimeout(close, 120));

/* ---------------------------------------------------------------- trending */
async function trending() {
  try {
    const list = await be.trending(8);
    if (!list.length) return;
    $("#trendingSec").hidden = false;
    $("#trending").innerHTML = String(html`${list.map(a => { const l = toLot(a); return html`<li><a class="tcard" href="lot.html?id=${l.id}"><span class="thumb"><img src="${l.img}" alt="" style="object-position:${l.pos}" loading="lazy"></span><span><b>${l.title}</b><span class="small muted tnum">${usd(l.current ?? l.startingPrice)}, ${l.bids} ${l.bids === 1 ? "bid" : "bids"}</span></span></a></li>`; })}`);
  } catch { /* the page works without it */ }
}

setInterval(() => $$("[data-end]", out).forEach(el => { if (/^\d/.test(el.textContent)) el.textContent = clock(Math.max(0, Math.ceil((Date.parse(el.dataset.end) - be.feed.serverNow()) / 1000))); }), 1000);
chips(); run(true); trending();
