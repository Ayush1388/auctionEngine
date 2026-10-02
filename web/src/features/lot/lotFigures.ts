import { type Lot, type Phase, isUrgent } from "../../domain/lot";
import { formatMoney } from "../../domain/money";
import { describeDuration, formatCountdownCompact, formatDay, formatDayTime } from "../../domain/time";

/** What a card or list row says about money and time in each phase (plan.md 4.3). */
export interface LotFigures {
  leftLabel: string;
  leftFigure: string;
  leftMuted: boolean;
  rightLabel: string;
  rightFigure: string;
  /** Shorter forms for the compact list row. */
  listLeftLabel: string;
  listRightFigure: string;
  /** Screen-reader wording for the right figure when digits alone are unclear. */
  rightSpoken: string;
  urgent: boolean;
  /** Tag on the photo; null for a plain live lot. */
  tag: string | null;
}

const bids = (count: number) => (count === 0 ? "No bids yet" : `${count} ${count === 1 ? "bid" : "bids"}`);

export function lotFigures(lot: Lot, phase: Phase, msLeft: number): LotFigures {
  const full = fullFigures(lot, phase, msLeft);
  return {
    ...full,
    // The row is narrow: "Starts 5 Oct" already says the lot has not opened.
    listLeftLabel: phase === "sold" ? "Sold" : phase === "scheduled" ? "" : full.leftLabel,
    listRightFigure: phase === "scheduled" ? `Starts ${formatDay(new Date(lot.startsAt))}` : full.rightFigure,
  };
}

function fullFigures(lot: Lot, phase: Phase, msLeft: number): Omit<LotFigures, "listLeftLabel" | "listRightFigure"> {
  const price = formatMoney(lot.currentBid ?? lot.startingPrice);
  const hasBids = lot.bidCount > 0 && lot.currentBid !== null;
  const liveLeft = {
    leftLabel: hasBids ? bids(lot.bidCount) : "Starting bid",
    leftFigure: price,
    leftMuted: false,
  };

  switch (phase) {
    case "scheduled":
      return {
        leftLabel: "Starting bid",
        leftFigure: formatMoney(lot.startingPrice),
        leftMuted: false,
        rightLabel: "Starts",
        rightFigure: formatDayTime(new Date(lot.startsAt)),
        rightSpoken: formatDayTime(new Date(lot.startsAt)),
        urgent: false,
        tag: "Starting soon",
      };
    case "live":
    case "endingSoon":
    case "finalMinutes":
      return {
        ...liveLeft,
        rightLabel: "Time left",
        rightFigure: formatCountdownCompact(msLeft),
        rightSpoken: describeDuration(msLeft),
        urgent: isUrgent(phase),
        tag: phase === "endingSoon" ? "Ending soon" : phase === "finalMinutes" ? "Final minutes" : null,
      };
    case "closing":
      return {
        ...liveLeft,
        rightLabel: "Time left",
        rightFigure: "Closing",
        rightSpoken: "Closing",
        urgent: false,
        tag: "Closing",
      };
    case "sold":
      return {
        leftLabel: `Sold, ${bids(lot.bidCount)}`,
        leftFigure: price,
        leftMuted: false,
        rightLabel: "Ended",
        rightFigure: formatDay(new Date(lot.endsAt)),
        rightSpoken: formatDay(new Date(lot.endsAt)),
        urgent: false,
        tag: "Sold",
      };
    case "unsold":
      return {
        leftLabel: "No bids",
        leftFigure: formatMoney(lot.startingPrice),
        leftMuted: true,
        rightLabel: "Ended",
        rightFigure: formatDay(new Date(lot.endsAt)),
        rightSpoken: formatDay(new Date(lot.endsAt)),
        urgent: false,
        tag: "Ended",
      };
    case "cancelled":
      return {
        leftLabel: "Starting bid",
        leftFigure: formatMoney(lot.startingPrice),
        leftMuted: true,
        rightLabel: "Status",
        rightFigure: "Cancelled",
        rightSpoken: "Cancelled",
        urgent: false,
        tag: "Cancelled",
      };
  }
}
