// Deployment settings: the one file to edit when the site is hosted.
//
// api: the base URL of the real API (for example "https://api.example.com").
//   Empty means the hosted site runs on the built-in demo engine, with no
//   backend at all. A visitor can still override it with ?api=... once.
// The real API must list this site's origin in CORS_ALLOWED_ORIGINS.
export const RUNTIME = {
  api: "",
};
