package config

import "context"

type TargetConfig struct {
	Name string `json:"name"`
	Uri  string `json:"uri"`
	User string `json:"user"`
}

// LoadConfig is a minimal stub. The harness reads connection info from
// environment variables for now; this function exists to be expanded later.
func LoadConfig(ctx context.Context, path string) ([]TargetConfig, error) {
	return []TargetConfig{}, nil
}
