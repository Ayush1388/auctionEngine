// The site's motion system (GSAP + ScrollTrigger). Transform and opacity only, every effect answers
// something (a page opening, a section arriving), and nothing runs under prefers-reduced-motion.
// Each function is meant to be called inside a gsap.context() so React can revert it on unmount.
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";

gsap.registerPlugin(ScrollTrigger);

export const reduced = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
const small = () => innerWidth < 700;
const d = n => (small() ? n * 0.5 : n);               // shorter travel on phones
const EASE = "power3.out", SLOW = "power2.out";
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const has = (s, r = document) => r.querySelector(s);

/** The home page opening shot: photo settles, then the panel assembles. */
export function homeIntro(root) {
  if (reduced() || !has(".stage-img", root)) return;
  gsap.from(".stage-img", { scale: 1.08, duration: 2.6, ease: SLOW, clearProps: "transform" });
  gsap.timeline({ defaults: { ease: EASE, duration: 0.8 }, delay: 0.15 })
    .from(".topbar", { autoAlpha: 0, duration: 0.6 })
    .from(".awards li", { autoAlpha: 0, y: d(10), stagger: 0.08 }, "-=0.2")
    .from(".wordmark", { autoAlpha: 0, y: d(28), duration: 1 }, "-=0.5")
    .from(".controls > *", { autoAlpha: 0, y: d(8), stagger: 0.06 }, "-=0.5")
    .from(".thumbs li", { autoAlpha: 0, x: d(-12), stagger: 0.07 }, "-=0.4")
    .from(".stage-info > *", { autoAlpha: 0, y: d(14), stagger: 0.1 }, "-=0.5");
}

/** The event hero: eyebrow, heading, dates, intro, CTAs. */
export function eventIntro(root) {
  if (reduced() || !has(".ev-hero-img", root)) return;
  gsap.from(".ev-hero-img", { scale: 1.08, duration: 2.6, ease: SLOW, clearProps: "transform" });
  gsap.timeline({ defaults: { ease: EASE, duration: 0.8 }, delay: 0.2 })
    .from(".ev-bar", { autoAlpha: 0, duration: 0.6 })
    .from(".ev-back", { autoAlpha: 0, y: d(8) }, "-=0.2")
    .from(".ev-kicker", { autoAlpha: 0, y: d(10) }, "-=0.4")
    .from(".ev-title", { autoAlpha: 0, y: d(24), duration: 1 }, "-=0.45")
    .from(".ev-dates", { autoAlpha: 0, y: d(12) }, "-=0.5")
    .from(".ev-intro", { autoAlpha: 0, y: d(12) }, "-=0.55")
    .from(".ev-cta .btn", { autoAlpha: 0, y: d(10), stagger: 0.08 }, "-=0.5");
}

/** Category bands: Classic is slow and editorial, the muscle band is a little faster and horizontal. */
export function bands(root) {
  if (reduced()) return;
  const band = (sel, photoFrom, copyFrom, dur) => {
    const b = has(sel, root); if (!b) return;
    const ph = b.querySelector(".band-photo"), cp = b.querySelector(".band-copy");
    gsap.timeline({ scrollTrigger: { trigger: b, start: "top 78%", once: true }, defaults: { ease: SLOW } })
      .from(ph, { autoAlpha: 0, x: d(photoFrom), duration: dur })
      .from(cp.children, { autoAlpha: 0, x: d(copyFrom), duration: dur * 0.8, stagger: 0.12 }, "-=" + dur * 0.7);
    if (innerWidth > 1000) gsap.fromTo(ph.querySelector("img"), { scale: 1.1 }, { scale: 1, ease: "none", scrollTrigger: { trigger: b, start: "top bottom", end: "bottom top", scrub: true } });
  };
  band("#classic", -60, 50, 1.4);   // photo from the left, text from the opposite side, slow
  band("#muscle", 48, -36, 0.9);    // photo from the right, text from the left, quicker
}

/** Cars on the block: image first, then title, then auction info; subtle stagger per batch. */
export function cards(root) {
  if (reduced()) return;
  const list = $$(".card", root);
  if (!list.length) return;
  list.forEach(c => {
    gsap.set(c.querySelector(".card-media img"), { autoAlpha: 0, yPercent: 5 });
    gsap.set($$(".card-body > *", c), { autoAlpha: 0, y: d(10) });
  });
  ScrollTrigger.batch(list, {
    start: "top 90%", once: true,
    onEnter: batch => batch.forEach((c, i) => {
      gsap.timeline({ delay: i * 0.1, defaults: { ease: EASE } })
        .to(c.querySelector(".card-media img"), { autoAlpha: 1, yPercent: 0, duration: 0.9, clearProps: "transform" })
        .to(c.querySelector("h3"), { autoAlpha: 1, y: 0, duration: 0.6 }, "-=0.55")
        .to($$(".card-body > :not(h3)", c), { autoAlpha: 1, y: 0, duration: 0.6, stagger: 0.06 }, "-=0.4");
    }),
  });
}

/** Upcoming auctions: rows enter once, no repeated animation. */
export function eventRows(root) {
  if (reduced()) return;
  const ev = $$(".event-list > li", root);
  if (!ev.length) return;
  gsap.set(ev, { autoAlpha: 0, y: d(16) });
  ScrollTrigger.batch(ev, { start: "top 90%", once: true, onEnter: b => gsap.to(b, { autoAlpha: 1, y: 0, duration: 0.8, ease: EASE, stagger: 0.12, clearProps: "transform" }) });
}

export { gsap, ScrollTrigger };
