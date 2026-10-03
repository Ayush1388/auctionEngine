// k6 load test: an OPEN model at a fixed arrival rate (v1.0).
//
// cmd/loadgen is a closed loop: each worker waits for its previous
// response, so when the server slows down, the load drops with it and
// latency looks better than users would see ("coordinated omission").
// Here k6 starts new requests at a fixed RATE whether or not earlier ones
// have finished. That is how real traffic arrives, and it is the right way
// to check an SLO such as "p99 < 300 ms at 300 req/s".
//
//   # 1. seed users and auctions, and log the users in
//   go run ./cmd/loadgen -seed-only loadtest/k6/seed.json
//   # 2. run (rate limits off on the API: all virtual users share one IP)
//   k6 run -e BASE=http://localhost:4000 -e RATE=300 loadtest/k6/auction.js
//
// The thresholds below make k6 exit non-zero when the SLO is missed, so
// this can gate a release in CI.

import http from 'k6/http';
import { check } from 'k6';
import { SharedArray } from 'k6/data';

const BASE = __ENV.BASE || 'http://localhost:4000';
const RATE = Number(__ENV.RATE || 300);           // requests per second
const DURATION = __ENV.DURATION || '1m';
const HOT = Number(__ENV.HOT || 0.5);              // share of traffic on the hottest auction

const seed = new SharedArray('seed', () => [JSON.parse(open('./seed.json'))])[0];

export const options = {
  scenarios: {
    traffic: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: 100,
      maxVUs: 1000,          // k6 adds VUs to keep the rate when responses slow down
    },
  },
  thresholds: {
    // Service-level objectives, per operation.
    'http_req_duration{op:bid}':    ['p(99)<300'],
    'http_req_duration{op:read}':   ['p(99)<100'],
    'http_req_duration{op:search}': ['p(99)<300'],
    // 5xx are failures. 422 (bid too low) and 503 from load shedding are
    // expected outcomes, so they are counted separately.
    'checks{check:no_5xx_except_shed}': ['rate>0.999'],
    'dropped_iterations': ['count<1'],   // k6 couldn't keep the rate: generator saturated
  },
};

function pickAuction() {
  if (Math.random() < HOT) return seed.auctions[0];
  return seed.auctions[Math.floor(Math.random() * seed.auctions.length)];
}

let high = 1000; // per-VU guess of the current price; too-low bids are expected

export default function () {
  const id = pickAuction();
  const roll = Math.random();
  let res;

  if (roll < 0.6) {
    high += 1 + Math.floor(Math.random() * 5);
    const token = seed.tokens[Math.floor(Math.random() * seed.tokens.length)];
    res = http.post(`${BASE}/v1/auctions/${id}/bids`, JSON.stringify({ amount: high }), {
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      tags: { op: 'bid' },
    });
    check(res, { 'bid accepted or too low': (r) => [201, 422, 503].includes(r.status) });
  } else if (roll < 0.9) {
    res = http.get(`${BASE}/v1/auctions/${id}`, { tags: { op: 'read' } });
  } else {
    const q = ['camera', 'vintage', 'watch', 'guitar', 'camra'][Math.floor(Math.random() * 5)];
    res = http.get(`${BASE}/v1/auctions/search?q=${q}`, { tags: { op: 'search' } });
  }

  check(res, { no_5xx_except_shed: (r) => r.status < 500 || r.status === 503 });
}
