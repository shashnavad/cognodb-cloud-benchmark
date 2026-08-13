package adapters

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/redis/go-redis/v9"
)

type FalkorDBAdapter struct {
	client *redis.Client
	graph  string
}

func NewFalkorDBAdapter() *FalkorDBAdapter { return &FalkorDBAdapter{} }

func (f *FalkorDBAdapter) Connect(ctx context.Context, uri, user, pass string) error {
	// Expected uri format: redis://host:port[/graphname]
	u, err := url.Parse(uri)
	if err != nil {
		return err
	}
	host := u.Host
	graph := strings.Trim(u.Path, "/")
	if graph == "" {
		graph = "G"
	}
	opt := &redis.Options{Addr: host, Password: pass}
	f.client = redis.NewClient(opt)
	f.graph = graph
	if err := f.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

func (f *FalkorDBAdapter) Close(ctx context.Context) error {
	if f.client != nil {
		return f.client.Close()
	}
	return nil
}

func (f *FalkorDBAdapter) ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error {
	// RedisGraph's GRAPH.QUERY doesn't support parameter maps via the client,
	// so inline simple params by replacing $param with quoted values when present.
	q := cypher
	for k, v := range params {
		q = strings.ReplaceAll(q, "$"+k, fmt.Sprintf("'%v'", v))
	}
	// GRAPH.QUERY <graph> <query>
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) PointLookup(ctx context.Context, id string) error {
	q := fmt.Sprintf("MATCH (u:User {id:'%s'}) RETURN u LIMIT 1", id)
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) Traversal(ctx context.Context, startID string, hops int) error {
	q := fmt.Sprintf("MATCH (u:User {id:'%s'})-[:MUTUAL_FOLLOW*%d]->(m) RETURN count(DISTINCT m)", startID, hops)
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) Aggregation(ctx context.Context) error {
	q := "MATCH (u:User)-[r:MUTUAL_FOLLOW]->() RETURN u.id, COUNT(r) AS degree ORDER BY degree DESC LIMIT 10"
	return f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err()
}

func (f *FalkorDBAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	// Ingest nodes
	if len(nodes) > 0 {
		// Build UNWIND-style list inline
		var parts []string
		for _, n := range nodes {
			parts = append(parts, fmt.Sprintf("{id:'%s', location:'%s', public_repos:%d}", n.ID, n.Location, n.PublicRepos))
		}
		q := fmt.Sprintf("UNWIND [%s] AS row MERGE (u:User {id: row.id}) SET u.location = row.location, u.public_repos = row.public_repos", strings.Join(parts, ","))
		if err := f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err(); err != nil {
			return err
		}
	}
	if len(rels) > 0 {
		var parts []string
		for _, r := range rels {
			parts = append(parts, fmt.Sprintf("{from:'%s', to:'%s'}", r.From, r.To))
		}
		q := fmt.Sprintf("UNWIND [%s] AS r MATCH (a:User {id: r.from}), (b:User {id: r.to}) MERGE (a)-[:MUTUAL_FOLLOW]->(b)", strings.Join(parts, ","))
		if err := f.client.Do(ctx, "GRAPH.QUERY", f.graph, q).Err(); err != nil {
			return err
		}
	}
	return nil
}
