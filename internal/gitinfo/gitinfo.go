package gitinfo

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Info holds git information about a project.
type Info struct {
	Branch     string
	Dirty      bool
	Ahead      int
	Behind     int
	LastCommit string
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

	// Ahead/Behind (ignore errors — no upstream is normal)
	aheadBehind, err := runGit(path, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err == nil {
		parts := strings.Split(aheadBehind, "\t")
		if len(parts) == 2 {
			info.Ahead, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			info.Behind, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
	}

	// Last commit
	lastCommit, err := runGit(path, "log", "-1", "--format=%h %s (%cr)")
	if err != nil || lastCommit == "" {
		info.LastCommit = "no commits yet"
	} else {
		info.LastCommit = lastCommit
	}

	return info
}
