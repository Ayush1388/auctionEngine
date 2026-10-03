package grpcsvc

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ayush1388/auctionEngine/internal/bidding"
	pb "github.com/Ayush1388/auctionEngine/internal/gen/biddingv1"
	"github.com/Ayush1388/auctionEngine/internal/realtime"
)

// Watcher streams auction updates; realtime.Hub satisfies it. nil disables
// WatchAuction (it then returns UNAVAILABLE).
type Watcher interface {
	Watch(ctx context.Context, auctionID uuid.UUID, buffer int) (<-chan []byte, bool)
}

// Server implements the generated BiddingServiceServer interface by
// translating protobuf messages to and from the bidding package. It holds no
// business logic of its own: the rules live in bidding.Service, the same
// code the monolith ran in-process.
type Server struct {
	pb.UnimplementedBiddingServiceServer // forward compatibility: new RPCs return UNIMPLEMENTED

	bids    *bidding.Service
	watcher Watcher
}

func NewServer(bids *bidding.Service, watcher Watcher) *Server {
	return &Server{bids: bids, watcher: watcher}
}

func (s *Server) PlaceBid(ctx context.Context, req *pb.PlaceBidRequest) (*pb.PlaceBidResponse, error) {
	auctionID, err := uuid.Parse(req.GetAuctionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid auction_id")
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	res, err := s.bids.PlaceBid(ctx, bidding.PlaceBidInput{
		AuctionID:      auctionID,
		UserID:         userID,
		Amount:         req.GetAmount(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	return &pb.PlaceBidResponse{
		Bid:        bidToProto(res.Bid),
		CurrentBid: res.CurrentBid,
		BidCount:   int32(res.BidCount),
		EndsAt:     timestamppb.New(res.EndsAt),
		Extended:   res.Extended,
		Replayed:   res.Replayed,
	}, nil
}

func (s *Server) ListBids(ctx context.Context, req *pb.ListBidsRequest) (*pb.ListBidsResponse, error) {
	auctionID, err := uuid.Parse(req.GetAuctionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid auction_id")
	}

	page, err := s.bids.History(ctx, auctionID, req.GetLimit(), req.GetCursor())
	if err != nil {
		return nil, toStatus(err)
	}

	resp := &pb.ListBidsResponse{Bids: make([]*pb.Bid, len(page.Bids))}
	for i, b := range page.Bids {
		resp.Bids[i] = bidToProto(b)
	}
	if page.NextCursor != nil {
		resp.NextCursor = page.NextCursor.Encode()
	}
	return resp, nil
}

// WatchAuction is a server-streaming RPC: it keeps sending snapshots until
// the client cancels (stream.Context() is done) or the server shuts down.
// Flow control is built into HTTP/2: if the client reads slowly, Send
// blocks, the hub's buffer for this watcher fills, and the watch is ended
// rather than buffering without limit.
func (s *Server) WatchAuction(req *pb.WatchAuctionRequest, stream pb.BiddingService_WatchAuctionServer) error {
	if s.watcher == nil {
		return status.Error(codes.Unavailable, "live updates are not configured on this server")
	}
	auctionID, err := uuid.Parse(req.GetAuctionId())
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid auction_id")
	}

	ctx := stream.Context()
	updates, ok := s.watcher.Watch(ctx, auctionID, 32)
	if !ok {
		return status.Error(codes.ResourceExhausted, "server at capacity")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, open := <-updates:
			if !open {
				if ctx.Err() != nil {
					return nil
				}
				return status.Error(codes.ResourceExhausted, "too slow to keep up")
			}
			snap, err := snapshotFromMessage(msg)
			if err != nil {
				continue // not an auction.updated message
			}
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}
}

func bidToProto(b bidding.Bid) *pb.Bid {
	return &pb.Bid{
		Id:        b.ID.String(),
		AuctionId: b.AuctionID.String(),
		UserId:    b.UserID.String(),
		Amount:    b.Amount,
		CreatedAt: timestamppb.New(b.CreatedAt),
	}
}

// snapshotFromMessage converts the JSON message the hub fans out
// (realtime.EventHandler) into the protobuf snapshot.
func snapshotFromMessage(msg []byte) (*pb.AuctionSnapshot, error) {
	var m struct {
		Type    string            `json:"type"`
		Cause   string            `json:"cause"`
		Auction realtime.Snapshot `json:"auction"`
	}
	if err := json.Unmarshal(msg, &m); err != nil || m.Type != "auction.updated" {
		return nil, status.Error(codes.Internal, "not an update")
	}
	a := m.Auction
	snap := &pb.AuctionSnapshot{
		Id:         a.ID.String(),
		Status:     a.Status,
		CurrentBid: a.CurrentBid,
		BidCount:   int32(a.BidCount),
		MinNextBid: a.MinNextBid,
		EndsAt:     timestamppb.New(a.EndsAt),
		Extensions: int32(a.Extensions),
		Version:    a.Version,
		Cause:      m.Cause,
	}
	if a.CurrentBidderID != nil {
		id := a.CurrentBidderID.String()
		snap.CurrentBidderId = &id
	}
	return snap, nil
}
