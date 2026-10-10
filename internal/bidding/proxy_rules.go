package bidding

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Ayush1388/auctionEngine/internal/auction"
)

// ProxyEntry is one bidder's standing instruction: "keep me winning, up to
// Max". Since orders ties: the earlier instruction wins an equal maximum.
type ProxyEntry struct {
	UserID uuid.UUID
	Max    int64
	Since  time.Time
}

// autoBid is the single automatic bid that should follow the current state.
type autoBid struct {
	UserID uuid.UUID
	Amount int64
}

// nextAutoBid decides the next automatic bid, or false when the auction is
// at rest. It is a pure function so the rules can be tested without a
// database, exactly like decide.
//
// funds(u) is the most u could have held on this auction: their available
// balance, plus what they already hold here when they are the leader. A
// maximum above that is capped, so the engine never promises money that is
// not there.
//
// The rule is the familiar one: the higher maximum wins, and pays one
// increment over the lower maximum (or its own maximum when that is less).
// A challenger with no opposing proxy just bids the minimum. One call
// settles a whole contest, so a loop over it ends in at most one step per
// distinct proxy.
func nextAutoBid(a auction.Auction, proxies []ProxyEntry, funds func(uuid.UUID) int64) (autoBid, bool) {
	if a.Status != auction.StatusActive {
		return autoBid{}, false
	}
	minNext := MinimumBid(a)
	inc := a.MinIncrement
	if inc < 1 {
		inc = 1
	}

	capOf := func(p ProxyEntry) int64 { return min(p.Max, funds(p.UserID)) }

	var leaderCap int64 = -1
	var challengers []ProxyEntry
	for _, p := range proxies {
		if p.UserID == a.OwnerID {
			continue
		}
		if a.CurrentBidderID != nil && p.UserID == *a.CurrentBidderID {
			leaderCap = capOf(p)
			continue
		}
		if capOf(p) >= minNext {
			challengers = append(challengers, p)
		}
	}
	if len(challengers) == 0 {
		return autoBid{}, false
	}
	sort.Slice(challengers, func(i, j int) bool {
		ci, cj := capOf(challengers[i]), capOf(challengers[j])
		if ci != cj {
			return ci > cj
		}
		return challengers[i].Since.Before(challengers[j].Since)
	})
	c := challengers[0]
	cCap := capOf(c)

	// The leader has no usable instruction: the challenger takes the lead at
	// the minimum.
	current := int64(0)
	if a.CurrentBid != nil {
		current = *a.CurrentBid
	}
	if a.CurrentBidderID == nil || leaderCap <= current {
		return autoBid{UserID: c.UserID, Amount: minNext}, true
	}

	if cCap > leaderCap {
		return autoBid{UserID: c.UserID, Amount: min(cCap, leaderCap+inc)}, true
	}
	// The leader's maximum holds (a tie goes to the leader, who was first).
	return autoBid{UserID: *a.CurrentBidderID, Amount: min(leaderCap, cCap+inc)}, true
}
