// Demo-only: one template, four events. Data is made up for the demo; no backend calls yet.
const money = n => "$" + n.toLocaleString("en-US");
const pad = n => String(n).padStart(2, "0");
const clock = s => { s = Math.max(0, s); return [Math.floor(s / 3600), Math.floor(s % 3600 / 60), s % 60].map(pad).join(":"); };
const $ = id => document.getElementById(id);

const CARS = {
  corvette: { img: "assets/corvette.jpg", alt: "White 1963 Chevrolet Corvette Sting Ray", title: "1963 Chevrolet Corvette Sting Ray", where: "Sydney, 41,200 km", bid: 118000, step: 1000, bids: 27, reserve: "Reserve met" },
  gullwing: { img: "assets/300sl.jpg", alt: "Red 1955 Mercedes-Benz 300 SL Gullwing", title: "1955 Mercedes-Benz 300 SL Gullwing", where: "Stuttgart, 62,000 km", bid: 1380000, step: 10000, bids: 31, reserve: "Reserve not met" },
  etype: { img: "assets/etype.jpg", alt: "Blue 1964 Jaguar E-Type Series 1 coupe", title: "1964 Jaguar E-Type Series 1", where: "Norwich, 58,300 km", bid: 168000, step: 2000, bids: 24, reserve: "Reserve met", pos: "42% 60%" },
  porsche: { img: "assets/porsche.jpg", alt: "Cream 1972 Porsche 911 Targa", title: "1972 Porsche 911 S Targa", where: "Lippe, 63,800 km", bid: 186000, step: 2000, bids: 41, reserve: "Reserve met" },
  gt500: { img: "assets/gt500.jpg", alt: "Black 1967 Shelby GT500 KR", title: "1967 Shelby GT500 KR", where: "Nuremberg, 68,000 km", bid: 214000, step: 2000, bids: 49, reserve: "Reserve met", pos: "45% 60%" },
  boss: { img: "assets/boss429.jpg", alt: "Red 1969 Ford Mustang Boss 429", title: "1969 Ford Mustang Boss 429", where: "Charlotte, 54,000 km", bid: 312000, step: 2500, bids: 22, reserve: "Reserve not met" },
  camaro: { img: "assets/camaro.jpg", alt: "Black 1967 Chevrolet Camaro SS convertible", title: "1967 Chevrolet Camaro SS 350", where: "Heidelberg, 52,000 km", bid: 78500, step: 1000, bids: 37, reserve: "Reserve met" },
  pontiac: { img: "assets/ivan.jpg", alt: "Red 1951 Pontiac Chieftain convertible", title: "1951 Pontiac Chieftain", where: "Miami, 88,000 km", bid: 64500, step: 500, bids: 18, reserve: "No reserve" },
  excalibur: { img: "assets/doina.jpg", alt: "Red Excalibur roadster", title: "1969 Excalibur Roadster", where: "New York, 31,000 km", bid: 71000, step: 1000, bids: 16, reserve: "No reserve", pos: "50% 62%" },
  mustang: { img: "assets/theodor.jpg", alt: "Red 1965 Ford Mustang coupe", title: "1965 Ford Mustang Coupe", where: "London, 72,400 km", bid: 38000, step: 500, bids: 33, reserve: "No reserve", pos: "50% 64%" },
  impala: { img: "assets/hero.jpg", alt: "Black 1963 Chevrolet Impala SS", title: "1963 Chevrolet Impala SS", where: "San Diego, 84,500 km", bid: 42500, step: 500, bids: 18, reserve: "No reserve", pos: "50% 70%" },
};

const EVENTS = {
  retromobile: {
    name: "Rétromobile Paris", short: "Rétromobile", img: "assets/events/retromobile.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Feb 3 to 7, 2027  |  Paris Expo, France",
    intro: "Five days of pre-war racers and postwar coachbuilt rarities in a single hall, with the auction held on the Friday night.",
    facts: [["Dates", "Feb 3 to 7, 2027", "5 days"], ["Venue", "Paris Expo", "Porte de Versailles"], ["Lots", "62", "Pre-war to 1975"], ["Estimate", "$18M to $24M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "Walk the halls, meet the specialists and hear the cars run before the sale. Every activity is open to registered bidders.",
    exp: [["Hall Walk", "Preview every lot with the specialist who catalogued it.", "Feb 3 to 6", "Hall 1, Paris Expo", "assets/exp/35828170.jpg"], ["Concours Display", "Restored cars shown under lights, judged by marque clubs.", "Feb 4 to 7", "Hall 2", "assets/exp/248687.jpg"], ["Paddock Start-Up", "A scheduled morning when selected lots are fired up in the yard.", "Feb 5", "Outdoor yard", "assets/exp/190537.jpg"], ["Collectors' Dinner", "A seated dinner for consignors and registered bidders.", "Feb 6", "Pavillon Gabriel", "assets/exp/5116976.jpg"]], layout: "wave",
    ft: ["gullwing", "Featured lot", "Lot 014, Rétromobile Paris", "A gullwing coupe with its original fitted luggage, shown at the Paris sale for the first time.", 1380000, 31, 18200],
    block: ["etype", "gullwing", "porsche", "corvette"], lead: "A curated selection crossing the block in Paris. Open a lot for the full file, then bid live.",
  },
  "villa-deste": {
    name: "Concorso d'Eleganza Villa d'Este", short: "Villa d'Este", img: "assets/events/villa-deste.jpg", pos: "50% 60%",
    kicker: "Live event", dates: "Apr 23 to 25, 2027  |  Lake Como, Italy",
    intro: "Concours-grade classics sold on the shore of the lake, in the same gardens where the jury judges the cars.",
    facts: [["Dates", "Apr 23 to 25, 2027", "3 days"], ["Venue", "Villa Erba", "Cernobbio, Lake Como"], ["Lots", "38", "Coachbuilt and concours"], ["Estimate", "$22M to $30M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "A weekend of gardens, judges and long lake-side lunches. Registered bidders get access to every part of it.",
    exp: [["Concours Lawn", "Walk the lawn as the jury judges the finest cars of the weekend.", "Apr 24 to 25", "Villa Erba gardens", "assets/events/villa-deste.jpg"], ["Coachbuilt Preview", "Study one-off bodies and original paint up close before the sale.", "Apr 23", "Villa Erba", "assets/exp/28611534.jpg"], ["Lake Drive", "A guided morning drive of selected lots along the western shore.", "Apr 24", "Lake Como", "assets/exp/29187235.jpg"], ["Gala Dinner", "An evening for bidders and consignors beside the water.", "Apr 24", "Villa d'Este terrace", "assets/exp/5864590.jpg"]], layout: "wave",
    ft: ["etype", "Featured lot", "Lot 009, Villa d'Este", "A Series 1 coupe in the original colour, with a complete file and one family owner for decades.", 168000, 24, 26400],
    block: ["gullwing", "etype", "excalibur", "pontiac"], lead: "Fine cars for the lake-side sale. Open a lot for the full file, then bid live.",
  },
  monterey: {
    name: "Monterey Car Week", short: "Monterey", img: "assets/events/pebble-beach.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Aug 12 to 15, 2027  |  Pebble Beach, California",
    intro: "A global celebration of automotive culture. Rare machines, iconic collectors and the year's biggest sale, timed to the concours.",
    facts: [["Dates", "Aug 12 to 15, 2027", "4 days"], ["Venue", "Pebble Beach", "Monterey, California"], ["Lots", "96", "Iconic cars"], ["Estimate", "$60M to $85M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "Four days of driving, design and exceptional cars. Registered bidders get the paddock, the lawn and the dinner.",
    exp: [["Track Sessions", "Take a passenger lap in selected lots on the circuit.", "Aug 12 to 14", "Laguna Seca", "assets/exp/9500812.jpg"], ["Concours Display", "Classics, bespoke builds and one-off creations on the fairway.", "Aug 15", "Pebble Beach lawn", "assets/exp/34262971.jpg"], ["Paddock Walk", "Exclusive access to teams, drivers and historic race cars.", "Aug 13", "Paddock area", "assets/exp/6154898.jpg"], ["Collectors' Dinner", "An evening with collectors, drivers and industry leaders.", "Aug 14", "Pebble Beach resort", "assets/exp/5864498.jpg"]], layout: "wave",
    ft: ["gt500", "Featured lot", "Lot 014, Monterey Car Week", "A fastback in period black with its original 428 engine, a fixture of the sale for its condition and file.", 214000, 49, 18480],
    block: ["gt500", "corvette", "porsche", "camaro"], lead: "A curated selection of vehicles crossing the block in Monterey. Open a lot for full details, then bid live.",
  },
  goodwood: {
    name: "Goodwood Revival", short: "Goodwood", img: "assets/events/goodwood-revival.jpg", pos: "50% 55%",
    kicker: "Live event", dates: "Sep 10 to 12, 2027  |  West Sussex, England",
    intro: "Race and road cars from the golden era, sold the same weekend they take to the circuit in period dress.",
    facts: [["Dates", "Sep 10 to 12, 2027", "3 days"], ["Venue", "Goodwood Motor Circuit", "Chichester, West Sussex"], ["Lots", "48", "1950s to 1960s"], ["Estimate", "$16M to $22M", "Combined"]],
    expTitle: "More Than an Auction", expLead: "The whole estate dresses for the 1960s. Registered bidders can see the cars run and then bid on them.",
    exp: [["Circuit Day", "Watch selected lots run on the track in period race meetings.", "Sep 10 to 12", "Goodwood circuit", "assets/events/goodwood-revival.jpg"], ["Paddock Preview", "Walk the paddock with the owners and the specialists.", "Sep 10", "Paddock", "assets/exp/29198149.jpg"], ["Period Village", "Vintage shops, stalls and a full 1960s high street.", "All weekend", "Goodwood village", "assets/events/pebble-beach.jpg"], ["Dinner on the Lawn", "A supper for bidders and consignors with the cars on show.", "Sep 11", "House lawn", "assets/exp/8995669.jpg"]], layout: "wave",
    ft: ["etype", "Featured lot", "Lot 021, Goodwood Revival", "A race-prepared coupe with period history, eligible for the weekend's own races.", 168000, 24, 21100],
    block: ["etype", "gt500", "boss", "mustang"], lead: "Cars chosen for the revival sale. Open a lot for the full file, then bid live.",
  },
};

const slug = new URLSearchParams(location.search).get("e");
const key = EVENTS[slug] ? slug : "monterey";
const ev = EVENTS[key];
document.title = "Marque | " + ev.name;

/* hero */
$("evImg").src = ev.img; $("evImg").alt = ev.name; $("evImg").style.objectPosition = ev.pos;
$("evKicker").textContent = ev.kicker; $("evTitle").textContent = ev.name; $("evDates").textContent = ev.dates; $("evIntro").textContent = ev.intro;
$("evFacts").innerHTML = ev.facts.map(f => `<div><dt>${f[0]}</dt><dd>${f[1]}</dd><span>${f[2]}</span></div>`).join("");

/* experience: ball and road, animated with GSAP ScrollTrigger */
$("expTitle").textContent = ev.expTitle; $("expLead").textContent = ev.expLead;
const X = $("x"); X.className = "x x-" + ev.layout;
X.innerHTML = (ev.layout === "wave" ? `<div class="x-pano" aria-hidden="true">${ev.exp.map(x => `<img src="${x[4]}" alt="">`).join("")}</div>` : "") + `<svg class="x-road" aria-hidden="true"><path class="x-road-bg"/><path class="x-road-fg"/></svg><div class="x-ball" aria-hidden="true"></div>` +
  `<ol class="x-items">` + ev.exp.map((x, i) => `<li class="x-item" data-i="${i}"><div class="x-photo"><img src="${x[4]}" alt="" loading="lazy"></div><div class="x-body"><span class="x-dot"></span><h3>${x[0]}</h3><p>${x[1]}</p><span class="mono x-meta">${x[2]}</span><span class="mono x-meta">${x[3]}</span></div></li>`).join("") + `</ol>` +
  (ev.layout === "snake" ? `<span class="x-amp" data-a="0">&amp;</span><span class="x-amp" data-a="1">&amp;</span>` : "");

function buildRoad() {
  const items = [...X.querySelectorAll(".x-item")], box = X.getBoundingClientRect(), W = box.width, H = box.height;
  const svg = X.querySelector(".x-road"); svg.setAttribute("viewBox", `0 0 ${W} ${H}`); svg.style.height = H + "px";
  const pt = d => { const r = d.getBoundingClientRect(); return [r.left - box.left + r.width / 2, r.top - box.top + r.height / 2]; };
  const P = items.map(it => pt(it.querySelector(".x-dot")));
  let d;
  if (ev.layout === "wave") {
    const pts = [[0, P[0][1]], ...P, [W, P[3][1]]];
    d = `M ${pts[0][0]} ${pts[0][1]}`;
    for (let i = 0; i < pts.length - 1; i++) {
      const p0 = pts[i - 1] || pts[i], p1 = pts[i], p2 = pts[i + 1], p3 = pts[i + 2] || p2;
      d += ` C ${p1[0] + (p2[0] - p0[0]) / 6} ${p1[1] + (p2[1] - p0[1]) / 6}, ${p2[0] - (p3[0] - p1[0]) / 6} ${p2[1] - (p3[1] - p1[1]) / 6}, ${p2[0]} ${p2[1]}`;
    }
  } else {
    const R = W - 14, r = 36, y1 = P[0][1], y2 = P[2][1];
    d = `M 0 ${y1} L ${P[0][0]} ${y1} L ${P[1][0]} ${y1} L ${R - r} ${y1} Q ${R} ${y1} ${R} ${y1 + r} L ${R} ${y2 - r} Q ${R} ${y2} ${R - r} ${y2} L ${P[2][0]} ${y2} L ${P[3][0]} ${y2} L 0 ${y2}`;
    X.querySelectorAll(".x-amp").forEach((a, n) => { const y = n ? y2 : y1, x = n ? (P[2][0] + P[3][0]) / 2 : (P[0][0] + P[1][0]) / 2; a.style.transform = `translate(${x - 20}px, ${y - 20}px)`; });
  }
  svg.querySelectorAll("path").forEach(p => p.setAttribute("d", d));
  const pano = X.querySelector(".x-pano");
  if (pano && ev.layout === "wave") {          // clip the photo band along the road so its lower edge is the wave
    const fg = svg.querySelector(".x-road-fg"), L = fg.getTotalLength(), pad = parseFloat(getComputedStyle(X.parentElement).paddingLeft) || 0, up = 8, pts = [];
    for (let i = 0; i <= 90; i++) { const q = fg.getPointAtLength(L * i / 90); pts.push(`${(q.x + pad).toFixed(1)}px ${(q.y - up).toFixed(1)}px`); }
    const Wp = W + 2 * pad, yS = P[0][1] - up, yE = P[3][1] - up;
    pano.style.clipPath = `polygon(0px 0px, ${Wp}px 0px, ${Wp}px ${yE}px, ${pts.reverse().join(", ")}, 0px ${yS}px)`;
  }
  return { path: svg.querySelector(".x-road-fg"), P };
}

let roadST;
function initRoad() {
  const { path, P } = buildRoad(), len = path.getTotalLength(), ball = X.querySelector(".x-ball"), items = [...X.querySelectorAll(".x-item")];
  // length fraction of each stop = nearest sampled point on the road
  const stops = P.map(p => { let best = 0, bd = 1e9; for (let l = 0; l <= len; l += 4) { const q = path.getPointAtLength(l), dd = (q.x - p[0]) ** 2 + (q.y - p[1]) ** 2; if (dd < bd) { bd = dd; best = l; } } return best / len; });
  path.style.strokeDasharray = len; path.style.strokeDashoffset = len;
  const place = f => { const q = path.getPointAtLength(f * len); gsap.set(ball, { x: q.x, y: q.y }); path.style.strokeDashoffset = len * (1 - f); items.forEach((it, i) => it.classList.toggle("on", f >= stops[i] - 0.015)); };
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { place(1); return; }
  place(0);
  roadST = ScrollTrigger.create({ trigger: X, start: `top+=${Math.round(P[0][1])} 82%`, end: `+=${Math.round(innerHeight * 0.65)}`, scrub: 1.4, onUpdate: s => place(s.progress) });
}
const startRoad = () => { if (innerWidth > 900) { X.classList.add("x-live"); initRoad(); } else X.classList.remove("x-live"); };
gsap.registerPlugin(ScrollTrigger);
let rz; addEventListener("resize", () => { clearTimeout(rz); rz = setTimeout(() => { if (roadST) roadST.kill(); X.querySelectorAll(".x-item").forEach(i => i.classList.remove("on")); startRoad(); }, 200); });
addEventListener("load", startRoad);

/* featured lot: first two events fade top and bottom, last two fade bottom only */
$("evFeature").classList.add(["retromobile", "villa-deste"].includes(key) ? "ft-both" : "ft-bottom");
/* featured lot */
const [ftKey, ftKick, ftMeta, ftText, ftBid, ftBids, ftEnd] = ev.ft;
const fc = CARS[ftKey];
$("ftKicker").textContent = ftKick; $("ftTitle").textContent = fc.title; $("ftMeta").textContent = ftMeta; $("ftText").textContent = ftText;
$("ftBid").textContent = money(ftBid); $("ftBids").textContent = ftBids;
$("ftImg").src = fc.img; $("ftImg").alt = fc.alt; $("ftImg").style.objectPosition = fc.pos || "50% 60%";
let ftLeft = ftEnd; $("ftTime").textContent = clock(ftLeft);

/* lots */
$("blockLead").textContent = ev.lead;
const lots = ev.block.map((k, i) => ({ ...CARS[k], id: k, n: 10 + i * 6, end: [5400, 28800, 43200, 1500][i] }));
$("evGrid").innerHTML = lots.map(l => `
  <article class="card" id="lot-${l.id}">
    <div class="card-media"><a class="card-link" href="lot.html?id=${l.id}" aria-label="Open ${l.title}"><img src="${l.img}" alt="${l.alt}" loading="lazy" style="object-position:${l.pos || "50% 60%"}"></a><button class="heart" aria-label="Save ${l.title}" aria-pressed="false">♡</button></div>
    <div class="card-body">
      <h3><a href="lot.html?id=${l.id}">${l.title}</a></h3><p class="where">${l.where}</p>
      <dl class="lot-stats"><div><dt>Current bid</dt><dd class="bid"></dd></div><div><dt>Ends in</dt><dd class="time"></dd></div></dl>
      <p class="lot-meta"><span class="bids"></span><span class="reserve">${l.reserve}</span></p>
      <button class="btn btn-block bid-btn">Bid <span class="next"></span></button>
    </div>
  </article>`).join("");
const cards = lots.map(l => { const el = $("lot-" + l.id); return { el, left: l.end, bid: l.bid, step: l.step, count: l.bids, bidEl: el.querySelector(".bid"), timeEl: el.querySelector(".time"), nextEl: el.querySelector(".next"), bidsEl: el.querySelector(".bids"), btn: el.querySelector(".bid-btn"), heart: el.querySelector(".heart") }; });

const toast = $("toast"); let tt;
const say = m => { toast.textContent = m; toast.classList.add("show"); clearTimeout(tt); tt = setTimeout(() => toast.classList.remove("show"), 2200); };
const draw = (c, flash) => { window.tickBid ? window.tickBid(c.bidEl, c.bid, money) : (c.bidEl.textContent = money(c.bid)); c.nextEl.textContent = money(c.bid + c.step); c.bidsEl.textContent = c.count + " bids"; if (flash) { c.el.classList.remove("flash"); void c.el.offsetWidth; c.el.classList.add("flash"); } };
cards.forEach(c => {
  draw(c); c.timeEl.textContent = clock(c.left); c.el.classList.toggle("urgent", c.left < 1800);
  c.btn.addEventListener("click", () => { if (c.left <= 0) return; c.bid += c.step; c.count++; draw(c, true); say("Bid placed: " + money(c.bid)); });
  c.heart.addEventListener("click", () => { const on = c.heart.classList.toggle("on"); c.heart.textContent = on ? "♥" : "♡"; c.heart.setAttribute("aria-pressed", on); });
});
setInterval(() => {
  ftLeft = Math.max(0, ftLeft - 1); $("ftTime").textContent = clock(ftLeft);
  cards.forEach(c => {
    if (c.left <= 0) return; c.left--; c.el.classList.toggle("urgent", c.left < 1800);
    if (c.left === 0) { c.timeEl.textContent = "Closed"; c.btn.disabled = true; c.btn.textContent = "Auction closed"; return; }
    c.timeEl.textContent = clock(c.left);
    if (Math.random() < 0.03) { c.bid += c.step; c.count++; draw(c, true); }
  });
}, 1000);

/* other events */
$("moreList").innerHTML = Object.entries(EVENTS).filter(([k]) => k !== key).map(([k, e]) => `<li><a href="event.html?e=${k}"><b>${e.name}</b><span>${e.dates.split("  |  ")[1]}</span><span>${e.facts[0][1]}</span></a></li>`).join("");

/* noir toggle */
const modeBtn = $("modeToggle");
const setTheme = dark => { if (dark) document.documentElement.setAttribute("data-theme", "dark"); else document.documentElement.removeAttribute("data-theme"); modeBtn.setAttribute("aria-checked", dark); try { localStorage.setItem("marque-theme", dark ? "dark" : "light"); } catch (e) {} };
modeBtn.setAttribute("aria-checked", document.documentElement.getAttribute("data-theme") === "dark");
modeBtn.addEventListener("click", () => setTheme(modeBtn.getAttribute("aria-checked") !== "true"));
