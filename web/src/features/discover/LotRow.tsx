import type { ReactNode } from "react";
import { Link } from "react-router";
import type { Auction } from "../../api/types";
import { toLot } from "../../domain/lot";
import { InlineError } from "../../ui/InlineError";
import { Skeleton } from "../../ui/Skeleton";
import { useIsTabletUp } from "../../ui/useMediaQuery";
import { LotCard } from "../lot/LotCard";

interface LotRowProps {
  id: string;
  title: string;
  viewAll: { to: string; label: string };
  auctions: Auction[] | undefined;
  isError: boolean;
  onRetry: () => void;
  /** Ids in the trending list, for the activity marker on cards. */
  activeIds?: ReadonlySet<string>;
}

// Four cards on wide screens, three at 1024, two rows of two on tablets.
const GRID = "grid gap-4 md:grid-cols-2 md:gap-6 lg:grid-cols-3 xl:grid-cols-4 lg:max-xl:[&>li:nth-child(n+4)]:hidden";
const ROW_SIZES = "(min-width: 90rem) 312px, (min-width: 64rem) 31vw, (min-width: 48rem) 47vw, 112px";

/** A titled row of up to four lots. Omitted entirely when it has nothing to show. */
export function LotRow({ id, title, viewAll, auctions, isError, onRetry, activeIds }: LotRowProps) {
  const isTabletUp = useIsTabletUp();
  if (auctions && auctions.length === 0) return null;

  let content: ReactNode;
  if (auctions) {
    content = isTabletUp ? (
      <ul className={GRID}>
        {auctions.slice(0, 4).map((auction) => (
          <li key={auction.id}>
            <LotCard lot={toLot(auction)} layout="grid" active={activeIds?.has(auction.id)} sizes={ROW_SIZES} />
          </li>
        ))}
      </ul>
    ) : (
      <ul className="divide-y divide-line">
        {auctions.slice(0, 4).map((auction) => (
          <li key={auction.id}>
            <LotCard lot={toLot(auction)} layout="list" />
          </li>
        ))}
      </ul>
    );
  } else if (isError) {
    content = <InlineError message="Could not load auctions." onRetry={onRetry} />;
  } else {
    content = <LotRowSkeleton />;
  }

  return (
    <section aria-labelledby={id}>
      <div className="mb-4 flex items-center justify-between gap-4 md:mb-6">
        <h2 id={id} className="heading text-36 text-brand md:text-44">
          {title}
        </h2>
        <Link to={viewAll.to} className="btn btn-line btn-sm shrink-0">
          {viewAll.label}
        </Link>
      </div>
      {content}
    </section>
  );
}

function LotRowSkeleton() {
  return (
    <div aria-busy="true" aria-label="Loading auctions">
      <ul className={`hidden md:grid ${GRID.replace("grid ", "")}`}>
        {[0, 1, 2, 3].map((index) => (
          <li key={index} className="overflow-hidden rounded-card border border-[var(--card-outline)] bg-surface">
            <Skeleton className="aspect-[3/2] w-full rounded-none" />
            <div className="space-y-3 px-4 pb-4 pt-3">
              <Skeleton className="h-3 w-1/3" />
              <Skeleton className="h-5 w-4/5" />
              <Skeleton className="h-20 w-full" />
            </div>
            <Skeleton className="h-[68px] w-full rounded-none" />
          </li>
        ))}
      </ul>
      <ul className="divide-y divide-line md:hidden">
        {[0, 1, 2, 3].map((index) => (
          <li key={index} className="flex gap-3 py-3">
            <Skeleton className="h-[75px] w-28 shrink-0" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-4 w-4/5" />
              <Skeleton className="h-4 w-1/2" />
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
