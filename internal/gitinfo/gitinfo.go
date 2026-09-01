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
	LastSync   time.Time            // last fetch/pull time (from .git/FETCH_HEAD mtime), zero if unknown
	DirtyFiles map[string]string    // relative path -> XY status code (e.g. "M", "A", "??", "D")
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
	// Trim only trailing whitespace/newlines. A leading space is significant
	// for commands like `git status --porcelain`, where the XY status code
	// can start with a literal space (e.g. " M file.txt").
	return strings.TrimRight(out.String(), " \t\r\n"), err
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

// StatusFor returns the git repo root and per-file dirty status for dir.
// Unlike Get, it does not require dir itself to contain a .git entry —
// it resolves the enclosing repo root via `git rev-parse --show-toplevel`,
// so it works for subfolders of a repo (e.g. monorepo packages) too.
// Returns ("", nil) if dir is not inside a git working tree.
func StatusFor(dir string) (repoRoot string, dirtyFiles map[string]string) {
	if gitBin == "" {
		return "", nil
	}
	root, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return "", nil
	}
	status, err := runGit(dir, "status", "--porcelain")
	if err != nil {
		return root, nil
	}
	return root, parsePorcelain(status)
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
		info.DirtyFiles = parsePorcelain(status)
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
	lastCommit, err := runGit(path, "log", "-1", "--date=short", "--format=%h (%cd, %cr) %s")
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

// parsePorcelain parses `git status --porcelain` output into a map of
// relative path -> status code (e.g. "M", "A", "??", "D").
// Handles quoted paths and rename "old -> new" format.
func parsePorcelain(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 4 {
			continue
		}
		xy := strings.TrimRight(line[:2], " ")
		rest := line[3:]

		// Handle quoted paths (e.g. "M  \"path with spaces\"")
		if strings.HasPrefix(rest, "\"") {
			rest = strings.Trim(rest, "\"")
		}

		// Handle rename "old -> new" (R  old -> new or "old" -> "new")
		// Take the destination (after " -> ")
		if idx := strings.Index(rest, " -> "); idx != -1 {
			rest = rest[idx+4:]
			if strings.HasPrefix(rest, "\"") {
				rest = strings.Trim(rest, "\"")
			}
		}

		path := strings.TrimSpace(rest)
		if path != "" {
			result[path] = xy
		}
	}
	return result
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
