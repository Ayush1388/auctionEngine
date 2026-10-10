// Search lab: one query, two backends, side by side.
// Elasticsearch is typo tolerant and ranks; the PostgreSQL fallback matches whole words and
// prefixes. In the demo engine both are modelled. On the real API the page asks for the
// fallback by switching the Elasticsearch fault on for the length of one query (operators only).
import { initShell } from "../shell.js";
import { mountTourBar } from "../tour.js";
import { html, raw, $, $$, esc, dur } from "../util.js";
import { toLot } from "../model.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
mountTourBar("search");
const body = $("#searchBody");
const sim = be.kind === "sim";
const SAMPLES = [["mustng", "a letter dropped"], ["porshe", "a letter missing"], ["corvet", "cut short"], ["mercedez", "a letter swapped"], ["boss 429", "exact words"]];
const st = { q: "mustng", running: false, es: null, pg: null, note: "", err: null };

const safe = h => String(h || "").split(/(<\/?em>)/).map(p => (p === "<em>" || p === "</em>" ? p : esc(p))).join("");
const BACKEND = b => (b === "postgres" ? "postgres" : "es");

async function timed(fn) {
  const t0 = performance.now();
  try { const p = await fn(); return { ms: performance.now() - t0, backend: BACKEND(p.backend), hits: p.auctions }; }
  catch (e) { return { ms: performance.now() - t0, error: e.message }; }
}

async function run() {
  if (st.running || !st.q.trim()) return;
  st.running = true; st.err = null; st.es = st.pg = null; st.note = ""; paint();
  const q = st.q.trim(), o = { status: "ACTIVE", limit: 12 };
  try {
    if (sim) {
      st.es = await timed(() => be.search(q, o));
      st.pg = await timed(() => be.search(q, { ...o, force: "postgres" }));
    } else {
      const first = await timed(() => be.search(q, o));
      (first.backend === "postgres" ? (st.pg = first) : (st.es = first));
      const missing = first.backend === "postgres" ? "es" : "pg";
      const c = be.session.isAdmin ? await be.chaos.get().catch(() => null) : null;
      if (c?.enabled) {
        const want = missing === "pg";                 // to get the fallback, break Elasticsearch for one query
        const was = c.faults.find(f => f.name === "elasticsearch")?.active;
        await be.chaos.set("elasticsearch", want);
        try {
          // the breaker needs a few failures to open; queries during that time still fall back, just slower
          const second = await timed(() => be.search(q, o));
          if (missing === "pg" && second.backend === "postgres") st.pg = second;
          if (missing === "es" && second.backend !== "postgres") st.es = second;
        } finally { await be.chaos.set("elasticsearch", !!was); }
      } else {
        st.note = missing === "pg"
          ? "Elasticsearch answered. To see the PostgreSQL fallback here too, sign in as the operator on an API started with CHAOS_ENABLED=true, or stop Elasticsearch."
          : "Elasticsearch is not running, so only the PostgreSQL fallback answered. Start it to compare.";
      }
    }
  } catch (e) { st.err = e.message; }
  st.running = false; paint();
}

function column(title, sub, r, other) {
  if (!r) return html`<section class="ops-sec"><h2 class="h2">${title}</h2><p class="muted">${sub}</p><div class="empty"><p>${st.running ? "Searching..." : "No answer from this backend."}</p></div></section>`;
  if (r.error) return html`<section class="ops-sec"><h2 class="h2">${title}</h2><p class="form-error">${r.error}</p></section>`;
  const lost = other && other.hits ? other.hits.filter(a => !r.hits.some(b => b.id === a.id)) : [];
  return html`
    <section class="ops-sec" aria-label="${title}">
      <h2 class="h2">${title}</h2><p class="muted">${sub}</p>
      <p class="tnum"><b>${r.hits.length}</b> results in <b>${dur(r.ms)}</b>${sim ? html` <span class="muted">(simulated)</span>` : ""}</p>
      ${r.hits.length ? html`<ol class="hits">${r.hits.map(a => html`<li><span>${a.highlight && a.highlight.includes("<em>") ? raw(safe(a.highlight)) : toLot(a).title}</span></li>`)}</ol>`
        : html`<div class="empty"><h3>Nothing found</h3><p>No whole word or prefix matches "${st.q}".</p></div>`}
      ${lost.length ? html`<p class="small"><b>Missed here:</b> ${lost.map(a => toLot(a).title).join(", ")}</p>` : ""}
    </section>`;
}

function paint() {
  body.setAttribute("aria-busy", "false");
  body.innerHTML = String(html`
    <form class="lab-controls" id="f">
      <label class="field" style="flex:1 1 280px"><span class="field-label">Search the open lots</span><input id="q" value="${st.q}" autocomplete="off" spellcheck="false"></label>
      <button class="btn ${st.running ? "busy" : ""}" type="submit" ${st.running ? "disabled" : ""}>Compare</button>
    </form>
    <div class="presets" style="margin-top:12px" role="group" aria-label="Sample queries">
      ${SAMPLES.map(([q, why]) => html`<button class="preset" data-q="${q}" aria-pressed="${st.q === q}" title="${why}">${q}</button>`)}
    </div>
    ${st.err ? html`<p class="form-error" role="alert">${st.err}</p>` : ""}
    ${st.note ? html`<p class="banner warn">${st.note}</p>` : ""}
    <div class="split-2" style="margin-top:24px">
      ${column("Elasticsearch", "Typo tolerant: one edit per word, ranked by title match.", st.es, st.pg)}
      ${column("PostgreSQL fallback", "Full-text search: whole words and prefixes. What users get while Elasticsearch is down.", st.pg, st.es)}
    </div>
    <p class="small muted">When Elasticsearch fails five calls in a row its breaker opens, and search is answered here until a trial request ten seconds later succeeds. The fallback is fast and always correct, but it forgives no typos.</p>`);
  $("#f").addEventListener("submit", e => { e.preventDefault(); st.q = $("#q").value; run(); });
  $$("[data-q]", body).forEach(b => b.addEventListener("click", () => { st.q = b.dataset.q; run(); }));
}
paint();
run();
