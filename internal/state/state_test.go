package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	statePathOverride = filepath.Join(tmp, "pw", "recent.json")
	defer func() { statePathOverride = "" }()

	// Load from non-existent file -> empty store
	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(s.Recent) != 0 {
		t.Error("expected empty recent map on first load")
	}

	// Touch some paths
	before := time.Now().Unix()
	s.Touch("/home/user/projects/alpha")
	s.Touch("/home/user/projects/beta")
	after := time.Now().Unix()

	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load again and verify
	s2, err := Load()
	if err != nil {
		t.Fatalf("Load() second time error: %v", err)
	}

	if len(s2.Recent) != 2 {
		t.Errorf("expected 2 entries, got %d", len(s2.Recent))
	}

	for _, path := range []string{"/home/user/projects/alpha", "/home/user/projects/beta"} {
		ts, ok := s2.Recent[path]
		if !ok {
			t.Errorf("path %q not found after round-trip", path)
		}
		if ts < before || ts > after {
			t.Errorf("timestamp %d not in expected range [%d, %d]", ts, before, after)
		}
	}
}

func TestLoadMissing(t *testing.T) {
	tmp := t.TempDir()
	statePathOverride = filepath.Join(tmp, "nonexistent", "recent.json")
	defer func() { statePathOverride = "" }()

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error on missing file: %v", err)
	}
	if s.Recent == nil {
		t.Error("Recent map should be initialized")
	}
	if s.Favorites == nil {
		t.Error("Favorites map should be initialized")
	}
}

func TestToggleFavoriteRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	statePathOverride = filepath.Join(tmp, "pw", "recent.json")
	defer func() { statePathOverride = "" }()

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if s.IsFavorite("/home/user/projects/gamma") {
		t.Error("expected not favorited initially")
	}

	if fav := s.ToggleFavorite("/home/user/projects/gamma"); !fav {
		t.Error("expected ToggleFavorite to return true after first toggle")
	}
	if !s.IsFavorite("/home/user/projects/gamma") {
		t.Error("expected favorited after toggle")
	}

	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	s2, err := Load()
	if err != nil {
		t.Fatalf("Load() second time error: %v", err)
	}
	if !s2.IsFavorite("/home/user/projects/gamma") {
		t.Error("expected favorite to persist through save/load")
	}

	if fav := s2.ToggleFavorite("/home/user/projects/gamma"); fav {
		t.Error("expected ToggleFavorite to return false after second toggle")
	}
	if s2.IsFavorite("/home/user/projects/gamma") {
		t.Error("expected not favorited after untoggle")
	}
	if _, ok := s2.Favorites["/home/user/projects/gamma"]; ok {
		t.Error("expected favorite key to be deleted, not stored as false")
	}
}
