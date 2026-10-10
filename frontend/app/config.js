// Where the app finds its backend. Query parameters are sticky: set once, remembered.
//
//   ?engine=sim            always use the in-browser demo engine
//   ?engine=live           always use the real API (error if unreachable)
//   ?engine=auto           forget the choice and detect again
//   ?api=http://host:4000  real API base URL
//   ?jaeger=http://host:16686  link bid traces to Jaeger
import { local } from "./util.js";
import { RUNTIME } from "./runtime.js";

const q = new URLSearchParams(location.search);
const remember = (param, key) => {
  const v = q.get(param);
  if (v === null) return;
  if (v === "" || v === "auto") local.del(key); else local.set(key, v);
};
remember("engine", "marque-engine");
remember("api", "marque-api");
remember("jaeger", "marque-jaeger");

const isLocal = ["localhost", "127.0.0.1", "[::1]"].includes(location.hostname) || location.protocol === "file:";

export const config = {
  /** "auto" | "sim" | "live" */
  engine: local.get("marque-engine", "auto"),
  /** Base URL of the real API. Empty when there is nothing to try (hosted demo). */
  api: (local.get("marque-api") || RUNTIME.api || (isLocal ? "http://localhost:4000" : "")).replace(/\/$/, ""),
  jaeger: (local.get("marque-jaeger") || "").replace(/\/$/, ""),
};

export const wsUrl = base => base.replace(/^http/, "ws") + "/v1/ws";
