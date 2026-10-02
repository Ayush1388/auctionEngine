import { TrendingUp } from "lucide-react";
import { Link } from "react-router";
import type { Lot } from "../../domain/lot";
import { WatchButton } from "../watchlist/WatchButton";
import { LotPhoto } from "./LotPhoto";
import { lotFigures } from "./lotFigures";
import { useLotClock } from "./useLotClock";

interface LotCardProps {
  lot: Lot;
  layout: "grid" | "list";
  /** True when the lot is in the trending list: shows the activity marker. */
  active?: boolean;
  /** `sizes` for the grid photo; depends on the grid the card sits in. */
  sizes?: string;
}

const GRID_SIZES = "(min-width: 90rem) 25vw, (min-width: 64rem) 33vw, (min-width: 48rem) 50vw, 100vw";

/**
 * One lot, in the fixed five-part order of plan.md 4.3: photo, location and
 * watch, title, spec rows, then the tinted footer band with money and clock.
 * The card never changes with the lot's era; only the photograph differs.
 */
export function LotCard({ lot, layout, active = false, sizes = GRID_SIZES }: LotCardProps) {
  const { phase, msLeft } = useLotClock(lot);
  const figures = lotFigures(lot, phase, msLeft);
  const href = `/auctions/${lot.id}`;

  if (layout === "list") {
    return (
      <article className="relative flex gap-3 py-3">
        <LotPhoto lot={lot} variant="thumb" sizes="112px" className="h-[75px] w-28 shrink-0 rounded-card" />
        <div className="flex min-w-0 flex-1 flex-col justify-between gap-1">
          <h3 className="heading line-clamp-2 text-16 text-ink">
            <Link to={href} className="after:absolute after:inset-0 hover:underline">
              {lot.title}
            </Link>
          </h3>
          <div className="flex items-baseline justify-between gap-3">
            <p className="flex min-w-0 items-baseline gap-2">
              <span className={`figure min-w-0 truncate text-16 ${figures.leftMuted ? "text-muted" : "text-ink"}`}>{figures.leftFigure}</span>
              {figures.listLeftLabel && <span className="meta min-w-0 shrink-[99] truncate text-12 text-muted">{figures.listLeftLabel}</span>}
            </p>
            <TimeFigure figures={figures} className="text-16" list />
          </div>
        </div>
      </article>
    );
  }

  return (
    <article className="relative flex h-full flex-col overflow-hidden rounded-card border border-[var(--card-outline)] bg-surface">
      <div className="relative border-b border-line">
        <LotPhoto lot={lot} variant="card" sizes={sizes} className="aspect-[3/2] w-full" />
        {figures.tag && (
          <p className="meta absolute left-3 top-3 flex items-center gap-1.5 rounded-card bg-elevated px-2 py-1 text-12 text-ink">
            {figures.urgent && <span aria-hidden="true" className="size-1.5 rounded-full bg-live" />}
            {figures.tag}
          </p>
        )}
      </div>

      <div className="flex flex-1 flex-col px-4 pb-4 pt-3">
        <div className="flex min-h-6 items-center justify-between gap-3">
          <p className="meta truncate text-12 text-muted">{lot.location ?? ""}</p>
          <div className="flex shrink-0 items-center gap-3">
            {active && (
              <span role="img" aria-label="Bidding is active" title="Bidding is active" className="text-muted">
                <TrendingUp aria-hidden="true" size={16} strokeWidth={1.5} />
              </span>
            )}
            <WatchButton lotId={lot.id} lotTitle={lot.title} />
          </div>
        </div>

        <h3 className="heading mt-1 line-clamp-2 min-h-[2.4em] text-18 text-ink">
          <Link to={href} className="after:absolute after:inset-0 hover:underline">
            {lot.title}
          </Link>
        </h3>

        {lot.cardSpecs.length > 0 && (
          <dl className="mt-3 divide-y divide-line">
            {lot.cardSpecs.map(([label, value]) => (
              <div key={label} className="meta flex items-baseline justify-between gap-4 py-1.5 text-13">
                <dt className="shrink-0 text-muted">{label}</dt>
                <dd className="truncate text-right text-ink">{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>

      <div className="mt-auto flex items-end justify-between gap-4 border-t border-line bg-bg-secondary px-4 py-3">
        <div className="min-w-0">
          <p className="meta truncate text-12 text-muted">{figures.leftLabel}</p>
          <p className={`figure text-20 leading-tight ${figures.leftMuted ? "text-muted" : "text-ink"}`}>{figures.leftFigure}</p>
        </div>
        <div className="shrink-0 text-right">
          <p className="meta text-12 text-muted">{figures.rightLabel}</p>
          <TimeFigure figures={figures} className="text-20 leading-tight" />
        </div>
      </div>
    </article>
  );
}

/** The time figure: red only in the last hour, with spoken wording for screen readers. */
function TimeFigure({
  figures,
  className,
  list = false,
}: {
  figures: ReturnType<typeof lotFigures>;
  className: string;
  /** Compact list row: shorter figure, and the label is spoken because none is visible. */
  list?: boolean;
}) {
  return (
    <p className={`figure shrink-0 ${figures.urgent ? "text-live" : "text-ink"} ${className}`}>
      <span aria-hidden="true">{list ? figures.listRightFigure : figures.rightFigure}</span>
      <span className="sr-only">{list ? `${figures.rightLabel}: ${figures.rightSpoken}` : figures.rightSpoken}</span>
    </p>
  );
}
