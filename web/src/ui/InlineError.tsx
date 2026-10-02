import { CircleAlert } from "lucide-react";

/** An error shown where the content would have been, with a way to retry. */
export function InlineError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex flex-wrap items-center gap-x-4 gap-y-2 border border-line bg-surface px-4 py-3 rounded-card">
      <p className="flex items-center gap-2 text-14 text-ink">
        <CircleAlert aria-hidden="true" size={18} strokeWidth={1.5} className="shrink-0 text-live" />
        {message}
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="meta min-h-11 cursor-pointer rounded-control border border-line-strong px-4 text-14 text-ink transition-colors duration-[120ms] hover:bg-bg-secondary"
      >
        Try again
      </button>
    </div>
  );
}
