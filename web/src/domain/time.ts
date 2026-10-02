import { LOCALE } from "./money";

export const HOUR = 60 * 60 * 1000;
export const MINUTE = 60 * 1000;
/** The backend's anti-sniping window (bidding.SnipeWindow). */
export const SNIPE_WINDOW = 2 * MINUTE;

const pad = (n: number) => String(n).padStart(2, "0");

/**
 * Countdown text (plan.md 4.5): over 24 hours "2d 04h 12m"; under 24 hours
 * "04h 12m 08s"; under 1 hour "12:08".
 */
export function formatCountdown(msLeft: number): string {
  const total = Math.max(0, Math.floor(msLeft / 1000));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;

  if (days > 0) return `${days}d ${pad(hours)}h ${pad(minutes)}m`;
  if (hours > 0) return `${pad(hours)}h ${pad(minutes)}m ${pad(seconds)}s`;
  return `${pad(minutes)}:${pad(seconds)}`;
}

/** Compact countdown for cards and lists: "2d 4h", "4h 12m", "42m 10s", "01:47". */
export function formatCountdownCompact(msLeft: number): string {
  const total = Math.max(0, Math.floor(msLeft / 1000));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;

  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${pad(minutes)}m`;
  if (total > 120) return `${minutes}m ${pad(seconds)}s`;
  return `${pad(minutes)}:${pad(seconds)}`;
}

/** Words for screen readers: "2 days 4 hours", "42 minutes", "under a minute". */
export function describeDuration(msLeft: number): string {
  const total = Math.max(0, Math.floor(msLeft / 1000));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const unit = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

  if (days > 0) return hours > 0 ? `${unit(days, "day")} ${unit(hours, "hour")}` : unit(days, "day");
  if (hours > 0) return minutes > 0 ? `${unit(hours, "hour")} ${unit(minutes, "minute")}` : unit(hours, "hour");
  if (minutes > 0) return unit(minutes, "minute");
  return "under a minute";
}

const dayMonth = new Intl.DateTimeFormat(LOCALE, { day: "numeric", month: "short" });
const dayMonthTime = new Intl.DateTimeFormat(LOCALE, {
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

/** "3 Oct" */
export const formatDay = (date: Date) => dayMonth.format(date);
/** "4 Oct, 18:00" in the viewer's time zone */
export const formatDayTime = (date: Date) => dayMonthTime.format(date);
