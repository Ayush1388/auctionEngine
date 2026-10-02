import { Link } from "react-router";
import type { Auction } from "../../api/types";
import { LiveLot } from "../lot/LiveLot";
import { LotCard } from "../lot/LotCard";

/** The lots that end first. Live: bids and clocks update without a reload. */
export function EndingSoonList({ auctions, since }: { auctions: Auction[]; since: number }) {
  return (
    <section aria-labelledby="ending-soon-heading" className="flex h-full flex-col">
      <div className="flex items-baseline justify-between gap-4 border-b border-line-strong pb-2">
        <h2 id="ending-soon-heading" className="heading text-28 text-brand">
          Ending soon
        </h2>
        <Link to="/auctions?status=live&sort=ending" className="meta -my-2 inline-flex min-h-11 items-center text-14 text-ink underline-offset-4 hover:underline">
          See all
        </Link>
      </div>
      <ul className="divide-y divide-line [&>li:nth-child(n+4)]:hidden lg:[&>li:nth-child(n+4)]:block">
        {auctions.map((auction) => (
          <li key={auction.id}>
            <LiveLot auction={auction} since={since}>
              {(lot) => <LotCard lot={lot} layout="list" />}
            </LiveLot>
          </li>
        ))}
      </ul>
    </section>
  );
}
