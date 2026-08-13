package adapters

import (
	"context"
	"fmt"
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
	})
	if err != nil {
		return fmt.Errorf("neo4j NewDriver: %w", err)
	}

	// Verify connectivity
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := drv.VerifyConnectivity(ctx2); err != nil {
		_ = drv.Close(ctx)
		return fmt.Errorf("verify connectivity: %w", err)
	}
	b.drv = drv

	// Ensure index exists on :User(id) for fast batch ingestion and lookups
	if err := b.EnsureIndexes(ctx2); err != nil {
		_ = drv.Close(ctx)
		return fmt.Errorf("ensure indexes: %w", err)
	}

	return nil
}

func (b *BoltAdapter) EnsureIndexes(ctx context.Context) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `CREATE INDEX user_id_idx IF NOT EXISTS FOR (u:User) ON (u.id)`
		res, err := tx.Run(ctx, cypher, nil)
		if err != nil {
			return nil, err
		}
		return res.Consume(ctx)
	})
	return err
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
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, "MATCH (u:User {id:$id}) RETURN u LIMIT 1", map[string]any{"id": id})
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

func (b *BoltAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	sess := b.drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Ingest nodes if present
	if len(nodes) > 0 {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `UNWIND $batch AS row MERGE (u:User {id: row.id}) SET u.location = row.location, u.public_repos = row.public_repos`
			batch := make([]map[string]any, 0, len(nodes))
			for _, n := range nodes {
				batch = append(batch, map[string]any{"id": n.ID, "location": n.Location, "public_repos": n.PublicRepos})
			}
			res, err := tx.Run(ctx, cypher, map[string]any{"batch": batch})
			if err != nil {
				return nil, err
			}
			return res.Consume(ctx)
		})
		if err != nil {
			return err
		}
	}

	// Ingest relationships if present
	if len(rels) > 0 {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := `UNWIND $batch AS r MATCH (a:User {id: r.from}), (b:User {id: r.to}) MERGE (a)-[:MUTUAL_FOLLOW]->(b)`
			batch := make([]map[string]any, 0, len(rels))
			for _, r := range rels {
				batch = append(batch, map[string]any{"from": r.From, "to": r.To, "type": r.Type})
			}
			res, err := tx.Run(ctx, cypher, map[string]any{"batch": batch})
			if err != nil {
				return nil, err
			}
			return res.Consume(ctx)
		})
		if err != nil {
			return err
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
