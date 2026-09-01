package preview

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"pw/internal/gitinfo"
	"pw/internal/project"
)

// FileEntry represents a single file/dir entry in the file explorer.
type FileEntry struct {
	Name      string
	IsDir     bool
	Size      int64
	ModTime   time.Time
	GitStatus string // XY code e.g. "M", "A", "??", "D", or ""
}

// ListDirEntries returns the entries of dir as FileEntry slice.
// gitStatuses is a map of relative-path-from-repo-root -> status code (may be nil).
// repoRoot is the root of the git repo used to compute relative paths for git status lookup.
func ListDirEntries(dir string, gitStatuses map[string]string, repoRoot string) ([]FileEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []FileEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fe := FileEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
		}
		if info, err2 := e.Info(); err2 == nil {
			fe.Size = info.Size()
			fe.ModTime = info.ModTime()
		}
		if gitStatuses != nil && repoRoot != "" {
			fullPath := filepath.Join(dir, e.Name())
			if rel, err2 := filepath.Rel(repoRoot, fullPath); err2 == nil {
				fe.GitStatus = gitStatuses[rel]
			}
		}
		result = append(result, fe)
	}
	return result, nil
}


// Preview holds the preview data for a project.
type Preview struct {
	Readme string
	Git    gitinfo.Info
	Files  []string
}

// Build constructs a Preview for the given project.
func Build(p project.Project) Preview {
	var pv Preview

	if p.IsGit {
		pv.Git = gitinfo.Get(p.Path)
	}

	// Find readme
	pv.Readme = findReadme(p.Path)

	// List files
	pv.Files = listFiles(p.Path)

	return pv
}

var readmeNames = []string{"readme.md", "readme", "readme.txt"}

func findReadme(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Build a map of lowercase name -> actual name
	nameMap := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			nameMap[strings.ToLower(e.Name())] = e.Name()
		}
	}

	for _, candidate := range readmeNames {
		actual, ok := nameMap[candidate]
		if !ok {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, actual))
		if err != nil {
			continue
		}

		// Cap to 2000 bytes
		if len(data) > 2000 {
			data = data[:2000]
		}

		// Cap to first 40 lines, trim trailing whitespace per line
		lines := strings.Split(string(data), "\n")
		if len(lines) > 40 {
			lines = lines[:40]
		}
		for i, l := range lines {
			lines[i] = strings.TrimRight(l, " \t\r")
		}
		return strings.Join(lines, "\n")
	}

	return ""
}

func listFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		files = append(files, name)
		if len(files) >= 20 {
			break
		}
	}
	return files
}
