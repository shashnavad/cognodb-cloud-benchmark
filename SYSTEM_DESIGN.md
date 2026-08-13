# CognoDB Cloud Benchmark — System Design Blueprint

Given the 24-hour execution timeline and background in Go and Python, this design uses **Go** for high-precision, low-overhead concurrent load generation (p50/p95 latency tracking) and **Python** for dataset ETL, orchestration, and chart generation.

**Revision note:** ArangoDB has been dropped in favor of **Kuzu**. ArangoDB uses AQL, a second query language, which would have required a separate query set and a separate HTTP adapter for one data point — disproportionate effort against a 1-day budget. Kuzu speaks Cypher like the other four targets, so the whole benchmark now runs on a single query language, and it needs only a thin adapter (no server/connection management, since it's embedded).

---

## 1. System Architecture Overview

The system consists of five decoupled components: **Dataset Preparation**, **Target Database Provisioning**, **Database Adapters**, **Benchmarking Engine (Go)**, and **Metrics Analysis Pipeline**.

```
                           +------------------------+
                           |  Public SNAP Dataset   |
                           | (musae-github / 289k)  |
                           +-----------+------------+
                                       |
                                       v
                           +------------------------+
                           |  Python ETL / Loader   |
                           |  (Normalizer & Split)  |
                           +-----------+------------+
                                       |
        +---------------+--------------+--------------+---------------+
        |               |              |              |               |
        v               v              v              v               v
 +-------------+  +-------------+  +-----------+  +-----------+  +-----------+
 |CognoDB Cloud|  |Neo4j AuraDB |  | Memgraph  |  | FalkorDB  |  |   Kuzu    |
 |(0.5 vCPU/   |  |(Free Tier)  |  |(0.5 vCPU/ |  |(0.5 vCPU/ |  |(embedded, |
 | 256MB RAM)  |  |             |  | 256MB RAM)|  | 256MB RAM)|  | no network|
 +------+------+  +------+------+  +-----+-----+  +-----+-----+  | hop)      |
        |                |               |              |       +-----+-----+
        +----------------+---------------+--------------+-------------+
                                       |
                                       v
                       +--------------------------------+
                       |   Go Benchmark Engine (Harness)|
                       |  - Latency Recorder (hdrhist)  |
                       |  - Worker Pools (1, 10, 40)    |
                       |  - Ping / Network Deductor      |
                       +---------------+----------------+
                                       | (JSON Results)
                                       v
                       +--------------------------------+
                       |  Python Plotter & Reporter     |
                       |  - Matplotlib / Seaborn        |
                       |  - Markdown Table Generator    |
                       +--------------------------------+
```

---

## 2. Target Databases & Hardware Parity Strategy

To ensure a fair evaluation (25% score weight) without resource advantages:

| Database | Deployment Target | Tier Specs / Capping | Query Language | Driver / Protocol |
|---|---|---|---|---|
| **CognoDB Cloud** | CognoDB Free Tier | 0.5 vCPU, 256 MB RAM, 1 GB Disk | Cypher | `neo4j-go-driver` (Bolt+s) |
| **Neo4j** | AuraDB Free | 0.5–1 vCPU, 1 GB RAM (throttled load) | Cypher | `neo4j-go-driver` (Bolt+s) |
| **Memgraph** | Docker Container | Capped 0.5 vCPU, 256 MB RAM (`--memory=256m --cpus=0.5`) | Cypher | `neo4j-go-driver` (Bolt) |
| **FalkorDB** | Docker Container | Capped 0.5 vCPU, 256 MB RAM | Cypher | `redis-go` (GRAPH.QUERY) |
| **Kuzu** | Embedded (in-process) | N/A — no server, linked directly into the Go binary | Cypher | Kuzu Go/CGo binding |

**CognoDB, Neo4j, and Memgraph share one adapter implementation** — all three speak Bolt/Cypher via `neo4j-go-driver`, differing only in connection URI and credentials. FalkorDB and Kuzu each need their own adapter, giving 3 adapters total across 5 targets instead of 5.

### Network & Fairness Mitigation (Crucial Methodology)

1. **Baseline TCP Ping Subtraction**: CognoDB and AuraDB run in cloud regions while self-hosted containers run on localhost/cloud VMs, so raw network latency will distort database execution numbers. The Go engine runs a pre-benchmark RTT (Round Trip Time) TCP ping sweep (100 probes) to measure baseline connection latency and reports both **Total Roundtrip Latency** and **Estimated DB Server Execution Latency**.
2. **Resource Capping**: For local/Docker-based graph databases, run under `--cpus=0.5 --memory=256m` to strictly enforce CognoDB free-tier limits.
3. **Kuzu asymmetry — disclose, don't hide**: Kuzu is embedded, so it has no network hop and no independently capped process the way the other four do. This is not a fairness violation to paper over; it's a legitimate methodology point. Frame it explicitly in the README as a zero-network baseline that shows how much of the other four platforms' latency is network vs. execution time — the assignment's evaluation criteria explicitly reward disclosed caveats over hidden ones.

---

## 3. Dataset Selection & Workload Design

### Recommended Dataset

- **Dataset**: SNAP `musae-github` (37,700 nodes, 289,003 undirected relationships).
- **Why**: Fits comfortably within 256 MB RAM limits while meeting the ≥100,000 relationship requirement.
- **Node Schema**: `User {id: STRING, location: STRING, public_repos: INT}`
- **Relationship Schema**: `(:User)-[:MUTUAL_FOLLOW]->(:User)`

### Workload Matrix & Equivalent Queries

All five targets now share a single Cypher query set — no AQL translation column needed.

| Workload Category | Metric / Operation | Cypher Query (CognoDB, Neo4j, Memgraph, FalkorDB, Kuzu) |
|---|---|---|
| **Data Ingestion** | Bulk Node & Rel Creation | `UNWIND $batch AS row CREATE ...` |
| **Lookups** | Point Lookup (Indexed) | `MATCH (u:User {id: $id}) RETURN u` |
| **Lookups** | Filtered Property Lookup | `MATCH (u:User) WHERE u.public_repos > 50 RETURN u LIMIT 10` |
| **Traversals** | 1-Hop Neighbors | `MATCH (u:User {id: $id})-[:MUTUAL_FOLLOW]->(m) RETURN m` |
| **Traversals** | 2-Hop Traversal | `MATCH (u:User {id: $id})-[:MUTUAL_FOLLOW*2]->(m) RETURN DISTINCT m LIMIT 100` |
| **Traversals** | 3-Hop Traversal | `MATCH (u:User {id: $id})-[:MUTUAL_FOLLOW*3]->(m) RETURN DISTINCT m LIMIT 100` |
| **Aggregations** | Degree Count / Grouping | `MATCH (u:User)-[r:MUTUAL_FOLLOW]->() RETURN u.id, COUNT(r) AS degree ORDER BY degree DESC LIMIT 10` |
| **Mixed Workload** | 80% Read / 20% Write Concurrent | Read: 2-hop query. Write: `CREATE (:User {id: $new_id})-[:MUTUAL_FOLLOW]->(u)` |

---

## 4. Benchmarking Engine Architecture (Go)

The Go harness guarantees sub-millisecond timer resolution and lock-free thread pools for concurrency sweeps.

### Core Go Components

1. **`Config Engine`**: Reads environment variables (`COGNODB_URI`, `NEO4J_URI`, `KUZU_DB_PATH`, etc.) — zero hardcoded credentials.
2. **`Adapter Interface`**:
   ```go
   type GraphDBAdapter interface {
       Connect(ctx context.Context, uri, user, pass string) error
       IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error
       PointLookup(ctx context.Context, id string) error
       Traversal(ctx context.Context, startID string, hops int) error
       Aggregation(ctx context.Context) error
       ExecuteWrite(ctx context.Context, writePayload WriteData) error
       Close() error
   }
   ```
   Three concrete implementations: `bolt_adapter.go` (CognoDB, Neo4j, Memgraph), `falkordb_adapter.go`, `kuzu_adapter.go`.
3. **`Warmup & Runner Loop`**:
   - **Cold Start**: Executes 1 query iteration immediately post-ingestion; records latency.
   - **Warm-up Phase**: Runs 20 unmeasured query iterations to populate buffer caches/memory.
   - **Measurement Phase**: Runs N ≥ 100 iterations using random `startID` targets sampled from the dataset to avoid query engine cache cheating.
4. **`Concurrency Engine (Worker Pool)`**:
   - Sweeps across 1, 10, and 40 concurrent workers (goroutines) for 60 seconds per test.
   - Tracks throughput (QPS) and latency distributions (p50, p95) using `HdrHistogram`.
   - Kuzu is single-process/embedded — concurrent access is in-process goroutines against the same handle rather than concurrent network clients; note this distinction in the mixed-workload results.

---

## 5. Repository Structure & Execution Workflow

To meet the single-command automation criteria (20% score weight):

```text
cognodb-cloud-benchmark/
├── README.md                   # Full results matrix, analysis, caveats & specs
├── Makefile                    # Single entry point: `make run-all`
├── docker-compose.yml          # Memgraph + FalkorDB only (capped specs)
├── data/
│   ├── download_snap.py        # Fetches SNAP dataset & normalizes to JSON
│   └── dataset_stats.json      # Output node/edge counts
├── harness/                    # Go Benchmark Harness
│   ├── go.mod
│   ├── main.go                 # CLI Orchestrator
│   ├── config/                 # Env & Flags
│   ├── adapters/               # bolt_adapter, falkordb_adapter, kuzu_adapter
│   └── metrics/                # HdrHistogram latency tracking & JSON exporter
├── scripts/
│   ├── plot_results.py         # Generates charts for README
│   └── generate_report.py      # Formats Markdown tables automatically
└── docs/
    └── BENCHMARK_ARTICLE.md    # Engaging technical evangelism blog post
```

Note: `docker-compose.yml` now only needs Memgraph and FalkorDB services. CognoDB and Neo4j AuraDB are cloud-hosted; Kuzu is embedded — neither needs a container.

### Automation Sequence (`make run-all`)

1. `python3 data/download_snap.py` — downloads SNAP GitHub graph, writes batched payloads (~5,000 entities/chunk).
2. `docker compose up -d` — boots Memgraph and FalkorDB with CPU/RAM caps enforced.
3. `go run harness/main.go --config=config.json`:
   - Measures network RTT to cloud instances.
   - Runs bulk ingestion for each target; records total wall-clock time and throughput (Nodes/s, Rels/s).
   - Runs cold/warm read workloads (1-hop, 2-hop, 3-hop, point lookup, aggregations).
   - Runs concurrency sweeps (1, 10, 40 clients) on mixed 80/20 read/write workloads.
   - Writes raw metrics to `results.json`.
4. `python3 scripts/plot_results.py` — reads `results.json`, outputs charts to `docs/img/*.png`.
5. `python3 scripts/generate_report.py` — injects formatted tables into `README.md`.

---

## 6. Communication & Technical Evangelism Strategy

The assessment allocates 40% total weight to README & Analysis (15%) and Communication/Article (20%).

### Content Structure for `docs/BENCHMARK_ARTICLE.md`

- **Title idea**: *Benchmarking CognoDB Cloud: How a Burstable 256MB Instance Handles Graph Workloads*
- **Core narrative**:
  1. **The Challenge**: How do modern graph cloud engines perform when constrained to entry/free tiers (256 MB RAM, 0.5 vCPU)?
  2. **Fairness First**: Methodology — normalizing network latency, keeping a single query language across all five targets, keeping specs strictly identical, and disclosing the one target (Kuzu) that structurally can't match the others' network profile.
  3. **The Data Speaks**: p50/p95 latency charts, highlighting where CognoDB excels and where the embedded baseline (Kuzu) shows how much of the other platforms' latency is pure network overhead.
  4. **Honest Caveats**: cloud free-tier CPU bursting limits, network variance, memory limits during deep 3-hop traversals, and the Kuzu embedded-vs-networked asymmetry.

---

## Next Steps

1. `harness/adapters/adapter.go` — the `GraphDBAdapter` interface everything else depends on.
2. `data/download_snap.py` — dataset ready before writing Go code against it.
3. `harness/adapters/bolt_adapter.go` — one implementation, three targets (CognoDB, Neo4j, Memgraph).
