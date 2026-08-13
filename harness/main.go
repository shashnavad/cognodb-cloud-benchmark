package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"cognodb-cloud-benchmark/harness/adapters"
	"cognodb-cloud-benchmark/harness/workload"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config (not required for this minimal scaffold)")
	flag.Parse()
	_ = configPath

	// Read connection info from environment variables. Do NOT hardcode secrets.
	uri := os.Getenv("NEO4J_URI")
	user := os.Getenv("NEO4J_USER")
	pass := os.Getenv("NEO4J_PASS")
	if uri == "" || user == "" || pass == "" {
		log.Fatalf("NEO4J_URI, NEO4J_USER and NEO4J_PASS must be set in the environment")
	}

	ctx := context.Background()
	// Create adapter and connect
	adapter := adapters.NewBoltAdapter()
	if err := adapter.Connect(ctx, uri, user, pass); err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer func() {
		_ = adapter.Close(ctx)
	}()

	fmt.Println("Connected to Neo4j-compatible endpoint.")

	// Option flags control ingest and measurement flows
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

	if *ingestFlag {
		fmt.Println("Starting ingestion from", *batchesDir)
		n, r, err := RunIngest(ctx, adapter, *batchesDir)
		if err != nil {
			log.Fatalf("ingest failed: %v", err)
		}
		fmt.Printf("Ingested nodes=%d rels=%d\n", n, r)
		// exit after ingest when requested
		return
	}

	if *measureFlag {
		fmt.Println("Running measurement workloads (iterations=", *iterations, ")")
		// Run a set of canonical queries and print p50/p95
		queryTypes := []string{"point", "traversal_1", "traversal_2", "traversal_3", "aggregation"}
		for _, q := range queryTypes {
			fmt.Printf("Measuring %s...\n", q)
			res, err := workload.RunMeasurementLoop(ctx, adapter, *nodesFile, q, *iterations)
			if err != nil {
				fmt.Printf("  error: %v\n", err)
				continue
			}
			fmt.Printf("  %s p50=%dµs p95=%dµs\n", q, res.P50, res.P95)
		}
		if *concurrencyFlag {
			// parse clients
			parts := strings.Split(*concurrencyClients, ",")
			ids, err := workload.LoadNodeIDs(*nodesFile)
			if err != nil {
				fmt.Printf("error loading node ids for concurrency: %v\n", err)
			} else {
				for _, p := range parts {
					n, err := strconv.Atoi(strings.TrimSpace(p))
					if err != nil {
						continue
					}
					fmt.Printf("Running concurrency sweep: clients=%d\n", n)
					cres, err := workload.RunConcurrencySweep(ctx, adapter, ids, n, *concurrencyDuration, *concurrencyRead)
					if err != nil {
						fmt.Printf("  concurrency error: %v\n", err)
						continue
					}
					fmt.Printf("  clients=%d qps=%.2f p50=%dµs p95=%dµs success=%d errors=%d\n", cres.Clients, cres.QPS, cres.P50_us, cres.P95_us, cres.SuccessOps, cres.ErrorOps)
				}
			}
		}
		return
	}

	fmt.Println("No action requested. Use --ingest or --measure. Minimal harness ready.")
}
