// Demo lot page. Bids here only change local state; the rival bidder is simulated.
const $ = id => document.getElementById(id);
const money = n => "$" + n.toLocaleString("en-US");
const pad = n => String(n).padStart(2, "0");
const id = new URLSearchParams(location.search).get("id");
const lot = LOTS[id] || LOTS.gt500;
const key = LOTS[id] ? id : "gt500";
const order = ["corvette", "gullwing", "etype", "porsche", "camaro", "gt500", "challenger", "boss429", "pontiac", "excalibur", "mustang", "impala"];
const ix = order.indexOf(key === "boss" ? "boss429" : key);
document.title = "Marque | " + lot.title;

/* header info */
$("lotNo").textContent = "Lot " + lot.lot;
$("lotTitle").textContent = lot.title;
$("lotSub").textContent = `${lot.event[1]}  |  ${lot.where}`;
$("chips").innerHTML = [lot.make, lot.model, lot.year, lot.type].map(c => `<li>${c}</li>`).join("");
$("prevLot").href = "lot.html?id=" + order[(ix + order.length - 1) % order.length];
$("nextLot").href = "lot.html?id=" + order[(ix + 1) % order.length];
$("backLink").href = "index.html#" + (["camaro", "gt500", "challenger", "boss429", "boss"].includes(key) ? "muscle" : "classic");
$("qImg").src = lot.img; $("qEvent").textContent = lot.event[1]; $("qText").textContent = "“" + lot.quote + "”";
$("qLink").href = "event.html?e=" + lot.event[0];

/* gallery: the catalogue has one photo per car, so the views are framings of it */
const views = [["100% 100%", 1, "50% " + lot.pos.split(" ")[1]], ["20% 50%", 1.7, "20% 55%"], ["80% 50%", 1.7, "80% 55%"], ["50% 40%", 1.7, "50% 40%"]];
let gi = 0;
$("gThumbs").innerHTML = views.map((v, i) => `<li><button aria-label="View ${i + 1}"><img src="${lot.img}" alt="" style="object-position:${v[2]};${i ? "transform:scale(1.7);transform-origin:" + v[0] : ""}"></button></li>`).join("");
const thumbs = [...$("gThumbs").querySelectorAll("button")];
function view(i) {
  gi = (i + views.length) % views.length; const v = views[gi], m = $("gMain");
  m.src = lot.img; m.alt = `${lot.title}, view ${gi + 1}`; m.style.objectPosition = gi ? v[2] : lot.pos; m.style.transformOrigin = v[0]; m.style.transform = `scale(${v[1]})`;
  thumbs.forEach((b, k) => b.classList.toggle("on", k === gi)); $("gCount").textContent = `${pad(gi + 1)} / ${pad(views.length)}`;
}
thumbs.forEach((b, i) => b.addEventListener("click", () => view(i)));
$("gPrev").addEventListener("click", () => view(gi - 1)); $("gNext").addEventListener("click", () => view(gi + 1)); view(0);

/* tabs */
const specRows = [["Engine", lot.engine], ["Transmission", lot.trans], ["Power", lot.power], ["Exterior", lot.colour], ["Interior", lot.interior], ["Mileage", lot.miles], ["Chassis no.", lot.chassis]];
const specHTML = `<dl class="lt-specs">${specRows.map(r => `<div><dt>${r[0]}</dt><dd>${r[1]}</dd></div>`).join("")}</dl>`;
const TABS = {
  Overview: `<div class="lt-ov"><div><h2>${lot.lead}</h2>${lot.text.map(t => `<p>${t}</p>`).join("")}</div>${specHTML}</div>`,
  Specifications: `<h2>Specifications</h2>${specHTML}`,
  History: `<h2>History</h2><p style="margin-top:14px;max-width:62ch;color:var(--fg-soft)">${lot.history}</p>`,
  Condition: `<h2>Condition</h2><ul class="lt-list"><li>Paint and brightwork: presented in ${lot.colour.toLowerCase()} with a consistent finish.</li><li>Mechanical: engine and gearbox run and shift as expected for a ${lot.year} ${lot.make}.</li><li>Interior: ${lot.interior.toLowerCase()}, with wear in keeping with ${lot.miles}.</li></ul>`,
  Documentation: `<h2>Documentation</h2><ul class="lt-list"><li>Ownership history and registration papers</li><li>Service and restoration invoices</li><li>Chassis and engine number verification</li></ul>`,
  Shipping: `<h2>Shipping</h2><p style="margin-top:14px;max-width:62ch;color:var(--fg-soft)">The car is in ${lot.where}. We arrange enclosed worldwide transport and export paperwork after payment clears. A quote is shown at checkout.</p>`,
};
const tabsEl = $("tabs");
tabsEl.innerHTML = Object.keys(TABS).map((t, i) => `<button role="tab" aria-selected="${i === 0}" data-t="${t}">${t}</button>`).join("");
function tab(t) { tabsEl.querySelectorAll("button").forEach(b => b.setAttribute("aria-selected", b.dataset.t === t)); $("panel").innerHTML = TABS[t]; if (window.gsap) gsap.from("#panel > *", { autoAlpha: 0, y: 8, duration: .4, ease: "power2.out" }); }
tabsEl.addEventListener("click", e => { if (e.target.dataset.t) tab(e.target.dataset.t); }); $("panel").innerHTML = TABS.Overview;

/* bidding state */
const toast = $("toast"); let tt;
const say = m => { toast.textContent = m; toast.classList.add("show"); clearTimeout(tt); tt = setTimeout(() => toast.classList.remove("show"), 2200); };
let bid = lot.bid, count = lot.bids, left = lot.end, last = 0, proxy = 0, proxyMode = false;
const names = ["Bidder #184", "Bidder #291", "Bidder #407", "Bidder #052"];
const hist = [];
for (let k = 0; k < 5; k++) hist.push({ n: count - k, who: names[k % 3], amt: bid - k * lot.step, t: Date.now() - [0, 2, 5, 8, 12][k] * 60000, you: false });
const ago = t => { const m = Math.floor((Date.now() - t) / 60000); return m < 1 ? "just now" : m + " min ago"; };
function drawHist(newRow) {
  $("hist").innerHTML = hist.slice(0, 6).map((h, i) => `<tr class="${h.you ? "you" : ""}${i === 0 && newRow ? " new" : ""}"><td>${h.n}</td><td>${h.you ? "You" : h.who}</td><td>${money(h.amt)}</td><td>${ago(h.t)}</td></tr>`).join("");
  if (newRow && window.gsap) { const r = $("hist").firstElementChild; gsap.from(r, { autoAlpha: 0, y: -10, duration: .5, ease: "power2.out" }); setTimeout(() => r && r.classList.remove("new"), 1600); }
}
const low = Math.round(lot.bid * 0.8 / 1000) * 1000, high = Math.round(lot.bid * 1.35 / 1000) * 1000;
$("estLow").textContent = money(low); $("estHigh").textContent = money(high); $("resv").textContent = lot.reserve;
function paint(rowNew, up) {
  window.tickBid ? window.tickBid($("curBid"), bid, money) : ($("curBid").textContent = money(bid));
  $("bidCount").textContent = count;
  $("mark").style.left = Math.max(0, Math.min(100, (bid - low) / (high - low) * 100)) + "%";
  $("minLine").textContent = "Minimum next bid: " + money(bid + lot.step);
  $("delta").textContent = last ? `+${money(last)}  just now` : "";
  if (!Number($("amt").value.replace(/\D/g, "")) || Number($("amt").value.replace(/\D/g, "")) < bid + lot.step) $("amt").value = money(bid + lot.step);
  drawHist(rowNew);
}
const amtVal = () => Number($("amt").value.replace(/\D/g, "")) || 0;
$("plus").onclick = () => { $("amt").value = money(amtVal() + lot.step); };
$("minus").onclick = () => { $("amt").value = money(Math.max(bid + lot.step, amtVal() - lot.step)); };
$("amt").addEventListener("blur", () => { $("amt").value = money(amtVal()); });
function status(msg, cls) { const s = $("status"); s.textContent = msg; s.className = "mono lt-status " + (cls || ""); }
function flash() { const b = document.querySelector(".lt-bidbox"); b.classList.remove("flash"); void b.offsetWidth; b.classList.add("flash"); }
$("modeBid").onclick = () => { proxyMode = false; $("modeBid").classList.add("on"); $("modeProxy").classList.remove("on"); $("modeBid").setAttribute("aria-selected", true); $("modeProxy").setAttribute("aria-selected", false); $("goBtn").textContent = "Place bid"; };
$("modeProxy").onclick = () => { proxyMode = true; $("modeProxy").classList.add("on"); $("modeBid").classList.remove("on"); $("modeProxy").setAttribute("aria-selected", true); $("modeBid").setAttribute("aria-selected", false); $("goBtn").textContent = "Set max"; };
$("bidForm").addEventListener("submit", e => {
  e.preventDefault();
  if (left <= 0) return status("Auction closed.", "err");
  const v = amtVal();
  if (v < bid + lot.step) { status(`Bid must be at least ${money(bid + lot.step)}.`, "err"); return; }
  if (proxyMode) { proxy = v; status(`Max bid set at ${money(v)}. We will bid for you in ${money(lot.step)} steps.`); say("Max bid saved"); return; }
  bid = v; count++; last = lot.step; hist.unshift({ n: count, who: "You", amt: bid, t: Date.now(), you: true }); paint(true);
  status("New high bid. You are the highest bidder.", ""); flash(); say("Bid placed: " + money(bid));
});
function rival() {
  if (left <= 0) return;
  bid += lot.step; count++; last = lot.step; hist.unshift({ n: count, who: names[Math.floor(Math.random() * 3)], amt: bid, t: Date.now(), you: false });
  let youWereHigh = hist[1] && hist[1].you;
  if (proxy && bid + lot.step <= proxy) { setTimeout(() => { bid += lot.step; count++; hist.unshift({ n: count, who: "You", amt: bid, t: Date.now(), you: true }); paint(true); status("Your max bid raised the price. You are the highest bidder."); }, 900); }
  else if (youWereHigh) status("You have been outbid. Raise your bid to take the lead.", "out");
  paint(true); flash();
}
setInterval(() => { left = Math.max(0, left - 1); $("clock").textContent = [Math.floor(left / 3600), Math.floor(left % 3600 / 60), left % 60].map(pad).join(":"); if (left === 0) { $("liveTag").textContent = "Closed"; $("liveTag").classList.remove("on"); $("goBtn").disabled = true; } }, 1000);
setInterval(() => drawHist(false), 30000);
(function loop() { setTimeout(() => { rival(); loop(); }, 22000 + Math.random() * 18000); })();
$("liveTag").classList.add("on"); $("amt").value = money(bid + lot.step); paint(false);
$("clock").textContent = [Math.floor(left / 3600), Math.floor(left % 3600 / 60), left % 60].map(pad).join(":");

/* save, share, theme */
$("saveBtn").onclick = e => { const on = e.currentTarget.getAttribute("aria-pressed") !== "true"; e.currentTarget.setAttribute("aria-pressed", on); e.currentTarget.textContent = on ? "Saved" : "Save"; };
$("shareBtn").onclick = () => { (navigator.clipboard ? navigator.clipboard.writeText(location.href) : Promise.reject()).then(() => say("Link copied"), () => say("Copy the address bar to share")); };
const modeBtn = $("modeToggle");
const setTheme = d => { d ? document.documentElement.setAttribute("data-theme", "dark") : document.documentElement.removeAttribute("data-theme"); modeBtn.setAttribute("aria-checked", d); try { localStorage.setItem("marque-theme", d ? "dark" : "light"); } catch (e) {} };
modeBtn.setAttribute("aria-checked", document.documentElement.getAttribute("data-theme") === "dark");
modeBtn.addEventListener("click", () => setTheme(modeBtn.getAttribute("aria-checked") !== "true"));
