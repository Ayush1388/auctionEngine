import { type KeyboardEvent, useId, useRef } from "react";
import { Link } from "react-router";
import type { Auction } from "../../api/types";
import { type Lot, toLot } from "../../domain/lot";
import { formatMoney } from "../../domain/money";
import { describeDuration, formatCountdown } from "../../domain/time";
import { LiveLot } from "../lot/LiveLot";
import { LotPhoto } from "../lot/LotPhoto";
import { lotFigures } from "../lot/lotFigures";
import { useLotClock } from "../lot/useLotClock";

interface FeaturedLotProps {
  /** Up to three auctions; the viewer picks which one is shown. */
  auctions: Auction[];
  since: number;
  selectedId: string;
  onSelect: (id: string) => void;
}

/**
 * The featured lot: the title set large in its era's voice, the photograph
 * beneath it, then one strip of figures. Thumbnails on the photo choose
 * between the most active lots. Selection is manual; nothing rotates.
 */
export function FeaturedLot({ auctions, since, selectedId, onSelect }: FeaturedLotProps) {
  const baseId = useId();
  const tabs = useRef<Array<HTMLButtonElement | null>>([]);
  const selectedIndex = Math.max(0, auctions.findIndex((auction) => auction.id === selectedId));
  const selected = auctions[selectedIndex];
  const hasSelector = auctions.length > 1;

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const moves: Record<string, number> = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 };
    let next: number;
    if (event.key in moves) next = (selectedIndex + moves[event.key] + auctions.length) % auctions.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = auctions.length - 1;
    else return;
    event.preventDefault();
    onSelect(auctions[next].id);
    tabs.current[next]?.focus();
  };

  return (
    <section aria-label="Featured lot">
      <LiveLot auction={selected} since={since}>
        {(lot) => (
          <FeaturedBody lot={lot}>
            <div className="relative -mx-4 md:mx-0">
              <div
                id={`${baseId}-panel`}
                role={hasSelector ? "tabpanel" : undefined}
                aria-labelledby={hasSelector ? `${baseId}-tab-${selectedIndex}` : undefined}
              >
                <LotPhoto
                  key={lot.id}
                  lot={lot}
                  variant="stage"
                  priority
                  sizes="(min-width: 64rem) 66vw, 100vw"
                  className="fade-in aspect-[3/2] max-h-[56vh] w-full md:aspect-[16/9]"
                />
              </div>

              {hasSelector && (
                <div
                  role="tablist"
                  aria-label="Featured lots"
                  aria-orientation="vertical"
                  onKeyDown={onKeyDown}
                  className="flex gap-2 px-4 pt-2 md:absolute md:bottom-3 md:left-3 md:flex-col md:p-0"
                >
                  {auctions.map((auction, index) => {
                    const thumbLot = toLot(auction);
                    const isSelected = index === selectedIndex;
                    return (
                      <button
                        key={auction.id}
                        ref={(element) => {
                          tabs.current[index] = element;
                        }}
                        type="button"
                        role="tab"
                        id={`${baseId}-tab-${index}`}
                        aria-selected={isSelected}
                        aria-controls={`${baseId}-panel`}
                        aria-label={thumbLot.title}
                        tabIndex={isSelected ? 0 : -1}
                        onClick={() => onSelect(auction.id)}
                        className={`size-14 cursor-pointer overflow-hidden rounded-card border-2 bg-surface p-0 transition-colors duration-[120ms] ${
                          isSelected ? "border-live" : "border-bg hover:border-line-strong"
                        }`}
                      >
                        <LotPhoto lot={thumbLot} variant="thumb" sizes="56px" className="size-full" />
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </FeaturedBody>
        )}
      </LiveLot>
    </section>
  );
}

function FeaturedBody({ lot, children }: { lot: Lot; children: React.ReactNode }) {
  const { phase, msLeft } = useLotClock(lot);
  const figures = lotFigures(lot, phase, msLeft);
  const running = phase === "live" || phase === "endingSoon" || phase === "finalMinutes";
  const modern = lot.era === "modern";

  return (
    <>
      <div className="pb-4 md:pb-5">
        <p className="meta flex items-center gap-2 text-14 text-ink-2">
          {running && <span aria-hidden="true" className="size-2 rounded-full bg-live" />}
          <span className={figures.urgent ? "text-live" : "text-ink"}>{figures.tag ?? "Live"}</span>
          {lot.location && <span className="text-muted">{lot.location}</span>}
        </p>
        <h2
          className={`mt-2 ${
            modern
              ? "title-modern text-[clamp(1.5rem,4.1vw,3.25rem)] leading-[1.06]"
              : "title-classic text-display leading-[0.94]"
          }`}
        >
          <Link to={`/auctions/${lot.id}`} className="hover:opacity-80">
            {lot.title}
          </Link>
        </h2>
      </div>

      {children}

      <div className="mt-4 flex flex-col gap-4 md:mt-5 md:flex-row md:items-end md:justify-between">
        <dl className="flex flex-wrap gap-x-6 gap-y-3 md:gap-x-12">
          <div>
            <dt className="meta text-13 text-muted">{lot.bidCount > 0 ? "Current bid" : "Starting bid"}</dt>
            <dd className="figure text-22 text-ink md:text-36">{formatMoney(lot.currentBid ?? lot.startingPrice)}</dd>
          </div>
          <div>
            <dt className="meta text-13 text-muted">Bids</dt>
            <dd className="figure text-22 text-ink md:text-36">{lot.bidCount}</dd>
          </div>
          <div>
            <dt className="meta text-13 text-muted">{figures.rightLabel}</dt>
            <dd className={`figure text-22 md:text-36 ${figures.urgent ? "text-live" : "text-ink"}`}>
              <span aria-hidden="true">{running ? formatCountdown(msLeft) : figures.rightFigure}</span>
              <span className="sr-only">{running ? describeDuration(msLeft) : figures.rightSpoken}</span>
            </dd>
          </div>
        </dl>
        <Link to={`/auctions/${lot.id}`} className="btn btn-solid shrink-0 md:min-h-12 md:px-8">
          View lot
        </Link>
      </div>
    </>
  );
}
