import type { ReactNode } from "react";
import type { Auction } from "../../api/types";
import { type Lot, toLot } from "../../domain/lot";
import { useLiveSnapshot } from "../../realtime/liveStore";

/**
 * Builds the Lot view model for an auction, overlaid with the latest live
 * snapshot that arrived after the REST data was fetched (`since`).
 */
export function LiveLot({ auction, since, children }: { auction: Auction; since: number; children: (lot: Lot) => ReactNode }) {
  const snapshot = useLiveSnapshot(auction.id, since);
  return <>{children(toLot(auction, snapshot))}</>;
}
