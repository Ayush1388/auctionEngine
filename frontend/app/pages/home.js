// Home and event pages: header controls, live cards, trending.
import { initShell } from "../shell.js";
import { hydrateCards } from "../cards.js";
import { toLot } from "../model.js";
import { html, $, usd } from "../util.js";

const be = await initShell({});
hydrateCards(be);
const strip = $("#homeTrending");
if (strip) {
  try {
    const list = await be.trending(8);
    if (list.length) {
      $("#trendSec").hidden = false;
      strip.innerHTML = String(html`${list.map(a => { const l = toLot(a); return html`<li><a class="tcard" href="lot.html?id=${l.id}"><span class="thumb"><img src="${l.img}" alt="" style="object-position:${l.pos}" loading="lazy"></span><span><b>${l.title}</b><span class="small muted tnum">${usd(l.current ?? l.startingPrice)}, ${l.bids} ${l.bids === 1 ? "bid" : "bids"}</span></span></a></li>`; })}`);
    }
  } catch { /* the page works without it */ }
}
