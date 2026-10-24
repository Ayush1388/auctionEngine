// Which Kafka partition an auction's bids and events land on.
//
// The producer uses franz-go's StickyKeyPartitioner with the default hasher,
// which is Kafka's own: murmur2 of the key bytes, made positive, modulo the
// partition count. The key is the auction ID as text. Because one auction
// always maps to one partition, and one consumer owns a partition, every bid
// for a lot is processed in order, while different lots run in parallel.
export const PARTITIONS = 12;

/** Kafka's murmur2 (org.apache.kafka.common.utils.Utils.murmur2). */
export function murmur2(bytes) {
  const m = 0x5bd1e995, r = 24;
  let h = (0x9747b28c ^ bytes.length) >>> 0;
  const len4 = bytes.length >> 2;
  const imul = Math.imul;
  for (let i = 0; i < len4; i++) {
    const j = i << 2;
    let k = (bytes[j] | (bytes[j + 1] << 8) | (bytes[j + 2] << 16) | (bytes[j + 3] << 24)) >>> 0;
    k = imul(k, m) >>> 0; k = (k ^ (k >>> r)) >>> 0; k = imul(k, m) >>> 0;
    h = imul(h, m) >>> 0; h = (h ^ k) >>> 0;
  }
  const rem = bytes.length & 3, base = len4 << 2;
  if (rem === 3) h = (h ^ (bytes[base + 2] << 16)) >>> 0;
  if (rem >= 2) h = (h ^ (bytes[base + 1] << 8)) >>> 0;
  if (rem >= 1) { h = (h ^ bytes[base]) >>> 0; h = imul(h, m) >>> 0; }
  h = (h ^ (h >>> 13)) >>> 0; h = imul(h, m) >>> 0; h = (h ^ (h >>> 15)) >>> 0;
  return h | 0;
}

export function partitionOf(auctionId, partitions = PARTITIONS) {
  const bytes = new TextEncoder().encode(String(auctionId));
  return (murmur2(bytes) & 0x7fffffff) % partitions;
}
