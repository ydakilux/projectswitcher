package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Store holds recent project usage data.
type Store struct {
	Recent    map[string]int64 `json:"recent"`    // absolute path -> unix seconds last used
	Favorites map[string]bool  `json:"favorites"` // absolute path -> true
}

// statePath returns the path to the state file.
func statePath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			// fallback: os.UserConfigDir returns %AppData% on Windows
			var err error
			base, err = os.UserConfigDir()
			if err != nil {
				base = "."
			}
		}
		return filepath.Join(base, "pw", "recent.json")
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "pw", "recent.json")
}

// statePathOverride is used in tests to override the default path.
var statePathOverride string

func getStatePath() string {
	if statePathOverride != "" {
		return statePathOverride
	}
	return statePath()
}

// Load reads the state file; returns empty store if missing.
func Load() (*Store, error) {
	path := getStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{Recent: map[string]int64{}, Favorites: map[string]bool{}}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return &Store{Recent: map[string]int64{}, Favorites: map[string]bool{}}, nil
	}
	if s.Recent == nil {
		s.Recent = map[string]int64{}
	}
	if s.Favorites == nil {
		s.Favorites = map[string]bool{}
	}
	return &s, nil
}

// Touch sets the last-used time for the given path.
func (s *Store) Touch(path string) {
	s.Recent[path] = time.Now().Unix()
}

// ToggleFavorite toggles favorite status for path and returns the new state.
func (s *Store) ToggleFavorite(path string) bool {
	if s.Favorites == nil {
		s.Favorites = map[string]bool{}
	}
	if s.Favorites[path] {
		delete(s.Favorites, path)
		return false
	}
	s.Favorites[path] = true
	return true
}

// IsFavorite reports whether path is favorited.
func (s *Store) IsFavorite(path string) bool {
	return s.Favorites[path]
}

// Save writes the state to disk atomically.
func (s *Store) Save() error {
	path := getStatePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file, then rename (atomic-ish)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
