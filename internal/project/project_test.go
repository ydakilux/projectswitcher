package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan(t *testing.T) {
	root := t.TempDir()

	// Create regular directories
	dirs := []string{"alpha", "beta", "zeta", "Charlie"}
	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create a git repo (with .git dir)
	gitDir := filepath.Join(root, "gitproject")
	if err := os.MkdirAll(filepath.Join(gitDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a git worktree (with .git file, not dir)
	worktreeDir := filepath.Join(root, "worktree")
	if err := os.Mkdir(worktreeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: ../.git/worktrees/worktree"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a dotfile-prefixed directory (should be skipped)
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file (should be skipped)
	if err := os.WriteFile(filepath.Join(root, "notadir.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink to a directory
	symlinkTarget := t.TempDir()
	symlinkPath := filepath.Join(root, "symlinked")
	symlinkOk := false
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Logf("skipping symlink test: %v", err)
	} else {
		symlinkOk = true
	}

	projects, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	// Build a map for easy lookup
	projectMap := make(map[string]Project)
	for _, p := range projects {
		projectMap[p.Name] = p
	}

	// Assert expected projects are present
	for _, name := range []string{"alpha", "beta", "Charlie", "gitproject", "worktree", "zeta"} {
		if _, ok := projectMap[name]; !ok {
			t.Errorf("expected project %q not found in results", name)
		}
	}

	// Assert symlinked dir is included
	if symlinkOk {
		if _, ok := projectMap["symlinked"]; !ok {
			t.Error("symlinked directory should be included")
		}
	}

	// Assert dotfile dir is not present
	if _, ok := projectMap[".hidden"]; ok {
		t.Error(".hidden should be skipped")
	}

	// Assert file is not present
	if _, ok := projectMap["notadir.txt"]; ok {
		t.Error("notadir.txt (file) should be skipped")
	}

	// Assert git detection
	if p, ok := projectMap["gitproject"]; ok {
		if !p.IsGit {
			t.Error("gitproject should have IsGit=true")
		}
	}
	if p, ok := projectMap["worktree"]; ok {
		if !p.IsGit {
			t.Error("worktree (with .git file) should have IsGit=true")
		}
	}
	if p, ok := projectMap["alpha"]; ok {
		if p.IsGit {
			t.Error("alpha should have IsGit=false")
		}
	}

	// Assert sorted alphabetically (case-insensitive)
	for i := 1; i < len(projects); i++ {
		a := strings.ToLower(projects[i-1].Name)
		b := strings.ToLower(projects[i].Name)
		if a > b {
			t.Errorf("projects not sorted: %q > %q", projects[i-1].Name, projects[i].Name)
		}
	}
}
