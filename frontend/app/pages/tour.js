// The backend tour: where a walkthrough of the system starts. One diagram, eight
// stages in the order a bid meets them, each with what happens, what it is built
// with, why, and a live demo page that runs against the real backend or its
// in-browser twin.
import { initShell } from "../shell.js";
import { mountSystem } from "../sysview.js";
import { STAGES } from "../tour.js";
import { html, $, $$, usd, uuid } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
const body = $("#tourBody");
const sim = be.kind === "sim";

body.setAttribute("aria-busy", "false");
body.innerHTML = String(html`
  <header class="tour-hero">
    <h1>How the backend works</h1>
    <p>An auction engine in Go. Follow one bid from your click to every screen in eight stages, then break things on purpose. Each stage has a live demo and says what it is built with and why.</p>
    <div class="tour-actions">
      <a class="btn" href="${STAGES[0].page}">Start at stage 1</a>
      <button class="btn btn-line" id="send" ${be.session.id ? "" : "disabled"}>Send a bid through the diagram</button>
      <span class="small muted" id="sendNote">${be.session.id ? "" : "Sign in to place a bid. Bids from anyone make pulses."}</span>
    </div>
    <div class="tour-status" id="status"></div>
  </header>

  <section aria-labelledby="mapH">
    <h2 class="visually" id="mapH">The system</h2>
    <div id="map"></div>
    <p class="small muted">Select a stage below to light the parts it uses. A pulse is drawn for every real bid and live update.</p>
  </section>

  <ol class="tour-stages" id="stages">
    ${STAGES.map(s => html`
      <li class="tstage" data-stage="${s.id}" tabindex="-1">
        <span class="tstage-n tnum" aria-hidden="true">${s.n}</span>
        <div class="tstage-body">
          <h2>${s.title}</h2>
          <div class="tstage-cols">
            <div class="stack">
              <p class="tstage-what">${s.what}</p>
              <p class="tstage-why"><b>Why this way.</b> ${s.why} <span class="mono muted">${s.adr}</span></p>
            </div>
            <div class="stack">
              <ul class="chips" aria-label="Built with">${s.tech.map(t => html`<li>${t}</li>`)}</ul>
              <p class="tstage-demo"><b>In the demo.</b> ${s.demo}</p>
            </div>
          </div>
          <div class="tstage-actions"><a class="btn btn-line btn-sm" href="${s.page}">Open stage ${s.n}</a></div>
        </div>
      </li>`)}
  </ol>

  <p class="small muted" style="margin-top:28px">${sim
    ? "You are on the in-browser demo engine: it follows the same rules as the Go services, and every timing and fault here is simulated and labelled. Run the real API to measure."
    : "You are connected to the real API: timings are measured, and the faults are injected into the running services."}</p>`);

const sys = mountSystem(be, { mapRoot: $("#map"), panelRoot: null, watch: 12 });

// the stage under the pointer or focus lights its parts of the diagram
const light = id => sys.map.highlight(STAGES.find(s => s.id === id)?.nodes || []);
$$(".tstage", body).forEach(el => {
  el.addEventListener("pointerenter", () => light(el.dataset.stage));
  el.addEventListener("focusin", () => light(el.dataset.stage));
  el.addEventListener("pointerleave", () => light(null));
});

// engine status, in plain words
sys.onTick(state => {
  const checks = state.ready?.checks || {};
  const items = [["API", state.ms != null], ...Object.entries(checks).map(([k, v]) => [k, v === "ok"])];
  $("#status").innerHTML = String(html`
    <span class="tag ${sim ? "tag-warn" : "tag-ok"}">${sim ? "Demo engine, simulated" : "Real backend"}</span>
    ${items.map(([k, ok]) => html`<span class="tag ${ok ? "tag-ok" : "tag-bad"}">${k} ${ok ? "ok" : "down"}</span>`)}`);
});

$("#send").addEventListener("click", async () => {
  const a = (await be.allAuctions({ status: "ACTIVE" }, 1)).find(x => x.owner_id !== be.session.id);
  if (!a) { $("#sendNote").textContent = "There is no open lot to bid on."; return; }
  const min = a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
  const amount = a.current_bidder_id === be.session.id ? a.current_bid + a.min_increment : min;
  const r = await be.placeBid(a.id, amount, { key: uuid() });
  sys.bid(r);
  $("#sendNote").textContent = r.ok ? `Placed ${usd(amount)}. Watch it cross the diagram.` : `Refused: ${r.error?.message || r.status}.`;
});
addEventListener("pagehide", () => sys.stop());
