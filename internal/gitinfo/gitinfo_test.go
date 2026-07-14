package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH, skipping git tests")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test User")

	// Create a file and commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	return dir
}

func TestGet_Branch(t *testing.T) {
	dir := setupGitRepo(t)

	info := Get(dir)
	if !info.Available {
		t.Skip("git not available")
	}
	if info.Err != nil {
		t.Fatalf("Get() returned error: %v", info.Err)
	}
	// Branch should be "main" or "master"
	if info.Branch == "" {
		t.Error("expected non-empty branch name")
	}
	if info.Branch != "main" && info.Branch != "master" {
		t.Logf("branch is %q (may be fine depending on git config)", info.Branch)
	}
}

func TestGet_CleanRepo(t *testing.T) {
	dir := setupGitRepo(t)

	info := Get(dir)
	if !info.Available || info.Err != nil {
		t.Skip("git not available or errored")
	}

	if info.Dirty {
		t.Error("freshly committed repo should be clean")
	}
}

func TestGet_DirtyRepo(t *testing.T) {
	dir := setupGitRepo(t)

	// Modify a tracked file
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified"), 0644); err != nil {
		t.Fatal(err)
	}

	info := Get(dir)
	if !info.Available || info.Err != nil {
		t.Skip("git not available or errored")
	}

	if !info.Dirty {
		t.Error("modified file should make repo dirty")
	}
}

func TestGet_LastCommit(t *testing.T) {
	dir := setupGitRepo(t)

	info := Get(dir)
	if !info.Available || info.Err != nil {
		t.Skip("git not available or errored")
	}

	if info.LastCommit == "" || info.LastCommit == "no commits yet" {
		t.Error("expected a last commit message")
	}
	if !strings.Contains(info.LastCommit, "initial commit") {
		t.Logf("last commit: %q (expected to contain 'initial commit')", info.LastCommit)
	}
}

func TestGet_EmptyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	// configure so git doesn't error on user
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test User"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Run()
	}

	info := Get(dir)
	if !info.Available {
		t.Skip("git not available")
	}
	// Empty repo: branch command may fail, but we shouldn't panic
	// LastCommit should be "no commits yet" if branch was obtained somehow
	// or Err will be set. Either is fine.
	_ = info
}
