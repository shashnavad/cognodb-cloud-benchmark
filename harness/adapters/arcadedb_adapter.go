package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type ArcadeDBAdapter struct {
	baseURL string
	client  *http.Client
	auth    string
}

func NewArcadeDBAdapter() *ArcadeDBAdapter { return &ArcadeDBAdapter{client: &http.Client{}} }

func (a *ArcadeDBAdapter) Connect(ctx context.Context, uri, user, pass string) error {
	a.baseURL = strings.TrimRight(uri, "/")
	if user != "" && pass != "" {
		a.auth = user + ":" + pass
	}
	// Try a simple GET to the base URL with a short timeout to validate connectivity.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(pingCtx, http.MethodGet, a.baseURL, nil)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("arcadedb ping: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("arcadedb ping: status %d", resp.StatusCode)
	}
	return nil
}

func (a *ArcadeDBAdapter) doQuery(ctx context.Context, path string, payload any) error {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if a.auth != "" {
		parts := strings.SplitN(a.auth, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		}
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("arcadedb query: status %d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *ArcadeDBAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	// Chunk ingestion to avoid large command payloads.
	const chunkSize = 200
	if len(nodes) > 0 {
		for i := 0; i < len(nodes); i += chunkSize {
			end := i + chunkSize
			if end > len(nodes) {
				end = len(nodes)
			}
			batch := nodes[i:end]
			var cmds []string
			for _, n := range batch {
				cmds = append(cmds, fmt.Sprintf("CREATE VERTEX User SET id = '%s', location = '%s', public_repos = %d", escapeSQL(n.ID), escapeSQL(n.Location), n.PublicRepos))
			}
			payload := map[string]any{"language": "sql", "command": strings.Join(cmds, ";")}
			if err := a.doQuery(ctx, "/command", payload); err != nil {
				return fmt.Errorf("arcade ingest nodes chunk: %w", err)
			}
		}
	}
	if len(rels) > 0 {
		for i := 0; i < len(rels); i += chunkSize {
			end := i + chunkSize
			if end > len(rels) {
				end = len(rels)
			}
			batch := rels[i:end]
			var cmds []string
			for _, r := range batch {
				cmds = append(cmds, fmt.Sprintf("CREATE EDGE MUTUAL_FOLLOW FROM (SELECT FROM User WHERE id = '%s') TO (SELECT FROM User WHERE id = '%s')", escapeSQL(r.From), escapeSQL(r.To)))
			}
			payload := map[string]any{"language": "sql", "command": strings.Join(cmds, ";")}
			if err := a.doQuery(ctx, "/command", payload); err != nil {
				return fmt.Errorf("arcade ingest rels chunk: %w", err)
			}
		}
	}
	return nil
}

// escapeSQL performs minimal escaping for single quotes and backslashes in SQL-style commands.
func escapeSQL(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func (a *ArcadeDBAdapter) PointLookup(ctx context.Context, id string) error {
	payload := map[string]any{"language": "sql", "command": fmt.Sprintf("SELECT FROM User WHERE id = '%s' LIMIT 1", id)}
	return a.doQuery(ctx, "/command", payload)
}

func (a *ArcadeDBAdapter) Traversal(ctx context.Context, startID string, hops int) error {
	payload := map[string]any{"language": "sql", "command": fmt.Sprintf("MATCH {class: User, as: u, where: (id = '%s')} .out('MUTUAL_FOLLOW'){maxDepth: %d} RETURN count(*)", startID, hops)}
	return a.doQuery(ctx, "/command", payload)
}

func (a *ArcadeDBAdapter) Aggregation(ctx context.Context) error {
	payload := map[string]any{"language": "sql", "command": "SELECT id, out('MUTUAL_FOLLOW').size() as degree FROM User ORDER BY degree DESC LIMIT 10"}
	return a.doQuery(ctx, "/command", payload)
}

func (a *ArcadeDBAdapter) ExecuteWrite(ctx context.Context, cypher string, params map[string]interface{}) error {
	payload := map[string]any{"language": "sql", "command": cypher}
	return a.doQuery(ctx, "/command", payload)
}

func (a *ArcadeDBAdapter) Close(ctx context.Context) error { return nil }
