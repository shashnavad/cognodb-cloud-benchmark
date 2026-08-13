package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ArcadeDBAdapter struct {
	client   *http.Client
	baseURL  string
	dbName   string
	user     string
	password string
	ridMap   sync.Map // Map[string]string for id -> @rid mapping
}

func NewArcadeDBAdapter() *ArcadeDBAdapter {
	return &ArcadeDBAdapter{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *ArcadeDBAdapter) Connect(ctx context.Context, uri, user, pass string) error {
	if !strings.Contains(uri, "://") {
		uri = "http://" + uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return err
	}

	a.baseURL = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	a.dbName = strings.Trim(u.Path, "/")
	if a.dbName == "" {
		a.dbName = "benchmark"
	}
	a.user = user
	a.password = pass

	// Ensure target database exists
	createDBCmd := map[string]string{
		"command": fmt.Sprintf("CREATE DATABASE %s", a.dbName),
	}
	body, _ := json.Marshal(createDBCmd)

	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/api/v1/server", bytes.NewBuffer(body))
	if err == nil {
		req.SetBasicAuth(a.user, a.password)
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}

	// Schema initialization
	// Schema initialization - Drop stale types from interrupted runs
	_, _ = a.executeCommand(ctx, "DROP TYPE MUTUAL_FOLLOW UNSAFE IF EXISTS", "sql")
	_, _ = a.executeCommand(ctx, "DROP TYPE User UNSAFE IF EXISTS", "sql")

	// Re-create types & unique index
	_, _ = a.executeCommand(ctx, "CREATE VERTEX TYPE User IF NOT EXISTS", "sql")
	_, _ = a.executeCommand(ctx, "CREATE PROPERTY User.id IF NOT EXISTS STRING", "sql")
	_, _ = a.executeCommand(ctx, "CREATE INDEX IF NOT EXISTS ON User (id) UNIQUE", "sql")
	_, _ = a.executeCommand(ctx, "CREATE EDGE TYPE MUTUAL_FOLLOW IF NOT EXISTS", "sql")
	return nil
}

func (a *ArcadeDBAdapter) Close(ctx context.Context) error {
	return nil
}

func (a *ArcadeDBAdapter) executeCommand(ctx context.Context, statement string, lang string) ([]byte, error) {
	payload := map[string]interface{}{
		"language": lang,
		"command":  statement,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/command/%s", a.baseURL, a.dbName)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.user, a.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("arcadedb status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (a *ArcadeDBAdapter) executeQuery(ctx context.Context, statement string) ([]byte, error) {
	payload := map[string]interface{}{
		"language": "sql",
		"command":  statement,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/query/%s", a.baseURL, a.dbName)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(a.user, a.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("arcadedb query status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (a *ArcadeDBAdapter) IngestBatch(ctx context.Context, nodes []Node, rels []Relationship) error {
	const chunkSize = 500

	// 1. Ingest Nodes
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		var statements []string
		for _, n := range batch {
			stmt := fmt.Sprintf("CREATE VERTEX User SET id = '%s', location = '%s', public_repos = %d;",
				escapeString(n.ID), escapeString(n.Location), n.PublicRepos)
			statements = append(statements, stmt)
		}

		batchSQL := strings.Join(statements, "\n")
		if _, err := a.executeCommand(ctx, batchSQL, "sqlscript"); err != nil {
			return fmt.Errorf("arcade ingest nodes chunk: %w", err)
		}
	}

	// 2. Fetch RIDs once if map is empty to populate id -> @rid in Go memory
	if len(nodes) > 0 {
		q := "SELECT id, @rid FROM User"
		respBytes, err := a.executeQuery(ctx, q)
		if err == nil {
			var queryRes struct {
				Result []struct {
					ID  string `json:"id"`
					RID string `json:"@rid"`
				} `json:"result"`
			}
			if jsonErr := json.Unmarshal(respBytes, &queryRes); jsonErr == nil {
				for _, row := range queryRes.Result {
					a.ridMap.Store(row.ID, row.RID)
				}
			}
		}
	}

	// 3. Ingest Relationships using fast direct @rid targets
	for i := 0; i < len(rels); i += chunkSize {
		end := i + chunkSize
		if end > len(rels) {
			end = len(rels)
		}
		batch := rels[i:end]

		var statements []string
		for _, r := range batch {
			fromRID, ok1 := a.ridMap.Load(r.From)
			toRID, ok2 := a.ridMap.Load(r.To)

			if ok1 && ok2 {
				stmt := fmt.Sprintf("CREATE EDGE MUTUAL_FOLLOW FROM %s TO %s;", fromRID, toRID)
				statements = append(statements, stmt)
			}
		}

		if len(statements) > 0 {
			batchSQL := strings.Join(statements, "\n")
			if _, err := a.executeCommand(ctx, batchSQL, "sqlscript"); err != nil {
				return fmt.Errorf("arcade ingest rels chunk: %w", err)
			}
		}
	}

	return nil
}

func (a *ArcadeDBAdapter) PointLookup(ctx context.Context, id string) error {
	q := fmt.Sprintf("SELECT FROM User WHERE id = '%s'", escapeString(id))
	_, err := a.executeQuery(ctx, q)
	return err
}

func (a *ArcadeDBAdapter) Traversal(ctx context.Context, startID string, hops int) error {
	q := fmt.Sprintf("SELECT count(*) FROM (TRAVERSE out('MUTUAL_FOLLOW') FROM (SELECT FROM User WHERE id = '%s') WHILE $depth <= %d)", escapeString(startID), hops)
	_, err := a.executeQuery(ctx, q)
	return err
}

func (a *ArcadeDBAdapter) Aggregation(ctx context.Context) error {
	q := "SELECT id, out('MUTUAL_FOLLOW').size() AS degree FROM User ORDER BY degree DESC LIMIT 10"
	_, err := a.executeQuery(ctx, q)
	return err
}

func (a *ArcadeDBAdapter) ExecuteWrite(ctx context.Context, query string, params map[string]interface{}) error {
	_, err := a.executeCommand(ctx, query, "sql")
	return err
}

func (a *ArcadeDBAdapter) ExecuteRead(ctx context.Context, query string, params map[string]interface{}) error {
	_, err := a.executeQuery(ctx, query)
	return err
}

func escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
