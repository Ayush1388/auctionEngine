// Deployment settings: the one place that says where the real API lives.
//
// api: the base URL of the real API.
//   VITE_API_URL overrides it at build time (Vercel: Project Settings, Environment Variables).
//   A production build (npm run build) defaults to the hosted showcase API on Render.
//   Local development (npm run dev) leaves it empty, so the site looks for localhost:4000
//   and otherwise runs on the built-in demo engine, with no backend at all.
//   A visitor can still override it with ?api=... once.
// The real API must list this site's origin in CORS_ALLOWED_ORIGINS.
const HOSTED_API = "https://auctionengine-api.onrender.com";
const env = (typeof import.meta !== "undefined" && import.meta.env) || {};

export const RUNTIME = {
  api: env.VITE_API_URL || (env.PROD ? HOSTED_API : ""),
};
