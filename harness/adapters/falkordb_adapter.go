package adapters

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type FalkorDBAdapter struct {
	client *redis.Client
	graph  string
}

func NewFalkorDBAdapter() *FalkorDBAdapter { return &FalkorDBAdapter{} }

func (f *FalkorDBAdapter) Connect(ctx context.Context, uri, user, pass string) error {
	// Guard: Prepend redis:// if scheme is missing so url.Parse succeeds
	if !strings.Contains(uri, "://") {
		uri = "redis://" + uri
	}

	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid falkordb uri: %w", err)
	}

	host := u.Host
	if host == "" {
		host = u.Path // Fallback if parsed as path
	}

	graph := strings.Trim(u.Path, "/")
	if graph == "" || graph == host {
		graph = "G"
	}

	opt := &redis.Options{
		Addr:         host,
		Password:     pass,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		DialTimeout:  10 * time.Second,
	}

	f.client = redis.NewClient(opt)
	f.graph = graph

	if err := f.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	// Create index on :User(id) upfront to avoid O(N^2) scans during ingestion
	_ = f.client.Do(ctx, "GRAPH.QUERY", f.graph, "CREATE INDEX FOR (u:User) ON (u.id)").Err()

	return nil
}

func (f *FalkorDBAdapter) Close(ctx context.Context) error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}

func (f *FalkorDBAdapter) ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error {
	q := cypher
	for k, v := range params {
		q = strings.ReplaceAll(q, "$"+k, fmt.Sprintf("'%v'", f.escape(fmt.Sprintf("%v", v))))
	}
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) ExecuteRead(ctx context.Context, cypher string, params map[string]interface{}) error {
	return f.ExecuteWrite(ctx, cypher, params)
}

func (f *FalkorDBAdapter) PointLookup(ctx context.Context, id string) error {
	q := fmt.Sprintf("MATCH (u:User {id:'%s'}) RETURN u LIMIT 1", f.escape(id))
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) Traversal(ctx context.Context, startID string, hops int) error {
	q := fmt.Sprintf("MATCH (u:User {id:'%s'})-[:MUTUAL_FOLLOW*%d]->(m) RETURN count(DISTINCT m)", f.escape(startID), hops)
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) Aggregation(ctx context.Context) error {
	q := "MATCH (u:User)-[r:MUTUAL_FOLLOW]->() RETURN u.id, COUNT(r) AS degree ORDER BY degree DESC LIMIT 10"
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	// Reduce chunk size from 500 to 200 to mitigate OOM spikes in Redis Graph memory
	const chunkSize = 200

	// 1. Ingest Nodes
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]
		var parts []string
		for _, n := range batch {
			parts = append(parts, fmt.Sprintf("{id:'%s', location:'%s', public_repos:%d}", f.escape(n.ID), f.escape(n.Location), n.PublicRepos))
		}
		q := fmt.Sprintf("UNWIND [%s] AS row CREATE (u:User {id: row.id, location: row.location, public_repos: row.public_repos})", strings.Join(parts, ","))
		if err := f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err(); err != nil {
			return fmt.Errorf("falkordb ingest nodes chunk err: %w", err)
		}
	}

	// 2. Ingest Relationships
	for i := 0; i < len(rels); i += chunkSize {
		end := i + chunkSize
		if end > len(rels) {
			end = len(rels)
		}
		batch := rels[i:end]
		var parts []string
		for _, r := range batch {
			parts = append(parts, fmt.Sprintf("{from:'%s', to:'%s'}", f.escape(r.From), f.escape(r.To)))
		}
		q := fmt.Sprintf("UNWIND [%s] AS r MATCH (a:User {id: r.from}), (b:User {id: r.to}) CREATE (a)-[:MUTUAL_FOLLOW]->(b)", strings.Join(parts, ","))
		if err := f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err(); err != nil {
			return fmt.Errorf("falkordb ingest rels chunk err: %w", err)
		}
	}
	return nil
}

func (f *FalkorDBAdapter) escape(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}
