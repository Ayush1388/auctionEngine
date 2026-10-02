/** Placeholder with the geometry of the content it stands in for. */
export function Skeleton({ className = "" }: { className?: string }) {
  return <div aria-hidden="true" className={`skeleton rounded-card ${className}`} />;
}
