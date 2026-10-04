// The demo accounts. The demo engine creates them on first run; the real backend
// has them after `go run ./cmd/demobots seed`. Same emails and password in both.
export const DEMO_PASSWORD = "marque-demo-password";
export const DEMO_ACCOUNTS = [
  { email: "demo@marque.test", name: "Demo bidder", role: "user" },
  { email: "collector@marque.test", name: "Collector", role: "user" },
  { email: "admin@marque.test", name: "Operator", role: "admin" },
];
