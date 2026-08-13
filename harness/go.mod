module cognodb-cloud-benchmark/harness

go 1.22

require github.com/neo4j/neo4j-go-driver/v5 v5.8.0

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
)

// STATUS: additional dependencies (redis, kuzu binding, hdrhistogram) may be
// added later as the harness is expanded.
require github.com/redis/go-redis/v9 v9.4.0
