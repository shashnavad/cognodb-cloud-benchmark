package adapters

import "context"

// Basic data model used by the harness. Fields are intentionally minimal —
// the benchmark uses these for ingestion and simple lookups.
type Node struct {
	ID          string `json:"id"`
	Location    string `json:"location,omitempty"`
	PublicRepos int    `json:"public_repos,omitempty"`
}

type Relationship struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type,omitempty"`
}

// GraphDBAdapter is the interface the harness expects from each adapter.
// Implementations should be safe for concurrent use where the underlying
// driver supports it; the harness will pass a context for cancellations.
type GraphDBAdapter interface {
	Connect(ctx context.Context, uri, user, pass string) error
	IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error
	PointLookup(ctx context.Context, id string) error
	Traversal(ctx context.Context, startID string, hops int) error
	Aggregation(ctx context.Context) error
	ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error
	Close(ctx context.Context) error
}
