// Architecture map: the live diagram, plus the Kafka lanes that show which
// partition each lot's bids travel through and how far behind the consumers are.
import { initShell } from "../shell.js";
import { mountTourBar } from "../tour.js";
import { mountSystem } from "../sysview.js";
import { partitionOf, PARTITIONS } from "../partition.js";
import { toLot } from "../model.js";
import { html, $, usd, uuid } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
mountTourBar("events");
const body = $("#sysBody");
const st = { lots: [], bids: 0 };

body.setAttribute("aria-busy", "false");
body.innerHTML = String(html`
  <div class="lab-controls" style="margin-bottom:16px">
    <button class="btn" id="bid" ${be.session.id ? "" : "disabled"}>Place a bid and watch it travel</button>
    <span class="small muted" id="note">${be.session.id ? "Bids anyone places on any open lot also make pulses." : "Sign in to place a bid. Bids from other windows still make pulses."}</span>
  </div>
  <div id="map"></div>
  <div id="nodePanel"></div>
  <section class="ops-sec" aria-labelledby="lnH" style="margin-top:clamp(28px,4vw,52px)">
    <h2 class="h2" id="lnH">Kafka lanes</h2>
    <p class="lead">Each lot is pinned to one of ${PARTITIONS} partitions by a hash of its ID, and one consumer owns a partition. That is why bids on one lot are processed in order while different lots run in parallel. A lane lights up when its consumer is behind.</p>
    <div class="lanes" id="lanes"></div>
    <p class="small muted" id="laneNote"></p>
    <ul class="tbl-lite" id="laneLots"></ul>
  </section>`);

const sys = mountSystem(be, { mapRoot: $("#map"), panelRoot: $("#nodePanel"), watch: 12 });

function lanes() {
  const lag = new Array(PARTITIONS).fill(0);
  const fam = sys.state.metrics?.families.find(f => f.name === "auction_kafka_consumer_lag");
  let known = false;
  for (const s of fam?.samples || []) { const p = Number(s.labels?.partition); if (p >= 0 && p < PARTITIONS) { lag[p] += s.value ?? 0; known = true; } }
  const mine = new Set(st.lots.map(l => l.part));
  $("#lanes").innerHTML = String(html`${lag.map((v, i) => html`
    <div class="lane ${v > 0 ? "has-lag" : ""} ${mine.has(i) ? "is-mine" : ""}" title="Partition ${i}: ${v} records waiting">
      <span>lane ${i}</span><b class="tnum">${known ? v : "-"}</b>
      <span class="lane-q" aria-hidden="true">${Array.from({ length: Math.min(v, 12) }, () => html`<i></i>`)}</span>
    </div>`)}`);
  $("#laneNote").textContent = known
    ? "Records waiting for a bid worker, per partition. Run Stress it on one lot: its lane fills and the others stay empty."
    : be.session.isAdmin ? "Waiting for the first metrics reading." : "Consumer lag is an operator metric. Sign in as the operator to see how far behind each lane is; the lot-to-lane mapping below is computed in your browser.";
  $("#laneLots").innerHTML = String(html`${st.lots.map(l => html`<li><span>${l.title}</span><span class="mono">lane ${l.part}</span></li>`)}`);
}

async function loadLots() {
  try {
    const list = await be.allAuctions({ status: "ACTIVE" }, 1);
    st.lots = list.map(a => ({ ...toLot(a), id: a.id, part: partitionOf(a.id) })).slice(0, 12);
  } catch { st.lots = []; }
  lanes();
}
sys.onTick(lanes);
loadLots();

$("#bid").addEventListener("click", async () => {
  const lots = st.lots.filter(l => l.ownerId !== be.session.id);
  const a = (await be.allAuctions({ status: "ACTIVE" }, 1)).filter(x => x.owner_id !== be.session.id)[0];
  if (!a) return;
  const amount = a.current_bidder_id === be.session.id ? a.current_bid + a.min_increment : (a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment);
  const r = await be.placeBid(a.id, amount, { key: uuid() });
  sys.bid(r);
  $("#note").textContent = r.ok ? `Placed ${usd(amount)} on ${toLot(a).title}. Select a box to see its numbers.` : `Refused: ${r.error?.message || r.status}.`;
  void lots;
});
addEventListener("pagehide", () => sys.stop());
