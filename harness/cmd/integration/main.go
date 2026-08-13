package main

import (
	"context"
	"fmt"
	"os"
	"time"

	adapters "cognodb-cloud-benchmark/harness/adapters"
)

func main() {
	ctx := context.Background()
	// short timeout context for ops
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodes := []adapters.Node{
		{ID: "u1", Location: "loc1", PublicRepos: 3},
		{ID: "u2", Location: "loc2", PublicRepos: 7},
	}
	rels := []adapters.Relationship{{From: "u1", To: "u2"}}

	// Bolt targets: CognoDB, Neo4j, Memgraph
	boltTargets := []struct {
		name    string
		uriEnv  string
		userEnv string
		passEnv string
	}{
		{"CognoDB", "BOLT_COGNODB_URI", "BOLT_COGNODB_USER", "BOLT_COGNODB_PASS"},
		{"Neo4j", "BOLT_NEO4J_URI", "BOLT_NEO4J_USER", "BOLT_NEO4J_PASS"},
		{"Memgraph", "BOLT_MEMGRAPH_URI", "BOLT_MEMGRAPH_USER", "BOLT_MEMGRAPH_PASS"},
	}
	for _, t := range boltTargets {
		uri := os.Getenv(t.uriEnv)
		if uri == "" {
			fmt.Printf("%s URI not set — skipping %s test\n", t.uriEnv, t.name)
			continue
		}
		fmt.Printf("== %s (Bolt) test ==\n", t.name)
		b := adapters.NewBoltAdapter()
		if err := b.Connect(ctx, uri, os.Getenv(t.userEnv), os.Getenv(t.passEnv)); err != nil {
			fmt.Printf("%s Connect error: %v\n", t.name, err)
		} else {
			if err := b.IngestBatch(ctx, nodes, rels); err != nil {
				fmt.Printf("%s Ingest error: %v\n", t.name, err)
			} else {
				fmt.Printf("%s ingest succeeded\n", t.name)
			}
			if err := b.PointLookup(ctx, "u1"); err != nil {
				fmt.Printf("%s PointLookup error: %v\n", t.name, err)
			} else {
				fmt.Printf("%s PointLookup succeeded\n", t.name)
			}
			if err := b.Traversal(ctx, "u1", 1); err != nil {
				fmt.Printf("%s Traversal error: %v\n", t.name, err)
			} else {
				fmt.Printf("%s Traversal succeeded\n", t.name)
			}
			b.Close(ctx)
		}
	}

	// FalkorDB (RedisGraph) test
	falkorURI := os.Getenv("FALKOR_URI")
	if falkorURI != "" {
		fmt.Println("== FalkorDB test ==")
		f := adapters.NewFalkorDBAdapter()
		if err := f.Connect(ctx, falkorURI, "", os.Getenv("FALKOR_PASS")); err != nil {
			fmt.Printf("Falkor Connect error: %v\n", err)
		} else {
			if err := f.IngestBatch(ctx, nodes, rels); err != nil {
				fmt.Printf("Falkor Ingest error: %v\n", err)
			} else {
				fmt.Println("Falkor ingest succeeded")
			}
			if err := f.PointLookup(ctx, "u1"); err != nil {
				fmt.Printf("Falkor PointLookup error: %v\n", err)
			} else {
				fmt.Println("Falkor PointLookup succeeded")
			}
			if err := f.Traversal(ctx, "u1", 1); err != nil {
				fmt.Printf("Falkor Traversal error: %v\n", err)
			} else {
				fmt.Println("Falkor Traversal succeeded")
			}
			f.Close(ctx)
		}
	} else {
		fmt.Println("FALKOR_URI not set — skipping FalkorDB test")
	}

	// ArcadeDB test
	arcadeURL := os.Getenv("ARCADE_URL")
	if arcadeURL != "" {
		fmt.Println("== ArcadeDB test ==")
		a := adapters.NewArcadeDBAdapter()
		if err := a.Connect(ctx, arcadeURL, os.Getenv("ARCADE_USER"), os.Getenv("ARCADE_PASS")); err != nil {
			fmt.Printf("Arcade Connect error: %v\n", err)
		} else {
			if err := a.IngestBatch(ctx, nodes, rels); err != nil {
				fmt.Printf("Arcade Ingest error: %v\n", err)
			} else {
				fmt.Println("Arcade ingest succeeded")
			}
			if err := a.PointLookup(ctx, "u1"); err != nil {
				fmt.Printf("Arcade PointLookup error: %v\n", err)
			} else {
				fmt.Println("Arcade PointLookup succeeded")
			}
			if err := a.Traversal(ctx, "u1", 1); err != nil {
				fmt.Printf("Arcade Traversal error: %v\n", err)
			} else {
				fmt.Println("Arcade Traversal succeeded")
			}
			a.Close(ctx)
		}
	} else {
		fmt.Println("ARCADE_URL not set — skipping ArcadeDB test")
	}
}
