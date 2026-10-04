package metrics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotIsJSONFriendlyAndLeavesOutRuntimeMetrics(t *testing.T) {
	Bids.WithLabelValues("accepted").Inc()
	BidDuration.WithLabelValues("pessimistic").Observe(0.004)

	families, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(families); err != nil { // +Inf buckets and NaN would break this
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}

	byName := map[string]Family{}
	for _, f := range families {
		if !strings.HasPrefix(f.Name, "auction_") {
			t.Errorf("runtime metric %q should not be in the snapshot", f.Name)
		}
		byName[f.Name] = f
	}

	bids := byName["auction_bids_total"]
	if bids.Type != "counter" || len(bids.Samples) == 0 || bids.Samples[0].Value == nil {
		t.Fatalf("bids counter: %+v", bids)
	}

	h := byName["auction_bid_duration_seconds"]
	if h.Type != "histogram" || len(h.Samples) == 0 || h.Samples[0].Count == nil || *h.Samples[0].Count < 1 {
		t.Fatalf("bid duration histogram: %+v", h)
	}
	prev := uint64(0)
	for _, b := range h.Samples[0].Buckets { // cumulative, like Prometheus
		if b.Count < prev {
			t.Fatalf("buckets are not cumulative: %+v", h.Samples[0].Buckets)
		}
		prev = b.Count
	}
}
