import { Heart } from "lucide-react";
import { toggleWatched, useIsWatched } from "./watchlist";

/** Icon-only watch toggle for cards. The name states the lot it acts on. */
export function WatchButton({ lotId, lotTitle }: { lotId: string; lotTitle: string }) {
  const watched = useIsWatched(lotId);
  return (
    <button
      type="button"
      aria-pressed={watched}
      aria-label={`Watch ${lotTitle}`}
      title={watched ? "Watching" : "Watch"}
      onClick={() => toggleWatched(lotId)}
      className="relative z-10 -m-2.5 grid size-11 shrink-0 cursor-pointer place-items-center rounded-control text-ink-2 transition-colors duration-[120ms] hover:text-ink"
    >
      <Heart aria-hidden="true" size={18} strokeWidth={1.5} fill={watched ? "currentColor" : "none"} />
    </button>
  );
}
