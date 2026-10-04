// Sell a car: create a listing, preview it, and manage my listings.
import { initShell, toast } from "../shell.js";
import { loadCatalog, buildCreateRequest, toLot, secondsLeft } from "../model.js";
import { html, raw, esc, $, $$, usd, parseUsd, uuid, clock, dateTime, local, debounce } from "../util.js";

const be = await initShell({ bar: true, footer: true, requireAuth: true });
if (!be.session.id) throw new Error("redirecting to sign in");
$("#sellBody").setAttribute("aria-busy", "false");

const cat = await loadCatalog();
const photos = [...new Map(cat.lots.map(l => [l.img, { img: l.img, pos: l.pos, alt: l.alt, key: l.key }])).values()];
let photo = photos[0], localPhoto = null, localData = null;

/* ---------------------------------------------------------------- photo */
const photosEl = $("#photos");
function drawPhotos() {
  photosEl.innerHTML = photos.map((p, i) => `<label class="ph ${p === photo ? "on" : ""}"><input type="radio" name="photo" value="${i}" ${p === photo ? "checked" : ""} class="visually"><img src="${esc(p.img)}" alt="${esc(p.alt)}" style="object-position:${esc(p.pos)}" loading="lazy"></label>`).join("");
  $$("input[name=photo]", photosEl).forEach(r => r.addEventListener("change", () => { photo = photos[+r.value]; localPhoto = null; localData = null; drawPhotos(); preview(); }));
}
drawPhotos();

$("#upload").addEventListener("change", async e => {
  const f = e.target.files[0], err = $("#upErr"); err.textContent = "";
  if (!f) return;
  if (!/^image\//.test(f.type)) { err.textContent = "Choose an image file."; return; }
  try {
    const bmp = await createImageBitmap(f), k = Math.min(1, 1400 / Math.max(bmp.width, bmp.height));
    const c = Object.assign(document.createElement("canvas"), { width: Math.round(bmp.width * k), height: Math.round(bmp.height * k) });
    c.getContext("2d").drawImage(bmp, 0, 0, c.width, c.height);
    localData = c.toDataURL("image/jpeg", 0.8); localPhoto = uuid();
    if (!local.set("marque-photo:" + localPhoto, localData)) { localData = localPhoto = null; err.textContent = "This browser has no room to keep that photo. Pick a house photo instead."; return; }
    preview();
  } catch { err.textContent = "That file could not be read as an image."; }
});

/* ---------------------------------------------------------------- the form */
const v = id => $("#" + id).value;
const num = id => Number(String(v(id)).replace(/[^0-9]/g, "")) || 0;
function startsAt() {
  const o = v("opens");
  return o === "custom" ? ($("#opensAt").value ? new Date($("#opensAt").value) : null) : new Date(Date.now() + (+o) * 60000 + (o === "1" ? 8000 : 0));   // a few seconds of margin: the API wants the future
}
function values() {
  const s = startsAt(), d = +v("duration");
  return {
    title: v("title"), year: num("year") || undefined, make: v("make"), model: v("model"), where: v("where"), category: v("category"), description: v("description"),
    img: photo.img, pos: photo.pos, localPhoto, startingPrice: num("startingPrice"), step: num("step"),
    startsAt: s ? s.toISOString() : "", endsAt: s ? new Date(s.getTime() + d * 60000).toISOString() : "",
  };
}

function showErrors(map) {
  $$(".field", $("#sellForm")).forEach(f => { f.classList.remove("invalid"); const e = f.querySelector(".err"); if (e) e.textContent = ""; });
  let first = null;
  for (const [k, m] of Object.entries(map)) {
    const f = $(`[data-f="${k}"]`); if (!f) continue;
    f.classList.add("invalid"); f.querySelector(".err").textContent = m; first ||= f.querySelector("input,select,textarea");
  }
  first?.focus();
}

/** Checks the person can fix before sending; the server still has the last word. */
function check(f) {
  const e = {}, now = new Date().getFullYear();
  if (!f.title.trim()) e.title = "Give the car a title.";
  if (!f.year || f.year < 1886 || f.year > now + 1) e.year = "Enter a year.";
  if (!f.make.trim()) e.make = "Enter the make.";
  if (!f.model.trim()) e.model = "Enter the model.";
  if (!f.where.trim()) e.where = "Say where the car is.";
  if (!f.startingPrice) e.startingPrice = "Enter an opening bid.";
  if (!f.step) e.step = "Enter the smallest raise, at least $1.";
  if (!f.startsAt) e.startsAt = "Pick when it opens.";
  return e;
}

const SERVER_FIELDS = { "item.name": "title", "item.type": "title", "item.description": "description", starting_price: "startingPrice", min_increment: "step", starts_at: "startsAt", ends_at: "duration" };
$("#sellForm").addEventListener("submit", async e => {
  e.preventDefault();
  const f = values(), problems = check(f), formErr = $("#formErr"); formErr.hidden = true;
  showErrors(problems);
  if (Object.keys(problems).length) return;
  const go = $("#submit"); go.classList.add("busy");
  try {
    const a = await be.createAuction(buildCreateRequest(f));
    toast("Listed. It opens " + (new Date(a.starts_at) - Date.now() < 90000 ? "in a moment" : dateTime(a.starts_at)));
    setTimeout(() => { location.href = "lot.html?id=" + a.id; }, 700);
  } catch (x) {
    go.classList.remove("busy");
    const mapped = {}; for (const [k, m] of Object.entries(x.fields || {})) mapped[SERVER_FIELDS[k] || k] = m;
    showErrors(mapped);
    if (!Object.keys(mapped).length) { formErr.hidden = false; formErr.textContent = x.network ? "Cannot reach the server. Nothing was listed." : x.message; }
  }
});

$("#opens").addEventListener("change", () => { $("#opensAt").hidden = v("opens") !== "custom"; preview(); });
$("#description").addEventListener("input", () => { $("#descCount").textContent = `${v("description").length} / 4000`; });
$("#sellForm").addEventListener("input", debounce(() => preview(), 120));
$("#sellForm").addEventListener("change", () => preview());

/* ---------------------------------------------------------------- the preview card */
function preview() {
  const f = values(), img = localData || photo.img;
  const mins = +v("duration"), opens = $("#opens").selectedOptions[0].textContent.toLowerCase();
  $("#preview").innerHTML = String(html`
    <article class="card">
      <div class="card-media"><img src="${img}" alt="" style="object-position:${photo.pos}"></div>
      <div class="card-body">
        <h3>${f.title || "Your car's title"}</h3>
        <p class="spec">${[f.make, f.model, f.year].filter(Boolean).join(" ") || "Make, model and year"}</p>
        <p class="where">${f.where || "Where the car is"}</p>
        <dl class="lot-stats"><div><dt>Opening bid</dt><dd class="bid">${usd(f.startingPrice * 100)}</dd></div><div><dt>Runs for</dt><dd class="time">${mins >= 1440 ? mins / 1440 + " d" : mins >= 60 ? mins / 60 + " h" : mins + " min"}</dd></div></dl>
        <p class="lot-meta"><span>Opens ${opens === "pick a date and time" ? (f.startsAt ? dateTime(f.startsAt) : "on a date you choose") : opens}</span><span class="reserve">${f.step ? "+" + usd(f.step * 100) + " a bid" : ""}</span></p>
      </div>
    </article>`);
}

/* ---------------------------------------------------------------- my listings */
const mine = $("#myListings");
let listings = [];
async function drawListings() {
  const tag = a => ({ ACTIVE: ["tag-active", "Open"], NOT_ACTIVE: ["tag-upcoming", "Opens soon"], COMPLETED: ["tag-closed", "Closed"], CANCELLED: ["tag-cancelled", "Cancelled"] })[a.status] || ["", a.status];
  mine.innerHTML = listings.length ? String(html`<ul class="lrows">${listings.map(a => {
    const l = toLot(a), [cls, label] = tag(a), cancellable = a.status === "NOT_ACTIVE" && Date.parse(a.starts_at) > Date.now();
    return html`<li class="lrow"><a class="thumb" href="lot.html?id=${a.id}" tabindex="-1" aria-hidden="true"><img src="${l.img}" alt="" style="object-position:${l.pos}"></a>
      <div><a href="lot.html?id=${a.id}"><b>${l.title}</b></a><p class="small muted">${l.bids} ${l.bids === 1 ? "bid" : "bids"}${l.current != null ? html`, now <span class="tnum">${usd(l.current)}</span>` : ""}${a.status === "ACTIVE" ? html`, ends in <span class="tnum">${clock(secondsLeft(l))}</span>` : a.status === "NOT_ACTIVE" ? html`, opens ${dateTime(a.starts_at)}` : ""}</p></div>
      <div class="brow-act"><span class="tag ${cls}">${label}</span>${cancellable ? html`<button class="btn btn-line btn-sm" data-cancel="${a.id}">Cancel</button>` : ""}</div></li>`;
  })}</ul>`) : `<div class="empty"><h3>No listings yet</h3><p>Fill in the form and your car appears here, live, with its bids.</p></div>`;
  $$("[data-cancel]", mine).forEach(b => b.addEventListener("click", async () => {
    if (!confirm("Cancel this listing? It has not opened yet, so nobody has bid.")) return;
    b.classList.add("busy");
    try { await be.cancelAuction(b.dataset.cancel); toast("Listing cancelled"); await loadListings(); }
    catch (x) { b.classList.remove("busy"); toast(x.status === 409 ? "It has already opened and can no longer be cancelled" : x.message); }
  }));
}
async function loadListings() {
  try { listings = (await be.listAuctions({ owner: "me", limit: 50 })).auctions; await drawListings(); }
  catch (x) { mine.innerHTML = String(html`<p class="form-error">${x.message}</p>`); }
}
loadListings();
setInterval(() => { if (listings.some(a => a.status === "NOT_ACTIVE" || a.status === "ACTIVE")) loadListings(); }, 15000);
preview();
