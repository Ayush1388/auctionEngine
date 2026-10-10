// murmur2 must match Kafka's own implementation, or the lane shown for a lot would be wrong.
// Vectors are from org.apache.kafka.common.utils.UtilsTest.
import test from "node:test";
import assert from "node:assert/strict";
import { murmur2, partitionOf, PARTITIONS } from "../app/partition.js";

const enc = s => new TextEncoder().encode(s);
test("murmur2 matches Kafka's test vectors", () => {
  assert.equal(murmur2(enc("21")), -973932308);
  assert.equal(murmur2(enc("foobar")), -790332482);
  assert.equal(murmur2(enc("a-little-bit-long-string")), -985981536);
  assert.equal(murmur2(enc("a-little-bit-longer-string")), -1486304829);
  assert.equal(murmur2(enc("lkjh234lh9fiuh90y23oiuhsafujhadof229phr9h19h89h8")), -58897971);
  assert.equal(murmur2(new Uint8Array([97, 98, 99])), 479470107);
});
test("an auction always maps to the same partition, within range", () => {
  const id = "3f2c9a52-6d6b-4a39-8f0e-1d3f6f0b9a11";
  assert.equal(partitionOf(id), partitionOf(id));
  for (let i = 0; i < 100; i++) { const p = partitionOf(crypto.randomUUID()); assert.ok(p >= 0 && p < PARTITIONS); }
});
