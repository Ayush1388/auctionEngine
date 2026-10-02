import { useQuery } from "@tanstack/react-query";
import { listAuctions, listTrending } from "../../api/auctions";

const REFRESH = 30_000;
const shared = { staleTime: REFRESH, refetchInterval: REFRESH, refetchOnWindowFocus: true } as const;

/**
 * The four requests behind Discover (plan.md 4.1). Card rows are not live:
 * they refresh on focus and every 30 seconds.
 */
export function useDiscoverData() {
  const trending = useQuery({
    queryKey: ["auctions", "trending", 8],
    queryFn: ({ signal }) => listTrending(8, signal),
    ...shared,
  });
  // One page of up to 100 active lots feeds both "Newly listed" (already
  // newest first) and "Ending soon" (sorted on the client until the API can sort).
  const active = useQuery({
    queryKey: ["auctions", "list", "ACTIVE", 100],
    queryFn: ({ signal }) => listAuctions("ACTIVE", 100, signal),
    ...shared,
  });
  const scheduled = useQuery({
    queryKey: ["auctions", "list", "NOT_ACTIVE", 8],
    queryFn: ({ signal }) => listAuctions("NOT_ACTIVE", 8, signal),
    ...shared,
  });
  const sold = useQuery({
    queryKey: ["auctions", "list", "COMPLETED", 8],
    queryFn: ({ signal }) => listAuctions("COMPLETED", 8, signal),
    ...shared,
  });
  return { trending, active, scheduled, sold };
}
