// Shared motion system (GSAP + ScrollTrigger). Transform/opacity only; content stays visible if this file fails.
(function () {
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const small = innerWidth < 700;
  const d = n => (small ? n * 0.5 : n);               // shorter travel on phones
  const EASE = "power3.out", SLOW = "power2.out";
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const has = s => document.querySelector(s);

  /* Bid changes: roll the number only when state really changes (called from the existing render code). */
  window.tickBid = function (el, value, fmt) {
    const parsed = parseInt((el.textContent || "").replace(/\D/g, ""), 10);
    const prev = el._v !== undefined ? el._v : (isNaN(parsed) ? undefined : parsed);
    el._v = value;
    if (prev === undefined || prev === value || reduce || !window.gsap) { el.textContent = fmt(value); return; }
    gsap.killTweensOf(el._o);
    const o = el._o = { v: prev };
    gsap.to(o, { v: value, duration: 0.7, ease: "power2.out", onUpdate: () => { el.textContent = fmt(Math.round(o.v)); }, onComplete: () => { el.textContent = fmt(value); } });
    gsap.fromTo(el, { y: -6 }, { y: 0, duration: 0.5, ease: EASE });
  };

  if (reduce || !window.gsap) return;
  gsap.registerPlugin(ScrollTrigger);

  /* Page enter: opacity only (a transformed ancestor would break the fixed drawer). */
  gsap.from("body", { autoAlpha: 0, duration: 0.45, ease: "power1.out", clearProps: "all" });

  /* 1. Hero: opening-shot reveal on the home page */
  if (has(".stage-img")) {
    gsap.from(".stage-img", { scale: 1.08, duration: 2.6, ease: SLOW, clearProps: "transform" });
    const tl = gsap.timeline({ defaults: { ease: EASE, duration: 0.8 }, delay: 0.15 });
    tl.from(".topbar", { autoAlpha: 0, duration: 0.6 })
      .from(".awards li", { autoAlpha: 0, y: d(10), stagger: 0.08 }, "-=0.2")
      .from(".wordmark", { autoAlpha: 0, y: d(28), duration: 1 }, "-=0.5")
      .from(".controls > *", { autoAlpha: 0, y: d(8), stagger: 0.06 }, "-=0.5")
      .from(".thumbs li", { autoAlpha: 0, x: d(-12), stagger: 0.07 }, "-=0.4")
      .from(".stage-info > *", { autoAlpha: 0, y: d(14), stagger: 0.1 }, "-=0.5");
  }

  /* Event hero: eyebrow, heading, dates, intro, CTAs */
  if (has(".ev-hero-img")) {
    gsap.from(".ev-hero-img", { scale: 1.08, duration: 2.6, ease: SLOW, clearProps: "transform" });
    gsap.timeline({ defaults: { ease: EASE, duration: 0.8 }, delay: 0.2 })
      .from(".ev-bar", { autoAlpha: 0, duration: 0.6 })
      .from(".ev-back", { autoAlpha: 0, y: d(8) }, "-=0.2")
      .from("#evKicker", { autoAlpha: 0, y: d(10) }, "-=0.4")
      .from("#evTitle", { autoAlpha: 0, y: d(24), duration: 1 }, "-=0.45")
      .from("#evDates", { autoAlpha: 0, y: d(12) }, "-=0.5")
      .from("#evIntro", { autoAlpha: 0, y: d(12) }, "-=0.55")
      .from(".ev-cta .btn", { autoAlpha: 0, y: d(10), stagger: 0.08 }, "-=0.5");
  }

  /* Category bands: Classic is slow and editorial, the muscle band is a little faster and horizontal */
  function band(sel, photoFrom, copyFrom, dur) {
    const b = has(sel); if (!b) return;
    const ph = b.querySelector(".band-photo"), cp = b.querySelector(".band-copy");
    const tl = gsap.timeline({ scrollTrigger: { trigger: b, start: "top 78%", once: true }, defaults: { ease: SLOW } });
    tl.from(ph, { autoAlpha: 0, x: d(photoFrom), duration: dur })
      .from(cp.children, { autoAlpha: 0, x: d(copyFrom), duration: dur * 0.8, stagger: 0.12 }, "-=" + dur * 0.7);
    if (innerWidth > 1000) gsap.fromTo(ph.querySelector("img"), { scale: 1.1 }, { scale: 1, ease: "none", scrollTrigger: { trigger: b, start: "top bottom", end: "bottom top", scrub: true } });
  }
  band("#classic", -60, 50, 1.4);   // photo from the left, text from the opposite side, slow
  band("#muscle", 48, -36, 0.9);    // photo from the right, text from the left, quicker

  /* Cars on the block: image first, then title, then auction info; subtle stagger per batch */
  const cards = $$(".card");
  if (cards.length) {
    cards.forEach(c => {
      gsap.set(c.querySelector(".card-media img"), { autoAlpha: 0, yPercent: 5 });
      gsap.set($$(".card-body > *", c), { autoAlpha: 0, y: d(10) });
    });
    ScrollTrigger.batch(cards, {
      start: "top 90%", once: true,
      onEnter: batch => batch.forEach((c, i) => {
        const tl = gsap.timeline({ delay: i * 0.1, defaults: { ease: EASE } });
        tl.to(c.querySelector(".card-media img"), { autoAlpha: 1, yPercent: 0, duration: 0.9, clearProps: "transform" })
          .to(c.querySelector("h3"), { autoAlpha: 1, y: 0, duration: 0.6 }, "-=0.55")
          .to($$(".card-body > :not(h3)", c), { autoAlpha: 1, y: 0, duration: 0.6, stagger: 0.06 }, "-=0.4");
      }),
    });
  }

  /* Upcoming auctions: rows enter once, no repeated animation */
  const ev = $$(".event-list > li");
  if (ev.length) {
    gsap.set(ev, { autoAlpha: 0, y: d(16) });
    ScrollTrigger.batch(ev, { start: "top 90%", once: true, onEnter: b => gsap.to(b, { autoAlpha: 1, y: 0, duration: 0.8, ease: EASE, stagger: 0.12, clearProps: "transform" }) });
  }
})();
