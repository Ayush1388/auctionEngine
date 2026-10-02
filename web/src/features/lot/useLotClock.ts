import { type Lot, type Phase, phaseOf } from "../../domain/lot";
import { HOUR, MINUTE } from "../../domain/time";
import { useNow } from "../../realtime/ticker";

/**
 * The lot's phase and the time left, re-evaluated on the shared ticker:
 * every second in the last hour, once a minute before that.
 */
export function useLotClock(lot: Lot): { phase: Phase; msLeft: number; now: number } {
  const active = lot.status === "ACTIVE";
  const fine = active && lot.endsAt - Date.now() < HOUR + MINUTE;
  const now = useNow(fine ? 1000 : 60_000);
  // With minute granularity "now" is rounded down; use the real clock for the phase.
  const clock = fine ? now : Math.max(now, Date.now() - (Date.now() % 1000));
  return { phase: phaseOf(lot, clock), msLeft: lot.endsAt - clock, now: clock };
}
