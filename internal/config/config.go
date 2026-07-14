package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds pw configuration.
type Config struct {
	Root string `json:"root"`
}

// loadFromDir reads config.json from the given directory.
// Returns empty Config (no error) if the file doesn't exist.
// Returns error only on parse failure.
func loadFromDir(dir string) (Config, error) {
	cfgPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config.json: %w", err)
	}
	return cfg, nil
}

// Load reads config.json from the same directory as the running executable.
// Uses os.Executable() + filepath.EvalSymlinks to resolve the real exe path
// (handles symlinks like ~/go/bin/pw -> actual binary).
// Returns empty Config (no error) if the file doesn't exist.
// Returns error only on parse failure.
func Load() (Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return Config{}, nil // can't locate exe, skip config
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Config{}, nil
	}
	return loadFromDir(filepath.Dir(exe))
}
