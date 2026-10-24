// The password policy, shown on the sign-up form and enforced by the demo engine.
// internal/user/password_policy.go enforces the same rules on the real API; keep the two in step.
export const MIN_LENGTH = 15;
export const MAX_LENGTH = 128;

const local = email => (email || "").split("@")[0].trim().toLowerCase();
const chars = s => [...s];

/** Each rule: what it asks of the person, and whether this password meets it. */
export const RULES = [
  { id: "length", label: `${MIN_LENGTH} to ${MAX_LENGTH} characters`, ok: pw => chars(pw).length >= MIN_LENGTH && chars(pw).length <= MAX_LENGTH },
  { id: "repeat", label: "Not one repeated character", ok: pw => chars(pw).length === 0 || new Set(chars(pw)).size > 1 },
  { id: "email", label: "Does not contain your email name", ok: (pw, email) => local(email).length < 4 || !pw.toLowerCase().includes(local(email)) },
];

/** { ok, results: [{ id, label, ok }], message } where message is the first problem, for the server-style error. */
export function checkPassword(pw = "", email = "") {
  const results = RULES.map(r => ({ id: r.id, label: r.label, ok: r.ok(pw, email) }));
  const bad = results.find(r => !r.ok);
  const message = !bad ? "" : bad.id === "length" ? (chars(pw).length > MAX_LENGTH ? `must be at most ${MAX_LENGTH} characters` : `must be at least ${MIN_LENGTH} characters`) : bad.id === "repeat" ? "must not be one repeated character" : "must not contain your email name";
  return { ok: !bad, results, message };
}
