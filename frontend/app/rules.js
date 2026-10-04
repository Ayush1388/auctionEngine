// Auction rules the client needs to explain, kept in one place. These mirror
// internal/bidding (SnipeWindow, MaxExtensions) and internal/auction.
export const SNIPE_WINDOW = 2 * 60 * 1000;
export const MAX_EXTENSIONS = 10;
export const MIN_DURATION = 60 * 1000;
export const MAX_DURATION = 30 * 24 * 3600 * 1000;
export const MAX_START_DELAY = 90 * 24 * 3600 * 1000;
