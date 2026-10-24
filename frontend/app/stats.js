// Small numeric helpers for the charts and the status page.

/** A tidy upper bound for an axis: 1, 2, 5 times a power of ten. */
export function niceMax(v) {
  if (!(v > 0)) return 1;
  const p = 10 ** Math.floor(Math.log10(v)), n = v / p;
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}

/**
 * The q-quantile (0..1) of a histogram from cumulative buckets [{le, count}] and the
 * total observation count, in the buckets' own unit. Observations above the last
 * bucket report that bucket's bound.
 */
export function quantile(buckets, total, q) {
  if (!buckets.length || !total) return 0;
  const rank = q * total;
  let prevLe = 0, prevCount = 0;
  for (const b of buckets) {
    if (b.count >= rank) {
      const span = b.count - prevCount;
      return span <= 0 ? b.le : prevLe + (b.le - prevLe) * ((rank - prevCount) / span);
    }
    prevLe = b.le; prevCount = b.count;
  }
  return prevLe;
}
