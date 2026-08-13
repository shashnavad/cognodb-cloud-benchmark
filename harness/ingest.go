package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cognodb-cloud-benchmark/harness/adapters"
)

// RunIngest reads batched node and rel JSON files from dir and calls the
// adapter's IngestBatch for each batch. It expects files named
// nodes_batch_*.json and rels_batch_*.json. The function returns the total
// number of nodes and relationships processed.
func RunIngest(ctx context.Context, adapter adapters.GraphDBAdapter, batchesDir string) (int, int, error) {
	var nodeFiles []string
	var relFiles []string
	err := filepath.WalkDir(batchesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Replace lines 28–33 with strings.HasPrefix:
		base := filepath.Base(path)
		switch {
		case strings.HasPrefix(base, "nodes_batch_"):
			nodeFiles = append(nodeFiles, path)
		case strings.HasPrefix(base, "rels_batch_"):
			relFiles = append(relFiles, path)
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	sort.Strings(nodeFiles)
	sort.Strings(relFiles)

	totalNodes := 0
	totalRels := 0

	// Ingest nodes batches
	fmt.Printf("Ingesting %d node batches...\n", len(nodeFiles))
	for i, f := range nodeFiles {
		fmt.Printf("  [Node Batch %d/%d] %s\n", i+1, len(nodeFiles), filepath.Base(f))
		data, err := os.ReadFile(f)
		if err != nil {
			return totalNodes, totalRels, err
		}
		var nodes []adapters.Node
		if err := json.Unmarshal(data, &nodes); err != nil {
			return totalNodes, totalRels, fmt.Errorf("unmarshal nodes %s: %w", f, err)
		}
		if err := adapter.IngestBatch(ctx, nodes, nil); err != nil {
			return totalNodes, totalRels, fmt.Errorf("ingest nodes %s: %w", f, err)
		}
		totalNodes += len(nodes)
	}

	// Ingest relationship batches
	fmt.Printf("Ingesting %d relationship batches...\n", len(relFiles))
	for i, f := range relFiles {
		fmt.Printf("  [Rel Batch %d/%d] %s\n", i+1, len(relFiles), filepath.Base(f))
		data, err := os.ReadFile(f)
		if err != nil {
			return totalNodes, totalRels, err
		}
		var rels []adapters.Relationship
		if err := json.Unmarshal(data, &rels); err != nil {
			return totalNodes, totalRels, fmt.Errorf("unmarshal rels %s: %w", f, err)
		}
		if err := adapter.IngestBatch(ctx, nil, rels); err != nil {
			return totalNodes, totalRels, fmt.Errorf("ingest rels %s: %w", f, err)
		}
		totalRels += len(rels)
	}

	return totalNodes, totalRels, nil
}
