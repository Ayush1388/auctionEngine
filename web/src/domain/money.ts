const CURRENCY = import.meta.env.VITE_CURRENCY ?? "INR";
/** Locale for amounts and dates: Indian grouping and day-first dates for INR, otherwise the browser's. */
export const LOCALE = CURRENCY === "INR" ? "en-IN" : undefined;

// Minor units per major unit for the configured currency (100 for INR).
const fractionDigits = new Intl.NumberFormat(LOCALE, { style: "currency", currency: CURRENCY }).resolvedOptions()
  .maximumFractionDigits ?? 2;
const MINOR = 10 ** fractionDigits;

const whole = new Intl.NumberFormat(LOCALE, { style: "currency", currency: CURRENCY, maximumFractionDigits: 0 });
const exact = new Intl.NumberFormat(LOCALE, { style: "currency", currency: CURRENCY });

/**
 * Formats an API amount (an integer in the smallest currency unit).
 * Whole amounts drop the decimals: 420000000 → ₹42,00,000.
 */
export function formatMoney(minorUnits: number): string {
  if (minorUnits % MINOR === 0) return whole.format(minorUnits / MINOR);
  return exact.format(minorUnits / MINOR);
}
