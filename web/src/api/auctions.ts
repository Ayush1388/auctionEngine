import { apiGet } from "./client";
import type { AuctionPage, AuctionStatus, TrendingResponse } from "./types";

/** GET /v1/auctions: newest first, keyset pagination. */
export function listAuctions(status: AuctionStatus, limit: number, signal?: AbortSignal) {
  return apiGet<AuctionPage>("/v1/auctions", { status, limit }, signal);
}

/** GET /v1/auctions/trending: active auctions with the most bids in the last hour. */
export function listTrending(limit: number, signal?: AbortSignal) {
  return apiGet<TrendingResponse>("/v1/auctions/trending", { limit }, signal);
}
