import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useLiveAuctions } from "../../realtime/socket";
import { InlineError } from "../../ui/InlineError";
import { Skeleton } from "../../ui/Skeleton";
import { CategoryTiles } from "./CategoryTiles";
import { EndingSoonList } from "./EndingSoonList";
import { FeaturedLot } from "./FeaturedLot";
import { LotRow } from "./LotRow";
import { useDiscoverData } from "./useDiscoverData";

/** Home: live lots first, no marketing hero (plan.md 4.1). */
export function DiscoverPage() {
  const { trending, active, scheduled, sold } = useDiscoverData();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const activeLots = active.data?.auctions;
  const trendingLots = trending.data?.auctions;

  // Featured: the three most active lots; fall back to the newest live lots
  // when nothing is trending or the trending request failed.
  const featured = useMemo(() => {
    if (trendingLots && trendingLots.length > 0) return trendingLots.slice(0, 3);
    if (activeLots && (trendingLots || trending.isError)) return activeLots.slice(0, 3);
    return undefined;
  }, [trendingLots, activeLots, trending.isError]);
  const featuredSince = trendingLots && trendingLots.length > 0 ? trending.dataUpdatedAt : active.dataUpdatedAt;

  // Ending soon: sorted on the client until the API can sort by end time.
  const endingSoon = useMemo(() => {
    if (!activeLots) return undefined;
    return [...activeLots].sort((a, b) => Date.parse(a.ends_at) - Date.parse(b.ends_at)).slice(0, 5);
  }, [activeLots]);

  const activeIds = useMemo(() => new Set(trendingLots?.map((auction) => auction.id)), [trendingLots]);

  // The first featured lot is chosen once and then kept, so a refresh that
  // reorders the trending list never swaps the lot under the viewer.
  if (featured && featured.length > 0 && !featured.some((auction) => auction.id === selectedId)) {
    setSelectedId(featured[0].id);
  }
  const selected = featured?.find((auction) => auction.id === selectedId) ?? featured?.[0];

  // Newly listed skips lots already shown in the row above when enough remain.
  const newlyListed = useMemo(() => {
    if (!activeLots) return undefined;
    const shown = new Set(trendingLots?.slice(0, 4).map((auction) => auction.id));
    const fresh = activeLots.filter((auction) => !shown.has(auction.id));
    return fresh.length >= 2 ? fresh : activeLots;
  }, [activeLots, trendingLots]);

  // Live updates for what is on screen in the top block only (at most 6).
  const liveIds = useMemo(() => {
    const ids = new Set<string>();
    if (selected) ids.add(selected.id);
    endingSoon?.forEach((auction) => ids.add(auction.id));
    return [...ids].sort();
  }, [selected, endingSoon]);
  useLiveAuctions(liveIds);

  const topFailed = active.isError && !activeLots && (trending.isError || !trendingLots?.length);
  const nothingLive = activeLots?.length === 0 && (!trendingLots || trendingLots.length === 0);

  const startingSoon = (
    <LotRow
      id="starting-soon-heading"
      title="Starting soon"
      viewAll={{ to: "/auctions?status=starting-soon", label: "View all" }}
      auctions={scheduled.data?.auctions}
      isError={scheduled.isError}
      onRetry={() => void scheduled.refetch()}
    />
  );

  return (
    <div className="mx-auto w-full max-w-[1384px] px-4 pb-16 md:px-6 lg:px-8">
      <h1 className="sr-only">Live collector car auctions</h1>

      {topFailed ? (
        <div className="pt-6">
          <InlineError
            message="Could not load auctions."
            onRetry={() => {
              void active.refetch();
              void trending.refetch();
            }}
          />
        </div>
      ) : nothingLive ? (
        <div className="space-y-12 pt-8">
          <div>
            <h2 className="heading text-28 text-ink">No auctions are live right now</h2>
            <p className="mt-2 max-w-[60ch] text-16 text-ink-2">
              New lots open for bidding at their start time. You can also list a car of your own.
            </p>
            <Link
              to="/sell"
              className="meta mt-4 inline-flex min-h-11 items-center rounded-control bg-action px-5 text-16 text-on-action transition-opacity duration-[120ms] hover:opacity-85"
            >
              Sell a car
            </Link>
          </div>
          {startingSoon}
        </div>
      ) : (
        <div className="grid gap-x-6 gap-y-8 md:pt-6 lg:grid-cols-12">
          <div className="min-w-0 lg:col-span-8">
            {featured && selected ? (
              <FeaturedLot auctions={featured} since={featuredSince} selectedId={selected.id} onSelect={setSelectedId} />
            ) : (
              <FeaturedSkeleton />
            )}
          </div>
          <div className="min-w-0 lg:col-span-4">
            {endingSoon ? (
              endingSoon.length > 0 && <EndingSoonList auctions={endingSoon} since={active.dataUpdatedAt} />
            ) : active.isError ? (
              <InlineError message="Could not load auctions ending soon." onRetry={() => void active.refetch()} />
            ) : (
              <EndingSoonSkeleton />
            )}
          </div>
        </div>
      )}

      <div className="mt-8 border-y border-line py-4 md:mt-10">
        <CategoryTiles />
      </div>

      <div className="mt-8 space-y-8 md:mt-12 md:space-y-12">
        {!nothingLive && (
          <>
            {!topFailed && (
            <>
              <LotRow
                id="most-active-heading"
                title="Most active now"
                viewAll={{ to: "/auctions?status=live&sort=bids", label: "View all live auctions" }}
                auctions={trendingLots}
                isError={trending.isError}
                onRetry={() => void trending.refetch()}
                activeIds={activeIds}
              />
              <LotRow
                id="newly-listed-heading"
                title="Newly listed"
                viewAll={{ to: "/auctions?status=live", label: "View all" }}
                auctions={newlyListed}
                isError={active.isError}
                onRetry={() => void active.refetch()}
                activeIds={activeIds}
              />
            </>
          )}
            {startingSoon}
          </>
        )}
        <LotRow
            id="recently-sold-heading"
            title="Recently sold"
            viewAll={{ to: "/auctions?status=sold", label: "View all" }}
            auctions={sold.data?.auctions}
            isError={sold.isError}
            onRetry={() => void sold.refetch()}
          />
      </div>
    </div>
  );
}

function FeaturedSkeleton() {
  return (
    <div aria-busy="true" aria-label="Loading featured lot">
      <Skeleton className="-mx-4 aspect-[3/2] max-h-[50vh] w-auto rounded-none md:mx-0 md:aspect-[16/9] md:w-full" />
      <div className="space-y-3 pt-4">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-10 w-3/4" />
        <Skeleton className="h-14 w-full" />
      </div>
    </div>
  );
}

function EndingSoonSkeleton() {
  return (
    <div aria-busy="true" aria-label="Loading auctions ending soon">
      <Skeleton className="h-7 w-32" />
      <ul className="mt-3 divide-y divide-line">
        {[0, 1, 2, 3, 4].map((index) => (
          <li key={index} className={`flex gap-3 py-3 ${index > 2 ? "hidden lg:flex" : ""}`}>
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
