import { useEffect, useSyncExternalStore } from "react";
import type { ServerMessage } from "../api/types";
import { applySnapshot } from "./liveStore";

const WS_URL = import.meta.env.VITE_WS_URL ?? "ws://localhost:4000/v1/ws";
/** The server allows 20 subscriptions per connection. */
const MAX_SUBSCRIPTIONS = 20;
const BACKOFF_START = 500;
const BACKOFF_MAX = 10_000;

export type ConnectionState = "idle" | "connecting" | "live" | "reconnecting";

/**
 * One WebSocket per tab with reference-counted subscriptions. The stream is
 * public and read-only: every message is a full snapshot of one auction.
 */
class SocketManager {
  private socket: WebSocket | null = null;
  private refs = new Map<string, number>();
  private backoff = BACKOFF_START;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private state: ConnectionState = "idle";
  private stateListeners = new Set<() => void>();

  subscribe(auctionId: string): () => void {
    const count = this.refs.get(auctionId) ?? 0;
    if (count === 0 && this.refs.size >= MAX_SUBSCRIPTIONS) return () => {};

    this.refs.set(auctionId, count + 1);
    if (count === 0) this.send({ action: "subscribe", auction_id: auctionId });
    this.ensureOpen();

    return () => {
      const current = this.refs.get(auctionId) ?? 0;
      if (current <= 1) {
        this.refs.delete(auctionId);
        this.send({ action: "unsubscribe", auction_id: auctionId });
        if (this.refs.size === 0) this.closeSoon();
      } else {
        this.refs.set(auctionId, current - 1);
      }
    };
  }

  getState = (): ConnectionState => this.state;

  onState = (listener: () => void): (() => void) => {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  };

  private setState(next: ConnectionState) {
    if (this.state === next) return;
    this.state = next;
    this.stateListeners.forEach((listener) => listener());
  }

  private send(message: object) {
    if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(message));
  }

  private ensureOpen() {
    if (this.socket || this.retryTimer) return;
    this.open(false);
  }

  private open(isRetry: boolean) {
    this.setState(isRetry ? "reconnecting" : "connecting");

    let socket: WebSocket;
    try {
      socket = new WebSocket(WS_URL);
    } catch {
      this.scheduleRetry(BACKOFF_MAX);
      return;
    }
    this.socket = socket;

    socket.onopen = () => {
      this.backoff = BACKOFF_START;
      this.setState("live");
      for (const auctionId of this.refs.keys()) {
        socket.send(JSON.stringify({ action: "subscribe", auction_id: auctionId }));
      }
    };

    socket.onmessage = (event) => {
      let message: ServerMessage;
      try {
        message = JSON.parse(String(event.data)) as ServerMessage;
      } catch {
        return;
      }
      if (message.type === "auction.updated") applySnapshot(message.auction);
    };

    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      if (this.refs.size === 0) {
        this.setState("idle");
        return;
      }
      // 1001: server restarting, reconnect at once. 1013: at capacity, wait.
      const delay = event.code === 1001 ? 0 : event.code === 1013 ? BACKOFF_MAX : this.nextBackoff();
      this.scheduleRetry(delay);
    };

    // An error is always followed by a close event, which handles the retry.
    socket.onerror = () => {};
  }

  private nextBackoff(): number {
    const delay = this.backoff + Math.random() * this.backoff * 0.3;
    this.backoff = Math.min(this.backoff * 2, BACKOFF_MAX);
    return delay;
  }

  private scheduleRetry(delay: number) {
    this.setState("reconnecting");
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      if (this.refs.size > 0) this.open(true);
      else this.setState("idle");
    }, delay);
  }

  /** Closes after a short idle, so a quick unsubscribe and resubscribe keeps the connection. */
  private closeSoon() {
    setTimeout(() => {
      if (this.refs.size === 0) this.close();
    }, 1000);
  }

  private close() {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    const socket = this.socket;
    this.socket = null;
    socket?.close(1000);
    this.setState("idle");
  }
}

const manager = new SocketManager();

/** Subscribes to live updates for the given auctions while mounted. */
export function useLiveAuctions(auctionIds: readonly string[]): void {
  const key = auctionIds.join(",");
  useEffect(() => {
    if (key === "") return;
    const releases = key.split(",").map((id) => manager.subscribe(id));
    return () => releases.forEach((release) => release());
  }, [key]);
}

export function useConnectionState(): ConnectionState {
  return useSyncExternalStore(manager.onState, manager.getState);
}
