import { useSyncExternalStore } from "react";

export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (notify) => {
      const list = window.matchMedia(query);
      list.addEventListener("change", notify);
      return () => list.removeEventListener("change", notify);
    },
    () => window.matchMedia(query).matches,
  );
}

/** Matches the md breakpoint in tokens.css (768px). */
export const useIsTabletUp = () => useMediaQuery("(min-width: 48rem)");
