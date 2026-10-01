package grpcsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	pb "github.com/Ayush1388/auctionEngine/internal/gen/biddingv1"
)

// retryPolicy is gRPC's built-in, transparent retry, configured per method.
//
// Retrying a write is only safe if it is idempotent. PlaceBid becomes
// idempotent because the client below always sends an idempotency key
// (it generates one when the end user didn't), so a retried call after a
// lost response returns the original bid instead of bidding twice.
//
//   - UNAVAILABLE: the server or network blipped (a deploy, a dropped
//     connection).
//   - ABORTED: the optimistic strategy lost a race; a retry usually wins.
//
// Backoff is exponential with jitter, so a fleet of clients doesn't retry
// in lockstep and knock the service over again (a "retry storm").
const serviceConfig = `{
  "methodConfig": [{
    "name": [{"service": "auctionengine.bidding.v1.BiddingService"}],
    "retryPolicy": {
      "maxAttempts": 3,
      "initialBackoff": "0.05s",
      "maxBackoff": "0.5s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE", "ABORTED"]
    }
  }]
}`

// Client is the gateway's view of the remote bidding service. It has the
// same methods and returns the same errors as bidding.Service, so it plugs
// into handlers.BidHandler unchanged.
type Client struct {
	conn *grpc.ClientConn
	rpc  pb.BiddingServiceClient
}

// Dial connects to addr ("biddingsvc:50051").
//
// The connection is plaintext (insecure.NewCredentials) because it runs on
// a private network between our own services. In production you would
// terminate TLS here (or use a service mesh) so traffic is encrypted and
// both sides are authenticated.
func Dial(addr, token string) (*Client, error) {
	conn, err := grpc.NewClient(addr, ClientOptions(token)...)
	if err != nil {
		return nil, fmt.Errorf("dial bidding service: %w", err)
	}
	return &Client{conn: conn, rpc: pb.NewBiddingServiceClient(conn)}, nil
}

// ClientOptions are the dial options every gateway connection uses:
// plaintext transport, the retry policy, and the interceptors that add the
// service token, request ID and a default 3 s deadline.
func ClientOptions(token string) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
		grpc.WithChainUnaryInterceptor(clientUnary(token, 3*time.Second)),
		grpc.WithChainStreamInterceptor(clientStream(token)),
	}
}

// NewClientFromConn wraps an existing connection (tests use an in-memory
// bufconn).
func NewClientFromConn(conn *grpc.ClientConn) *Client {
	return &Client{conn: conn, rpc: pb.NewBiddingServiceClient(conn)}
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) PlaceBid(ctx context.Context, in bidding.PlaceBidInput) (bidding.Result, error) {
	key := in.IdempotencyKey
	if key == "" {
		// Make the call safe to retry (see serviceConfig). Scoped to this
		// one HTTP request: a new click is still a new bid.
		key = "gw:" + randomKey()
	}

	resp, err := c.rpc.PlaceBid(ctx, &pb.PlaceBidRequest{
		AuctionId:      in.AuctionID.String(),
		UserId:         in.UserID.String(),
		Amount:         in.Amount,
		IdempotencyKey: key,
	})
	if err != nil {
		return bidding.Result{}, fromStatus(err)
	}

	bid, err := bidFromProto(resp.GetBid())
	if err != nil {
		return bidding.Result{}, err
	}
	return bidding.Result{
		Bid:        bid,
		Replayed:   resp.GetReplayed(),
		EndsAt:     resp.GetEndsAt().AsTime(),
		Extended:   resp.GetExtended(),
		CurrentBid: resp.GetCurrentBid(),
		BidCount:   int(resp.GetBidCount()),
	}, nil
}

func (c *Client) History(ctx context.Context, auctionID uuid.UUID, limit, cursor string) (bidding.HistoryPage, error) {
	resp, err := c.rpc.ListBids(ctx, &pb.ListBidsRequest{AuctionId: auctionID.String(), Limit: limit, Cursor: cursor})
	if err != nil {
		return bidding.HistoryPage{}, fromStatus(err)
	}

	page := bidding.HistoryPage{Bids: make([]bidding.Bid, 0, len(resp.GetBids()))}
	for _, b := range resp.GetBids() {
		bid, err := bidFromProto(b)
		if err != nil {
			return bidding.HistoryPage{}, err
		}
		page.Bids = append(page.Bids, bid)
	}
	if resp.GetNextCursor() != "" {
		next, err := auction.DecodeCursor(resp.GetNextCursor())
		if err != nil {
			return bidding.HistoryPage{}, err
		}
		page.NextCursor = &next
	}
	return page, nil
}

// Watch opens a WatchAuction stream.
func (c *Client) Watch(ctx context.Context, auctionID uuid.UUID) (pb.BiddingService_WatchAuctionClient, error) {
	return c.rpc.WatchAuction(ctx, &pb.WatchAuctionRequest{AuctionId: auctionID.String()})
}

func bidFromProto(b *pb.Bid) (bidding.Bid, error) {
	id, err1 := uuid.Parse(b.GetId())
	auctionID, err2 := uuid.Parse(b.GetAuctionId())
	userID, err3 := uuid.Parse(b.GetUserId())
	if err1 != nil || err2 != nil || err3 != nil {
		return bidding.Bid{}, fmt.Errorf("bidding service returned a malformed bid")
	}
	return bidding.Bid{ID: id, AuctionID: auctionID, UserID: userID, Amount: b.GetAmount(), CreatedAt: b.GetCreatedAt().AsTime()}, nil
}

func randomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
