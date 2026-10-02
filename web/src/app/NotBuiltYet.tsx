import { Link, useLocation } from "react-router";

/** Placeholder for routes that exist in the plan but are not implemented yet. */
export function NotBuiltYet() {
  const { pathname } = useLocation();
  return (
    <div className="mx-auto w-full max-w-[1384px] px-4 py-16 md:px-6 lg:px-8">
      <h1 className="heading text-28 text-ink">This page is not built yet</h1>
      <p className="mt-2 max-w-[60ch] text-16 text-ink-2">
        <span className="break-all">{pathname}</span> is part of the plan but has not been implemented. Only the Discover page exists so far.
      </p>
      <Link
        to="/"
        className="meta mt-6 inline-flex min-h-11 items-center rounded-control bg-action px-5 text-16 text-on-action transition-opacity duration-[120ms] hover:opacity-85"
      >
        Back to Discover
      </Link>
    </div>
  );
}
