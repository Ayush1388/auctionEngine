package auction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
	"github.com/Ayush1388/auctionEngine/internal/validation"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func validInput() auction.CreateInput {
	return auction.CreateInput{
		Item: auction.CreateItemInput{
			Name:        "Vintage camera",
			Type:        "electronics",
			Description: "Works, minor scratches",
		},
		StartingPrice: 50_000,
		StartsAt:      now.Add(time.Hour),
		EndsAt:        now.Add(25 * time.Hour),
	}
}

// Validation runs before any database access, so these tests need no DB.
func TestCreateValidation(t *testing.T) {
	service := auction.NewService(nil, nil)
	service.SetClock(func() time.Time { return now })

	tests := []struct {
		name   string
		modify func(*auction.CreateInput)
		field  string
		msg    string
	}{
		{"missing item name", func(in *auction.CreateInput) { in.Item.Name = "   " }, "item.name", "is required"},
		{"missing item type", func(in *auction.CreateInput) { in.Item.Type = "" }, "item.type", "is required"},
		{"negative price", func(in *auction.CreateInput) { in.StartingPrice = -1 }, "starting_price", "must be at least 0"},
		{"missing start", func(in *auction.CreateInput) { in.StartsAt = time.Time{} }, "starts_at", "is required"},
		{"start in the past", func(in *auction.CreateInput) { in.StartsAt = now.Add(-time.Second) }, "starts_at", "must be in the future"},
		{"start too far ahead", func(in *auction.CreateInput) {
			in.StartsAt = now.Add(91 * 24 * time.Hour)
			in.EndsAt = in.StartsAt.Add(time.Hour)
		}, "starts_at", "must be within 90 days"},
		{"end before start", func(in *auction.CreateInput) { in.EndsAt = in.StartsAt.Add(-time.Minute) }, "ends_at", "must be after starts_at"},
		{"end equals start", func(in *auction.CreateInput) { in.EndsAt = in.StartsAt }, "ends_at", "must be after starts_at"},
		{"too short", func(in *auction.CreateInput) { in.EndsAt = in.StartsAt.Add(30 * time.Second) }, "ends_at", "must be at least 1 minute after starts_at"},
		{"too long", func(in *auction.CreateInput) { in.EndsAt = in.StartsAt.Add(31 * 24 * time.Hour) }, "ends_at", "must be at most 30 days after starts_at"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validInput()
			tt.modify(&input)

			_, err := service.Create(context.Background(), uuid.New(), input)

			var problems *validation.Error
			if !errors.As(err, &problems) {
				t.Fatalf("got %v, want a validation error", err)
			}
			if got := problems.Fields[tt.field]; got != tt.msg {
				t.Fatalf("%s: got %q, want %q (all: %v)", tt.field, got, tt.msg, problems.Fields)
			}
		})
	}
}
