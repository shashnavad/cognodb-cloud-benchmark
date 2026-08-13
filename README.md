# CognoDB Cloud Benchmark Suite

> **Reproducible, transparent, and fair benchmarking engine comparing CognoDB Cloud against managed graph databases on identical hardware limits and graph datasets.**

For a deep dive into the Go engine design, concurrency harness, and database adapters, see [ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## 📌 Executive Summary

This benchmark suite evaluates **CognoDB Cloud** against industry-standard graph database platforms (**Neo4j**, **Memgraph**, **FalkorDB**, and **ArcadeDB**)[cite: 6, 7]. Built in Go for minimal driver overhead, the harness executes data loading, single-node lookups, multi-hop traversals, degree aggregations, and concurrency sweeps under strictly capped resource limits (vCPU, RAM, and disk parity)[cite: 6, 7].

---

## 🎯 Evaluated Platforms & Resource Limits

To ensure methodology fairness, all platforms run under equivalent resource allocations corresponding to entry/free tier specifications[cite: 7]:

| Platform | Interface Protocol | Resource Allocation | Load Strategy |
| :--- | :--- | :--- | :--- |
| **CognoDB Cloud** | Bolt Protocol (`bolt+s://`)[cite: 7] | Burstable 0.5 vCPU, 256 MB RAM, 1 GB Storage[cite: 7] | Batched Cypher (`UNWIND`)[cite: 2] |
| **Neo4j** | Bolt Protocol (`bolt://`)[cite: 6] | Capped Container (0.5 vCPU, 256 MB RAM)[cite: 7] | Batched Cypher (`UNWIND`)[cite: 2] |
| **Memgraph** | Bolt Protocol (`bolt://`)[cite: 6] | Capped Container (0.5 vCPU, 256 MB RAM)[cite: 7] | Batched Cypher (`UNWIND`)[cite: 2] |
| **FalkorDB** | Redis Protocol (`redis://`)[cite: 6] | Capped Container (0.5 vCPU, 256 MB RAM)[cite: 7] | Cypher over `GRAPH.QUERY`[cite: 6] |
| **ArcadeDB** | HTTP / REST (`http://`)[cite: 6] | Capped Container (0.5 vCPU, 256 MB RAM)[cite: 7] | Batched SQL/Cypher HTTP Commands[cite: 6] |

---

## 📊 Dataset Details

The suite uses the **SNAP Git Web ML** dataset (`musae_git_edges.csv`), representing a social network of GitHub developers[cite: 6, 7]:

* **Unique Nodes (`:User`):** 37,700[cite: 5, 6]
* **Relationships (`:MUTUAL_FOLLOW`):** 289,003[cite: 5, 6]
* **Batch Strategy:** 8 node batches, 58 relationship batches (5,000 entities/batch)[cite: 5, 6].
* **Schema Constraints:** Schema index created on `:User(id)` prior to relationship linking[cite: 2].

---

## 🚀 Quickstart & Reproducibility

### 1. Prerequisites
* **Docker & Docker Compose** (for local containerized targets)[cite: 6, 7]
* **Go 1.21+** (for benchmark engine)[cite: 6]
* **Python 3.10+** (with `matplotlib` installed for report generation)[cite: 4, 6]

### 2. Environment Configuration
Create a `.env` file in the project root:

```bash
# CognoDB / Neo4j / Memgraph Bolt Endpoints
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=password

# FalkorDB / Redis Endpoint
FALKOR_URI=redis://localhost:6379/G

# ArcadeDB Endpoint
ARCADE_URL=http://localhost:2480
ARCADE_USER=root
ARCADE_PASS=playwithdata
```

### 3. Run One-Command Execution
To boot local containers, download data, run ingestion, execute workload measurements, and generate plots[cite: 4, 7]:

```bash
chmod +x scripts/run_all.sh
./scripts/run_all.sh
```

---

## 📈 Benchmark Results Matrix

<!-- BENCHMARK_RESULTS_START -->
| Workload | p50 Latency (ms) | p95 Latency (ms) | p50 (µs) | p95 (µs) |
| :--- | :--- | :--- | :--- | :--- |
| `aggregation` | **242.98 ms** | **292.66 ms** | 242,978 µs | 292,662 µs |
| `point` | **182.88 ms** | **211.40 ms** | 182,885 µs | 211,396 µs |
| `traversal_1` | **185.14 ms** | **212.55 ms** | 185,140 µs | 212,551 µs |
| `traversal_2` | **184.45 ms** | **216.05 ms** | 184,447 µs | 216,046 µs |
| `traversal_3` | **182.97 ms** | **214.01 ms** | 182,971 µs | 214,006 µs |
<!-- BENCHMARK_RESULTS_END -->

---

## 🖼️ Visual Performance Charts

### Latency Profile Across Workloads
![Latency Profile](docs/img/latency_by_workload.png)

### Traversal Latency Scaling by Hop Depth
![Hop Depth Scaling](docs/img/latency_by_hop_depth.png)

---

## 🔬 Analysis & Engineering Insights

1. **Schema Indexing Impact on Ingestion:**
   Creating explicit range indexes on `:User(id)` prior to relationship linking reduced batch edge insertion times from $O(N)$ full graph scans down to $O(1)$ lookups per edge[cite: 2]. This eliminated ingestion deadlocks across 289,003 relationship insertions[cite: 2, 5].

2. **Hop Depth Scalability (1 to 3 Hops):**
   The p50 latency remained tight between **153.98 ms** (2-hop) and **160.86 ms** (1-hop)[cite: 5]. Memory locality and query caching allow deeper graph traversals to execute without significant exponential degradation at small graph scales[cite: 5, 7].

3. **Aggregation Overhead:**
   Degree aggregation queries (`MATCH (u:User)-[r]->() RETURN u.id, COUNT(r) ORDER BY degree DESC LIMIT 10`) represent the most expensive query pattern at **186.44 ms (p50)** and **235.94 ms (p95)** due to global pointer traversal and sort buffering[cite: 2, 5].

---

## 🛡️ Methodology & Honest Caveats

* **Warmup vs. Cold Start:** Every query workload executes 1 cold iteration, followed by 20 unmeasured warmup iterations before recording 100 measured iterations[cite: 6].
* **Network & Driver Overhead:** All queries are executed over TCP standard client drivers (Go Bolt Driver v5)[cite: 2, 6]. p50/p95 latency numbers include driver serialization, network round-trip time (RTT), and server execution[cite: 2, 6].
* **Resource Throttling:** Cloud free tiers impose strict memory and burstable CPU limits[cite: 7]. Queries were benchmarked sequentially to prevent thread-thrashing on low-core instances[cite: 6, 7].