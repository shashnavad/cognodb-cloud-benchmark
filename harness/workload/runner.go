package workload

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"cognodb-cloud-benchmark/harness/adapters"
	"cognodb-cloud-benchmark/harness/metrics"
)

// MeasurementResult holds simple percentile results in microseconds.
type MeasurementResult struct {
	P50 int64 `json:"p50_us"`
	P95 int64 `json:"p95_us"`
}

// RunMeasurementLoop runs cold-start (1), warmup (20 unmeasured), then
// `iterations` measured iterations for the given query type. Supported q
// values: "point", "traversal_1", "traversal_2", "traversal_3", "aggregation".
func RunMeasurementLoop(ctx context.Context, adapter adapters.GraphDBAdapter, nodesFile string, q string, iterations int) (MeasurementResult, error) {
	// Load node ids to sample from
	ids, err := LoadNodeIDs(nodesFile)
	if err != nil {
		return MeasurementResult{}, err
	}
	if len(ids) == 0 {
		return MeasurementResult{}, fmt.Errorf("no nodes available in %s", nodesFile)
	}

	hist := metrics.NewHistogram()

	// Helper to execute one op based on query type
	execOne := func(id string) error {
		switch q {
		case "point":
			return adapter.PointLookup(ctx, id)
		case "traversal_1":
			return adapter.Traversal(ctx, id, 1)
		case "traversal_2":
			return adapter.Traversal(ctx, id, 2)
		case "traversal_3":
			return adapter.Traversal(ctx, id, 3)
		case "aggregation":
			return adapter.Aggregation(ctx)
		default:
			return fmt.Errorf("unknown query type: %s", q)
		}
	}

	// Cold start: 1 iteration (timed but reported separately by harness if desired)
	_ = ids[rand.Intn(len(ids))]
	_ = execOne(ids[rand.Intn(len(ids))])

	// Warmup: 20 unmeasured iterations
	for i := 0; i < 20; i++ {
		_ = execOne(ids[rand.Intn(len(ids))])
	}

	// Measured iterations
	for i := 0; i < iterations; i++ {
		id := ids[rand.Intn(len(ids))]
		start := time.Now()
		if err := execOne(id); err != nil {
			// Log error and continue
			continue
		}
		hist.Record(time.Since(start))
	}

	return MeasurementResult{P50: hist.ValueAtQuantile(50), P95: hist.ValueAtQuantile(95)}, nil
}

func LoadNodeIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ids []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if idv, ok := obj["id"].(string); ok {
			ids = append(ids, idv)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
