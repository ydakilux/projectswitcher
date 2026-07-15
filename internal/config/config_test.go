package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDir_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"root": "/tmp/testroot"}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, found, err := loadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for existing config")
	}
	if cfg.Root != "/tmp/testroot" {
		t.Errorf("got root %q, want /tmp/testroot", cfg.Root)
	}
}

func TestLoadFromDir_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, found, err := loadFromDir(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if found {
		t.Fatal("expected found=false for missing config")
	}
	if cfg.Root != "" {
		t.Errorf("expected empty root, got %q", cfg.Root)
	}
}

func TestLoadFromDir_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadFromDir(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
