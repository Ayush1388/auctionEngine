// Shapes from api/openapi.json (auctionEngine API v1.0.0).
// Amounts are integers in the smallest currency unit.

export type AuctionStatus = "NOT_ACTIVE" | "ACTIVE" | "COMPLETED" | "CANCELLED";

export interface Item {
  id: string;
  name: string;
  type: string;
  description: string;
}

export interface Auction {
  id: string;
  owner_id: string;
  item: Item;
  starting_price: number;
  min_increment: number;
  current_bid: number | null;
  current_bidder_id: string | null;
  bid_count: number;
  extensions: number;
  settled_at: string | null;
  starts_at: string;
  ends_at: string;
  status: AuctionStatus;
  created_at: string;
  updated_at: string;
}

export interface AuctionPage {
  auctions: Auction[];
  next_cursor: string | null;
}

export interface TrendingResponse {
  auctions: Auction[];
}

/** Live part of an auction, pushed over GET /v1/ws. */
export interface AuctionSnapshot {
  id: string;
  status: AuctionStatus;
  current_bid: number | null;
  current_bidder_id: string | null;
  bid_count: number;
  min_next_bid: number;
  ends_at: string;
  extensions: number;
  version: number;
}

export type ServerMessage =
  | { type: "auction.updated"; cause: string; auction: AuctionSnapshot }
  | { type: "subscribed" | "unsubscribed"; auction_id: string }
  | { type: "error"; error: string };
