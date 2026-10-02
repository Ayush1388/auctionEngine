import type { Auction, AuctionSnapshot } from "../api/types";
import { cardSpecs, parseDescription } from "./description";
import { HOUR, SNIPE_WINDOW } from "./time";

export type Era = "classic" | "modern";

export type Phase =
  | "scheduled"
  | "live"
  | "endingSoon"
  | "finalMinutes"
  | "closing"
  | "sold"
  | "unsold"
  | "cancelled";

/** The view model the UI works with (plan.md 3.3), derived from an Auction. */
export interface Lot {
  id: string;
  shortRef: string;
  title: string;
  era: Era;
  year: string | null;
  /** Title without the leading year, for the photo placeholder. */
  makeModel: string;
  location: string | null;
  cardSpecs: Array<[label: string, value: string]>;
  photoSlug: string | null;
  status: Auction["status"];
  startingPrice: number;
  currentBid: number | null;
  bidCount: number;
  startsAt: number;
  endsAt: number;
  extensions: number;
}

const YEAR = /^\s*((?:18|19|20)\d{2})\b\s*/;

export function toLot(auction: Auction, snapshot?: AuctionSnapshot): Lot {
  const { specs } = parseDescription(auction.item.description ?? "");
  const title = auction.item.name;
  const yearMatch = YEAR.exec(title);
  const year = yearMatch?.[1] ?? specs.get("Year") ?? null;

  // The live snapshot carries the fields that change while an auction runs.
  const live = snapshot ?? auction;

  return {
    id: auction.id,
    shortRef: auction.id.slice(0, 8),
    title,
    era: deriveEra(auction.item.type, year),
    year,
    makeModel: yearMatch ? title.slice(yearMatch[0].length) : title,
    location: specs.get("Location") ?? null,
    cardSpecs: cardSpecs(specs),
    photoSlug: specs.get("Photos") ?? null,
    status: live.status,
    startingPrice: auction.starting_price,
    currentBid: live.current_bid,
    bidCount: live.bid_count,
    startsAt: Date.parse(auction.starts_at),
    endsAt: Date.parse(live.ends_at),
    extensions: live.extensions,
  };
}

/** item.type is "classic" or "modern" by convention; fall back to the year. */
function deriveEra(type: string, year: string | null): Era {
  const normalised = type.trim().toLowerCase();
  if (normalised === "classic") return "classic";
  if (normalised === "modern") return "modern";
  return year !== null && Number(year) < 1990 ? "classic" : "modern";
}

/** Phase rules from plan.md 3.3. `now` is the client clock in milliseconds. */
export function phaseOf(lot: Pick<Lot, "status" | "endsAt" | "currentBid" | "bidCount">, now: number): Phase {
  switch (lot.status) {
    case "NOT_ACTIVE":
      return "scheduled";
    case "CANCELLED":
      return "cancelled";
    case "COMPLETED":
      return lot.bidCount > 0 && lot.currentBid !== null ? "sold" : "unsold";
    case "ACTIVE": {
      const left = lot.endsAt - now;
      if (left <= 0) return "closing"; // bids are already refused; status flips within about a second
      if (left <= SNIPE_WINDOW) return "finalMinutes";
      if (left <= HOUR) return "endingSoon";
      return "live";
    }
  }
}

export const isUrgent = (phase: Phase) => phase === "endingSoon" || phase === "finalMinutes";
