// One import gives a page "the backend": the real API when it answers, the demo
// engine when it does not. The choice is made once per page load.
//
//   const be = await getBackend();
//   be.kind   "live" | "sim"
//   be.fellBack  set when the real API was tried and did not answer
import { config } from "./config.js";
import { createLive } from "./live.js";
import { createSim } from "./sim.js";

const withTimeout = (p, ms) => Promise.race([p, new Promise((_, rej) => setTimeout(() => rej(new Error("timeout")), ms))]);

let backend;
export function getBackend() {
  return backend ||= (async () => {
    const wantSim = config.engine === "sim" || (config.engine === "auto" && !config.api);
    if (!wantSim) {
      const live = createLive();
      try { await withTimeout(live.ready(), 1500); return live; }
      catch (e) {
        if (config.engine === "live") { live.unreachable = e; return live; }       // asked for the real one: let pages show the error
        live.feed.stop();
        const sim = createSim();
        sim.fellBack = { api: config.api, reason: e.message };
        await sim.ready();
        return sim;
      }
    }
    const sim = createSim();
    await sim.ready();
    return sim;
  })();
}
