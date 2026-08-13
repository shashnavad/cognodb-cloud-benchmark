# CognoDB Cloud Benchmark Engine — Architecture Specification

This document provides a technical deep dive into the Go-based harness architecture, driver adapter design, concurrency engine, and reporting pipeline for the **CognoDB Cloud Benchmark Suite**[cite: 6].

---

## 🏗️ Architectural Overview

The benchmark harness is structured as a modular, decoupled Go engine designed to measure graph database engine performance with microsecond precision while minimizing client-side allocation overhead[cite: 6].

```
+-----------------------------------------------------------------------+
|                            ORCHESTRATION                              |
|                          scripts/run_all.sh                           |
+-----------------------------------------------------------------------+
                                   |
         +-------------------------+-------------------------+
         |                                                   |
         v                                                   v
+------------------+                               +------------------+
|  DATA PREP (PY)  |                               |   GO HARNESS     |
| download_snap.py |                               |  harness/main.go |
+------------------+                               +------------------+
         |                                                   |
         v                                                   v
+------------------+                               +------------------+
| data/batches/*.json |                            | Adapter Interface|
+------------------+                               | GraphDBAdapter   |
                                                   +------------------+
                                                             |
                                      +----------------------+----------------------+
                                      |                      |                      |
                                      v                      v                      v
                               +--------------+       +--------------+       +--------------+
                               | BoltAdapter  |       | FalkorAdapter|       |ArcadeAdapter |
                               +--------------+       +--------------+       +--------------+
                                      |                      |                      |
                                      +----------------------+----------------------+
                                                             |
                                                             v
                                                   +------------------+
                                                   | Workload Runner  |
                                                   | (runner/sweep)   |
                                                   +------------------+
                                                             |
                                                             v
                                                   +------------------+
                                                   | Concurrent Hist  |
                                                   |  metrics/hist.go |
                                                   +------------------+
                                                             |
                                                             v
                                                   +------------------+
                                                   | results.json     |
                                                   +------------------+
                                                             |
                                         +-------------------+-------------------+
                                         |                                       |
                                         v                                       v
                               +-------------------+                   +-------------------+
                               |  plot_results.py  |                   |generate_report.py |
                               |  (Matplotlib)     |                   |    (README.md)    |
                               +-------------------+                   +-------------------+
```

---

## 📦 Core Component Modules

### 1. `harness/adapters/` — Unified Database Abstraction

All target databases implement the strict `GraphDBAdapter` Go interface[cite: 6]:

```go
type GraphDBAdapter interface {
    Connect(ctx context.Context, uri, user, pass string) error
    IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error
    PointLookup(ctx context.Context, id string) error
    Traversal(ctx context.Context, startID string, hops int) error
    Aggregation(ctx context.Context) error
    ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error
    Close(ctx context.Context) error
}
```

* **`BoltAdapter` (`bolt_adapter.go`):** Implements official `neo4j-go-driver/v5`[cite: 2]. Manages session lifetimes via `defer sess.Close(ctx)`, explicit write/read transactions, and forces result consumption via `res.Consume(ctx)` to ensure accurate query execution timing[cite: 2]. Automatically provisions range indexes on `:User(id)` during initial connection[cite: 2].
* **`FalkorDBAdapter` (`falkordb_adapter.go`):** Connects to RedisGraph/FalkorDB endpoints over `go-redis/v9` using pipelined Cypher queries via `GRAPH.QUERY` commands[cite: 6].
* **`ArcadeDBAdapter` (`arcadedb_adapter.go`):** Uses an HTTP REST transport sending batched SQL/Cypher command payloads with HTTP Basic Authentication[cite: 6].

---

### 2. `harness/workload/` — Execution Engine & Sampling

* **`runner.go` (`RunMeasurementLoop`):** Executes workload benchmarking[cite: 6]:
  1. **Node Sampling:** Randomly selects start nodes from `data/nodes.jsonl`[cite: 6].
  2. **Cold Run:** Executes 1 timed cold iteration[cite: 6].
  3. **Warmup Phase:** Executes 20 unmeasured iterations to warm query plans and buffer pools[cite: 6].
  4. **Measurement Phase:** Executes $N$ measured iterations (default: 100), recording execution durations into a concurrent histogram[cite: 6].

* **`concurrency.go` (`RunConcurrencySweep`):** Spawns $C$ worker goroutines (e.g., 1, 10, 40) executing a mixed read/write workload ratio (80% 2-hop traversals, 20% edge creations) over a fixed time duration[cite: 6]. Utilizes `sync/atomic` counters for QPS and error tracking[cite: 6].

---

### 3. `harness/metrics/` — Microsecond Histogram

* **`histogram.go`:** Thread-safe, low-overhead latency recorder[cite: 6].
* Stores observed durations in microsecond slices (`int64`) protected by a `sync.Mutex`[cite: 6].
* Quantile calculation (`ValueAtQuantile`) sorts duration samples and computes p50 and p95 percentiles without external heavy sampling dependencies[cite: 6].

---

## 🔄 End-to-End Orchestration Flow

1. **Container Initialization:** `scripts/run_all.sh` invokes Docker Compose with explicit CPU (`--cpus=0.5`) and memory limits (`--memory=256m`)[cite: 7].
2. **SNAP Dataset Preparation:** `data/download_snap.py` fetches raw CSV edges, synthesizes node attributes, builds `nodes.jsonl` / `relationships.jsonl`, and emits JSON batch files to `data/batches/`[cite: 6].
3. **Go Benchmark Execution:** `harness/main.go` parses flags, connects to the database adapter, executes batch ingestion, runs query measurement loops, and exports metrics to `results/results.json`[cite: 3].
4. **Plotting & Markdown Injection:** 
   * `scripts/plot_results.py` generates PNG visual charts in `docs/img/`[cite: 4].
   * `scripts/generate_report.py` reads `results/results.json` and updates the Markdown table between `<!-- BENCHMARK_RESULTS_START -->` markers in `README.md`[cite: 5].