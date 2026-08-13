package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type BoltAdapter struct {
	drv neo4j.DriverWithContext
}

func NewBoltAdapter() *BoltAdapter {
	return &BoltAdapter{}
}

func (b *BoltAdapter) Connect(ctx context.Context, uri, user, pass string) error {
	drv, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""), func(c *neo4j.Config) {
		if c.MaxConnectionPoolSize <= 0 {
			c.MaxConnectionPoolSize = 100
		}
		// Valid config field for driver connection pool wait time
		c.ConnectionAcquisitionTimeout = 2 * time.Minute
	})
	if err != nil {
		return fmt.Errorf("neo4j NewDriver: %w", err)
	}

	if err := drv.VerifyConnectivity(ctx); err != nil {
		_ = drv.Close(ctx)
		return fmt.Errorf("verify connectivity: %w", err)
	}
	b.drv = drv

	if err := b.EnsureIndexes(ctx); err != nil {
		_ = drv.Close(ctx)
		return fmt.Errorf("ensure indexes: %w", err)
	}

	return nil
}

func (b *BoltAdapter) EnsureIndexes(ctx context.Context) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	queries := []string{
		"CREATE INDEX IF NOT EXISTS FOR (n:User) ON (n.id)",
		"CREATE INDEX ON :User(id)",
	}

	var lastErr error
	succeeded := false
	for _, q := range queries {
		res, err := sess.Run(ctx, q, nil)
		if err == nil {
			_, err = res.Consume(ctx)
		}
		if err == nil {
			succeeded = true
			break
		}
		if isAlreadyExists(err) {
			succeeded = true
			break
		}
		lastErr = err
	}

	if !succeeded {
		return fmt.Errorf("no index creation syntax succeeded, last error: %w", lastErr)
	}
	return nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "equivalentschemarule")
}

func (b *BoltAdapter) Close(ctx context.Context) error {
	if b.drv != nil {
		return b.drv.Close(ctx)
	}
	return nil
}

func (b *BoltAdapter) PointLookup(ctx context.Context, id string) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := "MATCH (u:User {id: $id}) RETURN u.id LIMIT 1"
	res, err := sess.Run(ctx, cypher, map[string]any{"id": id})
	if err != nil {
		return err
	}

	if res.Next(ctx) {
		_ = res.Record()
	}
	return res.Err()
}

func (b *BoltAdapter) Traversal(ctx context.Context, startID string, hops int) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)
	pattern := fmt.Sprintf("MATCH (u:User {id:$id})-[:MUTUAL_FOLLOW*%d]->(m) RETURN count(DISTINCT m) AS cnt", hops)
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, pattern, map[string]any{"id": startID})
		if err != nil {
			return nil, err
		}
		if _, err := res.Single(ctx); err != nil {
			return nil, nil
		}
		return nil, nil
	})
	return err
}

// IngestBatch demonstrates passing neo4j.WithTxTimeout directly to managed transaction functions
func (b *BoltAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Set a 3-minute max execution limit per transaction config
	txConfig := neo4j.WithTxTimeout(3 * time.Minute)

	// Ingest nodes in sub-chunks
	if len(nodes) > 0 {
		chunkSize := 1000
		for i := 0; i < len(nodes); i += chunkSize {
			end := i + chunkSize
			if end > len(nodes) {
				end = len(nodes)
			}
			chunk := nodes[i:end]

			_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
				cypher := `UNWIND $batch AS row MERGE (u:User {id: row.id}) SET u.location = row.location, u.public_repos = row.public_repos`
				batch := make([]map[string]any, 0, len(chunk))
				for _, n := range chunk {
					batch = append(batch, map[string]any{"id": n.ID, "location": n.Location, "public_repos": n.PublicRepos})
				}
				res, err := tx.Run(ctx, cypher, map[string]any{"batch": batch})
				if err != nil {
					return nil, err
				}
				return res.Consume(ctx)
			}, txConfig) // Pass txConfig here
			if err != nil {
				return err
			}
		}
	}

	// Ingest relationships in sub-chunks
	if len(rels) > 0 {
		chunkSize := 1000
		for i := 0; i < len(rels); i += chunkSize {
			end := i + chunkSize
			if end > len(rels) {
				end = len(rels)
			}
			chunk := rels[i:end]

			_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
				cypher := `UNWIND $batch AS r MATCH (a:User {id: r.from}), (b:User {id: r.to}) MERGE (a)-[:MUTUAL_FOLLOW]->(b)`
				batch := make([]map[string]any, 0, len(chunk))
				for _, r := range chunk {
					batch = append(batch, map[string]any{"from": r.From, "to": r.To, "type": r.Type})
				}
				res, err := tx.Run(ctx, cypher, map[string]any{"batch": batch})
				if err != nil {
					return nil, err
				}
				return res.Consume(ctx)
			}, txConfig) // Pass txConfig here
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *BoltAdapter) Aggregation(ctx context.Context) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := "MATCH (u:User)-[r:MUTUAL_FOLLOW]->() RETURN u.id AS id, COUNT(r) AS degree ORDER BY degree DESC LIMIT 10"
		res, err := tx.Run(ctx, cypher, nil)
		if err != nil {
			return nil, err
		}
		return res.Consume(ctx)
	})
	return err
}

func (b *BoltAdapter) ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		return res.Consume(ctx)
	})
	return err
}
