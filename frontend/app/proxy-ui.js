// "Set a maximum bid" on the lot page: proxy bidding.
// The engine raises the person's bid for them, one step at a time, up to the maximum, so they
// do not have to watch the clock. Only the visible bid is ever held; the maximum is an instruction.
import { html, $, usd, parseUsd } from "./util.js";

export function mountProxy({ be, session, root, toast, onChange }) {
  let proxy = null, busy = false, err = "", open = false, timer = 0;
  const me = () => be.session.id;

  async function refresh() {
    if (!me() || !session.lot) { proxy = null; paint(); return; }
    try { proxy = await be.getProxy(session.id); err = ""; } catch (e) { err = e.message; }
    paint();
  }

  function paint() {
    const l = session.lot, live = l.status === "ACTIVE", step = l.step;
    if (!me()) { root.hidden = !live; $("#proxyBody", root).innerHTML = String(html`<p>Sign in to set a maximum.</p>`); return; }
    root.hidden = !live && !proxy;
    const leading = l.bidderId === me();
    const status = proxy
      ? html`<p class="lt-proxy-state"><b>Your maximum is ${usd(proxy.max_amount)}.</b> ${leading
          ? `You lead at ${usd(l.current)}. If someone outbids you, the engine bids for you in steps of ${usd(step)}.`
          : proxy.max_amount < l.minNext ? `You were outbid past your maximum. Raise it to rejoin.` : `Your maximum is still above the price; the engine will act on the next bid.`}</p>`
      : html`<p class="lt-proxy-state">Enter the most you will pay. When someone outbids you, the engine bids for you, ${usd(step)} at a time, up to that. Only your visible bid is held, never the maximum.</p>`;
    $("#proxyBody", root).innerHTML = String(html`
      ${status}
      <form class="lt-proxy-form" id="proxyForm" novalidate>
        <div class="lt-field"><label class="visually" for="proxyAmt">Your maximum in dollars</label><input id="proxyAmt" inputmode="numeric" autocomplete="off" placeholder="${usd(Math.max(l.minNext, proxy?.max_amount || 0) + step * 4)}"></div>
        <button class="btn lt-go" type="submit" ${busy || !live ? "disabled" : ""}>${proxy ? "Change maximum" : "Set maximum"}</button>
      </form>
      ${proxy ? html`<button class="linkish" id="proxyOff" ${busy ? "disabled" : ""}>Remove maximum</button>` : ""}
      ${err ? html`<p class="lt-proxy-err" role="alert">${err}</p>` : ""}`);
    $("#proxyForm", root).addEventListener("submit", submit);
    $("#proxyOff", root)?.addEventListener("click", remove);
  }

  async function submit(e) {
    e.preventDefault();
    const cents = parseUsd($("#proxyAmt", root).value);
    if (!cents) { err = "Enter an amount in dollars."; paint(); return; }
    busy = true; err = ""; paint();
    try {
      proxy = await be.setProxy(session.id, cents);
      toast(proxy.leading ? `Maximum set. You lead at ${usd(proxy.current_bid)}.` : `Maximum set to ${usd(proxy.max_amount)}.`);
      await session.reload(); onChange?.();
    } catch (x) {
      err = x.status === 422 && x.body?.minimum_amount != null ? `The maximum must be at least ${usd(x.body.minimum_amount)}.`
        : x.status === 422 && /insufficient/i.test(x.message) ? "Not enough available funds to place even the minimum bid." : x.message;
    }
    busy = false; paint();
  }
  async function remove() {
    busy = true; paint();
    try { await be.cancelProxy(session.id); proxy = null; toast("Maximum removed"); } catch (x) { err = x.message; }
    busy = false; paint();
  }

  root.addEventListener("toggle", () => { open = root.open; if (open) refresh(); });
  const again = () => { clearTimeout(timer); timer = setTimeout(() => { if (open) refresh(); }, 250); };
  session.on("lot", again);
  paint();
  return { refresh };
}
