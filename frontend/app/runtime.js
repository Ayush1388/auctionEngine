// Deployment settings: the one place that says where the real API lives.
//
// api: the base URL of the real API (for example "https://marque-api.onrender.com").
//   On Vercel set the VITE_API_URL environment variable and redeploy; no code edit is needed.
//   Empty means the hosted site runs on the built-in demo engine, with no backend at all.
//   A visitor can still override it with ?api=... once.
// The real API must list this site's origin in CORS_ALLOWED_ORIGINS.
export const RUNTIME = {
  api: (typeof import.meta !== "undefined" && import.meta.env && import.meta.env.VITE_API_URL) || "",
};
