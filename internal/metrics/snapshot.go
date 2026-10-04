package metrics

import (
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// Family is one metric in a form a dashboard can read without parsing the
// Prometheus text format.
type Family struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Help    string   `json:"help"`
	Samples []Sample `json:"samples"`
}

// Sample is one labelled series. Counters and gauges set Value; histograms set
// Count, Sum and Buckets (cumulative, like Prometheus).
type Sample struct {
	Labels  map[string]string `json:"labels,omitempty"`
	Value   *float64          `json:"value,omitempty"`
	Count   *uint64           `json:"count,omitempty"`
	Sum     *float64          `json:"sum,omitempty"`
	Buckets []Bucket          `json:"buckets,omitempty"`
}

// Bucket is one cumulative histogram bucket: observations <= Le.
type Bucket struct {
	Le    float64 `json:"le"`
	Count uint64  `json:"count"`
}

// Snapshot gathers this process's application metrics (the "auction_" ones;
// Go runtime and process collectors are left out) as JSON-friendly families,
// sorted by name. The status page in the frontend reads it from
// GET /v1/admin/metrics so it does not need access to the internal admin port.
func Snapshot() ([]Family, error) {
	gathered, err := Registry.Gather()
	if err != nil {
		return nil, err
	}

	out := make([]Family, 0, len(gathered))
	for _, mf := range gathered {
		if !strings.HasPrefix(mf.GetName(), namespace+"_") {
			continue
		}
		f := Family{
			Name:    mf.GetName(),
			Type:    strings.ToLower(mf.GetType().String()),
			Help:    mf.GetHelp(),
			Samples: make([]Sample, 0, len(mf.GetMetric())),
		}
		for _, m := range mf.GetMetric() {
			s := Sample{}
			if len(m.GetLabel()) > 0 {
				s.Labels = make(map[string]string, len(m.GetLabel()))
				for _, l := range m.GetLabel() {
					s.Labels[l.GetName()] = l.GetValue()
				}
			}
			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				v := m.GetCounter().GetValue()
				s.Value = &v
			case dto.MetricType_GAUGE:
				v := m.GetGauge().GetValue()
				s.Value = &v
			case dto.MetricType_HISTOGRAM:
				h := m.GetHistogram()
				c, sum := h.GetSampleCount(), h.GetSampleSum()
				s.Count, s.Sum = &c, &sum
				for _, b := range h.GetBucket() {
					s.Buckets = append(s.Buckets, Bucket{Le: b.GetUpperBound(), Count: b.GetCumulativeCount()})
				}
			default:
				continue
			}
			f.Samples = append(f.Samples, s)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
