package metrics

import (
	"sort"
	"sync"
	"time"
)

// Histogram is a simple concurrent latency recorder used for p50/p95.
// It stores observed durations in microseconds. Not memory-optimal but
// sufficient for the scaffold and reproducible percentiles.
type Histogram struct {
	mu   sync.Mutex
	vals []int64
}

func NewHistogram() *Histogram { return &Histogram{vals: make([]int64, 0, 1024)} }

func (h *Histogram) Record(d time.Duration) {
	us := d.Microseconds()
	h.mu.Lock()
	h.vals = append(h.vals, us)
	h.mu.Unlock()
}

// ValueAtQuantile returns the value at quantile q (0-100) in microseconds.
// If no samples recorded, returns 0.
func (h *Histogram) ValueAtQuantile(q float64) int64 {
	h.mu.Lock()
	if len(h.vals) == 0 {
		h.mu.Unlock()
		return 0
	}
	copied := make([]int64, len(h.vals))
	copy(copied, h.vals)
	h.mu.Unlock()
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	if q <= 0 {
		return copied[0]
	}
	if q >= 100 {
		return copied[len(copied)-1]
	}
	idx := int((q/100.0)*float64(len(copied)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(copied) {
		idx = len(copied) - 1
	}
	return copied[idx]
}
