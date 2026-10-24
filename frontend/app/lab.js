// Helpers shared by the "under the hood" pages (trust, chaos, system, race, search lab).
import { html, $, usd } from "./util.js";
import { toLot } from "./model.js";

/** Operator-only pages: draws a sign-in prompt into `root` and returns false when not an operator. */
export function operatorGate(be, root, { next, what }) {
  if (be.unreachable) {
    root.setAttribute("aria-busy", "false");
    root.innerHTML = String(html`<div class="empty"><h3>The API is not answering</h3><p>Nothing is listening at ${be.base}. Start it, or use the demo engine, which needs no server.</p><a class="btn" href="?engine=sim">Use the demo engine</a></div>`);
    return false;
  }
  if (be.session.isAdmin) return true;
  root.setAttribute("aria-busy", "false");
  root.innerHTML = String(html`
    <div class="empty"><h3>Operators only</h3>
      <p>${what} ${be.session.id ? "This account is not an operator." : "Sign in as the operator account to use it."}
        ${be.kind === "sim" ? "In the demo, the operator is admin@marque.test with the password on the sign-in page." : "Make an account an operator with: go run ./cmd/admin set-role <email> admin"}</p>
      <a class="btn" href="account.html?next=${next}">${be.session.id ? "Switch account" : "Sign in"}</a></div>`);
  return false;
}

/** Open lots a person may bid on (not their own), as { id, lot } with the model's field names. */
export async function openLots(be) {
  const me = be.session.id;
  const all = await be.allAuctions({ status: "ACTIVE" }, 3);
  return all.filter(a => a.owner_id !== me && Date.parse(a.ends_at) - Date.now() > 20_000).map(a => ({ id: a.id, raw: a, lot: toLot(a) }));
}

/** "1 bid" / "3 bids" */
export const plural = (n, one, many = one + "s") => `${n} ${n === 1 ? one : many}`;

/** A row of labelled numbers: [[label, value, note?], ...] */
export function tiles(items) {
  return html`<div class="tiles">${items.map(([l, v, n]) => html`<div class="tile"><span class="lab mono">${l}</span><b class="tnum">${v}</b>${n ? html`<span class="small muted">${n}</span>` : ""}</div>`)}</div>`;
}

export const money = usd;
export { $ };
