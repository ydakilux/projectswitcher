package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Info holds git information about a project.
type Info struct {
	Branch     string
	Upstream   string // e.g. "origin/main", empty if no upstream tracked
	Dirty      bool
	Ahead      int
	Behind     int
	LastCommit string
	LastSync   time.Time // last fetch/pull time (from .git/FETCH_HEAD mtime), zero if unknown
	Available  bool
	Err        error
}

// gitBin is the path to the git binary; empty means unavailable.
var gitBin string

func init() {
	path, err := exec.LookPath("git")
	if err == nil {
		gitBin = path
	}
}

func runGit(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	fullArgs := append([]string{"-C", path}, args...)
	cmd := exec.CommandContext(ctx, gitBin, fullArgs...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// fetchQuiet runs a best-effort `git fetch` to refresh remote-tracking refs
// before computing ahead/behind counts. Errors (no network, no remote, etc.)
// are ignored — ahead/behind simply falls back to the last-known state.
func fetchQuiet(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitBin, "-C", path, "fetch", "--quiet")
	_ = cmd.Run()
}

// Get returns git information for the project at path.
func Get(path string) Info {
	if gitBin == "" {
		return Info{Available: false}
	}

	info := Info{Available: true}

	// Branch
	branch, err := runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		info.Err = err
		return info
	}
	info.Branch = branch

	// Dirty
	status, err := runGit(path, "status", "--porcelain")
	if err == nil {
		info.Dirty = strings.TrimSpace(status) != ""
	}

	// Ahead/Behind. To get an accurate count we first do a quiet, best-effort
	// `git fetch` so the local remote-tracking ref isn't stale — otherwise
	// "behind" can silently read as 0 until something else fetches.
	fetchQuiet(path)

	aheadBehind, err := runGit(path, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err == nil {
		parts := strings.Split(aheadBehind, "\t")
		if len(parts) == 2 {
			info.Ahead, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			info.Behind, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
		// Upstream ref name, e.g. "origin/main" (only queried if @{u} resolved).
		if upstream, uerr := runGit(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); uerr == nil {
			info.Upstream = upstream
		}
	}

	// Last commit
	lastCommit, err := runGit(path, "log", "-1", "--format=%h %s (%cr)")
	if err != nil || lastCommit == "" {
		info.LastCommit = "no commits yet"
	} else {
		info.LastCommit = lastCommit
	}

	// Last sync (fetch/pull) time — derived from .git/FETCH_HEAD mtime.
	// Fall back to .git/HEAD mtime (updated on clone/checkout) if FETCH_HEAD
	// doesn't exist yet (repo never fetched/pulled since clone).
	info.LastSync = lastSyncTime(path)

	return info
}

// lastSyncTime returns the last fetch/pull time for the repo at path,
// based on filesystem mtimes of well-known git bookkeeping files.
// Returns the zero time if it cannot be determined.
func lastSyncTime(path string) time.Time {
	gitDir, err := runGit(path, "rev-parse", "--git-dir")
	if err != nil || gitDir == "" {
		return time.Time{}
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}

	if fi, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD")); err == nil {
		return fi.ModTime()
	}
	if fi, err := os.Stat(filepath.Join(gitDir, "HEAD")); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// Pull runs `git pull` in the given repo path with a longer timeout
// (network operation). Returns combined stdout+stderr output and any error.
func Pull(path string) (string, error) {
	if gitBin == "" {
		return "", errors.New("git not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitBin, "-C", path, "pull", "--ff-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}
