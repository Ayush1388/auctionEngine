// Turns API auctions into what the pages draw, and back.
//
// The API's item has a name, a type and a free-text description. A car needs
// more (year, make, photo, specs), so listings carry a small metadata block at
// the end of the description:
//
//     <prose the seller wrote>
//
//     [[marque:{"year":1963,"make":"Chevrolet",...}]]
//
// parseItem() strips it for display; encodeDescription() writes it. Listings
// created elsewhere (no block) still render, with a generic photo.
import { toCents, local } from "./util.js";

const MARK = "[[marque:";
const FALLBACK_IMG = "assets/hero.jpg";

export function encodeDescription(prose, meta) {
  return `${(prose || "").trim()}\n\n${MARK}${JSON.stringify(meta)}]]`;
}

export function parseItem(item = {}) {
  const d = item.description || "";
  const i = d.lastIndexOf(MARK);
  let meta = {}, prose = d;
  if (i >= 0 && d.trimEnd().endsWith("]]")) {
    try { meta = JSON.parse(d.slice(i + MARK.length).trimEnd().slice(0, -2)); prose = d.slice(0, i).trim(); } catch { /* keep prose as written */ }
  }
  return { title: item.name || "Untitled lot", kind: item.type || "", prose, meta };
}

let catalogP;
/** The shared catalogue (also what the Go seeder reads). */
export function loadCatalog() {
  return catalogP ||= fetch(new URL("catalog.json", import.meta.url)).then(r => r.json()).catch(() => ({ lots: [] }));
}

/** Catalogue entry -> the metadata block stored with a seeded auction. */
export function metaOf(lot) {
  const { key, category, lot: lotNo, alt, img, pos, year, make, model, type, where, spec, event, reserve, engine, trans, power, colour, interior, miles, chassis, history, quote, lead } = lot;
  return { key, category, lot: lotNo, alt, img, pos, year, make, model, type, where, spec, event, reserve, engine, trans, power, colour, interior, miles, chassis, history, quote, lead };
}
export const proseOf = lot => [lot.lead, ...lot.text].join("\n\n");

/** An auction from the API, ready to draw. */
export function toLot(a) {
  const it = parseItem(a.item);
  const m = it.meta;
  const hasBids = a.current_bid != null;
  const minNext = hasBids ? a.current_bid + a.min_increment : Math.max(a.starting_price, 1);
  const paras = it.prose.split(/\n{2,}/).map(s => s.trim()).filter(Boolean);
  return {
    id: a.id,
    key: m.key || null,
    category: m.category || "classic",
    ownerId: a.owner_id,
    title: it.title,
    alt: m.alt || it.title,
    img: (m.localPhoto && local.get("marque-photo:" + m.localPhoto)) || m.img || FALLBACK_IMG,
    pos: m.pos || "50% 60%",
    year: m.year, make: m.make, model: m.model, type: m.type || it.kind,
    where: m.where || "",
    spec: m.spec || "",
    lotNo: m.lot || "",
    event: m.event || null,
    reserve: m.reserve || "No reserve",
    specs: [["Engine", m.engine], ["Transmission", m.trans], ["Power", m.power], ["Exterior", m.colour], ["Interior", m.interior], ["Mileage", m.miles], ["Chassis no.", m.chassis]].filter(r => r[1]),
    lead: paras[0] || "",
    text: paras.slice(1),
    history: m.history || "",
    quote: m.quote || "",
    status: a.status,
    startingPrice: a.starting_price,
    step: a.min_increment,
    current: hasBids ? a.current_bid : null,
    bidderId: a.current_bidder_id,
    bids: a.bid_count,
    minNext,
    startsAt: a.starts_at,
    endsAt: a.ends_at,
    extensions: a.extensions,
    settledAt: a.settled_at,
    version: a.version ?? null,
    createdAt: a.created_at,
    raw: a,
  };
}

/** Merge a WebSocket snapshot into a lot (snapshot fields win). */
export function applySnapshot(lot, s) {
  return {
    ...lot,
    status: s.status,
    current: s.current_bid,
    bidderId: s.current_bidder_id,
    bids: s.bid_count,
    minNext: s.min_next_bid,
    endsAt: s.ends_at,
    extensions: s.extensions,
    version: s.version,
  };
}

/** Seconds left on a lot, using a server-aligned clock. */
export const secondsLeft = (lot, now = Date.now()) => Math.max(0, Math.ceil((new Date(lot.endsAt).getTime() - now) / 1000));
export const isLive = lot => lot.status === "ACTIVE";

/** Build a CreateAuction request from the Sell form. */
export function buildCreateRequest(f) {
  return {
    item: {
      name: f.title.trim(),
      type: "car",
      description: encodeDescription(f.description, {
        category: f.category, img: f.img, pos: f.pos || "50% 60%", localPhoto: f.localPhoto || undefined, alt: f.title.trim(), year: f.year, make: f.make.trim(), model: f.model.trim(),
        where: f.where.trim(), reserve: "No reserve", spec: [f.make, f.model, f.year].filter(Boolean).join(" "), engine: f.engine || undefined, miles: f.miles || undefined, colour: f.colour || undefined,
      }),
    },
    starting_price: toCents(f.startingPrice),
    min_increment: toCents(f.step),
    starts_at: f.startsAt,
    ends_at: f.endsAt,
  };
}
