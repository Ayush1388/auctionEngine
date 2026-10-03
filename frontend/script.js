// Demo-only behaviour: slideshow, drawer, countdowns, simulated bids. No backend calls yet.
const fmtMoney = n => "$" + n.toLocaleString("en-US");
const pad = n => String(n).padStart(2, "0");
const fmtClock = s => { s = Math.max(0, s); return [Math.floor(s / 3600), Math.floor((s % 3600) / 60), s % 60].map(pad).join(":"); };

const toast = document.getElementById("toast");
let toastTimer;
function say(msg) {
  toast.textContent = msg;
  toast.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove("show"), 2200);
}

/* ---------- Hero slideshow ---------- */
const slides = [
  { img: "assets/hero.jpg",    alt: "Black 1963 Chevrolet Impala lowrider at dusk", title: "1963 Chevrolet Impala SS", lot: "Lot 001, no reserve", bid: 42500, pos: "50% 82%", tag: "A noir icon" },
  { img: "assets/ivan.jpg",    alt: "Red 1951 Pontiac Chieftain convertible",       title: "1951 Pontiac Chieftain",   lot: "Lot 019, Miami",      bid: 64500, pos: "50% 35%", tag: "Sunlit survivor" },
  { img: "assets/doina.jpg",   alt: "Red Excalibur roadster",                       title: "1969 Excalibur Roadster",  lot: "Lot 024, New York",   bid: 71000, pos: "50% 65%", tag: "Neo-classic" },
  { img: "assets/theodor.jpg", alt: "Red 1965 Ford Mustang coupe",                  title: "1965 Ford Mustang Coupe",  lot: "Lot 026, London",     bid: 38000, pos: "50% 64%", tag: "Pony car" },
];
const stageImg = document.getElementById("stageImg");
const thumbBtns = [...document.querySelectorAll("#thumbs button")];
let current = 0;

function show(i) {
  current = (i + slides.length) % slides.length;
  const s = slides[current];
  stageImg.classList.add("fade");
  setTimeout(() => {
    stageImg.src = s.img; stageImg.alt = s.alt; stageImg.style.objectPosition = s.pos;
    stageImg.classList.remove("fade");
  }, 250);
  document.getElementById("slideTitle").textContent = s.title;
  document.getElementById("slideLot").textContent = s.lot;
  document.getElementById("slideBid").textContent = fmtMoney(s.bid);
  document.getElementById("slideName").textContent = s.tag;
  thumbBtns.forEach(b => b.classList.toggle("on", +b.dataset.slide === current));
}
thumbBtns.forEach(b => b.addEventListener("click", () => show(+b.dataset.slide)));
document.getElementById("prev").addEventListener("click", () => show(current - 1));
document.getElementById("next").addEventListener("click", () => show(current + 1));

/* ---------- Drawer ---------- */
const drawer = document.getElementById("drawer");
const burger = document.getElementById("burger");
function setDrawer(open) {
  drawer.classList.toggle("open", open);
  drawer.setAttribute("aria-hidden", !open);
  burger.setAttribute("aria-expanded", open);
  if (open) drawer.querySelector("input").focus(); else burger.focus();
}
burger.addEventListener("click", () => setDrawer(true));
document.getElementById("drawerClose").addEventListener("click", () => setDrawer(false));
drawer.querySelectorAll("a").forEach(a => a.addEventListener("click", () => setDrawer(false)));
document.addEventListener("keydown", e => { if (e.key === "Escape" && drawer.classList.contains("open")) setDrawer(false); });

/* ---------- Lots ---------- */
const lots = {
  classic: [
    { id: "corvette", img: "assets/corvette.jpg", alt: "White 1963 Chevrolet Corvette Sting Ray convertible", title: "1963 Chevrolet Corvette Sting Ray", spec: "327 V8, 4-speed, red interior", where: "Sydney, 41,200 km", bid: 118000, step: 1000, end: 5400, bids: 27, reserve: "Reserve met" },
    { id: "gullwing", img: "assets/300sl.jpg", alt: "Red 1955 Mercedes-Benz 300 SL Gullwing with its doors raised", title: "1955 Mercedes-Benz 300 SL Gullwing", spec: "3.0 fuel-injected six, cream leather", where: "Stuttgart, 62,000 km", bid: 1380000, step: 10000, end: 64800, bids: 31, reserve: "Reserve not met", pos: "50% 55%" },
    { id: "etype", img: "assets/etype.jpg", alt: "Blue 1964 Jaguar E-Type Series 1 coupe in profile", title: "1964 Jaguar E-Type Series 1 Coupe", spec: "3.8 straight-six, 4-speed, wire wheels", where: "Norwich, 58,300 km", bid: 168000, step: 2000, end: 28800, bids: 24, reserve: "Reserve met", pos: "42% 62%" },
    { id: "porsche", img: "assets/porsche.jpg", alt: "Cream 1972 Porsche 911 Targa on a country road", title: "1972 Porsche 911 S Targa", spec: "2.4 flat-six, 5-speed, matching numbers", where: "Lippe, 63,800 km", bid: 186000, step: 2000, end: 43200, bids: 41, reserve: "Reserve met" },
  ],
  muscle: [
    { id: "camaro", img: "assets/camaro.jpg", alt: "Black 1967 Chevrolet Camaro SS convertible with white stripes", title: "1967 Chevrolet Camaro SS 350", spec: "350 V8, white stripes, convertible", where: "Heidelberg, 52,000 km", bid: 78500, step: 1000, end: 7200, bids: 37, reserve: "Reserve met", pos: "50% 60%" },
    { id: "gt500", img: "assets/gt500.jpg", alt: "Black 1967 Shelby GT500 KR fastback", title: "1967 Shelby GT500 KR", spec: "428 Police Interceptor V8, fastback", where: "Nuremberg, 68,000 km", bid: 214000, step: 2000, end: 1500, bids: 49, reserve: "Reserve met", pos: "45% 60%" },
    { id: "challenger", img: "assets/challenger.jpg", alt: "Lime green 1970 Dodge Challenger R/T with a black hood stripe", title: "1970 Dodge Challenger R/T", spec: "426 Hemi, black stripe, Sublime Green", where: "Melbourne, 61,000 km", bid: 168000, step: 2000, end: 21600, bids: 29, reserve: "No reserve", pos: "50% 55%" },
    { id: "boss429", img: "assets/boss429.jpg", alt: "Red 1969 Ford Mustang Boss 429 with its hood raised", title: "1969 Ford Mustang Boss 429", spec: "429 semi-hemi V8, Candy Apple Red", where: "Charlotte, 54,000 km", bid: 312000, step: 2500, end: 43200, bids: 22, reserve: "Reserve not met", pos: "50% 55%" },
  ],
};

function cardHTML(l) {
  return `
    <article class="card" id="lot-${l.id}">
      <div class="card-media"><a class="card-link" href="lot.html?id=${l.id}" aria-label="Open ${l.title}"><img src="${l.img}" alt="${l.alt}" loading="lazy"${l.pos ? ` style="object-position:${l.pos}"` : ""}></a><button class="heart" aria-label="Save ${l.title}" aria-pressed="false">♡</button></div>
      <div class="card-body">
        <h3><a href="lot.html?id=${l.id}">${l.title}</a></h3>
        <p class="spec">${l.spec}</p>
        <p class="where">${l.where}</p>
        <dl class="lot-stats">
          <div><dt>Current bid</dt><dd class="bid"></dd></div>
          <div><dt>Ends in</dt><dd class="time"></dd></div>
        </dl>
        <p class="lot-meta"><span class="bids"></span><span class="reserve">${l.reserve}</span></p>
        <button class="btn btn-block bid-btn">Bid <span class="next"></span></button>
      </div>
    </article>`;
}
document.getElementById("classicGrid").innerHTML = lots.classic.map(cardHTML).join("");
document.getElementById("muscleGrid").innerHTML = lots.muscle.map(cardHTML).join("");

const cards = [...lots.classic, ...lots.muscle].map(l => {
  const el = document.getElementById("lot-" + l.id);
  return { el, remaining: l.end, bid: l.bid, step: l.step, count: l.bids,
    bidEl: el.querySelector(".bid"), timeEl: el.querySelector(".time"), nextEl: el.querySelector(".next"),
    bidsEl: el.querySelector(".bids"), btn: el.querySelector(".bid-btn"), heart: el.querySelector(".heart") };
});

function render(c, flash) {
  window.tickBid ? window.tickBid(c.bidEl, c.bid, fmtMoney) : (c.bidEl.textContent = fmtMoney(c.bid));
  c.nextEl.textContent = fmtMoney(c.bid + c.step);
  c.bidsEl.textContent = c.count + " bids";
  if (flash) { c.el.classList.remove("flash"); void c.el.offsetWidth; c.el.classList.add("flash"); }
}
cards.forEach(c => {
  render(c, false);
  c.timeEl.textContent = fmtClock(c.remaining);
  c.el.classList.toggle("urgent", c.remaining < 1800);
  c.btn.addEventListener("click", () => {
    if (c.remaining <= 0) return;
    c.bid += c.step; c.count++; render(c, true); say("Bid placed: " + fmtMoney(c.bid));
  });
  c.heart.addEventListener("click", () => {
    const on = c.heart.classList.toggle("on");
    c.heart.textContent = on ? "♥" : "♡";
    c.heart.setAttribute("aria-pressed", on);
  });
});

/* ---------- Clock tick ---------- */
const heroClock = document.getElementById("heroClock");
let heroRemaining = 1 * 86400 + 16 * 3600 + 23 * 60 + 45;
const fmtHero = s => [Math.floor(s / 86400), Math.floor(s % 86400 / 3600), Math.floor(s % 3600 / 60), s % 60].map(pad).join(" : ");
heroClock.textContent = fmtHero(heroRemaining);

setInterval(() => {
  heroRemaining = Math.max(0, heroRemaining - 1);
  heroClock.textContent = fmtHero(heroRemaining);
  cards.forEach(c => {
    if (c.remaining <= 0) return;
    c.remaining--;
    c.el.classList.toggle("urgent", c.remaining < 1800);
    if (c.remaining === 0) {
      c.timeEl.textContent = "Closed"; c.el.classList.add("closed");
      c.btn.disabled = true; c.btn.textContent = "Auction closed";
      return;
    }
    c.timeEl.textContent = fmtClock(c.remaining);
    if (Math.random() < 0.03) { c.bid += c.step; c.count++; render(c, true); } // simulate another bidder
  });
}, 1000);

/* ---------- Stat counters ---------- */
const io = new IntersectionObserver(entries => {
  entries.forEach(e => {
    if (!e.isIntersecting) return;
    io.unobserve(e.target);
    const { count, prefix = "", suffix = "" } = e.target.dataset;
    const start = performance.now();
    const tick = now => {
      const t = Math.min(1, (now - start) / 1200);
      e.target.textContent = prefix + Math.round(+count * (1 - Math.pow(1 - t, 3))) + suffix;
      if (t < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
}, { threshold: 0.4 });
document.querySelectorAll("[data-count]").forEach(el => io.observe(el));

/* ---------- Noir (dark) theme ---------- */
const modeBtn = document.getElementById("modeToggle");
function setTheme(dark) {
  if (dark) document.documentElement.setAttribute("data-theme", "dark");
  else document.documentElement.removeAttribute("data-theme");
  modeBtn.setAttribute("aria-checked", dark);
  try { localStorage.setItem("marque-theme", dark ? "dark" : "light"); } catch (e) {}
}
modeBtn.setAttribute("aria-checked", document.documentElement.getAttribute("data-theme") === "dark");
modeBtn.addEventListener("click", () => setTheme(modeBtn.getAttribute("aria-checked") !== "true"));
