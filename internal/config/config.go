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
// Returns (cfg, true, nil) if found and valid.
// Returns (empty, false, nil) if the file doesn't exist.
// Returns (empty, false, err) on read or parse failure.
func loadFromDir(dir string) (Config, bool, error) {
	cfgPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("config.json: %w", err)
	}
	return cfg, true, nil
}

// Load reads config.json from the same directory as the running executable.
// Uses os.Executable() + filepath.EvalSymlinks to resolve the real exe path
// (handles symlinks like ~/go/bin/pw -> actual binary).
// Returns (cfg, true, nil) if found and valid.
// Returns (empty, false, nil) if the file doesn't exist.
// Returns (empty, false, err) on read or parse failure.
func Load() (Config, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return Config{}, false, nil // can't locate exe, skip config
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Config{}, false, nil
	}
	cfg, found, err := loadFromDir(filepath.Dir(exe))
	return cfg, found, err
}
