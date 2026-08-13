package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cognodb-cloud-benchmark/harness/adapters"
	"cognodb-cloud-benchmark/harness/workload"
)

func main() {
	targetFlag := flag.String("target", "neo4j", "target platform: cognodb, neo4j, memgraph, falkordb, arcadedb")
	ingestFlag := flag.Bool("ingest", false, "run ingestion from data/batches")
	measureFlag := flag.Bool("measure", false, "run measurement workloads")
	batchesDir := flag.String("batches-dir", "../data/batches", "path to batches directory")
	nodesFile := flag.String("nodes-file", "../data/nodes.jsonl", "path to nodes.jsonl for sampling")
	iterations := flag.Int("iterations", 100, "number of measured iterations per query")
	concurrencyFlag := flag.Bool("concurrency", false, "run concurrency sweep")
	concurrencyClients := flag.String("concurrency-clients", "1,10,40", "comma-separated client counts for sweep")
	concurrencyDuration := flag.Int("concurrency-duration", 60, "seconds per concurrency sweep")
	concurrencyRead := flag.Int("concurrency-read", 80, "percent reads in mixed workload (0-100)")

	flag.Parse()

	ctx := context.Background()
	target := strings.ToLower(*targetFlag)

	var adapter adapters.GraphDBAdapter
	switch target {
	case "cognodb":
		uri := os.Getenv("BOLT_COGNODB_URI")
		user := getEnv("BOLT_COGNODB_USER", "cognodb")
		pass := os.Getenv("BOLT_COGNODB_PASS")
		b := adapters.NewBoltAdapter()
		if err := b.Connect(ctx, uri, user, pass); err != nil {
			log.Fatalf("[CognoDB] Connect failed: %v", err)
		}
		adapter = b
	case "neo4j":
		uri := os.Getenv("NEO4J_URI")
		user := getEnv("NEO4J_USER", "neo4j")
		pass := os.Getenv("NEO4J_PASS")
		b := adapters.NewBoltAdapter()
		if err := b.Connect(ctx, uri, user, pass); err != nil {
			log.Fatalf("[Neo4j] Connect failed: %v", err)
		}
		adapter = b
	case "memgraph":
		uri := getEnv("MEMGRAPH_URI", "bolt://localhost:7687")
		b := adapters.NewBoltAdapter()
		if err := b.Connect(ctx, uri, "", ""); err != nil {
			log.Fatalf("[Memgraph] Connect failed: %v", err)
		}
		adapter = b
	case "falkordb":
		uri := getEnv("FALKOR_URI", "redis://localhost:6379/G")
		pass := os.Getenv("FALKOR_PASS")
		f := adapters.NewFalkorDBAdapter()
		if err := f.Connect(ctx, uri, "", pass); err != nil {
			log.Fatalf("[FalkorDB] Connect failed: %v", err)
		}
		adapter = f
	case "arcadedb":
		uri := getEnv("ARCADE_URL", "http://localhost:2480")
		user := getEnv("ARCADE_USER", "root")
		pass := getEnv("ARCADE_PASS", "playwithdata")
		a := adapters.NewArcadeDBAdapter()
		if err := a.Connect(ctx, uri, user, pass); err != nil {
			log.Fatalf("[ArcadeDB] Connect failed: %v", err)
		}
		adapter = a
	default:
		log.Fatalf("Unknown target platform: %s", target)
	}

	defer func() {
		_ = adapter.Close(ctx)
	}()

	fmt.Printf("Connected successfully to target: [%s]\n", target)

	if *ingestFlag {
		fmt.Printf("[%s] Starting ingestion from %s...\n", target, *batchesDir)
		n, r, err := RunIngest(ctx, adapter, *batchesDir)
		if err != nil {
			log.Fatalf("[%s] Ingest failed: %v", target, err)
		}
		fmt.Printf("[%s] Ingest complete: nodes=%d rels=%d\n", target, n, r)

		if indexer, ok := adapter.(interface{ EnsureIndexes(context.Context) error }); ok {
			fmt.Printf("[%s] Building and verifying indexes post-ingestion...\n", target)
			if err := indexer.EnsureIndexes(ctx); err != nil {
				log.Printf("[%s] Warning: post-ingest index build returned error: %v\n", target, err)
			}
			time.Sleep(2 * time.Second)
		}
	}

	if *measureFlag {
		if indexer, ok := adapter.(interface{ EnsureIndexes(context.Context) error }); ok {
			_ = indexer.EnsureIndexes(ctx)
		}

		fmt.Printf("[%s] Running measurement workloads (iterations=%d)...\n", target, *iterations)
		queryTypes := []string{"point", "traversal_1", "traversal_2", "traversal_3", "aggregation"}
		targetResults := make(map[string]map[string]int64)

		for _, q := range queryTypes {
			fmt.Printf("  [%s] Measuring %s...\n", target, q)
			res, err := workload.RunMeasurementLoop(ctx, adapter, *nodesFile, q, *iterations)
			if err != nil {
				fmt.Printf("    error: %v\n", err)
				continue
			}
			fmt.Printf("    %s p50=%dµs p95=%dµs\n", q, res.P50, res.P95)
			targetResults[q] = map[string]int64{
				"p50_us": res.P50,
				"p95_us": res.P95,
			}
		}

		saveResults(target, targetResults)

		if *concurrencyFlag {
			parts := strings.Split(*concurrencyClients, ",")
			ids, err := workload.LoadNodeIDs(*nodesFile)
			if err == nil {
				for _, p := range parts {
					n, _ := strconv.Atoi(strings.TrimSpace(p))
					if n > 0 {
						cres, err := workload.RunConcurrencySweep(ctx, adapter, ids, n, *concurrencyDuration, *concurrencyRead)
						if err == nil {
							fmt.Printf("  [%s] concurrency clients=%d qps=%.2f p50=%dµs p95=%dµs\n", target, cres.Clients, cres.QPS, cres.P50_us, cres.P95_us)
						}
					}
				}
			}
		}
	}
}

func saveResults(target string, newMetrics map[string]map[string]int64) {
	resultsPath := filepath.Join("..", "results", "results.json")
	_ = os.MkdirAll(filepath.Dir(resultsPath), 0755)

	existing := make(map[string]map[string]map[string]int64)
	if data, err := os.ReadFile(resultsPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	existing[target] = newMetrics

	if data, err := json.MarshalIndent(existing, "", "  "); err == nil {
		_ = os.WriteFile(resultsPath, data, 0644)
		fmt.Printf("Updated %s for [%s]\n", resultsPath, target)
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
