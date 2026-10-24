// What every page shares and no single page owns: the wallet readout, the alerts that turn live
// updates into "you were outbid", and the system pulse. Plain data and events, no React in here;
// components subscribe through the hooks in src/hooks.
import { toLot } from "../../app/model.js";
import { participatedIds } from "../../app/mybids.js";
import { usd, local, uuid, Emitter } from "../../app/util.js";
import { toast } from "./toast.js";

export const shell = new Emitter();          // "wallet" (a wallet), "alerts" (the list changed)
let be = null;
export const setBackend = b => { be = b; };

/* ------------------------------------------------------------------ wallet */
export let lastWallet = null;
export async function refreshWallet() {
  if (!be?.session.id) { lastWallet = null; shell.emit("wallet", null); return null; }
  try {
    lastWallet = await be.wallet();
    shell.emit("wallet", lastWallet);
    return lastWallet;
  } catch { return null; }
}

/* ------------------------------------------------------------------ alerts */
const notesKey = () => `marque-alerts:${be.kind}:${be.session.id}`;
export const loadNotes = () => (be?.session.id ? local.json(notesKey(), []) : []);
const saveNotes = n => { local.setJson(notesKey(), n.slice(0, 40)); shell.emit("alerts"); };
export const markAllRead = () => saveNotes(loadNotes().map(n => ({ ...n, read: true })));
export const markRead = id => saveNotes(loadNotes().map(n => (n.id === id ? { ...n, read: true } : n)));

export function notify(n) {
  if (!be?.session.id) return;
  saveNotes([{ id: uuid(), at: new Date().toISOString(), read: false, ...n }, ...loadNotes()]);
  toast(n.title);
}

const titles = new Map();
async function titleOf(id) {
  if (titles.has(id)) return titles.get(id);
  try { const t = toLot(await be.getAuction(id)).title; titles.set(id, t); return t; } catch { return "a lot"; }
}

/** The auctions the alert engine is following: every one I have bid on. */
const followed = new Map();
export function follow(auctionId) {
  if (followed.has(auctionId) || !be?.session.id) return;
  followed.set(auctionId, be.feed.subscribe(auctionId, ev => onUpdate(ev)));
}
export function resetAlerts() { followed.forEach(off => off()); followed.clear(); }

async function onUpdate(ev) {
  const me = be.session.id, a = ev.auction;
  if (!me) return;
  if (ev.bid && ev.bid.previous_bidder_id === me && ev.bid.bidder_id !== me) {
    notify({ kind: "outbid", title: `You were outbid on ${await titleOf(a.id)}`, body: `New high bid ${usd(a.current_bid)}. Your hold was released.`, href: `/lot/${a.id}` });
    refreshWallet();
  } else if (ev.cause === "auction.completed") {
    const t = await titleOf(a.id);
    notify(a.current_bidder_id === me
      ? { kind: "won", title: `You won ${t}`, body: `Winning bid ${usd(a.current_bid)}. Payment is being settled.`, href: `/lot/${a.id}` }
      : { kind: "lost", title: `${t} has closed`, body: `It sold for ${usd(a.current_bid)}. Your hold was released.`, href: `/lot/${a.id}` });
    refreshWallet();
  } else if (ev.cause === "auction.settled" && a.current_bidder_id === me) {
    refreshWallet();
  }
}

export async function startAlerts() {
  if (!be?.session.id) return;
  try { (await participatedIds(be, 1)).forEach(follow); } catch { /* alerts still work for lots bid on from now */ }
}

/* ------------------------------------------------------------------ system pulse */
// Is every dependency answering, and how long does a round trip take. Reads the public probes,
// so it works for every visitor.
export const pulse = new Emitter();          // "tick"
export let lastPulse = null;
const rtt = [];
let pulseTimer = 0;

async function pulseOnce() {
  if (document.hidden || !be) return;
  let ready = null, ms = null;
  try { ms = (await be.livez()).ms; } catch { /* shown as down */ }
  try { ready = await be.readyz(); } catch (e) { ready = e.body?.checks ? e.body : null; }
  const checks = ready?.checks || {};
  const failing = Object.entries(checks).filter(([, v]) => v !== "ok").map(([k]) => k);
  const state = ms == null || ready?.status === "unavailable" ? "bad" : failing.length ? "warn" : "ok";
  if (ms != null) { rtt.push(ms); if (rtt.length > 40) rtt.shift(); }
  lastPulse = { ok: state === "ok", state, checks, failing, ms, history: [...rtt], ready };
  pulse.emit("tick", lastPulse);
}
export function startPulse() {
  clearInterval(pulseTimer);
  pulseOnce();
  pulseTimer = setInterval(pulseOnce, 5000);
}
export const pulseNow = () => pulseOnce();
