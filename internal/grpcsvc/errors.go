// Package grpcsvc exposes bidding over gRPC (v0.9) and provides the client
// the API gateway uses to call it.
//
//	HTTP client ──► API gateway (cmd/api) ──gRPC──► bidding service (cmd/biddingsvc) ──► PostgreSQL
//	                 handlers.BidHandler            grpcsvc.Server
//	                   uses grpcsvc.Client            calls bidding.Service
//
// The gateway's handlers depend on a small interface (handlers.Bidder).
// bidding.Service satisfies it in-process; grpcsvc.Client satisfies it over
// the network. That is dependency inversion paying off: moving bidding into
// its own process didn't change a single handler or HTTP test.
//
// # Error mapping
//
// Errors cross the network as gRPC status codes, not Go values. This file
// maps domain errors to codes on the server, and back to the very same Go
// errors on the client, so errors.Is/As keep working on the gateway side.
//
//	auction.ErrNotFound            NOT_FOUND
//	bidding.ErrOwnAuction          PERMISSION_DENIED
//	invalid input                  INVALID_ARGUMENT  (+ BadRequest field violations)
//	business rule (too low, …)     FAILED_PRECONDITION (+ ErrorInfo reason/metadata)
//	bidding.ErrContention          ABORTED  (concurrency conflict: retry)
//	anything else                  INTERNAL (details are logged, never sent)
package grpcsvc

import (
	"errors"
	"fmt"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/bidding"
	"github.com/Ayush1388/auctionEngine/internal/validation"
	"github.com/Ayush1388/auctionEngine/internal/wallet"
)

const errorDomain = "bidding.auctionengine"

// ErrorInfo reasons.
const (
	reasonNotActive   = "AUCTION_NOT_ACTIVE"
	reasonEnded       = "AUCTION_ENDED"
	reasonTooLow      = "BID_TOO_LOW"
	reasonNoFunds     = "INSUFFICIENT_FUNDS"
	reasonKeyMismatch = "IDEMPOTENCY_MISMATCH"
	reasonInvalidKey  = "INVALID_IDEMPOTENCY_KEY"
	reasonInvalidAmt  = "INVALID_AMOUNT"
)

// toStatus converts a domain error into a gRPC status (server side).
func toStatus(err error) error {
	if err == nil {
		return nil
	}

	var tooLow *bidding.BidTooLowError
	var problems *validation.Error

	switch {
	case errors.As(err, &tooLow):
		return withInfo(codes.FailedPrecondition, err.Error(), reasonTooLow,
			map[string]string{"minimum_amount": strconv.FormatInt(tooLow.Minimum, 10)})
	case errors.As(err, &problems):
		st := status.New(codes.InvalidArgument, err.Error())
		br := &errdetails.BadRequest{}
		for field, msg := range problems.Fields {
			br.FieldViolations = append(br.FieldViolations, &errdetails.BadRequest_FieldViolation{Field: field, Description: msg})
		}
		if detailed, derr := st.WithDetails(br); derr == nil {
			return detailed.Err()
		}
		return st.Err()
	case errors.Is(err, auction.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, bidding.ErrOwnAuction):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, bidding.ErrAuctionNotActive):
		return withInfo(codes.FailedPrecondition, err.Error(), reasonNotActive, nil)
	case errors.Is(err, bidding.ErrAuctionEnded):
		return withInfo(codes.FailedPrecondition, err.Error(), reasonEnded, nil)
	case errors.Is(err, wallet.ErrInsufficientFunds):
		return withInfo(codes.FailedPrecondition, err.Error(), reasonNoFunds, nil)
	case errors.Is(err, bidding.ErrIdempotencyMismatch):
		return withInfo(codes.FailedPrecondition, err.Error(), reasonKeyMismatch, nil)
	case errors.Is(err, bidding.ErrInvalidKey):
		return withInfo(codes.InvalidArgument, err.Error(), reasonInvalidKey, nil)
	case errors.Is(err, bidding.ErrInvalidAmount):
		return withInfo(codes.InvalidArgument, err.Error(), reasonInvalidAmt, nil)
	case errors.Is(err, bidding.ErrContention):
		return status.Error(codes.Aborted, err.Error())
	default:
		// Never leak internals (SQL errors, stack details) to callers;
		// the server interceptor logs the real error.
		return status.Error(codes.Internal, "internal error")
	}
}

func withInfo(code codes.Code, msg, reason string, meta map[string]string) error {
	st := status.New(code, msg)
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: errorDomain, Metadata: meta})
	if err != nil {
		return st.Err()
	}
	return detailed.Err()
}

// fromStatus converts a gRPC error back into the domain error the
// in-process service would have returned (client side).
func fromStatus(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	var info *errdetails.ErrorInfo
	var badRequest *errdetails.BadRequest
	for _, d := range st.Details() {
		switch v := d.(type) {
		case *errdetails.ErrorInfo:
			info = v
		case *errdetails.BadRequest:
			badRequest = v
		}
	}

	if info != nil && info.Domain == errorDomain {
		switch info.Reason {
		case reasonTooLow:
			min, _ := strconv.ParseInt(info.Metadata["minimum_amount"], 10, 64)
			return &bidding.BidTooLowError{Minimum: min}
		case reasonNotActive:
			return bidding.ErrAuctionNotActive
		case reasonEnded:
			return bidding.ErrAuctionEnded
		case reasonNoFunds:
			return wallet.ErrInsufficientFunds
		case reasonKeyMismatch:
			return bidding.ErrIdempotencyMismatch
		case reasonInvalidKey:
			return bidding.ErrInvalidKey
		case reasonInvalidAmt:
			return bidding.ErrInvalidAmount
		}
	}

	switch st.Code() {
	case codes.NotFound:
		return auction.ErrNotFound
	case codes.PermissionDenied:
		return bidding.ErrOwnAuction
	case codes.Aborted:
		return bidding.ErrContention
	case codes.InvalidArgument:
		if badRequest != nil {
			problems := &validation.Error{}
			for _, v := range badRequest.FieldViolations {
				problems.Add(v.Field, v.Description)
			}
			return problems
		}
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		// The service is down, slow or shedding load: retryable, 503.
		return fmt.Errorf("%w: %s", bidding.ErrUnavailable, st.Message())
	}
	// Internal, Unauthenticated (a misconfigured token), …: a server-side
	// problem the end user can't fix; the HTTP layer returns 500.
	return err
}
