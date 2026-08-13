package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config holds harness options and database connection parameters.
type Config struct {
	// Database Endpoints & Credentials
	CognoDBURI  string
	CognoDBUser string
	CognoDBPass string

	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	MemgraphURI string
	FalkorDBURI string
	ArcadeDBURI string

	// Tuning Parameters
	MaxConnectionPoolSize int
	BatchSize             int
	Workers               int
	Timeout               time.Duration
}

// LoadConfig fetches settings from environment variables with safe default fallbacks.
func LoadConfig() *Config {
	cfg := &Config{
		// Target Database Connection Strings
		CognoDBURI:  getEnv("BOLT_COGNODB_URI", "bolt://localhost:7687"),
		CognoDBUser: getEnv("BOLT_COGNODB_USER", "admin"),
		CognoDBPass: getEnv("BOLT_COGNODB_PASS", "password"),

		Neo4jURI:  getEnv("NEO4J_URI", ""),
		Neo4jUser: getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASS", ""),

		MemgraphURI: getEnv("MEMGRAPH_URI", "bolt://localhost:7687"),
		FalkorDBURI: getEnv("FALKORDB_URI", "redis://localhost:6379"),
		ArcadeDBURI: getEnv("ARCADEDB_URI", "http://localhost:2480"),

		// Engine Parameters
		MaxConnectionPoolSize: getEnvAsInt("MAX_CONNECTION_POOL_SIZE", 100),
		BatchSize:             getEnvAsInt("BATCH_SIZE", 5000),
		Workers:               getEnvAsInt("WORKERS", 10),
		Timeout:               time.Duration(getEnvAsInt("TIMEOUT_SECONDS", 30)) * time.Second,
	}

	// Guardrail: Neo4j Go Driver panics/fails if MaxConnectionPoolSize <= 0
	if cfg.MaxConnectionPoolSize <= 0 {
		cfg.MaxConnectionPoolSize = 100
	}

	// Validate critical credentials
	if cfg.Neo4jURI == "" || cfg.Neo4jPass == "" {
		log.Fatal("NEO4J_URI, NEO4J_USER and NEO4J_PASS must be set in the environment")
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			return i
		}
	}
	return defaultValue
}
