package workload

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"cognodb-cloud-benchmark/harness/adapters"
	"cognodb-cloud-benchmark/harness/metrics"
)

type ConcurrencyResult struct {
	Clients    int     `json:"clients"`
	DurationS  int     `json:"duration_s"`
	QPS        float64 `json:"qps"`
	P50_us     int64   `json:"p50_us"`
	P95_us     int64   `json:"p95_us"`
	SuccessOps int64   `json:"success_ops"`
	ErrorOps   int64   `json:"error_ops"`
}

// RunConcurrencySweep runs a mixed workload (readPercent% reads, rest writes)
// for `durationSec` seconds using `clients` goroutines. Reads perform a
// 2-hop traversal; writes perform a MERGE create for a synthetic node and a
// relationship to an existing node.
func RunConcurrencySweep(ctx context.Context, adapter adapters.GraphDBAdapter, nodes []string, clients int, durationSec int, readPercent int) (ConcurrencyResult, error) {
	if len(nodes) == 0 {
		return ConcurrencyResult{}, fmt.Errorf("no nodes available for concurrency sweep")
	}
	var success int64
	var errors int64
	hist := metrics.NewHistogram()

	stopCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSec)*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for {
				select {
				case <-stopCtx.Done():
					return
				default:
				}
				// Choose op
				opRoll := r.Intn(100)
				target := nodes[r.Intn(len(nodes))]
				start := time.Now()
				var err error
				if opRoll < readPercent {
					// read: 2-hop traversal
					err = adapter.Traversal(ctx, target, 2)
				} else {
					// write: append a relationship between two existing nodes (avoid growing dataset)
					other := nodes[r.Intn(len(nodes))]
					// ensure different nodes
					if other == target {
						// pick next index
						other = nodes[(r.Intn(len(nodes))+1)%len(nodes)]
					}
					cy := "MATCH (a:User {id:$from}), (b:User {id:$to}) CREATE (a)-[:MUTUAL_FOLLOW]->(b)"
					params := map[string]interface{}{"from": target, "to": other}
					err = adapter.ExecuteWrite(ctx, cy, params)
				}
				if err != nil {
					atomic.AddInt64(&errors, 1)
				} else {
					atomic.AddInt64(&success, 1)
					hist.Record(time.Since(start))
				}
			}
		}(i)
	}
	wg.Wait()

	dur := float64(durationSec)
	succ := atomic.LoadInt64(&success)
	qps := float64(succ) / dur

	return ConcurrencyResult{
		Clients:    clients,
		DurationS:  durationSec,
		QPS:        qps,
		P50_us:     hist.ValueAtQuantile(50),
		P95_us:     hist.ValueAtQuantile(95),
		SuccessOps: succ,
		ErrorOps:   atomic.LoadInt64(&errors),
	}, nil
}
